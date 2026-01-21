// Package systemd provides systemd service management for local and remote hosts.
package systemd

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"

	"github.com/coreos/go-systemd/v22/dbus"

	"home_server_dashboard/services"
)

// ServiceEntry represents a systemd service with optional flags.
type ServiceEntry struct {
	// Name is the unit name (e.g., "docker.service")
	Name string
	// User is the username for user-level systemd services (empty for system services).
	// When set, the service is managed via `systemctl --user` instead of system D-Bus.
	User string
	// ReadOnly if true, disables start/stop/restart actions for ALL users
	ReadOnly bool
	// Ports are the port numbers advertised for this service in the UI
	Ports []uint16
}

// SSHConfig holds SSH connection settings for remote hosts.
type SSHConfig struct {
	// Username is the SSH username to use when connecting.
	// If empty, the default SSH user (usually current user) is used.
	Username string
	// Port is the SSH port to use when connecting.
	// If 0, the default SSH port (22) is used.
	Port int
}

// Provider implements services.Provider for systemd services.
type Provider struct {
	hostName  string
	address   string
	entries   []ServiceEntry
	isLocal   bool
	sshConfig *SSHConfig
}

// portsToPortInfo converts a slice of port numbers to PortInfo structs.
func portsToPortInfo(ports []uint16) []services.PortInfo {
	if len(ports) == 0 {
		return nil
	}
	result := make([]services.PortInfo, len(ports))
	for i, port := range ports {
		result[i] = services.PortInfo{
			HostPort:      port,
			ContainerPort: port,
			Protocol:      "tcp",
		}
	}
	return result
}

// NewProvider creates a new systemd provider for the given host.
// Deprecated: Use NewProviderWithEntries for read-only support.
func NewProvider(hostName, address string, unitNames []string) *Provider {
	entries := make([]ServiceEntry, 0, len(unitNames))
	for _, name := range unitNames {
		entries = append(entries, ServiceEntry{Name: name, ReadOnly: false})
	}
	return NewProviderWithEntries(hostName, address, entries, nil)
}

// NewProviderWithEntries creates a new systemd provider with service entries that may include flags.
// sshConfig is optional and only used for remote hosts.
func NewProviderWithEntries(hostName, address string, entries []ServiceEntry, sshConfig *SSHConfig) *Provider {
	isLocal := address == "localhost" || address == "127.0.0.1"
	return &Provider{
		hostName:  hostName,
		address:   address,
		entries:   entries,
		isLocal:   isLocal,
		sshConfig: sshConfig,
	}
}

// getSSHTarget returns the SSH target string (user@host or just host).
func (p *Provider) getSSHTarget() string {
	if p.sshConfig != nil && p.sshConfig.Username != "" {
		return p.sshConfig.Username + "@" + p.address
	}
	return p.address
}

// getSSHBaseArgs returns the common SSH arguments including port if configured.
func (p *Provider) getSSHBaseArgs() []string {
	args := []string{"-o", "ConnectTimeout=5", "-o", "StrictHostKeyChecking=accept-new"}
	if p.sshConfig != nil && p.sshConfig.Port > 0 {
		args = append(args, "-p", fmt.Sprintf("%d", p.sshConfig.Port))
	}
	return args
}

// Name returns the provider name.
func (p *Provider) Name() string {
	return "systemd"
}

// findEntry finds a ServiceEntry by unit name, returning the entry and whether it was found.
func (p *Provider) findEntry(unitName string) (ServiceEntry, bool) {
	for _, entry := range p.entries {
		if entry.Name == unitName {
			return entry, true
		}
	}
	return ServiceEntry{}, false
}

// GetServices returns all configured systemd services.
func (p *Provider) GetServices(ctx context.Context) ([]services.ServiceInfo, error) {
	if p.isLocal {
		return p.getLocalServices(ctx)
	}
	return p.getRemoteServices(ctx)
}

