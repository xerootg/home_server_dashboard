// Package traefik provides Traefik router hostname lookup.
package traefik

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// Config holds Traefik API connection settings.
type Config struct {
	// Enabled determines whether to query Traefik for this host.
	Enabled bool `json:"enabled"`
	// APIPort is the port where Traefik API is listening (default 8080).
	APIPort int `json:"api_port"`
}

// Router represents a Traefik HTTP router from the API response.
type Router struct {
	Name        string `json:"name"`
	Rule        string `json:"rule"`
	Service     string `json:"service"`
	Status      string `json:"status"`
	EntryPoints []string `json:"entryPoints,omitempty"`
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

// Client provides access to the Traefik API.
type Client struct {
	hostName    string
	hostAddress string
	apiPort     int
	httpClient  *http.Client
	sshConfig   *SSHConfig

	// SSH tunnel management
	tunnelMu  sync.Mutex
	tunnelCmd *exec.Cmd
	localPort int

	// Matcher lookup service for hostname extraction with state tracking
	matcherService *MatcherLookupService
}

// NewClient creates a new Traefik API client.
// For remote hosts, it will create an SSH tunnel when needed.
// sshConfig is optional and only used for remote hosts.
func NewClient(hostName, hostAddress string, apiPort int, sshConfig *SSHConfig) *Client {
	if apiPort == 0 {
		apiPort = 8080 // Default Traefik API port
	}
	return &Client{
		hostName:       hostName,
		hostAddress:    hostAddress,
		apiPort:        apiPort,
		sshConfig:      sshConfig,
		matcherService: NewMatcherLookupService(hostName),
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// getSSHTarget returns the SSH target string (user@host or just host).
func (c *Client) getSSHTarget() string {
	if c.sshConfig != nil && c.sshConfig.Username != "" {
		return c.sshConfig.Username + "@" + c.hostAddress
	}
	return c.hostAddress
}

// isLocal returns true if the host address is localhost.
func (c *Client) isLocal() bool {
	return c.hostAddress == "localhost" || c.hostAddress == "127.0.0.1"
}

// getAPIBaseURL returns the base URL for the Traefik API.
// For remote hosts, this sets up an SSH tunnel and returns the local tunnel endpoint.
func (c *Client) getAPIBaseURL(ctx context.Context) (string, error) {
	if c.isLocal() {
		return fmt.Sprintf("http://localhost:%d", c.apiPort), nil
	}
	
	// For remote hosts, use SSH tunnel
	localPort, err := c.ensureTunnel(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to create SSH tunnel: %w", err)
	}
	
	return fmt.Sprintf("http://localhost:%d", localPort), nil
}

// ensureTunnel creates an SSH tunnel to the remote Traefik API port.
// Returns the local port to connect to.
func (c *Client) ensureTunnel(ctx context.Context) (int, error) {
	c.tunnelMu.Lock()
	defer c.tunnelMu.Unlock()
	
	// Check if existing tunnel is still running
	if c.tunnelCmd != nil && c.tunnelCmd.Process != nil {
		// Check if process is still alive
		if c.tunnelCmd.ProcessState == nil || !c.tunnelCmd.ProcessState.Exited() {
			return c.localPort, nil
		}
		// Tunnel died, clean up
		c.tunnelCmd = nil
		c.localPort = 0
	}
	
	// Find a free local port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("failed to find free port: %w", err)
	}
	localPort := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	
	// Build SSH arguments
	// Create SSH tunnel: ssh -L localPort:localhost:apiPort -N hostAddress
	// Use -o options to make it more robust for automated use
	sshArgs := []string{
		"-L", fmt.Sprintf("%d:localhost:%d", localPort, c.apiPort),
		"-N",                            // Don't execute remote command
		"-o", "StrictHostKeyChecking=no",
		"-o", "BatchMode=yes",           // Don't ask for password
		"-o", "ConnectTimeout=5",
		"-o", "ServerAliveInterval=30",
		"-o", "ServerAliveCountMax=3",
	}
	// Add custom SSH port if configured
	if c.sshConfig != nil && c.sshConfig.Port > 0 {
		sshArgs = append(sshArgs, "-p", fmt.Sprintf("%d", c.sshConfig.Port))
	}
	// Add target (user@host or just host)
	sshArgs = append(sshArgs, c.getSSHTarget())
	
	cmd := exec.CommandContext(ctx, "ssh", sshArgs...)
	
	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("failed to start SSH tunnel: %w", err)
	}
	
	// Wait a moment for tunnel to establish
	time.Sleep(500 * time.Millisecond)
	
	// Verify tunnel is working by trying to connect
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("localhost:%d", localPort), 2*time.Second)
	if err != nil {
		cmd.Process.Kill()
		cmd.Wait()
		return 0, fmt.Errorf("SSH tunnel failed to establish: %w", err)
	}
	conn.Close()
	
	c.tunnelCmd = cmd
	c.localPort = localPort
	
	return localPort, nil
}

