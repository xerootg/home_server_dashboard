// Package config provides shared configuration loading for the dashboard.
package config

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"sync"

	"github.com/tailscale/hujson"
)

// OIDCGroupConfig defines the services a group can access.
type OIDCGroupConfig struct {
	// Services maps host names to lists of service names the group can access.
	// Service names can be Docker service names or systemd unit names.
	Services map[string][]string `json:"services"`
}

// OIDCConfig holds OpenID Connect authentication settings.
type OIDCConfig struct {
	// ServiceURL is the dashboard's public URL (used for redirect URI).
	ServiceURL string `json:"service_url"`
	// Callback is the path for the OIDC callback endpoint.
	Callback string `json:"callback"`
	// ConfigURL is the OIDC provider's discovery URL (.well-known/openid-configuration).
	ConfigURL string `json:"config_url"`
	// ClientID is the OAuth2 client ID.
	ClientID string `json:"client_id"`
	// ClientSecret is the OAuth2 client secret.
	ClientSecret string `json:"client_secret"`
	// GroupsClaim is the claim name where user groups are found (default: "groups").
	GroupsClaim string `json:"groups_claim,omitempty"`
	// AdminGroup is the group name that grants admin/global access (default: "admin").
	AdminGroup string `json:"admin_group,omitempty"`
	// Groups maps OIDC group names to their allowed services configuration.
	// Users who are members of these groups will have access to the specified services.
	// Group permissions are additive - a user in multiple groups gets access to all services.
	Groups map[string]*OIDCGroupConfig `json:"groups,omitempty"`
}

// TraefikConfig holds Traefik API connection settings for a host.
type TraefikConfig struct {
	// Enabled determines whether to query Traefik for this host.
	Enabled bool `json:"enabled"`
	// APIPort is the port where Traefik API is listening (default 8080).
	APIPort int `json:"api_port"`
}

// HomeAssistantConfig holds Home Assistant API connection settings for a host.
type HomeAssistantConfig struct {
	// Port is the Home Assistant API port (default 8123).
	Port int `json:"port"`
	// UseHTTPS determines whether to use HTTPS for the API connection.
	UseHTTPS bool `json:"use_https"`
	// IgnoreHTTPSErrors skips TLS certificate verification (for self-signed certs).
	IgnoreHTTPSErrors bool `json:"ignore_https_errors"`
	// LongLivedToken is the Home Assistant long-lived access token for API auth.
	LongLivedToken string `json:"longlivedtoken"`
	// IsHomeAssistantOS indicates this is a Home Assistant OS installation with Supervisor.
	// When true, the dashboard will query for addons, core, supervisor, and host logs.
	IsHomeAssistantOS bool `json:"is_homeassistant_operatingsystem"`
	// SSHAddonPort is the port for the SSH & Web Terminal addon (default 22).
	// Used to tunnel Supervisor API requests through SSH.
	// The SUPERVISOR_TOKEN environment variable must be set on the dashboard host.
	SSHAddonPort int `json:"ssh_addon_port,omitempty"`
}

// HostConfig represents a single host's configuration.
type HostConfig struct {
	Name               string               `json:"name"`
	Address            string               `json:"address"`
	NIC                []string             `json:"nic"`
	SystemdServices    []string             `json:"systemd_services"`
	DockerComposeRoots []string             `json:"docker_compose_roots"`
	Traefik            TraefikConfig        `json:"traefik"`
	HomeAssistant      *HomeAssistantConfig `json:"homeassistant,omitempty"`
}

// HasHomeAssistant returns true if this host has Home Assistant configured.
func (h *HostConfig) HasHomeAssistant() bool {
	return h.HomeAssistant != nil && h.HomeAssistant.LongLivedToken != ""
}

// HasSupervisorAPI returns true if this host has Supervisor API access configured.
// Requires HAOS mode and SSH addon port, plus SUPERVISOR_TOKEN environment variable.
func (h *HostConfig) HasSupervisorAPI() bool {
	return h.HasHomeAssistant() &&
		h.HomeAssistant.IsHomeAssistantOS &&
		h.HomeAssistant.SSHAddonPort > 0
}

