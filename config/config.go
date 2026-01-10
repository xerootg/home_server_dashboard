// Package config provides shared configuration loading for the dashboard.
package config

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"sync"

	"github.com/tailscale/hujson"
)

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
	// AdminClaim is the claim name to check for admin access (default: "groups").
	// The claim value should contain "admin" or the user should have admin=true.
	AdminClaim string `json:"admin_claim,omitempty"`
}

// TraefikConfig holds Traefik API connection settings for a host.
type TraefikConfig struct {
	// Enabled determines whether to query Traefik for this host.
	Enabled bool `json:"enabled"`
	// APIPort is the port where Traefik API is listening (default 8080).
	APIPort int `json:"api_port"`
}

// HostConfig represents a single host's configuration.
type HostConfig struct {
	Name               string        `json:"name"`
	Address            string        `json:"address"`
	NIC                []string      `json:"nic"`
	SystemdServices    []string      `json:"systemd_services"`
	DockerComposeRoots []string      `json:"docker_compose_roots"`
	Traefik            TraefikConfig `json:"traefik"`
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

// Config represents the complete dashboard configuration.
type Config struct {
	Hosts []HostConfig `json:"hosts"`
	OIDC  *OIDCConfig  `json:"oidc,omitempty"`
	Local *LocalConfig `json:"local,omitempty"`
}

// IsOIDCEnabled returns true if OIDC authentication is configured and enabled.
func (c *Config) IsOIDCEnabled() bool {
	return c.OIDC != nil && c.OIDC.ConfigURL != "" && c.OIDC.ClientID != ""
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