// getLocalServices queries systemd services on localhost via D-Bus.
func (p *Provider) getLocalServices(ctx context.Context) ([]services.ServiceInfo, error) {
	var result []services.ServiceInfo

	// Separate system entries from user entries
	var systemEntries, userEntries []ServiceEntry
	for _, entry := range p.entries {
		if entry.User != "" {
			userEntries = append(userEntries, entry)
		} else {
			systemEntries = append(systemEntries, entry)
		}
	}

	// Process system services via system D-Bus
	if len(systemEntries) > 0 {
		systemResults, err := p.getLocalSystemServices(ctx, systemEntries)
		if err != nil {
			return nil, err
		}
		result = append(result, systemResults...)
	}

	// Process user services via user D-Bus
	if len(userEntries) > 0 {
		userResults, err := p.getLocalUserServices(ctx, userEntries)
		if err != nil {
			// Log warning but continue - user services might fail if running as different user
			fmt.Printf("Warning: failed to get user services: %v\n", err)
		}
		result = append(result, userResults...)
	}

	return result, nil
}

// getLocalSystemServices queries system-level systemd services via D-Bus.
func (p *Provider) getLocalSystemServices(ctx context.Context, entries []ServiceEntry) ([]services.ServiceInfo, error) {
	conn, err := dbus.NewSystemConnectionContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to systemd: %w", err)
	}
	defer conn.Close()

	units, err := conn.ListUnitsContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list units: %w", err)
	}

	// Create a map of desired unit names to their entries for quick lookup
	desiredUnits := make(map[string]ServiceEntry)
	for _, entry := range entries {
		desiredUnits[entry.Name] = entry
	}

	var result []services.ServiceInfo
	for _, unit := range units {
		entry, found := desiredUnits[unit.Name]
		if !found {
			continue
		}

		// Map systemd states to our status format
		state := "stopped"
		if unit.ActiveState == "active" {
			state = "running"
		}

		status := fmt.Sprintf("%s (%s)", unit.ActiveState, unit.SubState)

		// Get unit description from D-Bus
		description := p.getLocalUnitDescription(ctx, conn, unit.Name)

		result = append(result, services.ServiceInfo{
			Name:          unit.Name,
			Project:       "systemd",
			ContainerName: unit.Name,
			State:         state,
			Status:        status,
			Image:         "-",
			Source:        "systemd",
			Host:          p.hostName,
			Description:   description,
			ReadOnly:      entry.ReadOnly,
			Ports:         portsToPortInfo(entry.Ports),
		})

		// Remove from desired units to track what we found
		delete(desiredUnits, unit.Name)
	}

	// For units not found in running list, check if they exist but are inactive
	for unitName, entry := range desiredUnits {
		info, err := p.getLocalUnitInfo(ctx, conn, unitName)
		if err != nil {
			result = append(result, services.ServiceInfo{
				Name:          unitName,
				Project:       "systemd",
				ContainerName: unitName,
				State:         "stopped",
				Status:        "not found",
				Image:         "-",
				Source:        "systemd",
				Host:          p.hostName,
				ReadOnly:      entry.ReadOnly,
			Ports:         portsToPortInfo(entry.Ports),
			})
			continue
		}
		info.ReadOnly = entry.ReadOnly
		info.Ports = portsToPortInfo(entry.Ports)
		result = append(result, info)
	}

	return result, nil
}

