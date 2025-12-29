// Package docker provides Docker container service management.
package docker

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"

	"home_server_dashboard/services"
)

// Docker label constants for dashboard configuration
const (
	// LabelPrefix is the common prefix for all dashboard labels
	LabelPrefix = "home.server.dashboard"
	// LabelDescription is the label for service description
	LabelDescription = LabelPrefix + ".description"
	// LabelHidden is the label to hide an entire service from the dashboard
	LabelHidden = LabelPrefix + ".hidden"
	// LabelPortsPrefix is the prefix for port-specific labels
	LabelPortsPrefix = LabelPrefix + ".ports"
	// LabelPortsHidden is the label for a comma-separated list of hidden port numbers
	LabelPortsHidden = LabelPortsPrefix + ".hidden"
	// LabelRemapPortPrefix is the prefix for port remapping labels (home.server.dashboard.remapport.<port>=<service>)
	LabelRemapPortPrefix = LabelPrefix + ".remapport"
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
	svcList, _ := p.GetServicesWithRemaps(ctx)
	return svcList, nil
}

// GetServicesWithRemaps returns all Docker Compose containers as services,
// along with any port remapping information from container labels.
func (p *Provider) GetServicesWithRemaps(ctx context.Context) ([]services.ServiceInfo, []PortRemap) {
	containers, err := p.client.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		return nil, nil
	}

	var result []services.ServiceInfo
	var allRemaps []PortRemap
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

		// Extract non-localhost exposed ports with label customizations
		ports := extractExposedPorts(ctr.Ports, ctr.Labels)

		// Extract port remapping information
		remaps := parsePortRemaps(ctr.Labels, service)
		allRemaps = append(allRemaps, remaps...)

		// Extract custom description from label
		description := ctr.Labels[LabelDescription]

		// Check if service should be hidden
		hidden := isLabelTrue(ctr.Labels[LabelHidden])

		// Extract Traefik service name if explicitly defined in labels
		traefikServiceName := extractTraefikServiceName(ctr.Labels)

		result = append(result, services.ServiceInfo{
			Name:               service,
			Project:            project,
			ContainerName:      containerName,
			State:              ctr.State,
			Status:             ctr.Status,
			Image:              ctr.Image,
			Source:             "docker",
			Host:               p.hostName,
			Ports:              ports,
			Description:        description,
			Hidden:             hidden,
			TraefikServiceName: traefikServiceName,
		})
	}

	return result, allRemaps
}

// extractExposedPorts filters ports to only include those bound to non-localhost addresses.
// This includes ports bound to 0.0.0.0 (all interfaces) or empty IP (also all interfaces).
// Deduplicates ports by host_port:protocol combination.
// Applies label customizations for port labels and hidden status.
func extractExposedPorts(ports []container.Port, labels map[string]string) []services.PortInfo {
	// Parse hidden ports from comma-separated list
	hiddenPorts := parseHiddenPorts(labels[LabelPortsHidden])

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

		// Get port-specific label and hidden status
		portLabel := getPortLabel(labels, port.PublicPort)
		portHidden := hiddenPorts[port.PublicPort] || isPortHiddenByLabel(labels, port.PublicPort)

		// Include ports bound to 0.0.0.0, empty (all interfaces), or specific non-localhost IPs
		result = append(result, services.PortInfo{
			HostPort:      port.PublicPort,
			ContainerPort: port.PrivatePort,
			Protocol:      port.Type,
			Label:         portLabel,
			Hidden:        portHidden,
		})
	}
	return result
}

// isLabelTrue checks if a label value represents a true boolean.
// Accepts "true", "1", "yes" (case-insensitive).
func isLabelTrue(value string) bool {
	v := strings.ToLower(strings.TrimSpace(value))
	return v == "true" || v == "1" || v == "yes"
}

// parseHiddenPorts parses a comma-separated list of port numbers into a set.
// Example: "8080,443,9000" -> {8080: true, 443: true, 9000: true}
func parseHiddenPorts(value string) map[uint16]bool {
	result := make(map[uint16]bool)
	if value == "" {
		return result
	}
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(part)
		if port, err := strconv.ParseUint(part, 10, 16); err == nil {
			result[uint16(port)] = true
		}
	}
	return result
}

