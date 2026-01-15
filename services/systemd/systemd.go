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
	// ReadOnly if true, disables start/stop/restart actions for ALL users
	ReadOnly bool
}

// Provider implements services.Provider for systemd services.
type Provider struct {
	hostName string
	address  string
	entries  []ServiceEntry
	isLocal  bool
}

// NewProvider creates a new systemd provider for the given host.
// Deprecated: Use NewProviderWithEntries for read-only support.
func NewProvider(hostName, address string, unitNames []string) *Provider {
	entries := make([]ServiceEntry, 0, len(unitNames))
	for _, name := range unitNames {
		entries = append(entries, ServiceEntry{Name: name, ReadOnly: false})
	}
	return NewProviderWithEntries(hostName, address, entries)
}

// NewProviderWithEntries creates a new systemd provider with service entries that may include flags.
func NewProviderWithEntries(hostName, address string, entries []ServiceEntry) *Provider {
	isLocal := address == "localhost" || address == "127.0.0.1"
	return &Provider{
		hostName: hostName,
		address:  address,
		entries:  entries,
		isLocal:  isLocal,
	}
}

// Name returns the provider name.
func (p *Provider) Name() string {
	return "systemd"
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
	for _, entry := range p.entries {
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
			})
			continue
		}
		info.ReadOnly = entry.ReadOnly
		result = append(result, info)
	}

	return result, nil
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
		info, err := p.getRemoteUnitInfo(ctx, entry.Name)
		if err != nil {
			result = append(result, services.ServiceInfo{
				Name:          entry.Name,
				Project:       "systemd",
				ContainerName: entry.Name,
				State:         "stopped",
				Status:        "unreachable",
				Image:         "-",
				Source:        "systemd",
				Host:          p.hostName,
				ReadOnly:      entry.ReadOnly,
			})
			continue
		}
		info.ReadOnly = entry.ReadOnly
		result = append(result, info)
	}

	return result, nil
}

// getRemoteUnitInfo gets info for a single unit via SSH.
func (p *Provider) getRemoteUnitInfo(ctx context.Context, unitName string) (services.ServiceInfo, error) {
	cmd := exec.CommandContext(ctx, "ssh", "-o", "ConnectTimeout=5", "-o", "StrictHostKeyChecking=accept-new",
		p.address, "systemctl", "show", unitName, "--property=ActiveState,SubState,LoadState,Description")

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
	return &SystemdService{
		unitName: name,
		hostName: p.hostName,
		address:  p.address,
		isLocal:  p.isLocal,
	}, nil
}

// GetLogs streams logs for a specific unit.
func (p *Provider) GetLogs(ctx context.Context, unitName string, tailLines int, follow bool) (io.ReadCloser, error) {
	svc := &SystemdService{
		unitName: unitName,
		hostName: p.hostName,
		address:  p.address,
		isLocal:  p.isLocal,
	}
	return svc.GetLogs(ctx, tailLines, follow)
}

// SystemdService represents a single systemd unit.
type SystemdService struct {
	unitName string
	hostName string
	address  string
	isLocal  bool
}

// GetInfo returns the current status of the unit.
func (s *SystemdService) GetInfo(ctx context.Context) (services.ServiceInfo, error) {
	if s.isLocal {
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

	// Remote
	provider := &Provider{address: s.address, hostName: s.hostName}
	return provider.getRemoteUnitInfo(ctx, s.unitName)
}

// GetLogs returns a stream of logs for the unit.
func (s *SystemdService) GetLogs(ctx context.Context, tailLines int, follow bool) (io.ReadCloser, error) {
	var cmd *exec.Cmd

	args := []string{"-u", s.unitName, "-n", fmt.Sprintf("%d", tailLines), "--no-pager", "-o", "short-iso"}
	if follow {
		args = append(args, "-f")
	}

	if s.isLocal {
		cmd = exec.CommandContext(ctx, "journalctl", args...)
	} else {
		sshArgs := []string{"-o", "ConnectTimeout=5", "-o", "StrictHostKeyChecking=accept-new", s.address, "journalctl"}
		sshArgs = append(sshArgs, args...)
		cmd = exec.CommandContext(ctx, "ssh", sshArgs...)
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
// Authorization is handled by polkit rules.
func (s *SystemdService) runLocalSystemctl(ctx context.Context, action string) error {
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

// runRemoteSystemctl uses SSH with sudo to control a remote systemd unit.
// Requires sudoers configuration on the remote host.
func (s *SystemdService) runRemoteSystemctl(ctx context.Context, action string) error {
	cmd := exec.CommandContext(ctx, "ssh", "-o", "ConnectTimeout=5", "-o", "StrictHostKeyChecking=accept-new",
		s.address, "sudo", "systemctl", action, s.unitName)

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