// getLocalUserServices queries user-level systemd services.
// Uses user D-Bus connection when possible, falls back to systemctl --user command.
func (p *Provider) getLocalUserServices(ctx context.Context, entries []ServiceEntry) ([]services.ServiceInfo, error) {
	var result []services.ServiceInfo

	// Group entries by user
	userGroups := make(map[string][]ServiceEntry)
	for _, entry := range entries {
		userGroups[entry.User] = append(userGroups[entry.User], entry)
	}

	// Process each user's services
	for user, userEntries := range userGroups {
		// Try to connect to user's D-Bus session
		conn, err := dbus.NewUserConnectionContext(ctx)
		if err == nil {
			// We have a user D-Bus connection (likely running as this user)
			defer conn.Close()
			for _, entry := range userEntries {
				info, err := p.getUserUnitInfo(ctx, conn, entry, user)
				if err != nil {
					result = append(result, services.ServiceInfo{
						Name:          entry.Name,
						Project:       "systemd-user",
						ContainerName: fmt.Sprintf("%s@%s", user, entry.Name),
						State:         "stopped",
						Status:        "not found",
						Image:         "-",
						Source:        "systemd",
						Host:          p.hostName,
						ReadOnly:      entry.ReadOnly,
						Ports:         portsToPortInfo(entry.Ports),
					})
					continue
				}
				result = append(result, info)
			}
		} else {
			// Fall back to systemctl --user command via exec
			for _, entry := range userEntries {
				info, err := p.getUserUnitInfoViaExec(ctx, entry, user)
				if err != nil {
					result = append(result, services.ServiceInfo{
						Name:          entry.Name,
						Project:       "systemd-user",
						ContainerName: fmt.Sprintf("%s@%s", user, entry.Name),
						State:         "stopped",
						Status:        "error: " + err.Error(),
						Image:         "-",
						Source:        "systemd",
						Host:          p.hostName,
						ReadOnly:      entry.ReadOnly,
						Ports:         portsToPortInfo(entry.Ports),
					})
					continue
				}
				result = append(result, info)
			}
		}
	}

	return result, nil
}

// getUserUnitInfo gets info for a user service via D-Bus connection.
func (p *Provider) getUserUnitInfo(ctx context.Context, conn *dbus.Conn, entry ServiceEntry, user string) (services.ServiceInfo, error) {
	prop, err := conn.GetUnitPropertyContext(ctx, entry.Name, "ActiveState")
	if err != nil {
		return services.ServiceInfo{}, err
	}

	activeState := strings.Trim(prop.Value.String(), "\"")
	state := "stopped"
	if activeState == "active" {
		state = "running"
	}

	subProp, _ := conn.GetUnitPropertyContext(ctx, entry.Name, "SubState")
	subState := "unknown"
	if subProp != nil {
		subState = strings.Trim(subProp.Value.String(), "\"")
	}

	// Get unit description
	description := ""
	descProp, _ := conn.GetUnitPropertyContext(ctx, entry.Name, "Description")
	if descProp != nil {
		description = strings.Trim(descProp.Value.String(), "\"")
	}

	return services.ServiceInfo{
		Name:          entry.Name,
		Project:       "systemd-user",
		ContainerName: fmt.Sprintf("%s@%s", user, entry.Name),
		State:         state,
		Status:        fmt.Sprintf("%s (%s)", activeState, subState),
		Image:         "-",
		Source:        "systemd",
		Host:          p.hostName,
		Description:   description,
		ReadOnly:      entry.ReadOnly,
		Ports:         portsToPortInfo(entry.Ports),
	}, nil
}

// getUserUnitInfoViaExec gets user service info by running systemctl --user command.
// This is used when D-Bus connection fails (e.g., running as different user).
func (p *Provider) getUserUnitInfoViaExec(ctx context.Context, entry ServiceEntry, user string) (services.ServiceInfo, error) {
	// Run systemctl --user show as the target user
	cmd := exec.CommandContext(ctx, "systemctl", "--user", "--machine="+user+"@", "show",
		entry.Name, "--property=ActiveState,SubState,LoadState,Description")

	output, err := cmd.Output()
	if err != nil {
		return services.ServiceInfo{}, fmt.Errorf("systemctl --user failed: %w", err)
	}

	// Parse the output
	props := make(map[string]string)
	for _, line := range strings.Split(string(output), "\n") {
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			props[parts[0]] = strings.TrimSpace(parts[1])
		}
	}

	activeState := props["ActiveState"]
	subState := props["SubState"]
	loadState := props["LoadState"]
	description := props["Description"]

	state := "stopped"
	if activeState == "active" {
		state = "running"
	}

	status := fmt.Sprintf("%s (%s)", activeState, subState)
	if loadState == "not-found" {
		status = "not found"
	}

	return services.ServiceInfo{
		Name:          entry.Name,
		Project:       "systemd-user",
		ContainerName: fmt.Sprintf("%s@%s", user, entry.Name),
		State:         state,
		Status:        status,
		Image:         "-",
		Source:        "systemd",
		Host:          p.hostName,
		Description:   description,
		ReadOnly:      entry.ReadOnly,
		Ports:         portsToPortInfo(entry.Ports),
	}, nil
}