// Close cleans up any SSH tunnel resources.
func (c *Client) Close() error {
	c.tunnelMu.Lock()
	defer c.tunnelMu.Unlock()
	
	if c.tunnelCmd != nil && c.tunnelCmd.Process != nil {
		c.tunnelCmd.Process.Kill()
		c.tunnelCmd.Wait()
		c.tunnelCmd = nil
		c.localPort = 0
	}
	return nil
}

// GetRouters fetches all HTTP routers from the Traefik API.
func (c *Client) GetRouters(ctx context.Context) ([]Router, error) {
	baseURL, err := c.getAPIBaseURL(ctx)
	if err != nil {
		return nil, err
	}
	
	url := fmt.Sprintf("%s/api/http/routers", baseURL)
	
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch routers: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Traefik API returned status %d: %s", resp.StatusCode, string(body))
	}
	
	var routers []Router
	if err := json.NewDecoder(resp.Body).Decode(&routers); err != nil {
		return nil, fmt.Errorf("failed to decode routers: %w", err)
	}
	
	return routers, nil
}

// ExtractHostnames extracts all hostnames from a Traefik rule string.
// Handles rules like: Host(`example.com`), Host(`a.com`) || Host(`b.com`)
// Also handles HostRegexp patterns where possible.
// This is a convenience function that creates a temporary matcher service.
func ExtractHostnames(rule string) []string {
	matchers := ExtractMatchers(rule)
	var hostnames []string
	seen := make(map[string]bool)
	for _, m := range matchers {
		if !seen[m.Hostname] {
			hostnames = append(hostnames, m.Hostname)
			seen[m.Hostname] = true
		}
	}
	return hostnames
}

// ServiceHostMapping maps a service name to its Traefik hostnames.
type ServiceHostMapping struct {
	ServiceName string
	Hostnames   []string
}

// GetServiceHostMappings fetches all routers and extracts service->hostname mappings.
// The serviceName in the mapping will be the Traefik service name (e.g., "myservice@docker").
// Uses the MatcherLookupService to track state changes and log appropriately.
// Also returns router-name-based mappings for cases where a Docker service creates a router
// that points to a different backend (e.g., jellyfin@docker router -> jellyfin-svc@file service).
func (c *Client) GetServiceHostMappings(ctx context.Context) (map[string][]string, error) {
	routers, err := c.GetRouters(ctx)
	if err != nil {
		return nil, err
	}

	result := make(map[string][]string)
	for _, router := range routers {
		if router.Status != "enabled" {
			continue
		}

		// Use the matcher service to process the router with state tracking
		hostnames := c.matcherService.ProcessRouter(router.Name, router.Rule, nil)
		if len(hostnames) == 0 {
			continue
		}

		// Traefik service names may have @provider suffix, normalize to just the service name
		serviceName := normalizeServiceName(router.Service)

		// Add mapping for the service name
		addHostnames(result, serviceName, hostnames)

		// Also add mapping for the router name, in case it differs from the service name.
		// This handles cases where a Docker service creates a router that points to a different
		// backend (e.g., Docker labels create "jellyfin@docker" router that points to "jellyfin-svc@file").
		// The Docker service "jellyfin" should still show the Traefik URL.
		routerName := normalizeServiceName(router.Name)
		if routerName != serviceName {
			addHostnames(result, routerName, hostnames)
		}
	}

	return result, nil
}

// addHostnames adds hostnames to a result map, avoiding duplicates.
func addHostnames(result map[string][]string, key string, hostnames []string) {
	existing := result[key]
	for _, h := range hostnames {
		// Avoid duplicates
		found := false
		for _, e := range existing {
			if e == h {
				found = true
				break
			}
		}
		if !found {
			existing = append(existing, h)
		}
	}
	result[key] = existing
}

// GetMatcherService returns the matcher lookup service for advanced state management.
func (c *Client) GetMatcherService() *MatcherLookupService {
	return c.matcherService
}

// GetClaimedBackendServices returns a map of backend service names that are "claimed" by
// routers owned by Docker or other providers. This is used to filter out Traefik services
// that should not appear separately because their router is owned by an existing service.
// For example, if Docker service "jellyfin" creates router "jellyfin@docker" pointing to
// "jellyfin-svc@file", then "jellyfin-svc" should be filtered out.
// existingServices is a set of service names that already exist (Docker/systemd).
func (c *Client) GetClaimedBackendServices(ctx context.Context, existingServices map[string]bool) (map[string]bool, error) {
	routers, err := c.GetRouters(ctx)
	if err != nil {
		return nil, err
	}

	claimed := make(map[string]bool)
	for _, router := range routers {
		// Normalize router name (strip @provider suffix)
		routerName := normalizeServiceName(router.Name)

		// Check if the router is owned by an existing Docker/systemd service
		if existingServices[routerName] {
			// This router is owned by an existing service
			// Mark its backend service as claimed
			backendName := normalizeServiceName(router.Service)
			if backendName != routerName {
				// Only mark if the backend is different from the router owner
				claimed[backendName] = true
			}
		}
	}

	return claimed, nil
}

// normalizeServiceName strips the @provider suffix from Traefik service names.
// e.g., "myservice@docker" -> "myservice"
func normalizeServiceName(name string) string {
	if idx := strings.LastIndex(name, "@"); idx != -1 {
		return name[:idx]
	}
	return name
}