// GetSSHAddonPort returns the SSH addon port for tunneling Supervisor API requests.
// Returns 22 as default if not specified.
func (h *HostConfig) GetSSHAddonPort() int {
	if h.HomeAssistant == nil || h.HomeAssistant.SSHAddonPort == 0 {
		return 22
	}
	return h.HomeAssistant.SSHAddonPort
}

// GetHomeAssistantEndpoint returns the full URL for the Home Assistant API.
// Returns empty string if Home Assistant is not configured.
func (h *HostConfig) GetHomeAssistantEndpoint() string {
	if !h.HasHomeAssistant() {
		return ""
	}
	port := h.HomeAssistant.Port
	if port == 0 {
		port = 8123
	}
	scheme := "http"
	if h.HomeAssistant.UseHTTPS {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s:%d/api/", scheme, h.Address, port)
}

// IsLocal returns true if this host is the local machine.
func (h *HostConfig) IsLocal() bool {
	return h.Address == "localhost" || h.Address == "127.0.0.1"
}

// isPrivateIP checks if an IP address is a private network address.
func isPrivateIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	// Check for private IPv4 ranges: 10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16
	private := []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
	}
	for _, cidr := range private {
		_, block, _ := net.ParseCIDR(cidr)
		if block.Contains(ip) {
			return true
		}
	}
	return false
}

// GetPrivateIP returns the private network IP address for this host.
// If the host address is already a private IP, it returns that.
// If NIC interfaces are specified, it tries to get the IP from those interfaces.
// Returns empty string if no private IP can be determined.
func (h *HostConfig) GetPrivateIP() string {
	// First, check if address is already a private IP
	if ip := net.ParseIP(h.Address); ip != nil && isPrivateIP(ip) {
		return h.Address
	}

	// Try to get IP from specified NIC interfaces (only works for local host)
	if h.IsLocal() && len(h.NIC) > 0 {
		for _, nicName := range h.NIC {
			iface, err := net.InterfaceByName(nicName)
			if err != nil {
				continue
			}
			addrs, err := iface.Addrs()
			if err != nil {
				continue
			}
			for _, addr := range addrs {
				var ip net.IP
				switch v := addr.(type) {
				case *net.IPNet:
					ip = v.IP
				case *net.IPAddr:
					ip = v.IP
				}
				// Return first private IPv4 address found
				if ip != nil && ip.To4() != nil && isPrivateIP(ip) {
					return ip.String()
				}
			}
		}
	}

	return ""
}

// LocalConfig holds local authentication settings for non-OIDC access.
type LocalConfig struct {
	// Admins is a comma-separated list of local usernames with admin access.
	Admins string `json:"admins"`
}

// GotifyConfig holds Gotify notification settings.
type GotifyConfig struct {
	// Enabled determines whether Gotify notifications are active.
	Enabled bool `json:"enabled"`
	// Hostname is the Gotify server URL (e.g., "https://gotify.example.com").
	Hostname string `json:"hostname"`
	// Token is the application token for sending messages.
	Token string `json:"token"`
}

// IsValid returns true if the Gotify configuration is complete and enabled.
func (g *GotifyConfig) IsValid() bool {
	return g != nil && g.Enabled && g.Hostname != "" && g.Token != ""
}

// Config represents the complete dashboard configuration.
type Config struct {
	Hosts  []HostConfig  `json:"hosts"`
	OIDC   *OIDCConfig   `json:"oidc,omitempty"`
	Local  *LocalConfig  `json:"local,omitempty"`
	Gotify *GotifyConfig `json:"gotify,omitempty"`
	// Port is the HTTP server port (default 9001).
	Port int `json:"port,omitempty"`
}

// IsOIDCEnabled returns true if OIDC authentication is configured and enabled.
func (c *Config) IsOIDCEnabled() bool {
	return c.OIDC != nil && c.OIDC.ConfigURL != "" && c.OIDC.ClientID != ""
}

// GetPort returns the configured HTTP server port, or 9001 if not specified.
func (c *Config) GetPort() int {
	if c.Port == 0 {
		return 9001
	}
	return c.Port
}

// GetLocalHostName returns the name of the localhost host config, or "localhost" if not found.
func (c *Config) GetLocalHostName() string {
	for _, host := range c.Hosts {
		if host.IsLocal() {
			return host.Name
		}
	}
	return "localhost"
}