// getLocalUnitInfo gets info for a single unit via D-Bus.
func (p *Provider) getLocalUnitInfo(ctx context.Context, conn *dbus.Conn, unitName string) (services.ServiceInfo, error) {
	prop, err := conn.GetUnitPropertyContext(ctx, unitName, "ActiveState")
	if err != nil {
		return services.ServiceInfo{}, err
	}

	activeState := strings.Trim(prop.Value.String(), "\"")
	state := "stopped"
	if activeState == "active" {
		state = "running"
	}

	subProp, _ := conn.GetUnitPropertyContext(ctx, unitName, "SubState")
	subState := "unknown"
	if subProp != nil {
		subState = strings.Trim(subProp.Value.String(), "\"")
	}

	// Get unit description
	description := p.getLocalUnitDescription(ctx, conn, unitName)

	return services.ServiceInfo{
		Name:          unitName,
		Project:       "systemd",
		ContainerName: unitName,
		State:         state,
		Status:        fmt.Sprintf("%s (%s)", activeState, subState),
		Image:         "-",
		Source:        "systemd",
		Host:          p.hostName,
		Description:   description,
	}, nil
}

// getLocalUnitDescription gets the description for a unit via D-Bus.
func (p *Provider) getLocalUnitDescription(ctx context.Context, conn *dbus.Conn, unitName string) string {
	prop, err := conn.GetUnitPropertyContext(ctx, unitName, "Description")
	if err != nil {
		return ""
	}
	return strings.Trim(prop.Value.String(), "\"")
}

// getRemoteServices queries systemd services on a remote host via SSH.
func (p *Provider) getRemoteServices(ctx context.Context) ([]services.ServiceInfo, error) {
	var result []services.ServiceInfo

	for _, entry := range p.entries {
		var info services.ServiceInfo
		var err error

		if entry.User != "" {
			// User service: use systemctl --user via SSH
			info, err = p.getRemoteUserUnitInfo(ctx, entry)
		} else {
			// System service: use systemctl via SSH
			info, err = p.getRemoteUnitInfo(ctx, entry.Name)
		}

		if err != nil {
			project := "systemd"
			containerName := entry.Name
			if entry.User != "" {
				project = "systemd-user"
				containerName = fmt.Sprintf("%s@%s", entry.User, entry.Name)
			}
			result = append(result, services.ServiceInfo{
				Name:          entry.Name,
				Project:       project,
				ContainerName: containerName,
				State:         "stopped",
				Status:        "unreachable",
				Image:         "-",
				Source:        "systemd",
				Host:          p.hostName,
				ReadOnly:      entry.ReadOnly,
				Ports:         portsToPortInfo(entry.Ports),
			})
			continue
		}
		info.ReadOnly = entry.ReadOnly
		info.Ports = portsToPortInfo(entry.Ports)
		result = append(result, info)
	}

	return result, nil
}

// getRemoteUserUnitInfo gets info for a user service via SSH.
func (p *Provider) getRemoteUserUnitInfo(ctx context.Context, entry ServiceEntry) (services.ServiceInfo, error) {
	// For user services, we need to run systemctl --user as the specified user
	// Using sudo -u <user> with XDG_RUNTIME_DIR set
	shellCmd := fmt.Sprintf("sudo -u %s XDG_RUNTIME_DIR=/run/user/$(id -u %s) systemctl --user show %s --property=ActiveState,SubState,LoadState,Description",
		entry.User, entry.User, entry.Name)

	sshArgs := p.getSSHBaseArgs()
	sshArgs = append(sshArgs, p.getSSHTarget(), "bash", "-c", shellCmd)
	cmd := exec.CommandContext(ctx, "ssh", sshArgs...)

	output, err := cmd.Output()
	if err != nil {
		return services.ServiceInfo{}, fmt.Errorf("SSH failed: %w", err)
	}

	// Parse the output
	props := make(map[string]string)
	for _, line := range strings.Split(string(output), "\n") {
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			props[parts[0]] = strings.TrimSpace(parts[1])
		}
	}

	activeState := props["ActiveState"]
	subState := props["SubState"]
	loadState := props["LoadState"]
	description := props["Description"]

	state := "stopped"
	if activeState == "active" {
		state = "running"
	}

	status := fmt.Sprintf("%s (%s)", activeState, subState)
	if loadState == "not-found" {
		status = "not found"
	}

	return services.ServiceInfo{
		Name:          entry.Name,
		Project:       "systemd-user",
		ContainerName: fmt.Sprintf("%s@%s", entry.User, entry.Name),
		State:         state,
		Status:        status,
		Image:         "-",
		Source:        "systemd",
		Host:          p.hostName,
		Description:   description,
		ReadOnly:      entry.ReadOnly,
		Ports:         portsToPortInfo(entry.Ports),
	}, nil
}

