// Package docker provides Docker container service management.
package docker

import (
	"bufio"
	"context"
	"fmt"
	"io"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"

	"home_server_dashboard/services"
)

// Provider implements services.Provider for Docker containers.
type Provider struct {
	hostName string
	client   *client.Client
}

// NewProvider creates a new Docker provider for the given host.
func NewProvider(hostName string) (*Provider, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("failed to create docker client: %w", err)
	}

	return &Provider{
		hostName: hostName,
		client:   cli,
	}, nil
}

// Close closes the Docker client connection.
func (p *Provider) Close() error {
	if p.client != nil {
		return p.client.Close()
	}
	return nil
}

// Name returns the provider name.
func (p *Provider) Name() string {
	return "docker"
}

// GetServices returns all Docker Compose containers as services.
func (p *Provider) GetServices(ctx context.Context) ([]services.ServiceInfo, error) {
	containers, err := p.client.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}

	var result []services.ServiceInfo
	for _, ctr := range containers {
		// Docker Compose labels
		project := ctr.Labels["com.docker.compose.project"]
		service := ctr.Labels["com.docker.compose.service"]

		// Skip non-compose containers
		if project == "" || service == "" {
			continue
		}

		containerName := ""
		if len(ctr.Names) > 0 {
			containerName = ctr.Names[0]
			// Remove leading slash
			if len(containerName) > 0 && containerName[0] == '/' {
				containerName = containerName[1:]
			}
		}

		// Extract non-localhost exposed ports
		ports := extractExposedPorts(ctr.Ports)

		// Extract custom description from label
		description := ctr.Labels["home.server.dashboard.description"]

		result = append(result, services.ServiceInfo{
			Name:          service,
			Project:       project,
			ContainerName: containerName,
			State:         ctr.State,
			Status:        ctr.Status,
			Image:         ctr.Image,
			Source:        "docker",
			Host:          p.hostName,
			Ports:         ports,
			Description:   description,
		})
	}

	return result, nil
}

// extractExposedPorts filters ports to only include those bound to non-localhost addresses.
// This includes ports bound to 0.0.0.0 (all interfaces) or empty IP (also all interfaces).
// Deduplicates ports by host_port:protocol combination.
func extractExposedPorts(ports []container.Port) []services.PortInfo {
	seen := make(map[string]bool)
	var result []services.PortInfo
	for _, port := range ports {
		// Skip ports without a public port (not published)
		if port.PublicPort == 0 {
			continue
		}
		// Skip localhost-only bindings (127.0.0.1)
		if port.IP == "127.0.0.1" {
			continue
		}
		// Deduplicate by host_port:protocol
		key := fmt.Sprintf("%d:%s", port.PublicPort, port.Type)
		if seen[key] {
			continue
		}
		seen[key] = true
		// Include ports bound to 0.0.0.0, empty (all interfaces), or specific non-localhost IPs
		result = append(result, services.PortInfo{
			HostPort:      port.PublicPort,
			ContainerPort: port.PrivatePort,
			Protocol:      port.Type,
		})
	}
	return result
}

// GetService returns a specific Docker service by container name.
func (p *Provider) GetService(name string) (services.Service, error) {
	return &DockerService{
		containerName: name,
		hostName:      p.hostName,
		client:        p.client,
	}, nil
}

// GetLogs streams logs for a specific container.
func (p *Provider) GetLogs(ctx context.Context, containerName string, tailLines int, follow bool) (io.ReadCloser, error) {
	options := container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     follow,
		Tail:       fmt.Sprintf("%d", tailLines),
		Timestamps: true,
	}

	logs, err := p.client.ContainerLogs(ctx, containerName, options)
	if err != nil {
		return nil, fmt.Errorf("failed to get container logs: %w", err)
	}

	// Wrap with demultiplexer to strip Docker's 8-byte header
	return &dockerLogReader{reader: bufio.NewReader(logs), closer: logs}, nil
}

// DockerService represents a single Docker container service.
type DockerService struct {
	containerName string
	hostName      string
	client        *client.Client
}

