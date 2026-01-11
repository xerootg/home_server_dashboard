// Package homeassistant provides a service provider for Home Assistant instances.
// It uses the Home Assistant REST API to check health status and trigger actions
// like restart. For HAOS installations, it also uses the Supervisor API to
// discover and manage addons via SSH tunnel to the internal supervisor endpoint.
package homeassistant

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"

	ha "github.com/mutablelogic/go-client/pkg/homeassistant"

	"home_server_dashboard/config"
	"home_server_dashboard/services"
)

// Addon represents a Home Assistant addon from the Supervisor API.
type Addon struct {
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Description string `json:"description"`
	State       string `json:"state"`  // "started", "stopped", "error", "unknown"
	Version     string `json:"version"`
	Installed   bool   `json:"installed"`
	Available   bool   `json:"available"`
	Icon        bool   `json:"icon"` // Whether addon has icon available
	Logo        bool   `json:"logo"` // Whether addon has logo available
}

// AddonsResponse is the Supervisor API response for /addons.
type AddonsResponse struct {
	Result string  `json:"result"` // "ok" or "error"
	Data   struct {
		Addons []Addon `json:"addons"`
	} `json:"data"`
	Message string `json:"message,omitempty"` // Error message if result != "ok"
}

// SupervisorInfo is the response from /supervisor/info
type SupervisorInfo struct {
	Result string `json:"result"`
	Data   struct {
		Version string `json:"version"`
		Healthy bool   `json:"healthy"`
		Channel string `json:"channel"`
	} `json:"data"`
}

// CoreInfo is the response from /core/info
type CoreInfo struct {
	Result string `json:"result"`
	Data   struct {
		Version string `json:"version"`
		State   string `json:"state"` // "running", "stopped"
	} `json:"data"`
}

// HostInfo is the response from /host/info
type HostInfo struct {
	Result string `json:"result"`
	Data   struct {
		Hostname        string `json:"hostname"`
		OperatingSystem string `json:"operating_system"`
		Kernel          string `json:"kernel"`
	} `json:"data"`
}

// Provider implements the services.Provider interface for Home Assistant.
type Provider struct {
	hostConfig       *config.HostConfig
	client           *ha.Client
	supervisorClient *http.Client  // HTTP client for Supervisor API (tunneled through SSH)
	sshClient        *ssh.Client   // SSH connection for tunneling
	supervisorToken  string        // Token from SUPERVISOR_TOKEN env var
	hostName         string
}

// Service implements the services.Service interface for Home Assistant.
type Service struct {
	provider    *Provider
	info        services.ServiceInfo
	addonSlug   string // For addon services, the addon slug
	serviceType string // "core", "supervisor", "host", or "addon"
}

// sshTunnelTransport is an http.RoundTripper that dials through an SSH tunnel.
type sshTunnelTransport struct {
	sshClient *ssh.Client
	target    string // e.g., "supervisor:80"
}

// RoundTrip implements http.RoundTripper for SSH tunnel transport.
func (t *sshTunnelTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Dial through SSH tunnel to the target
	conn, err := t.sshClient.Dial("tcp", t.target)
	if err != nil {
		return nil, fmt.Errorf("failed to dial through SSH tunnel: %w", err)
	}

	// Create a transport that uses this connection
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return conn, nil
		},
		DisableKeepAlives: true, // Each request gets fresh connection
	}

	return transport.RoundTrip(req)
}