// getRemoteUnitInfo gets info for a single unit via SSH.
func (p *Provider) getRemoteUnitInfo(ctx context.Context, unitName string) (services.ServiceInfo, error) {
	sshArgs := p.getSSHBaseArgs()
	sshArgs = append(sshArgs, p.getSSHTarget(), "systemctl", "show", unitName, "--property=ActiveState,SubState,LoadState,Description")
	cmd := exec.CommandContext(ctx, "ssh", sshArgs...)

	output, err := cmd.Output()
	if err != nil {
		return services.ServiceInfo{}, fmt.Errorf("SSH failed: %w", err)
	}

	// Parse the output
	props := make(map[string]string)
	for _, line := range strings.Split(string(output), "\n") {
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			props[parts[0]] = strings.TrimSpace(parts[1])
		}
	}

	activeState := props["ActiveState"]
	subState := props["SubState"]
	loadState := props["LoadState"]
	description := props["Description"]

	state := "stopped"
	if activeState == "active" {
		state = "running"
	}

	status := fmt.Sprintf("%s (%s)", activeState, subState)
	if loadState == "not-found" {
		status = "not found"
	}

	return services.ServiceInfo{
		Name:          unitName,
		Project:       "systemd",
		ContainerName: unitName,
		State:         state,
		Status:        status,
		Image:         "-",
		Source:        "systemd",
		Host:          p.hostName,
		Description:   description,
	}, nil
}

// GetService returns a specific systemd service by unit name.
func (p *Provider) GetService(name string) (services.Service, error) {
	entry, _ := p.findEntry(name)
	return &SystemdService{
		unitName:  name,
		hostName:  p.hostName,
		address:   p.address,
		isLocal:   p.isLocal,
		user:      entry.User,
		sshConfig: p.sshConfig,
	}, nil
}

// GetLogs streams logs for a specific unit.
func (p *Provider) GetLogs(ctx context.Context, unitName string, tailLines int, follow bool) (io.ReadCloser, error) {
	entry, _ := p.findEntry(unitName)
	svc := &SystemdService{
		unitName:  unitName,
		hostName:  p.hostName,
		address:   p.address,
		isLocal:   p.isLocal,
		user:      entry.User,
		sshConfig: p.sshConfig,
	}
	return svc.GetLogs(ctx, tailLines, follow)
}

// SystemdService represents a single systemd unit.
type SystemdService struct {
	unitName  string
	hostName  string
	address   string
	isLocal   bool
	user      string     // User for user-level services (empty for system services)
	sshConfig *SSHConfig // SSH configuration for remote hosts
}

// getSSHTarget returns the SSH target string (user@host or just host).
func (s *SystemdService) getSSHTarget() string {
	if s.sshConfig != nil && s.sshConfig.Username != "" {
		return s.sshConfig.Username + "@" + s.address
	}
	return s.address
}

// getSSHBaseArgs returns the common SSH arguments including port if configured.
func (s *SystemdService) getSSHBaseArgs() []string {
	args := []string{"-o", "ConnectTimeout=5", "-o", "StrictHostKeyChecking=accept-new"}
	if s.sshConfig != nil && s.sshConfig.Port > 0 {
		args = append(args, "-p", fmt.Sprintf("%d", s.sshConfig.Port))
	}
	return args
}

