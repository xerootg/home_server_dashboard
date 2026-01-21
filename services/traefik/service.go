// Package traefik provides Traefik service discovery and management.
package traefik

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"home_server_dashboard/services"
)

// TraefikAPIService represents a service from the Traefik API.
type TraefikAPIService struct {
	Name         string            `json:"name"`
	Type         string            `json:"type"`         // loadbalancer, weighted, mirroring
	Status       string            `json:"status"`       // enabled, disabled
	Provider     string            `json:"provider"`     // docker, file, etc.
	ServerStatus map[string]string `json:"serverStatus"` // server URL -> status (UP, DOWN)
	LoadBalancer *LoadBalancer     `json:"loadBalancer,omitempty"`
}

// LoadBalancer contains load balancer configuration.
type LoadBalancer struct {
	Servers []Server `json:"servers,omitempty"`
}

// Server represents a backend server in Traefik.
type Server struct {
	URL string `json:"url"`
}

// Provider implements services.Provider for Traefik services.
type Provider struct {
	hostName    string
	hostAddress string
	client      *Client
}

// NewProvider creates a new Traefik service provider.
// sshConfig is optional and only used for remote hosts.
func NewProvider(hostName, hostAddress string, apiPort int, sshConfig *SSHConfig) *Provider {
	return &Provider{
		hostName:    hostName,
		hostAddress: hostAddress,
		client:      NewClient(hostName, hostAddress, apiPort, sshConfig),
	}
}

// Name returns the provider name.
func (p *Provider) Name() string {
	return "traefik"
}

// Close cleans up provider resources.
func (p *Provider) Close() error {
	return p.client.Close()
}

// GetServices returns all Traefik services that are not backed by Docker or systemd.
// existingServices is a set of service names that already exist (Docker/systemd).
func (p *Provider) GetServices(ctx context.Context, existingServices map[string]bool) ([]services.ServiceInfo, error) {
	traefikServices, err := p.client.GetTraefikServices(ctx)
	if err != nil {
		return nil, err
	}

	// Get hostname mappings for enrichment
	hostMappings, err := p.client.GetServiceHostMappings(ctx)
	if err != nil {
		log.Printf("Warning: failed to get Traefik host mappings for %s: %v", p.hostName, err)
		hostMappings = make(map[string][]string)
	}

	// Get claimed backend services (services that are backends for routers owned by Docker/systemd)
	claimedServices, err := p.client.GetClaimedBackendServices(ctx, existingServices)
	if err != nil {
		log.Printf("Warning: failed to get claimed backend services for %s: %v", p.hostName, err)
		claimedServices = make(map[string]bool)
	}

	var result []services.ServiceInfo
	for _, svc := range traefikServices {
		// Normalize service name (strip @provider suffix)
		normalizedName := normalizeServiceName(svc.Name)

		// Skip services that already exist as Docker or systemd
		if existingServices[normalizedName] {
			continue
		}

		// Skip services that are claimed by a router owned by an existing Docker/systemd service
		// For example, if Docker "jellyfin" creates router "jellyfin@docker" → "jellyfin-svc@file",
		// then "jellyfin-svc" should not appear separately
		if claimedServices[normalizedName] {
			continue
		}

		// Skip internal Traefik services (api@internal, dashboard@internal, etc.)
		if strings.HasSuffix(svc.Name, "@internal") {
			continue
		}

		// Determine state based on status and server health
		state := "stopped"
		status := "disabled"
		if svc.Status == "enabled" {
			// Check if any server is UP
			hasUpServer := false
			allDown := true
			for _, serverStatus := range svc.ServerStatus {
				if serverStatus == "UP" {
					hasUpServer = true
					allDown = false
					break
				}
				if serverStatus != "DOWN" {
					allDown = false
				}
			}

			if hasUpServer {
				state = "running"
				status = "healthy"
			} else if allDown && len(svc.ServerStatus) > 0 {
				state = "stopped"
				status = "all servers down"
			} else if len(svc.ServerStatus) == 0 {
				// No servers configured yet
				state = "stopped"
				status = "no servers"
			} else {
				state = "running"
				status = "degraded"
			}
		}

		info := services.ServiceInfo{
			Name:          normalizedName,
			Project:       "traefik",
			ContainerName: svc.Name, // Full name with provider suffix
			State:         state,
			Status:        status,
			Image:         "-",
			Source:        "traefik",
			Host:          p.hostName,
			Description:   fmt.Sprintf("Traefik %s service", svc.Type),
		}

		// Add Traefik URLs if available
		if hostnames, ok := hostMappings[normalizedName]; ok {
			for _, hostname := range hostnames {
				info.TraefikURLs = append(info.TraefikURLs, "https://"+hostname)
			}
		}

		result = append(result, info)
	}

	return result, nil
}

