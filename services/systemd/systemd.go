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

// Provider implements services.Provider for systemd services.
type Provider struct {
	hostName  string
	address   string
	unitNames []string
	isLocal   bool
}

// NewProvider creates a new systemd provider for the given host.
func NewProvider(hostName, address string, unitNames []string) *Provider {
	isLocal := address == "localhost" || address == "127.0.0.1"
	return &Provider{
		hostName:  hostName,
		address:   address,
		unitNames: unitNames,
		isLocal:   isLocal,
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

	// Create a set of desired unit names for quick lookup
	desiredUnits := make(map[string]bool)
	for _, name := range p.unitNames {
		desiredUnits[name] = true
	}

	var result []services.ServiceInfo
	for _, unit := range units {
		if !desiredUnits[unit.Name] {
			continue
		}

		// Map systemd states to our status format
		state := "stopped"
		if unit.ActiveState == "active" {
			state = "running"
		}

		status := fmt.Sprintf("%s (%s)", unit.ActiveState, unit.SubState)

		result = append(result, services.ServiceInfo{
			Name:          unit.Name,
			Project:       "systemd",
			ContainerName: unit.Name,
			State:         state,
			Status:        status,
			Image:         "-",
			Source:        "systemd",
			Host:          p.hostName,
		})

		// Remove from desired units to track what we found
		delete(desiredUnits, unit.Name)
	}

	// For units not found in running list, check if they exist but are inactive
	for unitName := range desiredUnits {
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
			})
			continue
		}
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

	return services.ServiceInfo{
		Name:          unitName,
		Project:       "systemd",
		ContainerName: unitName,
		State:         state,
		Status:        fmt.Sprintf("%s (%s)", activeState, subState),
		Image:         "-",
		Source:        "systemd",
		Host:          p.hostName,
	}, nil
}

// getRemoteServices queries systemd services on a remote host via SSH.
func (p *Provider) getRemoteServices(ctx context.Context) ([]services.ServiceInfo, error) {
	var result []services.ServiceInfo

	for _, unitName := range p.unitNames {
		info, err := p.getRemoteUnitInfo(ctx, unitName)
		if err != nil {
			result = append(result, services.ServiceInfo{
				Name:          unitName,
				Project:       "systemd",
				ContainerName: unitName,
				State:         "stopped",
				Status:        "unreachable",
				Image:         "-",
				Source:        "systemd",
				Host:          p.hostName,
			})
			continue
		}
		result = append(result, info)
	}

	return result, nil
}

// getRemoteUnitInfo gets info for a single unit via SSH.
func (p *Provider) getRemoteUnitInfo(ctx context.Context, unitName string) (services.ServiceInfo, error) {
	cmd := exec.CommandContext(ctx, "ssh", "-o", "ConnectTimeout=5", "-o", "StrictHostKeyChecking=accept-new",
		p.address, "systemctl", "show", unitName, "--property=ActiveState,SubState,LoadState")

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

		return services.ServiceInfo{
			Name:          s.unitName,
			Project:       "systemd",
			ContainerName: s.unitName,
			State:         state,
			Status:        fmt.Sprintf("%s (%s)", activeState, subState),
			Image:         "-",
			Source:        "systemd",
			Host:          s.hostName,
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
func (s *SystemdService) runSystemctl(ctx context.Context, action string) error {
	var cmd *exec.Cmd

	if s.isLocal {
		cmd = exec.CommandContext(ctx, "systemctl", action, s.unitName)
	} else {
		cmd = exec.CommandContext(ctx, "ssh", "-o", "ConnectTimeout=5", "-o", "StrictHostKeyChecking=accept-new",
			s.address, "systemctl", action, s.unitName)
	}

	return cmd.Run()
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