// getPortLabel retrieves the custom label for a specific port from Docker labels.
// Looks for: home.server.dashboard.ports.<port>.label
func getPortLabel(labels map[string]string, port uint16) string {
	key := fmt.Sprintf("%s.%d.label", LabelPortsPrefix, port)
	return labels[key]
}

// isPortHiddenByLabel checks if a specific port is hidden via its own label.
// Looks for: home.server.dashboard.ports.<port>.hidden
func isPortHiddenByLabel(labels map[string]string, port uint16) bool {
	key := fmt.Sprintf("%s.%d.hidden", LabelPortsPrefix, port)
	return isLabelTrue(labels[key])
}

// PortRemap represents a port that should be remapped from one service to another.
// This is used when a service runs in another container's network namespace
// (e.g., qbittorrent running in gluetun's network).
type PortRemap struct {
	Port          uint16 // The host port to remap
	TargetService string // The service name that should own this port
	SourceService string // The service name that exposes this port
}

// traefikServicesPrefix is the prefix for Traefik HTTP services labels.
const traefikServicesPrefix = "traefik.http.services."

// extractTraefikServiceName extracts the Traefik service name from Docker labels.
// Traefik service labels follow the pattern: traefik.http.services.<name>.loadbalancer...
// Returns empty string if no Traefik service label is found.
func extractTraefikServiceName(labels map[string]string) string {
	for key := range labels {
		if !strings.HasPrefix(key, traefikServicesPrefix) {
			continue
		}
		// Extract service name from: traefik.http.services.<name>.loadbalancer...
		rest := strings.TrimPrefix(key, traefikServicesPrefix)
		if idx := strings.Index(rest, "."); idx > 0 {
			return rest[:idx]
		}
	}
	return ""
}

// parsePortRemaps extracts port remapping information from container labels.
// Looks for labels like: home.server.dashboard.remapport.<port>=<target_service>
// Returns a list of port remaps for this container.
func parsePortRemaps(labels map[string]string, sourceService string) []PortRemap {
	var remaps []PortRemap
	prefix := LabelRemapPortPrefix + "."
	for key, value := range labels {
		if !strings.HasPrefix(key, prefix) {
			continue
		}
		// Extract port number from key
		portStr := strings.TrimPrefix(key, prefix)
		port, err := strconv.ParseUint(portStr, 10, 16)
		if err != nil {
			continue
		}
		targetService := strings.TrimSpace(value)
		if targetService == "" {
			continue
		}
		remaps = append(remaps, PortRemap{
			Port:          uint16(port),
			TargetService: targetService,
			SourceService: sourceService,
		})
	}
	return remaps
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

	// Extract non-localhost exposed ports from network settings with label customizations
	ports := extractPortsFromInspect(inspect.NetworkSettings, inspect.Config.Labels)

	// Extract custom description from label
	description := inspect.Config.Labels[LabelDescription]

	// Check if service should be hidden
	hidden := isLabelTrue(inspect.Config.Labels[LabelHidden])

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
		Hidden:        hidden,
	}, nil
}

// extractPortsFromInspect extracts non-localhost ports from container inspect network settings.
// Deduplicates ports by host_port:protocol combination.
// Applies label customizations for port labels and hidden status.
func extractPortsFromInspect(settings *container.NetworkSettings, labels map[string]string) []services.PortInfo {
	if settings == nil {
		return nil
	}

	// Parse hidden ports from comma-separated list
	hiddenPorts := parseHiddenPorts(labels[LabelPortsHidden])

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

			// Get port-specific label and hidden status
			portLabel := getPortLabel(labels, hostPort)
			portHidden := hiddenPorts[hostPort] || isPortHiddenByLabel(labels, hostPort)

			result = append(result, services.PortInfo{
				HostPort:      hostPort,
				ContainerPort: uint16(portProto.Int()),
				Protocol:      portProto.Proto(),
				Label:         portLabel,
				Hidden:        portHidden,
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