// getSSHClientConfig returns SSH client configuration using the user's default keys.
func getSSHClientConfig() (*ssh.ClientConfig, error) {
	// Get user's home directory for SSH keys
	currentUser, err := user.Current()
	if err != nil {
		return nil, fmt.Errorf("failed to get current user: %w", err)
	}

	// Try common SSH key locations
	keyPaths := []string{
		filepath.Join(currentUser.HomeDir, ".ssh", "id_ed25519"),
		filepath.Join(currentUser.HomeDir, ".ssh", "id_rsa"),
		filepath.Join(currentUser.HomeDir, ".ssh", "id_ecdsa"),
	}

	var signers []ssh.Signer
	for _, keyPath := range keyPaths {
		keyData, err := os.ReadFile(keyPath)
		if err != nil {
			continue // Key file doesn't exist, try next
		}

		signer, err := ssh.ParsePrivateKey(keyData)
		if err != nil {
			log.Printf("Warning: failed to parse SSH key %s: %v", keyPath, err)
			continue
		}

		signers = append(signers, signer)
	}

	if len(signers) == 0 {
		return nil, fmt.Errorf("no SSH keys found in ~/.ssh (tried id_ed25519, id_rsa, id_ecdsa)")
	}

	return &ssh.ClientConfig{
		User: "hassio",
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signers...),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // TODO: consider host key verification
		Timeout:         10 * time.Second,
	}, nil
}

// fetchSupervisorToken retrieves the SUPERVISOR_TOKEN from the SSH addon container.
// The token is stored at /run/s6/container_environment/SUPERVISOR_TOKEN inside the addon.
// This token rotates on each HAOS reboot.
func fetchSupervisorToken(client *ssh.Client) (string, error) {
	session, err := client.NewSession()
	if err != nil {
		return "", fmt.Errorf("failed to create SSH session: %w", err)
	}
	defer session.Close()

	// Read the token from the s6 container environment
	output, err := session.Output("cat /run/s6/container_environment/SUPERVISOR_TOKEN")
	if err != nil {
		return "", fmt.Errorf("failed to read SUPERVISOR_TOKEN: %w", err)
	}

	token := strings.TrimSpace(string(output))
	if token == "" {
		return "", fmt.Errorf("SUPERVISOR_TOKEN is empty")
	}

	return token, nil
}

// NewProvider creates a new Home Assistant provider for the given host config.
// Returns nil if Home Assistant is not configured for this host.
func NewProvider(hostConfig *config.HostConfig) (*Provider, error) {
	if !hostConfig.HasHomeAssistant() {
		return nil, nil
	}

	endpoint := hostConfig.GetHomeAssistantEndpoint()
	if endpoint == "" {
		return nil, fmt.Errorf("failed to build Home Assistant endpoint for host %s", hostConfig.Name)
	}

	haClient, err := ha.New(endpoint, hostConfig.HomeAssistant.LongLivedToken)
	if err != nil {
		return nil, fmt.Errorf("failed to create Home Assistant client: %w", err)
	}

	// If using HTTPS with certificate verification disabled, create an isolated
	// transport instead of using OptSkipVerify() which modifies the global http.DefaultTransport
	if hostConfig.HomeAssistant.UseHTTPS && hostConfig.HomeAssistant.IgnoreHTTPSErrors {
		haClient.Client.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
	}

	provider := &Provider{
		hostConfig: hostConfig,
		client:     haClient,
		hostName:   hostConfig.Name,
	}

	// Set up Supervisor API access via SSH tunnel if HAOS is configured
	if hostConfig.HasSupervisorAPI() {
		// Create SSH client config
		sshConfig, err := getSSHClientConfig()
		if err != nil {
			log.Printf("Warning: Failed to get SSH config for %s: %v", hostConfig.Name, err)
		} else {
			// Connect to SSH addon
			sshAddr := fmt.Sprintf("%s:%d", hostConfig.Address, hostConfig.GetSSHAddonPort())
			sshClient, err := ssh.Dial("tcp", sshAddr, sshConfig)
			if err != nil {
				log.Printf("Warning: Failed to connect to SSH addon at %s: %v", sshAddr, err)
			} else {
				provider.sshClient = sshClient

				// Fetch SUPERVISOR_TOKEN from the SSH addon container
				supervisorToken, err := fetchSupervisorToken(sshClient)
				if err != nil {
					log.Printf("Warning: Failed to fetch SUPERVISOR_TOKEN from %s: %v", sshAddr, err)
					sshClient.Close()
				} else {
					provider.supervisorToken = supervisorToken

					// Create HTTP client with SSH tunnel transport
					provider.supervisorClient = &http.Client{
						Timeout: 30 * time.Second,
						Transport: &sshTunnelTransport{
							sshClient: sshClient,
							target:    "supervisor:80",
						},
					}
					log.Printf("SSH tunnel established to %s for Supervisor API", sshAddr)
				}
			}
		}
	}

	return provider, nil
}

