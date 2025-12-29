// Package config provides shared configuration loading for the dashboard.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

// HostConfig represents a single host's configuration.
type HostConfig struct {
	Name               string   `json:"name"`
	Address            string   `json:"address"`
	SystemdServices    []string `json:"systemd_services"`
	DockerComposeRoots []string `json:"docker_compose_roots"`
}

// IsLocal returns true if this host is the local machine.
func (h *HostConfig) IsLocal() bool {
	return h.Address == "localhost" || h.Address == "127.0.0.1"
}

// Config represents the complete dashboard configuration.
type Config struct {
	Hosts []HostConfig `json:"hosts"`
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
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", path, err)
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

// Get returns the currently loaded global configuration.
func Get() *Config {
	configMutex.RLock()
	defer configMutex.RUnlock()
	return globalConfig
}

// Default returns a default configuration for when no config file exists.
func Default() *Config {
	return &Config{
		Hosts: []HostConfig{
			{
				Name:               "localhost",
				Address:            "localhost",
				SystemdServices:    []string{},
				DockerComposeRoots: []string{},
			},
		},
	}
}
