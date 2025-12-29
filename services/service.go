// Package services provides interfaces and types for managing services.
package services

import (
	"context"
	"io"
)

// PortInfo represents an exposed port on a service.
type PortInfo struct {
	HostPort      uint16 `json:"host_port"`      // Port exposed on the host
	ContainerPort uint16 `json:"container_port"` // Port on the container
	Protocol      string `json:"protocol"`       // "tcp" or "udp"
}

// ServiceInfo represents the status information for any service.
type ServiceInfo struct {
	Name          string     `json:"name"`           // Service/unit name
	Project       string     `json:"project"`        // Docker project or "systemd"
	ContainerName string     `json:"container_name"` // Container name or unit name
	State         string     `json:"state"`          // "running" or "stopped"
	Status        string     `json:"status"`         // Human-readable status
	Image         string     `json:"image"`          // Docker image or "-"
	Source        string     `json:"source"`         // "docker" or "systemd"
	Host          string     `json:"host"`           // Host name from config
	HostIP        string     `json:"host_ip"`        // Private IP address for port links
	Ports         []PortInfo `json:"ports"`          // Exposed ports (non-localhost bindings)
}

// LogStreamer provides a stream of log data.
type LogStreamer interface {
	// Read reads log data into the buffer. Implements io.Reader.
	Read(p []byte) (n int, err error)
	// Close closes the log stream and releases resources.
	Close() error
}

// Service defines the common interface for all service types (Docker, systemd, etc.).
type Service interface {
	// GetInfo returns the current status information for the service.
	GetInfo(ctx context.Context) (ServiceInfo, error)

	// GetLogs returns a stream of logs for the service.
	// The caller is responsible for closing the stream.
	GetLogs(ctx context.Context, tailLines int, follow bool) (io.ReadCloser, error)

	// Start starts the service.
	Start(ctx context.Context) error

	// Stop stops the service.
	Stop(ctx context.Context) error

	// Restart restarts the service.
	Restart(ctx context.Context) error

	// GetName returns the service name.
	GetName() string

	// GetHost returns the host name where the service runs.
	GetHost() string

	// GetSource returns the service source type ("docker" or "systemd").
	GetSource() string
}

// Provider defines an interface for discovering and managing services.
type Provider interface {
	// Name returns the provider name (e.g., "docker", "systemd").
	Name() string

	// GetServices returns all services managed by this provider.
	GetServices(ctx context.Context) ([]ServiceInfo, error)

	// GetService returns a specific service by name.
	GetService(name string) (Service, error)

	// GetLogs streams logs for a specific service.
	GetLogs(ctx context.Context, serviceName string, tailLines int, follow bool) (io.ReadCloser, error)
}