// GetInfo returns the current status of the unit.
func (s *SystemdService) GetInfo(ctx context.Context) (services.ServiceInfo, error) {
	if s.isLocal {
		// For user services, use user D-Bus or exec
		if s.user != "" {
			return s.getLocalUserInfo(ctx)
		}
		// System service uses system D-Bus
		conn, err := dbus.NewSystemConnectionContext(ctx)
		if err != nil {
			return services.ServiceInfo{}, fmt.Errorf("failed to connect to systemd: %w", err)
		}
		defer conn.Close()

		prop, err := conn.GetUnitPropertyContext(ctx, s.unitName, "ActiveState")
		if err != nil {
			return services.ServiceInfo{}, err
		}

		activeState := strings.Trim(prop.Value.String(), "\"")
		state := "stopped"
		if activeState == "active" {
			state = "running"
		}

		subProp, _ := conn.GetUnitPropertyContext(ctx, s.unitName, "SubState")
		subState := "unknown"
		if subProp != nil {
			subState = strings.Trim(subProp.Value.String(), "\"")
		}

		// Get unit description
		description := ""
		descProp, _ := conn.GetUnitPropertyContext(ctx, s.unitName, "Description")
		if descProp != nil {
			description = strings.Trim(descProp.Value.String(), "\"")
		}

		return services.ServiceInfo{
			Name:          s.unitName,
			Project:       "systemd",
			ContainerName: s.unitName,
			State:         state,
			Status:        fmt.Sprintf("%s (%s)", activeState, subState),
			Image:         "-",
			Source:        "systemd",
			Host:          s.hostName,
			Description:   description,
		}, nil
	}

	// Remote - use the entry to determine if it's a user service
	if s.user != "" {
		provider := &Provider{address: s.address, hostName: s.hostName}
		return provider.getRemoteUserUnitInfo(ctx, ServiceEntry{Name: s.unitName, User: s.user})
	}
	provider := &Provider{address: s.address, hostName: s.hostName}
	return provider.getRemoteUnitInfo(ctx, s.unitName)
}

// getLocalUserInfo gets info for a local user service.
func (s *SystemdService) getLocalUserInfo(ctx context.Context) (services.ServiceInfo, error) {
	// Try user D-Bus connection first
	conn, err := dbus.NewUserConnectionContext(ctx)
	if err == nil {
		defer conn.Close()

		prop, err := conn.GetUnitPropertyContext(ctx, s.unitName, "ActiveState")
		if err != nil {
			return services.ServiceInfo{}, err
		}

		activeState := strings.Trim(prop.Value.String(), "\"")
		state := "stopped"
		if activeState == "active" {
			state = "running"
		}

		subProp, _ := conn.GetUnitPropertyContext(ctx, s.unitName, "SubState")
		subState := "unknown"
		if subProp != nil {
			subState = strings.Trim(subProp.Value.String(), "\"")
		}

		description := ""
		descProp, _ := conn.GetUnitPropertyContext(ctx, s.unitName, "Description")
		if descProp != nil {
			description = strings.Trim(descProp.Value.String(), "\"")
		}

		return services.ServiceInfo{
			Name:          s.unitName,
			Project:       "systemd-user",
			ContainerName: fmt.Sprintf("%s@%s", s.user, s.unitName),
			State:         state,
			Status:        fmt.Sprintf("%s (%s)", activeState, subState),
			Image:         "-",
			Source:        "systemd",
			Host:          s.hostName,
			Description:   description,
		}, nil
	}

	// Fall back to exec with --machine option
	cmd := exec.CommandContext(ctx, "systemctl", "--user", "--machine="+s.user+"@", "show",
		s.unitName, "--property=ActiveState,SubState,LoadState,Description")

	output, err := cmd.Output()
	if err != nil {
		return services.ServiceInfo{}, fmt.Errorf("systemctl --user failed: %w", err)
	}

	props := make(map[string]string)
	for _, line := range strings.Split(string(output), "\n") {
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			props[parts[0]] = strings.TrimSpace(parts[1])
		}
	}

	activeState := props["ActiveState"]
	subState := props["SubState"]
	loadState := props["LoadState"]
	description := props["Description"]

	state := "stopped"
	if activeState == "active" {
		state = "running"
	}

	status := fmt.Sprintf("%s (%s)", activeState, subState)
	if loadState == "not-found" {
		status = "not found"
	}

	return services.ServiceInfo{
		Name:          s.unitName,
		Project:       "systemd-user",
		ContainerName: fmt.Sprintf("%s@%s", s.user, s.unitName),
		State:         state,
		Status:        status,
		Image:         "-",
		Source:        "systemd",
		Host:          s.hostName,
		Description:   description,
	}, nil
}