// Close closes any open connections (SSH tunnel).
func (p *Provider) Close() error {
	if p.sshClient != nil {
		return p.sshClient.Close()
	}
	return nil
}

// Name returns the provider name.
func (p *Provider) Name() string {
	return "homeassistant"
}

// GetServices returns services for this Home Assistant instance.
// For standard HA, returns just the core HA service.
// For HAOS, returns Core, Supervisor, Host, and all installed Addons.
func (p *Provider) GetServices(ctx context.Context) ([]services.ServiceInfo, error) {
	// Always include the core HA service
	coreInfo, err := p.getServiceInfo(ctx)
	if err != nil {
		return nil, err
	}

	// If not HAOS, just return the core service
	if !p.HasSupervisorAPI() {
		return []services.ServiceInfo{coreInfo}, nil
	}

	// For HAOS, get additional services
	servicesList := []services.ServiceInfo{coreInfo}

	// Add Supervisor service
	supervisorInfo := p.getSupervisorServiceInfo(ctx)
	servicesList = append(servicesList, supervisorInfo)

	// Add Host service
	hostInfo := p.getHostServiceInfo(ctx)
	servicesList = append(servicesList, hostInfo)

	// Add all installed addons
	addons, err := p.GetAddons(ctx)
	if err != nil {
		log.Printf("Failed to get addons from %s: %v", p.hostName, err)
	} else {
		for _, addon := range addons {
			addonInfo := p.addonToServiceInfo(addon)
			servicesList = append(servicesList, addonInfo)
		}
	}

	return servicesList, nil
}

// GetService returns a specific service by name.
func (p *Provider) GetService(name string) (services.Service, error) {
	// Core HA service
	if name == "homeassistant" || name == "ha-core" {
		return &Service{
			provider:    p,
			serviceType: "core",
		}, nil
	}

	// Supervisor service (HAOS only)
	if name == "ha-supervisor" {
		if !p.HasSupervisorAPI() {
			return nil, fmt.Errorf("supervisor not available on non-HAOS installation")
		}
		return &Service{
			provider:    p,
			serviceType: "supervisor",
		}, nil
	}

	// Host service (HAOS only)
	if name == "ha-host" {
		if !p.HasSupervisorAPI() {
			return nil, fmt.Errorf("host service not available on non-HAOS installation")
		}
		return &Service{
			provider:    p,
			serviceType: "host",
		}, nil
	}

	// Addon services (HAOS only)
	if strings.HasPrefix(name, "addon-") {
		if !p.HasSupervisorAPI() {
			return nil, fmt.Errorf("addons not available on non-HAOS installation")
		}
		slug := strings.TrimPrefix(name, "addon-")
		return &Service{
			provider:    p,
			serviceType: "addon",
			addonSlug:   slug,
		}, nil
	}

	return nil, fmt.Errorf("service not found: %s", name)
}

// getSupervisorServiceInfo builds ServiceInfo for the Supervisor.
func (p *Provider) getSupervisorServiceInfo(ctx context.Context) services.ServiceInfo {
	info := services.ServiceInfo{
		Name:          "ha-supervisor",
		Project:       "homeassistant",
		ContainerName: "hassio_supervisor",
		State:         "running",
		Status:        "Running",
		Image:         "-",
		Source:        "homeassistant",
		Host:          p.hostName,
		HostIP:        p.hostConfig.Address,
		Description:   "Home Assistant Supervisor",
	}

	// Try to get supervisor info for version
	if supervisorInfo, err := p.GetSupervisorInfo(ctx); err == nil {
		info.Status = fmt.Sprintf("v%s", supervisorInfo.Data.Version)
		if !supervisorInfo.Data.Healthy {
			info.State = "unhealthy"
			info.Status = "Unhealthy - " + info.Status
		}
	}

	return info
}

