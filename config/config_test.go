// Package config provides shared configuration loading for the dashboard.
package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHostConfig_IsLocal(t *testing.T) {
	tests := []struct {
		name     string
		address  string
		expected bool
	}{
		{"localhost string", "localhost", true},
		{"loopback IP", "127.0.0.1", true},
		{"remote IP", "192.168.1.100", false},
		{"remote hostname", "server.example.com", false},
		{"empty address", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &HostConfig{Address: tt.address}
			if got := h.IsLocal(); got != tt.expected {
				t.Errorf("IsLocal() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestConfig_GetLocalHostName(t *testing.T) {
	tests := []struct {
		name     string
		config   Config
		expected string
	}{
		{
			name: "finds localhost by localhost address",
			config: Config{
				Hosts: []HostConfig{
					{Name: "remote", Address: "192.168.1.100"},
					{Name: "mynas", Address: "localhost"},
				},
			},
			expected: "mynas",
		},
		{
			name: "finds localhost by 127.0.0.1",
			config: Config{
				Hosts: []HostConfig{
					{Name: "localbox", Address: "127.0.0.1"},
				},
			},
			expected: "localbox",
		},
		{
			name: "returns default when no localhost",
			config: Config{
				Hosts: []HostConfig{
					{Name: "remote1", Address: "192.168.1.100"},
					{Name: "remote2", Address: "192.168.1.101"},
				},
			},
			expected: "localhost",
		},
		{
			name:     "returns default for empty hosts",
			config:   Config{Hosts: []HostConfig{}},
			expected: "localhost",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.config.GetLocalHostName(); got != tt.expected {
				t.Errorf("GetLocalHostName() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestConfig_GetHostByName(t *testing.T) {
	config := Config{
		Hosts: []HostConfig{
			{Name: "server1", Address: "192.168.1.1"},
			{Name: "server2", Address: "192.168.1.2"},
			{Name: "nas", Address: "localhost"},
		},
	}

	tests := []struct {
		name       string
		searchName string
		wantNil    bool
		wantAddr   string
	}{
		{"find existing host", "server1", false, "192.168.1.1"},
		{"find another host", "nas", false, "localhost"},
		{"host not found", "nonexistent", true, ""},
		{"empty name", "", true, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := config.GetHostByName(tt.searchName)
			if tt.wantNil {
				if got != nil {
					t.Errorf("GetHostByName(%q) = %v, want nil", tt.searchName, got)
				}
			} else {
				if got == nil {
					t.Errorf("GetHostByName(%q) = nil, want host", tt.searchName)
				} else if got.Address != tt.wantAddr {
					t.Errorf("GetHostByName(%q).Address = %v, want %v", tt.searchName, got.Address, tt.wantAddr)
				}
			}
		})
	}
}

func TestLoad(t *testing.T) {
	// Create a temporary config file
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "test_services.json")

	validJSON := `{
		"hosts": [
			{
				"name": "testhost",
				"address": "localhost",
				"systemd_services": ["docker.service", "nginx.service"],
				"docker_compose_roots": ["/home/user/projects"]
			}
		]
	}`

	err := os.WriteFile(configPath, []byte(validJSON), 0644)
	if err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	t.Run("valid config file", func(t *testing.T) {
		cfg, err := Load(configPath)
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}

		if len(cfg.Hosts) != 1 {
			t.Errorf("Expected 1 host, got %d", len(cfg.Hosts))
		}

		host := cfg.Hosts[0]
		if host.Name != "testhost" {
			t.Errorf("Host name = %v, want testhost", host.Name)
		}
		if host.Address != "localhost" {
			t.Errorf("Host address = %v, want localhost", host.Address)
		}
		if len(host.SystemdServices) != 2 {
			t.Errorf("Expected 2 systemd services, got %d", len(host.SystemdServices))
		}
		if len(host.DockerComposeRoots) != 1 {
			t.Errorf("Expected 1 docker compose root, got %d", len(host.DockerComposeRoots))
		}
	})

	t.Run("file not found", func(t *testing.T) {
		_, err := Load("/nonexistent/path/config.json")
		if err == nil {
			t.Error("Load() should return error for nonexistent file")
		}
	})

	t.Run("invalid JSON", func(t *testing.T) {
		invalidPath := filepath.Join(tempDir, "invalid.json")
		err := os.WriteFile(invalidPath, []byte("{ invalid json }"), 0644)
		if err != nil {
			t.Fatalf("Failed to write invalid config: %v", err)
		}

		_, err = Load(invalidPath)
		if err == nil {
			t.Error("Load() should return error for invalid JSON")
		}
	})
}

func TestGet(t *testing.T) {
	// Reset global config
	configMutex.Lock()
	globalConfig = nil
	configMutex.Unlock()

	// Test Get returns nil when no config loaded
	if got := Get(); got != nil {
		t.Error("Get() should return nil before Load()")
	}

	// Create and load a config
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "test.json")
	err := os.WriteFile(configPath, []byte(`{"hosts": [{"name": "test", "address": "localhost"}]}`), 0644)
	if err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	_, err = Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Now Get should return the config
	cfg := Get()
	if cfg == nil {
		t.Error("Get() should return config after Load()")
	}
	if len(cfg.Hosts) != 1 {
		t.Errorf("Expected 1 host, got %d", len(cfg.Hosts))
	}
}

func TestDefault(t *testing.T) {
	cfg := Default()

	if cfg == nil {
		t.Fatal("Default() returned nil")
	}

	if len(cfg.Hosts) != 1 {
		t.Errorf("Expected 1 default host, got %d", len(cfg.Hosts))
	}

	host := cfg.Hosts[0]
	if host.Name != "localhost" {
		t.Errorf("Default host name = %v, want localhost", host.Name)
	}
	if host.Address != "localhost" {
		t.Errorf("Default host address = %v, want localhost", host.Address)
	}
	if !host.IsLocal() {
		t.Error("Default host should be local")
	}
}
