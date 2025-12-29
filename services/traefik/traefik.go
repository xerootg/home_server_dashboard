// Package traefik provides Traefik router hostname lookup.
package traefik

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os/exec"
	"regexp"
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

// Client provides access to the Traefik API.
type Client struct {
	hostName    string
	hostAddress string
	apiPort     int
	httpClient  *http.Client
	
	// SSH tunnel management
	tunnelMu    sync.Mutex
	tunnelCmd   *exec.Cmd
	localPort   int
}

// NewClient creates a new Traefik API client.
// For remote hosts, it will create an SSH tunnel when needed.
func NewClient(hostName, hostAddress string, apiPort int) *Client {
	if apiPort == 0 {
		apiPort = 8080 // Default Traefik API port
	}
	return &Client{
		hostName:    hostName,
		hostAddress: hostAddress,
		apiPort:     apiPort,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
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
	
	// Create SSH tunnel: ssh -L localPort:localhost:apiPort -N hostAddress
	// Use -o options to make it more robust for automated use
	cmd := exec.CommandContext(ctx, "ssh",
		"-L", fmt.Sprintf("%d:localhost:%d", localPort, c.apiPort),
		"-N",                            // Don't execute remote command
		"-o", "StrictHostKeyChecking=no",
		"-o", "BatchMode=yes",           // Don't ask for password
		"-o", "ConnectTimeout=5",
		"-o", "ServerAliveInterval=30",
		"-o", "ServerAliveCountMax=3",
		c.hostAddress,
	)
	
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

// hostPattern matches Host(`hostname`) in Traefik rules.
// Supports both single and double quotes, and backticks.
var hostPattern = regexp.MustCompile(`Host\s*\(\s*[\x60"']([^)\x60"']+)[\x60"']\s*\)`)

// warnedHostRegexp tracks which routers have already logged HostRegexp warnings
// to avoid spamming logs on every refresh.
var warnedHostRegexp = make(map[string]bool)
var warnedHostRegexpMu sync.Mutex

// ExtractHostnames extracts all hostnames from a Traefik rule string.
// Handles rules like: Host(`example.com`), Host(`a.com`) || Host(`b.com`)
func ExtractHostnames(rule string) []string {
	matches := hostPattern.FindAllStringSubmatch(rule, -1)
	var hostnames []string
	for _, match := range matches {
		if len(match) > 1 {
			hostnames = append(hostnames, match[1])
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

		// Warn about HostRegexp patterns - we can't reliably extract hostnames from regex
		// Only log once per router to avoid log spam on refreshes
		if strings.Contains(router.Rule, "HostRegexp") {
			warnedHostRegexpMu.Lock()
			if !warnedHostRegexp[router.Name] {
				warnedHostRegexp[router.Name] = true
				log.Printf("Warning: router %q uses HostRegexp which cannot be extracted as a static hostname. Rule: %s", router.Name, router.Rule)
			}
			warnedHostRegexpMu.Unlock()
		}

		hostnames := ExtractHostnames(router.Rule)
		if len(hostnames) == 0 {
			continue
		}
		
		// Traefik service names may have @provider suffix, normalize to just the service name
		serviceName := normalizeServiceName(router.Service)
		
		existing := result[serviceName]
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
		result[serviceName] = existing
	}
	
	return result, nil
}

// normalizeServiceName strips the @provider suffix from Traefik service names.
// e.g., "myservice@docker" -> "myservice"
func normalizeServiceName(name string) string {
	if idx := strings.LastIndex(name, "@"); idx != -1 {
		return name[:idx]
	}
	return name
}