// GetService returns a specific Traefik service by name.
func (p *Provider) GetService(name string) (services.Service, error) {
	return &TraefikService{
		name:     name,
		hostName: p.hostName,
		client:   p.client,
	}, nil
}

// GetLogs returns logs for a Traefik service (not supported).
func (p *Provider) GetLogs(ctx context.Context, serviceName string, tailLines int, follow bool) (io.ReadCloser, error) {
	msg := "Logs are not supported for Traefik services. " +
		"Traefik services are external services registered in Traefik and may not have accessible logs.\n"
	return io.NopCloser(bytes.NewReader([]byte(msg))), nil
}

// TraefikService implements services.Service for Traefik-managed services.
type TraefikService struct {
	name     string
	hostName string
	client   *Client
}

// GetInfo returns the current status information for the Traefik service.
func (s *TraefikService) GetInfo(ctx context.Context) (services.ServiceInfo, error) {
	traefikServices, err := s.client.GetTraefikServices(ctx)
	if err != nil {
		return services.ServiceInfo{}, err
	}

	// Find our service
	for _, svc := range traefikServices {
		normalizedName := normalizeServiceName(svc.Name)
		if normalizedName == s.name || svc.Name == s.name {
			// Determine state based on status and server health
			state := "stopped"
			status := "disabled"
			if svc.Status == "enabled" {
				hasUpServer := false
				for _, serverStatus := range svc.ServerStatus {
					if serverStatus == "UP" {
						hasUpServer = true
						break
					}
				}
				if hasUpServer {
					state = "running"
					status = "healthy"
				} else if len(svc.ServerStatus) > 0 {
					state = "stopped"
					status = "all servers down"
				} else {
					state = "stopped"
					status = "no servers"
				}
			}

			return services.ServiceInfo{
				Name:          normalizedName,
				Project:       "traefik",
				ContainerName: svc.Name,
				State:         state,
				Status:        status,
				Image:         "-",
				Source:        "traefik",
				Host:          s.hostName,
				Description:   fmt.Sprintf("Traefik %s service", svc.Type),
			}, nil
		}
	}

	return services.ServiceInfo{}, fmt.Errorf("service %s not found", s.name)
}

// GetLogs returns logs for the Traefik service (not supported).
func (s *TraefikService) GetLogs(ctx context.Context, tailLines int, follow bool) (io.ReadCloser, error) {
	msg := "Logs are not supported for Traefik services. " +
		"Traefik services are external services registered in Traefik and may not have accessible logs.\n"
	return io.NopCloser(bytes.NewReader([]byte(msg))), nil
}

// Start is not supported for Traefik services.
func (s *TraefikService) Start(ctx context.Context) error {
	return fmt.Errorf("start is not supported for Traefik services - these are external services managed outside this dashboard")
}

// Stop is not supported for Traefik services.
func (s *TraefikService) Stop(ctx context.Context) error {
	return fmt.Errorf("stop is not supported for Traefik services - these are external services managed outside this dashboard")
}

// Restart is not supported for Traefik services.
func (s *TraefikService) Restart(ctx context.Context) error {
	return fmt.Errorf("restart is not supported for Traefik services - these are external services managed outside this dashboard")
}

// GetName returns the service name.
func (s *TraefikService) GetName() string {
	return s.name
}

// GetHost returns the host name.
func (s *TraefikService) GetHost() string {
	return s.hostName
}

// GetSource returns "traefik".
func (s *TraefikService) GetSource() string {
	return "traefik"
}

// GetTraefikServices fetches all services from the Traefik API.
func (c *Client) GetTraefikServices(ctx context.Context) ([]TraefikAPIService, error) {
	baseURL, err := c.getAPIBaseURL(ctx)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/api/http/services", baseURL)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch services: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Traefik API returned status %d: %s", resp.StatusCode, string(body))
	}

	var traefikServices []TraefikAPIService
	if err := json.NewDecoder(resp.Body).Decode(&traefikServices); err != nil {
		return nil, fmt.Errorf("failed to decode services: %w", err)
	}

	return traefikServices, nil
}