// GetHostByName returns the host config with the given name, or nil if not found.
func (c *Config) GetHostByName(name string) *HostConfig {
	for i := range c.Hosts {
		if c.Hosts[i].Name == name {
			return &c.Hosts[i]
		}
	}
	return nil
}

// Global configuration instance
var (
	globalConfig *Config
	configMutex  sync.RWMutex
)

// Load reads and parses the configuration file.
// Supports JSON with comments (//, /* */) and trailing commas.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", path, err)
	}

	// Sanitize JSON: strip comments and trailing commas
	data, err = standardizeJSON(data)
	if err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", path, err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", path, err)
	}

	// Store as global config
	configMutex.Lock()
	globalConfig = &cfg
	configMutex.Unlock()

	return &cfg, nil
}

// standardizeJSON strips comments and trailing commas from JSON.
func standardizeJSON(b []byte) ([]byte, error) {
	ast, err := hujson.Parse(b)
	if err != nil {
		return nil, err
	}
	ast.Standardize()
	return ast.Pack(), nil
}

// Get returns the currently loaded global configuration.
func Get() *Config {
	configMutex.RLock()
	defer configMutex.RUnlock()
	return globalConfig
}

// Default returns a default configuration for when no config file exists.
// It also stores the default as the global configuration.
func Default() *Config {
	cfg := &Config{
		Hosts: []HostConfig{
			{
				Name:               "localhost",
				Address:            "localhost",
				SystemdServices:    []string{},
				DockerComposeRoots: []string{},
			},
		},
	}

	// Store as global config
	configMutex.Lock()
	globalConfig = cfg
	configMutex.Unlock()

	return cfg
}

// GetAllConfiguredServices returns a set of all services configured across all hosts.
// The returned map has keys in the format "host:service" for quick lookup.
func (c *Config) GetAllConfiguredServices() map[string]bool {
	services := make(map[string]bool)
	for _, host := range c.Hosts {
		// Add systemd services
		for _, svc := range host.SystemdServices {
			services[host.Name+":"+svc] = true
		}
		// Note: Docker services are discovered at runtime, not from config,
		// so we can't validate them at startup. Those will be checked at runtime.
	}
	return services
}

// ValidateGroupConfigs checks if services referenced in group configs exist in the host config.
// It logs warnings for any services that are referenced but not found.
// Note: This only validates systemd services since Docker services are runtime-discovered.
// Docker service validation happens at runtime when services are filtered.
func (c *Config) ValidateGroupConfigs() {
	if c.OIDC == nil || c.OIDC.Groups == nil {
		return
	}

	configuredServices := c.GetAllConfiguredServices()

	// Track hosts that exist
	validHosts := make(map[string]bool)
	for _, host := range c.Hosts {
		validHosts[host.Name] = true
	}

	for groupName, groupConfig := range c.OIDC.Groups {
		if groupConfig == nil || groupConfig.Services == nil {
			continue
		}

		for hostName, services := range groupConfig.Services {
			// Check if host exists
			if !validHosts[hostName] {
				log.Printf("Warning: OIDC group '%s' references non-existent host '%s'", groupName, hostName)
				continue
			}

			for _, svcName := range services {
				key := hostName + ":" + svcName
				// Only warn for systemd services (which are in config)
				// Docker services might be valid but we can't check until runtime
				if !configuredServices[key] {
					// Check if it might be a Docker service (not ending in .service or .timer)
					if !isSystemdUnit(svcName) {
						// Could be a Docker service, which we can't validate at startup
						continue
					}
					log.Printf("Warning: OIDC group '%s' references non-existent systemd service '%s' on host '%s'",
						groupName, svcName, hostName)
				}
			}
		}
	}
}

// isSystemdUnit checks if a service name looks like a systemd unit.
func isSystemdUnit(name string) bool {
	suffixes := []string{".service", ".timer", ".socket", ".mount", ".target", ".path", ".scope", ".slice"}
	for _, suffix := range suffixes {
		if len(name) > len(suffix) && name[len(name)-len(suffix):] == suffix {
			return true
		}
	}
	return false
}