// getHostServiceInfo builds ServiceInfo for the Host OS.
func (p *Provider) getHostServiceInfo(ctx context.Context) services.ServiceInfo {
	info := services.ServiceInfo{
		Name:          "ha-host",
		Project:       "homeassistant",
		ContainerName: "host",
		State:         "running",
		Status:        "Running",
		Image:         "-",
		Source:        "homeassistant",
		Host:          p.hostName,
		HostIP:        p.hostConfig.Address,
		Description:   "Home Assistant OS Host",
	}

	// Try to get host info
	if hostInfo, err := p.GetHostInfo(ctx); err == nil {
		info.Status = fmt.Sprintf("%s (%s)", hostInfo.Data.OperatingSystem, hostInfo.Data.Kernel)
		info.Description = fmt.Sprintf("Host: %s", hostInfo.Data.Hostname)
	}

	return info
}

// addonToServiceInfo converts an Addon to ServiceInfo.
func (p *Provider) addonToServiceInfo(addon Addon) services.ServiceInfo {
	return services.ServiceInfo{
		Name:          "addon-" + addon.Slug,
		Project:       "homeassistant-addons",
		ContainerName: "addon_" + addon.Slug,
		State:         addonStateToServiceState(addon.State),
		Status:        fmt.Sprintf("%s (v%s)", addon.State, addon.Version),
		Image:         "-",
		Source:        "homeassistant-addon",
		Host:          p.hostName,
		HostIP:        p.hostConfig.Address,
		Description:   addon.Description,
	}
}

// GetLogs returns logs for the specified service.
// For HAOS, supports Core, Supervisor, Host, and Addon logs.
// For non-HAOS, returns a stub message.
func (p *Provider) GetLogs(ctx context.Context, serviceName string, tailLines int, follow bool) (io.ReadCloser, error) {
	// If we have Supervisor API access, route to appropriate log source
	if p.HasSupervisorAPI() {
		switch {
		case serviceName == "homeassistant" || serviceName == "ha-core":
			return p.GetCoreLogs(ctx, follow)
		case serviceName == "ha-supervisor":
			return p.GetSupervisorLogs(ctx, follow)
		case serviceName == "ha-host":
			return p.GetHostLogs(ctx, follow)
		case strings.HasPrefix(serviceName, "addon-"):
			slug := strings.TrimPrefix(serviceName, "addon-")
			return p.GetAddonLogs(ctx, slug, follow)
		}
	}

	// Fallback message for non-HAOS or unknown service
	msg := `═══════════════════════════════════════════════════════════════
Logs are not available for Home Assistant via this dashboard.

Home Assistant logs can be viewed through:
- The Home Assistant web UI: Settings → System → Logs
- The Home Assistant CLI: ha core logs
- Direct file access: /config/home-assistant.log

═══════════════════════════════════════════════════════════════
`
	return io.NopCloser(bytes.NewReader([]byte(msg))), nil
}

// CheckHealth checks if the Home Assistant API is reachable.
// Returns "running" if healthy, "stopped" if unreachable.
func (p *Provider) CheckHealth(ctx context.Context) (state, status string, err error) {
	msg, err := p.client.Health(ctx)
	if err != nil {
		return "stopped", "Unreachable: " + err.Error(), err
	}

	if msg == "API running." {
		return "running", "API running", nil
	}

	return "running", msg, nil
}