// GetInfo returns the current status of the container.
func (s *DockerService) GetInfo(ctx context.Context) (services.ServiceInfo, error) {
	inspect, err := s.client.ContainerInspect(ctx, s.containerName)
	if err != nil {
		return services.ServiceInfo{}, fmt.Errorf("failed to inspect container: %w", err)
	}

	state := "stopped"
	if inspect.State.Running {
		state = "running"
	}

	project := inspect.Config.Labels["com.docker.compose.project"]
	service := inspect.Config.Labels["com.docker.compose.service"]

	// Extract non-localhost exposed ports from network settings
	ports := extractPortsFromInspect(inspect.NetworkSettings)

	// Extract custom description from label
	description := inspect.Config.Labels["home.server.dashboard.description"]

	return services.ServiceInfo{
		Name:          service,
		Project:       project,
		ContainerName: s.containerName,
		State:         state,
		Status:        inspect.State.Status,
		Image:         inspect.Image,
		Source:        "docker",
		Host:          s.hostName,
		Ports:         ports,
		Description:   description,
	}, nil
}

// extractPortsFromInspect extracts non-localhost ports from container inspect network settings.
// Deduplicates ports by host_port:protocol combination.
func extractPortsFromInspect(settings *container.NetworkSettings) []services.PortInfo {
	if settings == nil {
		return nil
	}
	seen := make(map[string]bool)
	var result []services.PortInfo
	for portProto, bindings := range settings.Ports {
		for _, binding := range bindings {
			// Skip localhost-only bindings
			if binding.HostIP == "127.0.0.1" {
				continue
			}
			// Parse host port
			hostPort := parsePort(binding.HostPort)
			if hostPort == 0 {
				continue
			}
			// Deduplicate by host_port:protocol
			key := fmt.Sprintf("%d:%s", hostPort, portProto.Proto())
			if seen[key] {
				continue
			}
			seen[key] = true
			result = append(result, services.PortInfo{
				HostPort:      hostPort,
				ContainerPort: uint16(portProto.Int()),
				Protocol:      portProto.Proto(),
			})
		}
	}
	return result
}

// parsePort parses a port string to uint16.
func parsePort(s string) uint16 {
	if s == "" {
		return 0
	}
	var port int
	n, err := fmt.Sscanf(s, "%d", &port)
	if err != nil || n != 1 || port < 0 || port > 65535 {
		return 0
	}
	// Ensure the entire string was a valid number (no trailing chars)
	expected := fmt.Sprintf("%d", port)
	if s != expected {
		return 0
	}
	return uint16(port)
}

// GetLogs returns a stream of logs for the container.
func (s *DockerService) GetLogs(ctx context.Context, tailLines int, follow bool) (io.ReadCloser, error) {
	options := container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     follow,
		Tail:       fmt.Sprintf("%d", tailLines),
		Timestamps: true,
	}

	logs, err := s.client.ContainerLogs(ctx, s.containerName, options)
	if err != nil {
		return nil, fmt.Errorf("failed to get container logs: %w", err)
	}

	return &dockerLogReader{reader: bufio.NewReader(logs), closer: logs}, nil
}

// Start starts the container.
func (s *DockerService) Start(ctx context.Context) error {
	return s.client.ContainerStart(ctx, s.containerName, container.StartOptions{})
}

// Stop stops the container.
func (s *DockerService) Stop(ctx context.Context) error {
	return s.client.ContainerStop(ctx, s.containerName, container.StopOptions{})
}

// Restart restarts the container.
func (s *DockerService) Restart(ctx context.Context) error {
	return s.client.ContainerRestart(ctx, s.containerName, container.StopOptions{})
}

// GetName returns the container name.
func (s *DockerService) GetName() string {
	return s.containerName
}

// GetHost returns the host name.
func (s *DockerService) GetHost() string {
	return s.hostName
}

// GetSource returns "docker".
func (s *DockerService) GetSource() string {
	return "docker"
}

// dockerLogReader wraps Docker logs and strips the 8-byte multiplexing header.
type dockerLogReader struct {
	reader *bufio.Reader
	closer io.Closer
}

// Read reads log lines, stripping Docker's 8-byte header.
func (r *dockerLogReader) Read(p []byte) (n int, err error) {
	line, err := r.reader.ReadBytes('\n')
	if err != nil {
		return 0, err
	}

	// Docker log lines have 8-byte header for multiplexed streams
	content := line
	if len(line) > 8 {
		content = line[8:]
	}

	n = copy(p, content)
	return n, nil
}

// Close closes the underlying log stream.
func (r *dockerLogReader) Close() error {
	return r.closer.Close()
}