// GetLogs returns a stream of logs for the unit.
func (s *SystemdService) GetLogs(ctx context.Context, tailLines int, follow bool) (io.ReadCloser, error) {
	var cmd *exec.Cmd

	// Build base args for journalctl
	args := []string{"-u", s.unitName, "-n", fmt.Sprintf("%d", tailLines), "--no-pager", "-o", "short-iso"}
	if follow {
		args = append(args, "-f")
	}

	if s.isLocal {
		if s.user != "" {
			// For local user services, just use --user flag
			// This works when the dashboard runs as the same user
			args = append([]string{"--user"}, args...)
		}
		cmd = exec.CommandContext(ctx, "journalctl", args...)
	} else {
		if s.user != "" {
			// For remote user services, run as that user via sudo
			// Add --user flag to the args
			userArgs := append([]string{"--user"}, args...)
			shellCmd := fmt.Sprintf("sudo -u %s XDG_RUNTIME_DIR=/run/user/$(id -u %s) journalctl %s",
				s.user, s.user, strings.Join(userArgs, " "))
			sshArgs := s.getSSHBaseArgs()
			sshArgs = append(sshArgs, s.getSSHTarget(), "bash", "-c", shellCmd)
			cmd = exec.CommandContext(ctx, "ssh", sshArgs...)
		} else {
			sshArgs := s.getSSHBaseArgs()
			sshArgs = append(sshArgs, s.getSSHTarget(), "journalctl")
			sshArgs = append(sshArgs, args...)
			cmd = exec.CommandContext(ctx, "ssh", sshArgs...)
		}
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start journalctl: %w", err)
	}

	return &journalReader{
		stdout: stdout,
		cmd:    cmd,
	}, nil
}

// Start starts the unit.
func (s *SystemdService) Start(ctx context.Context) error {
	return s.runSystemctl(ctx, "start")
}

// Stop stops the unit.
func (s *SystemdService) Stop(ctx context.Context) error {
	return s.runSystemctl(ctx, "stop")
}

// Restart restarts the unit.
func (s *SystemdService) Restart(ctx context.Context) error {
	return s.runSystemctl(ctx, "restart")
}

// runSystemctl runs a systemctl command on the unit.
// For local units, uses D-Bus directly (requires polkit authorization).
// For remote units, uses SSH with sudo (requires sudoers configuration).
func (s *SystemdService) runSystemctl(ctx context.Context, action string) error {
	if s.isLocal {
		return s.runLocalSystemctl(ctx, action)
	}
	return s.runRemoteSystemctl(ctx, action)
}

// runLocalSystemctl uses D-Bus to control a local systemd unit.
// This avoids the NoNewPrivileges restriction when using sudo.
// Authorization is handled by polkit rules for system services.
// For user services, uses user D-Bus connection or systemctl --user command.
func (s *SystemdService) runLocalSystemctl(ctx context.Context, action string) error {
	// For user services, use user D-Bus or systemctl --user
	if s.user != "" {
		return s.runLocalUserSystemctl(ctx, action)
	}

	// System service uses system D-Bus
	conn, err := dbus.NewSystemConnectionContext(ctx)
	if err != nil {
		return fmt.Errorf("failed to connect to systemd: %w", err)
	}
	defer conn.Close()

	// resultChan receives the job path when the action completes
	resultChan := make(chan string, 1)

	switch action {
	case "start":
		_, err = conn.StartUnitContext(ctx, s.unitName, "replace", resultChan)
	case "stop":
		_, err = conn.StopUnitContext(ctx, s.unitName, "replace", resultChan)
	case "restart":
		_, err = conn.RestartUnitContext(ctx, s.unitName, "replace", resultChan)
	default:
		return fmt.Errorf("unknown action: %s", action)
	}

	if err != nil {
		return fmt.Errorf("failed to %s unit: %w", action, err)
	}

	// Wait for the job to complete
	select {
	case result := <-resultChan:
		if result != "done" && result != "skipped" {
			return fmt.Errorf("job failed with result: %s", result)
		}
	case <-ctx.Done():
		return ctx.Err()
	}

	return nil
}