// getServiceInfo builds the ServiceInfo for the Home Assistant instance.
func (p *Provider) getServiceInfo(ctx context.Context) (services.ServiceInfo, error) {
	state, status, err := p.CheckHealth(ctx)

	// Build port info for the HA web UI
	port := p.hostConfig.HomeAssistant.Port
	if port == 0 {
		port = 8123
	}

	info := services.ServiceInfo{
		Name:          "homeassistant",
		Project:       "homeassistant",
		ContainerName: "homeassistant",
		State:         state,
		Status:        status,
		Image:         "-",
		Source:        "homeassistant",
		Host:          p.hostName,
		HostIP:        p.hostConfig.Address,
		Ports: []services.PortInfo{
			{
				HostPort:      uint16(port),
				ContainerPort: uint16(port),
				Protocol:      "tcp",
				Label:         "Web UI",
			},
		},
		TraefikURLs: nil,
		Description: "Home Assistant home automation platform",
	}

	if err != nil {
		log.Printf("Home Assistant on %s is unreachable: %v", p.hostName, err)
	}

	return info, nil // Return info even on error so service appears in list
}

// Restart triggers a Home Assistant restart via the homeassistant.restart service.
func (p *Provider) Restart(ctx context.Context) error {
	// Call the homeassistant.restart service
	// The Home Assistant API endpoint for this is POST /api/services/homeassistant/restart
	_, err := p.client.Call(ctx, "restart", "homeassistant.homeassistant")
	if err != nil {
		// Try alternative entity format
		_, err = p.client.Call(ctx, "restart", "")
		if err != nil {
			return fmt.Errorf("failed to restart Home Assistant: %w", err)
		}
	}
	return nil
}

// ============================================================================
// Supervisor API methods (for HAOS installations via SSH tunnel)
// ============================================================================

// supervisorRequest makes an authenticated request to the Supervisor API via SSH tunnel.
// The request is tunneled through SSH to http://supervisor:80 on the HAOS host.
func (p *Provider) supervisorRequest(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	if p.supervisorClient == nil {
		return nil, fmt.Errorf("supervisor API not configured - check SSH connection")
	}

	if p.supervisorToken == "" {
		return nil, fmt.Errorf("SUPERVISOR_TOKEN not available - SSH addon may not have token access")
	}

	// The URL is to "supervisor" which is the internal HA supervisor hostname
	// This gets tunneled through SSH to the actual supervisor
	url := "http://supervisor" + path
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+p.supervisorToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	return p.supervisorClient.Do(req)
}

// supervisorLogsRequest makes a request to a Supervisor logs endpoint.
// Uses Accept: text/plain since logs are returned as plain text, not JSON.
func (p *Provider) supervisorLogsRequest(ctx context.Context, path string) (*http.Response, error) {
	if p.supervisorClient == nil {
		return nil, fmt.Errorf("supervisor API not configured - check SSH connection")
	}

	if p.supervisorToken == "" {
		return nil, fmt.Errorf("SUPERVISOR_TOKEN not available - SSH addon may not have token access")
	}

	url := "http://supervisor" + path
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+p.supervisorToken)
	req.Header.Set("Accept", "text/plain")

	return p.supervisorClient.Do(req)
}

// GetAddons returns the list of installed addons from the Supervisor API.
func (p *Provider) GetAddons(ctx context.Context) ([]Addon, error) {
	resp, err := p.supervisorRequest(ctx, "GET", "/addons", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("supervisor API returned %d: %s", resp.StatusCode, string(body))
	}

	var addonsResp AddonsResponse
	if err := json.NewDecoder(resp.Body).Decode(&addonsResp); err != nil {
		return nil, fmt.Errorf("failed to decode addons response: %w", err)
	}

	if addonsResp.Result != "ok" {
		return nil, fmt.Errorf("supervisor API error: %s", addonsResp.Message)
	}

	// The /addons endpoint returns all addons the system knows about.
	// Filter to addons that are available (meaning they are installed).
	var installed []Addon
	for _, addon := range addonsResp.Data.Addons {
		if addon.Available {
			installed = append(installed, addon)
		}
	}

	return installed, nil
}

// GetAddonLogs returns the logs for a specific addon.
func (p *Provider) GetAddonLogs(ctx context.Context, slug string, follow bool) (io.ReadCloser, error) {
	path := fmt.Sprintf("/addons/%s/logs", slug)
	if follow {
		path += "/follow"
	}

	resp, err := p.supervisorLogsRequest(ctx, path)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("supervisor API returned %d: %s", resp.StatusCode, string(body))
	}

	return resp.Body, nil
}

// GetCoreLogs returns Home Assistant Core logs.
func (p *Provider) GetCoreLogs(ctx context.Context, follow bool) (io.ReadCloser, error) {
	path := "/core/logs"
	if follow {
		path += "/follow"
	}

	resp, err := p.supervisorLogsRequest(ctx, path)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("supervisor API returned %d: %s", resp.StatusCode, string(body))
	}

	return resp.Body, nil
}

// GetSupervisorLogs returns Supervisor logs.
func (p *Provider) GetSupervisorLogs(ctx context.Context, follow bool) (io.ReadCloser, error) {
	path := "/supervisor/logs"
	if follow {
		path += "/follow"
	}

	resp, err := p.supervisorLogsRequest(ctx, path)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("supervisor API returned %d: %s", resp.StatusCode, string(body))
	}

	return resp.Body, nil
}

// GetHostLogs returns Host OS logs.
func (p *Provider) GetHostLogs(ctx context.Context, follow bool) (io.ReadCloser, error) {
	path := "/host/logs"
	if follow {
		path += "/follow"
	}

	resp, err := p.supervisorLogsRequest(ctx, path)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("supervisor API returned %d: %s", resp.StatusCode, string(body))
	}

	return resp.Body, nil
}

// GetSupervisorInfo returns info about the Supervisor.
func (p *Provider) GetSupervisorInfo(ctx context.Context) (*SupervisorInfo, error) {
	resp, err := p.supervisorRequest(ctx, "GET", "/supervisor/info", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("supervisor API returned %d: %s", resp.StatusCode, string(body))
	}

	var info SupervisorInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("failed to decode supervisor info: %w", err)
	}

	return &info, nil
}

// GetCoreInfo returns info about Home Assistant Core.
func (p *Provider) GetCoreInfo(ctx context.Context) (*CoreInfo, error) {
	resp, err := p.supervisorRequest(ctx, "GET", "/core/info", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("supervisor API returned %d: %s", resp.StatusCode, string(body))
	}

	var info CoreInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("failed to decode core info: %w", err)
	}

	return &info, nil
}

// GetHostInfo returns info about the Host OS.
func (p *Provider) GetHostInfo(ctx context.Context) (*HostInfo, error) {
	resp, err := p.supervisorRequest(ctx, "GET", "/host/info", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("supervisor API returned %d: %s", resp.StatusCode, string(body))
	}

	var info HostInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("failed to decode host info: %w", err)
	}

	return &info, nil
}

// AddonControl controls an addon (start, stop, restart).
func (p *Provider) AddonControl(ctx context.Context, slug, action string) error {
	if action != "start" && action != "stop" && action != "restart" {
		return fmt.Errorf("invalid action: %s", action)
	}

	path := fmt.Sprintf("/addons/%s/%s", slug, action)
	resp, err := p.supervisorRequest(ctx, "POST", path, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("addon %s %s failed (%d): %s", slug, action, resp.StatusCode, string(body))
	}

	return nil
}

// CoreControl controls HA Core via Supervisor API (start, stop, restart).
// This is only available on HAOS installations with Supervisor API access.
func (p *Provider) CoreControl(ctx context.Context, action string) error {
	if action != "start" && action != "stop" && action != "restart" {
		return fmt.Errorf("invalid action: %s", action)
	}

	path := fmt.Sprintf("/core/%s", action)
	resp, err := p.supervisorRequest(ctx, "POST", path, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("core %s failed (%d): %s", action, resp.StatusCode, string(body))
	}

	return nil
}

// HasSupervisorAPI returns true if the Supervisor API is available.
func (p *Provider) HasSupervisorAPI() bool {
	return p.supervisorClient != nil && p.hostConfig.HasSupervisorAPI()
}