// runLocalUserSystemctl controls a local user service.
// Uses user D-Bus if available, otherwise falls back to systemctl --user command.
func (s *SystemdService) runLocalUserSystemctl(ctx context.Context, action string) error {
	// Try user D-Bus connection first
	conn, err := dbus.NewUserConnectionContext(ctx)
	if err == nil {
		defer conn.Close()

		resultChan := make(chan string, 1)

		switch action {
		case "start":
			_, err = conn.StartUnitContext(ctx, s.unitName, "replace", resultChan)
		case "stop":
			_, err = conn.StopUnitContext(ctx, s.unitName, "replace", resultChan)
		case "restart":
			_, err = conn.RestartUnitContext(ctx, s.unitName, "replace", resultChan)
		default:
			return fmt.Errorf("unknown action: %s", action)
		}

		if err != nil {
			return fmt.Errorf("failed to %s user unit: %w", action, err)
		}

		select {
		case result := <-resultChan:
			if result != "done" && result != "skipped" {
				return fmt.Errorf("job failed with result: %s", result)
			}
		case <-ctx.Done():
			return ctx.Err()
		}

		return nil
	}

	// Fall back to systemctl --user with --machine option
	cmd := exec.CommandContext(ctx, "systemctl", "--user", "--machine="+s.user+"@", action, s.unitName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		outputStr := strings.TrimSpace(string(output))
		if outputStr != "" {
			return fmt.Errorf("%w: %s", err, outputStr)
		}
		return err
	}
	return nil
}

// runRemoteSystemctl uses SSH with sudo to control a remote systemd unit.
// Requires sudoers configuration on the remote host.
// For user services, runs systemctl --user as the specified user.
func (s *SystemdService) runRemoteSystemctl(ctx context.Context, action string) error {
	var cmd *exec.Cmd

	if s.user != "" {
		// For user services, run systemctl --user as the specified user via sudo
		shellCmd := fmt.Sprintf("sudo -u %s XDG_RUNTIME_DIR=/run/user/$(id -u %s) systemctl --user %s %s",
			s.user, s.user, action, s.unitName)
		sshArgs := s.getSSHBaseArgs()
		sshArgs = append(sshArgs, s.getSSHTarget(), "bash", "-c", shellCmd)
		cmd = exec.CommandContext(ctx, "ssh", sshArgs...)
	} else {
		// System service uses sudo systemctl
		sshArgs := s.getSSHBaseArgs()
		sshArgs = append(sshArgs, s.getSSHTarget(), "sudo", "systemctl", action, s.unitName)
		cmd = exec.CommandContext(ctx, "ssh", sshArgs...)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		outputStr := strings.TrimSpace(string(output))
		if outputStr != "" {
			return fmt.Errorf("%w: %s", err, outputStr)
		}
		return err
	}
	return nil
}

// GetName returns the unit name.
func (s *SystemdService) GetName() string {
	return s.unitName
}

// GetHost returns the host name.
func (s *SystemdService) GetHost() string {
	return s.hostName
}

// GetSource returns "systemd".
func (s *SystemdService) GetSource() string {
	return "systemd"
}

// journalReader wraps journalctl output.
type journalReader struct {
	stdout io.ReadCloser
	cmd    *exec.Cmd
}

// Read reads from the journal output.
func (r *journalReader) Read(p []byte) (n int, err error) {
	return r.stdout.Read(p)
}

// Close closes the journal stream and kills the process.
func (r *journalReader) Close() error {
	r.stdout.Close()
	if r.cmd.Process != nil {
		r.cmd.Process.Kill()
	}
	return r.cmd.Wait()
}