// addonStateToServiceState converts addon state to standardized service state.
func addonStateToServiceState(state string) string {
	switch strings.ToLower(state) {
	case "started":
		return "running"
	case "stopped", "error", "unknown":
		return "stopped"
	default:
		return "stopped"
	}
}

// Service interface implementation

// GetInfo returns the current status of the service.
func (s *Service) GetInfo(ctx context.Context) (services.ServiceInfo, error) {
	switch s.serviceType {
	case "supervisor":
		return s.provider.getSupervisorServiceInfo(ctx), nil
	case "host":
		return s.provider.getHostServiceInfo(ctx), nil
	case "addon":
		addons, err := s.provider.GetAddons(ctx)
		if err != nil {
			return services.ServiceInfo{}, err
		}
		for _, addon := range addons {
			if addon.Slug == s.addonSlug {
				return s.provider.addonToServiceInfo(addon), nil
			}
		}
		return services.ServiceInfo{}, fmt.Errorf("addon not found: %s", s.addonSlug)
	default: // "core" or empty
		return s.provider.getServiceInfo(ctx)
	}
}

// GetLogs returns logs for this service.
func (s *Service) GetLogs(ctx context.Context, tailLines int, follow bool) (io.ReadCloser, error) {
	return s.provider.GetLogs(ctx, s.GetName(), tailLines, follow)
}

// Start starts the service.
// Supported for addons and HA Core (HAOS only via Supervisor API).
func (s *Service) Start(ctx context.Context) error {
	switch s.serviceType {
	case "addon":
		return s.provider.AddonControl(ctx, s.addonSlug, "start")
	case "core", "":
		// Core start is only supported via Supervisor API on HAOS
		if s.provider.HasSupervisorAPI() {
			return s.provider.CoreControl(ctx, "start")
		}
		return fmt.Errorf("start is not supported for Home Assistant Core on non-HAOS installations")
	default:
		return fmt.Errorf("start is not supported for %s", s.GetName())
	}
}

// Stop stops the service.
// Supported for addons and HA Core (HAOS only via Supervisor API).
func (s *Service) Stop(ctx context.Context) error {
	switch s.serviceType {
	case "addon":
		return s.provider.AddonControl(ctx, s.addonSlug, "stop")
	case "core", "":
		// Core stop is only supported via Supervisor API on HAOS
		if s.provider.HasSupervisorAPI() {
			return s.provider.CoreControl(ctx, "stop")
		}
		return fmt.Errorf("stop is not supported for Home Assistant Core on non-HAOS installations")
	default:
		return fmt.Errorf("stop is not supported for %s", s.GetName())
	}
}

// Restart restarts the service.
// For HA Core on HAOS, uses Supervisor API. Otherwise, uses HA REST API.
func (s *Service) Restart(ctx context.Context) error {
	switch s.serviceType {
	case "addon":
		return s.provider.AddonControl(ctx, s.addonSlug, "restart")
	case "supervisor", "host":
		return fmt.Errorf("restart is not supported for %s", s.GetName())
	default: // "core" or empty
		// Prefer Supervisor API on HAOS, fallback to HA REST API
		if s.provider.HasSupervisorAPI() {
			return s.provider.CoreControl(ctx, "restart")
		}
		return s.provider.Restart(ctx)
	}
}

// GetName returns the service name.
func (s *Service) GetName() string {
	switch s.serviceType {
	case "supervisor":
		return "ha-supervisor"
	case "host":
		return "ha-host"
	case "addon":
		return "addon-" + s.addonSlug
	default:
		return "homeassistant"
	}
}

// GetHost returns the host name where the service runs.
func (s *Service) GetHost() string {
	return s.provider.hostName
}

// GetSource returns the service source type.
func (s *Service) GetSource() string {
	if s.serviceType == "addon" {
		return "homeassistant-addon"
	}
	return "homeassistant"
}
