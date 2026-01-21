// Package config provides shared configuration loading for the dashboard.
package config

import (
	"net"
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

	t.Run("JSON with line comments", func(t *testing.T) {
		commentPath := filepath.Join(tempDir, "comments.json")
		jsonWithComments := `{
			// This is a comment
			"hosts": [
				{
					"name": "testhost", // inline comment
					"address": "localhost",
					"systemd_services": ["docker.service"],
					"docker_compose_roots": []
				}
			]
		}`
		err := os.WriteFile(commentPath, []byte(jsonWithComments), 0644)
		if err != nil {
			t.Fatalf("Failed to write config with comments: %v", err)
		}

		cfg, err := Load(commentPath)
		if err != nil {
			t.Fatalf("Load() should handle line comments, got error = %v", err)
		}
		if len(cfg.Hosts) != 1 || cfg.Hosts[0].Name != "testhost" {
			t.Errorf("Config not parsed correctly with comments")
		}
	})

	t.Run("JSON with block comments", func(t *testing.T) {
		blockCommentPath := filepath.Join(tempDir, "block_comments.json")
		jsonWithBlockComments := `{
			/* This is a block comment */
			"hosts": [
				{
					"name": "blocktest",
					"address": "localhost",
					"systemd_services": [],
					"docker_compose_roots": []
				}
			]
		}`
		err := os.WriteFile(blockCommentPath, []byte(jsonWithBlockComments), 0644)
		if err != nil {
			t.Fatalf("Failed to write config with block comments: %v", err)
		}

		cfg, err := Load(blockCommentPath)
		if err != nil {
			t.Fatalf("Load() should handle block comments, got error = %v", err)
		}
		if len(cfg.Hosts) != 1 || cfg.Hosts[0].Name != "blocktest" {
			t.Errorf("Config not parsed correctly with block comments")
		}
	})

	t.Run("JSON with trailing commas", func(t *testing.T) {
		trailingPath := filepath.Join(tempDir, "trailing.json")
		jsonWithTrailing := `{
			"hosts": [
				{
					"name": "trailingtest",
					"address": "localhost",
					"systemd_services": ["docker.service",],
					"docker_compose_roots": [],
				},
			],
		}`
		err := os.WriteFile(trailingPath, []byte(jsonWithTrailing), 0644)
		if err != nil {
			t.Fatalf("Failed to write config with trailing commas: %v", err)
		}

		cfg, err := Load(trailingPath)
		if err != nil {
			t.Fatalf("Load() should handle trailing commas, got error = %v", err)
		}
		if len(cfg.Hosts) != 1 || cfg.Hosts[0].Name != "trailingtest" {
			t.Errorf("Config not parsed correctly with trailing commas")
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

func TestIsPrivateIP(t *testing.T) {
	tests := []struct {
		name     string
		ip       string
		expected bool
	}{
		{"10.0.0.0/8 start", "10.0.0.1", true},
		{"10.0.0.0/8 end", "10.255.255.254", true},
		{"172.16.0.0/12 start", "172.16.0.1", true},
		{"172.16.0.0/12 end", "172.31.255.254", true},
		{"172.15.x.x not private", "172.15.0.1", false},
		{"172.32.x.x not private", "172.32.0.1", false},
		{"192.168.0.0/16 start", "192.168.0.1", true},
		{"192.168.0.0/16 end", "192.168.255.254", true},
		{"public IP", "8.8.8.8", false},
		{"localhost", "127.0.0.1", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip := net.ParseIP(tt.ip)
			got := isPrivateIP(ip)
			if got != tt.expected {
				t.Errorf("isPrivateIP(%q) = %v, want %v", tt.ip, got, tt.expected)
			}
		})
	}

	// Test nil IP
	t.Run("nil IP", func(t *testing.T) {
		if isPrivateIP(nil) {
			t.Error("isPrivateIP(nil) should return false")
		}
	})
}

func TestHostConfig_GetPrivateIP(t *testing.T) {
	tests := []struct {
		name     string
		host     HostConfig
		expected string
	}{
		{
			name:     "address is already private IP",
			host:     HostConfig{Address: "192.168.1.100"},
			expected: "192.168.1.100",
		},
		{
			name:     "address is 10.x.x.x private IP",
			host:     HostConfig{Address: "10.0.0.5"},
			expected: "10.0.0.5",
		},
		{
			name:     "address is 172.16.x.x private IP",
			host:     HostConfig{Address: "172.16.0.10"},
			expected: "172.16.0.10",
		},
		{
			name:     "address is public IP returns empty",
			host:     HostConfig{Address: "8.8.8.8"},
			expected: "",
		},
		{
			name:     "localhost without NIC returns empty",
			host:     HostConfig{Address: "localhost"},
			expected: "",
		},
		{
			name:     "localhost with nonexistent NIC returns empty",
			host:     HostConfig{Address: "localhost", NIC: []string{"nonexistent-nic-12345"}},
			expected: "",
		},
		{
			name:     "hostname without NIC returns empty",
			host:     HostConfig{Address: "server.example.com"},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.host.GetPrivateIP()
			if got != tt.expected {
				t.Errorf("GetPrivateIP() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestConfig_IsOIDCEnabled(t *testing.T) {
	tests := []struct {
		name     string
		config   Config
		expected bool
	}{
		{
			name: "OIDC fully configured",
			config: Config{
				Hosts: []HostConfig{},
				OIDC: &OIDCConfig{
					ServiceURL:   "https://dashboard.example.com",
					Callback:     "/oidc/callback",
					ConfigURL:    "https://auth.example.com/.well-known/openid-configuration",
					ClientID:     "client123",
					ClientSecret: "secret456",
				},
			},
			expected: true,
		},
		{
			name: "OIDC nil",
			config: Config{
				Hosts: []HostConfig{},
				OIDC:  nil,
			},
			expected: false,
		},
		{
			name: "OIDC empty config",
			config: Config{
				Hosts: []HostConfig{},
				OIDC:  &OIDCConfig{},
			},
			expected: false,
		},
		{
			name: "OIDC missing ConfigURL",
			config: Config{
				Hosts: []HostConfig{},
				OIDC: &OIDCConfig{
					ServiceURL:   "https://dashboard.example.com",
					Callback:     "/oidc/callback",
					ConfigURL:    "",
					ClientID:     "client123",
					ClientSecret: "secret456",
				},
			},
			expected: false,
		},
		{
			name: "OIDC missing ClientID",
			config: Config{
				Hosts: []HostConfig{},
				OIDC: &OIDCConfig{
					ServiceURL:   "https://dashboard.example.com",
					Callback:     "/oidc/callback",
					ConfigURL:    "https://auth.example.com/.well-known/openid-configuration",
					ClientID:     "",
					ClientSecret: "secret456",
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.config.IsOIDCEnabled(); got != tt.expected {
				t.Errorf("IsOIDCEnabled() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestConfig_GetPort(t *testing.T) {
	tests := []struct {
		name     string
		config   Config
		expected int
	}{
		{
			name:     "returns default port when not set",
			config:   Config{},
			expected: 9001,
		},
		{
			name:     "returns configured port",
			config:   Config{Port: 8080},
			expected: 8080,
		},
		{
			name:     "returns default when port is zero",
			config:   Config{Port: 0},
			expected: 9001,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.config.GetPort(); got != tt.expected {
				t.Errorf("GetPort() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestLoad_OIDCConfig(t *testing.T) {
	tempDir := t.TempDir()

	t.Run("config with OIDC", func(t *testing.T) {
		configPath := filepath.Join(tempDir, "oidc_config.json")
		jsonContent := `{
			"hosts": [
				{
					"name": "testhost",
					"address": "localhost",
					"systemd_services": [],
					"docker_compose_roots": []
				}
			],
			"oidc": {
				"service_url": "https://dashboard.example.com",
				"callback": "/oidc/callback",
				"config_url": "https://auth.example.com/.well-known/openid-configuration",
				"client_id": "myclient",
				"client_secret": "mysecret",
				"groups_claim": "roles",
				"admin_group": "superadmin"
			}
		}`

		err := os.WriteFile(configPath, []byte(jsonContent), 0644)
		if err != nil {
			t.Fatalf("Failed to write test config: %v", err)
		}

		cfg, err := Load(configPath)
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}

		if cfg.OIDC == nil {
			t.Fatal("Expected OIDC config to be set")
		}

		if cfg.OIDC.ServiceURL != "https://dashboard.example.com" {
			t.Errorf("OIDC.ServiceURL = %v, want https://dashboard.example.com", cfg.OIDC.ServiceURL)
		}
		if cfg.OIDC.Callback != "/oidc/callback" {
			t.Errorf("OIDC.Callback = %v, want /oidc/callback", cfg.OIDC.Callback)
		}
		if cfg.OIDC.ConfigURL != "https://auth.example.com/.well-known/openid-configuration" {
			t.Errorf("OIDC.ConfigURL = %v, want https://auth.example.com/.well-known/openid-configuration", cfg.OIDC.ConfigURL)
		}
		if cfg.OIDC.ClientID != "myclient" {
			t.Errorf("OIDC.ClientID = %v, want myclient", cfg.OIDC.ClientID)
		}
		if cfg.OIDC.ClientSecret != "mysecret" {
			t.Errorf("OIDC.ClientSecret = %v, want mysecret", cfg.OIDC.ClientSecret)
		}
		if cfg.OIDC.GroupsClaim != "roles" {
			t.Errorf("OIDC.GroupsClaim = %v, want roles", cfg.OIDC.GroupsClaim)
		}
		if cfg.OIDC.AdminGroup != "superadmin" {
			t.Errorf("OIDC.AdminGroup = %v, want superadmin", cfg.OIDC.AdminGroup)
		}

		if !cfg.IsOIDCEnabled() {
			t.Error("Expected IsOIDCEnabled() to return true")
		}
	})

	t.Run("config without OIDC", func(t *testing.T) {
		configPath := filepath.Join(tempDir, "no_oidc_config.json")
		jsonContent := `{
			"hosts": [
				{
					"name": "testhost",
					"address": "localhost",
					"systemd_services": [],
					"docker_compose_roots": []
				}
			]
		}`

		err := os.WriteFile(configPath, []byte(jsonContent), 0644)
		if err != nil {
			t.Fatalf("Failed to write test config: %v", err)
		}

		cfg, err := Load(configPath)
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}

		if cfg.OIDC != nil {
			t.Error("Expected OIDC config to be nil when not specified")
		}

		if cfg.IsOIDCEnabled() {
			t.Error("Expected IsOIDCEnabled() to return false")
		}
	})
}

func TestConfig_GetAllConfiguredServices(t *testing.T) {
	cfg := &Config{
		Hosts: []HostConfig{
			{
				Name:            "nas",
				Address:         "localhost",
				SystemdServices: []string{"docker.service", "ssh.service"},
			},
			{
				Name:            "server2",
				Address:         "192.168.1.100",
				SystemdServices: []string{"ollama.service"},
			},
		},
	}

	services := cfg.GetAllConfiguredServices()

	expected := map[string]bool{
		"nas:docker.service":    true,
		"nas:ssh.service":       true,
		"server2:ollama.service": true,
	}

	if len(services) != len(expected) {
		t.Errorf("Expected %d services, got %d", len(expected), len(services))
	}

	for key := range expected {
		if !services[key] {
			t.Errorf("Missing expected service: %s", key)
		}
	}
}

func TestIsSystemdUnit(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"service unit", "docker.service", true},
		{"timer unit", "backup.timer", true},
		{"socket unit", "docker.socket", true},
		{"mount unit", "home.mount", true},
		{"target unit", "multi-user.target", true},
		{"docker service name", "audiobookshelf", false},
		{"docker service with hyphen", "my-app", false},
		{"empty string", "", false},
		{"just suffix", ".service", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isSystemdUnit(tt.input)
			if result != tt.expected {
				t.Errorf("isSystemdUnit(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestOIDCGroupConfig_Parsing(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "group_config.json")

	jsonContent := `{
		"hosts": [
			{
				"name": "nas",
				"address": "localhost",
				"systemd_services": ["docker.service"],
				"docker_compose_roots": []
			}
		],
		"oidc": {
			"service_url": "https://dash.example.com",
			"callback": "/oidc/callback",
			"config_url": "https://auth.example.com/.well-known/openid-configuration",
			"client_id": "test-client",
			"client_secret": "test-secret",
			"groups": {
				"poweruser": {
					"services": {
						"nas": ["docker.service", "audiobookshelf"]
					}
				},
				"bookreader": {
					"services": {
						"nas": ["traefik", "audiobookshelf"]
					}
				}
			}
		}
	}`

	err := os.WriteFile(configPath, []byte(jsonContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.OIDC == nil {
		t.Fatal("Expected OIDC config to be present")
	}

	if cfg.OIDC.Groups == nil {
		t.Fatal("Expected OIDC groups to be present")
	}

	if len(cfg.OIDC.Groups) != 2 {
		t.Errorf("Expected 2 groups, got %d", len(cfg.OIDC.Groups))
	}

	poweruser := cfg.OIDC.Groups["poweruser"]
	if poweruser == nil {
		t.Fatal("Expected poweruser group to be present")
	}

	if poweruser.Services == nil {
		t.Fatal("Expected poweruser services to be present")
	}

	nasServices := poweruser.Services["nas"]
	if len(nasServices) != 2 {
		t.Errorf("Expected 2 services for nas, got %d", len(nasServices))
	}
}

func TestGotifyConfig_IsValid(t *testing.T) {
	tests := []struct {
		name     string
		config   *GotifyConfig
		expected bool
	}{
		{
			name:     "nil config",
			config:   nil,
			expected: false,
		},
		{
			name:     "disabled config",
			config:   &GotifyConfig{Enabled: false, Hostname: "https://gotify.example.com", Token: "token"},
			expected: false,
		},
		{
			name:     "missing hostname",
			config:   &GotifyConfig{Enabled: true, Token: "token"},
			expected: false,
		},
		{
			name:     "missing token",
			config:   &GotifyConfig{Enabled: true, Hostname: "https://gotify.example.com"},
			expected: false,
		},
		{
			name:     "empty config",
			config:   &GotifyConfig{Enabled: true},
			expected: false,
		},
		{
			name:     "valid config",
			config:   &GotifyConfig{Enabled: true, Hostname: "https://gotify.example.com", Token: "token"},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.config.IsValid(); got != tt.expected {
				t.Errorf("IsValid() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestLoad_GotifyConfig(t *testing.T) {
	tempDir := t.TempDir()

	t.Run("config with Gotify", func(t *testing.T) {
		configPath := filepath.Join(tempDir, "gotify_config.json")
		jsonContent := `{
			"hosts": [
				{
					"name": "testhost",
					"address": "localhost",
					"systemd_services": [],
					"docker_compose_roots": []
				}
			],
			"gotify": {
				"enabled": true,
				"hostname": "https://gotify.example.com",
				"token": "mytoken123"
			}
		}`

		err := os.WriteFile(configPath, []byte(jsonContent), 0644)
		if err != nil {
			t.Fatalf("Failed to write test config: %v", err)
		}

		cfg, err := Load(configPath)
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}

		if cfg.Gotify == nil {
			t.Fatal("Expected Gotify config to be set")
		}

		if !cfg.Gotify.Enabled {
			t.Error("Expected Gotify to be enabled")
		}
		if cfg.Gotify.Hostname != "https://gotify.example.com" {
			t.Errorf("Gotify hostname = %v, want https://gotify.example.com", cfg.Gotify.Hostname)
		}
		if cfg.Gotify.Token != "mytoken123" {
			t.Errorf("Gotify token = %v, want mytoken123", cfg.Gotify.Token)
		}
		if !cfg.Gotify.IsValid() {
			t.Error("Expected Gotify config to be valid")
		}
	})

	t.Run("config without Gotify", func(t *testing.T) {
		configPath := filepath.Join(tempDir, "no_gotify_config.json")
		jsonContent := `{
			"hosts": [
				{
					"name": "testhost",
					"address": "localhost",
					"systemd_services": [],
					"docker_compose_roots": []
				}
			]
		}`

		err := os.WriteFile(configPath, []byte(jsonContent), 0644)
		if err != nil {
			t.Fatalf("Failed to write test config: %v", err)
		}

		cfg, err := Load(configPath)
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}

		if cfg.Gotify != nil {
			t.Error("Expected Gotify config to be nil when not specified")
		}
	})
}

func TestHomeAssistantConfig_HasHomeAssistant(t *testing.T) {
	tests := []struct {
		name     string
		host     HostConfig
		expected bool
	}{
		{
			name: "has valid config with token",
			host: HostConfig{
				HomeAssistant: &HomeAssistantConfig{
					Port:           8123,
					UseHTTPS:       true,
					LongLivedToken: "valid-token",
				},
			},
			expected: true,
		},
		{
			name: "nil homeassistant config",
			host: HostConfig{
				HomeAssistant: nil,
			},
			expected: false,
		},
		{
			name: "empty token",
			host: HostConfig{
				HomeAssistant: &HomeAssistantConfig{
					Port:           8123,
					LongLivedToken: "",
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.host.HasHomeAssistant()
			if result != tt.expected {
				t.Errorf("HasHomeAssistant() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestHomeAssistantConfig_GetHomeAssistantEndpoint(t *testing.T) {
	tests := []struct {
		name     string
		host     HostConfig
		expected string
	}{
		{
			name: "http with default port",
			host: HostConfig{
				Address: "192.168.1.100",
				HomeAssistant: &HomeAssistantConfig{
					Port:           0, // should default to 8123
					UseHTTPS:       false,
					LongLivedToken: "token",
				},
			},
			expected: "http://192.168.1.100:8123/api/",
		},
		{
			name: "https with custom port",
			host: HostConfig{
				Address: "homeassistant.local",
				HomeAssistant: &HomeAssistantConfig{
					Port:           443,
					UseHTTPS:       true,
					LongLivedToken: "token",
				},
			},
			expected: "https://homeassistant.local:443/api/",
		},
		{
			name: "https with standard port",
			host: HostConfig{
				Address: "192.168.1.50",
				HomeAssistant: &HomeAssistantConfig{
					Port:              8123,
					UseHTTPS:          true,
					IgnoreHTTPSErrors: true,
					LongLivedToken:    "my-token",
				},
			},
			expected: "https://192.168.1.50:8123/api/",
		},
		{
			name: "nil config returns empty",
			host: HostConfig{
				Address:       "192.168.1.100",
				HomeAssistant: nil,
			},
			expected: "",
		},
		{
			name: "empty token returns empty",
			host: HostConfig{
				Address: "192.168.1.100",
				HomeAssistant: &HomeAssistantConfig{
					Port:           8123,
					LongLivedToken: "",
				},
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.host.GetHomeAssistantEndpoint()
			if result != tt.expected {
				t.Errorf("GetHomeAssistantEndpoint() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestLoad_HomeAssistantConfig(t *testing.T) {
	// Create a temporary directory for test config files
	tempDir := t.TempDir()

	t.Run("parses homeassistant config", func(t *testing.T) {
		configPath := filepath.Join(tempDir, "ha_config.json")
		jsonContent := `{
			"hosts": [
				{
					"name": "hahost",
					"address": "192.168.1.50",
					"homeassistant": {
						"port": 8123,
						"use_https": true,
						"ignore_https_errors": true,
						"longlivedtoken": "my-long-lived-token"
					},
					"systemd_services": [],
					"docker_compose_roots": []
				}
			]
		}`

		err := os.WriteFile(configPath, []byte(jsonContent), 0644)
		if err != nil {
			t.Fatalf("Failed to write test config: %v", err)
		}

		cfg, err := Load(configPath)
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}

		if len(cfg.Hosts) != 1 {
			t.Fatalf("Expected 1 host, got %d", len(cfg.Hosts))
		}

		host := cfg.Hosts[0]
		if host.HomeAssistant == nil {
			t.Fatal("HomeAssistant config is nil")
		}

		if host.HomeAssistant.Port != 8123 {
			t.Errorf("Port = %d, want 8123", host.HomeAssistant.Port)
		}
		if !host.HomeAssistant.UseHTTPS {
			t.Error("UseHTTPS = false, want true")
		}
		if !host.HomeAssistant.IgnoreHTTPSErrors {
			t.Error("IgnoreHTTPSErrors = false, want true")
		}
		if host.HomeAssistant.LongLivedToken != "my-long-lived-token" {
			t.Errorf("LongLivedToken = %q, want %q", host.HomeAssistant.LongLivedToken, "my-long-lived-token")
		}

		if !host.HasHomeAssistant() {
			t.Error("HasHomeAssistant() = false, want true")
		}

		expectedEndpoint := "https://192.168.1.50:8123/api/"
		if host.GetHomeAssistantEndpoint() != expectedEndpoint {
			t.Errorf("GetHomeAssistantEndpoint() = %q, want %q", host.GetHomeAssistantEndpoint(), expectedEndpoint)
		}
	})

	t.Run("host without homeassistant config", func(t *testing.T) {
		configPath := filepath.Join(tempDir, "no_ha_config.json")
		jsonContent := `{
			"hosts": [
				{
					"name": "normalhost",
					"address": "localhost",
					"systemd_services": ["docker.service"],
					"docker_compose_roots": []
				}
			]
		}`

		err := os.WriteFile(configPath, []byte(jsonContent), 0644)
		if err != nil {
			t.Fatalf("Failed to write test config: %v", err)
		}

		cfg, err := Load(configPath)
		if err != nil {
			t.Fatalf("Load() error = %v", err)
		}

		if len(cfg.Hosts) != 1 {
			t.Fatalf("Expected 1 host, got %d", len(cfg.Hosts))
		}

		host := cfg.Hosts[0]
		if host.HomeAssistant != nil {
			t.Error("HomeAssistant config should be nil when not specified")
		}

		if host.HasHomeAssistant() {
			t.Error("HasHomeAssistant() = true, want false")
		}

		if host.GetHomeAssistantEndpoint() != "" {
			t.Errorf("GetHomeAssistantEndpoint() = %q, want empty", host.GetHomeAssistantEndpoint())
		}
	})
}

func TestParseSystemdServiceEntry(t *testing.T) {
	tests := []struct {
		name         string
		entry        string
		wantName     string
		wantUser     string
		wantReadOnly bool
		wantPorts    []uint16
	}{
		{"plain service name", "docker.service", "docker.service", "", false, nil},
		{"readonly service", "nas-dashboard.service:ro", "nas-dashboard.service", "", true, nil},
		{"uppercase RO ignored", "test.service:RO", "test.service:RO", "", false, nil},
		{"empty string", "", "", "", false, nil},
		{"only :ro suffix", ":ro", "", "", true, nil},
		// User service tests
		{"user service", "xero:zunesync.service", "zunesync.service", "xero", false, nil},
		{"user service readonly", "xero:zunesync.service:ro", "zunesync.service", "xero", true, nil},
		{"user service with timer", "alice:backup.timer", "backup.timer", "alice", false, nil},
		{"user service with timer readonly", "bob:cleanup.timer:ro", "cleanup.timer", "bob", true, nil},
		{"user service with socket", "user:myapp.socket", "myapp.socket", "user", false, nil},
		// Edge cases: no dot means no user prefix detection (backwards compat for weird names)
		{"no dot in name", "nodot", "nodot", "", false, nil},
		{"colon but no dot", "some:thing", "some:thing", "", false, nil},
		{"colon after dot", "my.weird:name.service", "my.weird:name.service", "", false, nil},
		// Port tests
		{"service with single port", "myapp.service#8080", "myapp.service", "", false, []uint16{8080}},
		{"service with multiple ports", "myapp.service#8080,8443", "myapp.service", "", false, []uint16{8080, 8443}},
		{"service with ports and readonly", "myapp.service#8080:ro", "myapp.service", "", true, []uint16{8080}},
		{"service with multiple ports and readonly", "myapp.service#8080,9000:ro", "myapp.service", "", true, []uint16{8080, 9000}},
		{"user service with port", "xero:myapp.service#3000", "myapp.service", "xero", false, []uint16{3000}},
		{"user service with ports and readonly", "xero:myapp.service#3000,4000:ro", "myapp.service", "xero", true, []uint16{3000, 4000}},
		{"port with spaces", "myapp.service#8080, 8443", "myapp.service", "", false, []uint16{8080, 8443}},
		{"empty port ignored", "myapp.service#8080,,9000", "myapp.service", "", false, []uint16{8080, 9000}},
		{"invalid port ignored", "myapp.service#8080,invalid,9000", "myapp.service", "", false, []uint16{8080, 9000}},
		{"port 0 ignored", "myapp.service#0,8080", "myapp.service", "", false, []uint16{8080}},
		{"port over 65535 ignored", "myapp.service#8080,99999", "myapp.service", "", false, []uint16{8080}},
		{"just hash no ports", "myapp.service#", "myapp.service", "", false, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseSystemdServiceEntry(tt.entry)
			if result.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", result.Name, tt.wantName)
			}
			if result.User != tt.wantUser {
				t.Errorf("User = %q, want %q", result.User, tt.wantUser)
			}
			if result.ReadOnly != tt.wantReadOnly {
				t.Errorf("ReadOnly = %v, want %v", result.ReadOnly, tt.wantReadOnly)
			}
			if len(result.Ports) != len(tt.wantPorts) {
				t.Errorf("Ports = %v, want %v", result.Ports, tt.wantPorts)
			} else {
				for i, port := range result.Ports {
					if port != tt.wantPorts[i] {
						t.Errorf("Ports[%d] = %d, want %d", i, port, tt.wantPorts[i])
					}
				}
			}
		})
	}
}

func TestHostConfig_GetSystemdServiceEntries(t *testing.T) {
	host := HostConfig{
		Name:    "testhost",
		Address: "localhost",
		SystemdServices: []string{
			"docker.service",
			"nas-dashboard.service:ro",
			"nginx.service",
			"webapp.service#8080,8443",
		},
	}

	entries := host.GetSystemdServiceEntries()

	if len(entries) != 4 {
		t.Fatalf("Expected 4 entries, got %d", len(entries))
	}

	// Check first entry (not readonly)
	if entries[0].Name != "docker.service" {
		t.Errorf("entries[0].Name = %q, want %q", entries[0].Name, "docker.service")
	}
	if entries[0].ReadOnly {
		t.Error("entries[0].ReadOnly = true, want false")
	}
	if len(entries[0].Ports) != 0 {
		t.Errorf("entries[0].Ports = %v, want nil", entries[0].Ports)
	}

	// Check second entry (readonly)
	if entries[1].Name != "nas-dashboard.service" {
		t.Errorf("entries[1].Name = %q, want %q", entries[1].Name, "nas-dashboard.service")
	}
	if !entries[1].ReadOnly {
		t.Error("entries[1].ReadOnly = false, want true")
	}

	// Check third entry (not readonly)
	if entries[2].Name != "nginx.service" {
		t.Errorf("entries[2].Name = %q, want %q", entries[2].Name, "nginx.service")
	}
	if entries[2].ReadOnly {
		t.Error("entries[2].ReadOnly = true, want false")
	}

	// Check fourth entry (with ports)
	if entries[3].Name != "webapp.service" {
		t.Errorf("entries[3].Name = %q, want %q", entries[3].Name, "webapp.service")
	}
	if len(entries[3].Ports) != 2 {
		t.Errorf("entries[3].Ports length = %d, want 2", len(entries[3].Ports))
	} else {
		if entries[3].Ports[0] != 8080 {
			t.Errorf("entries[3].Ports[0] = %d, want 8080", entries[3].Ports[0])
		}
		if entries[3].Ports[1] != 8443 {
			t.Errorf("entries[3].Ports[1] = %d, want 8443", entries[3].Ports[1])
		}
	}
}

func TestHostConfig_GetSystemdServiceNames(t *testing.T) {
	host := HostConfig{
		Name:    "testhost",
		Address: "localhost",
		SystemdServices: []string{
			"docker.service",
			"nas-dashboard.service:ro",
			"nginx.service:ro",
		},
	}

	names := host.GetSystemdServiceNames()

	if len(names) != 3 {
		t.Fatalf("Expected 3 names, got %d", len(names))
	}

	expected := []string{"docker.service", "nas-dashboard.service", "nginx.service"}
	for i, name := range names {
		if name != expected[i] {
			t.Errorf("names[%d] = %q, want %q", i, name, expected[i])
		}
	}
}

func TestHostConfig_GetSystemdServiceEntriesWithUserServices(t *testing.T) {
	host := HostConfig{
		Name:    "testhost",
		Address: "localhost",
		SystemdServices: []string{
			"docker.service",
			"nas-dashboard.service:ro",
			"xero:zunesync.service",
			"alice:backup.timer:ro",
		},
	}

	entries := host.GetSystemdServiceEntries()

	if len(entries) != 4 {
		t.Fatalf("Expected 4 entries, got %d", len(entries))
	}

	// Check first entry (system service, not readonly)
	if entries[0].Name != "docker.service" || entries[0].User != "" || entries[0].ReadOnly {
		t.Errorf("entries[0] = {Name: %q, User: %q, ReadOnly: %v}, want {Name: \"docker.service\", User: \"\", ReadOnly: false}",
			entries[0].Name, entries[0].User, entries[0].ReadOnly)
	}

	// Check second entry (system service, readonly)
	if entries[1].Name != "nas-dashboard.service" || entries[1].User != "" || !entries[1].ReadOnly {
		t.Errorf("entries[1] = {Name: %q, User: %q, ReadOnly: %v}, want {Name: \"nas-dashboard.service\", User: \"\", ReadOnly: true}",
			entries[1].Name, entries[1].User, entries[1].ReadOnly)
	}

	// Check third entry (user service, not readonly)
	if entries[2].Name != "zunesync.service" || entries[2].User != "xero" || entries[2].ReadOnly {
		t.Errorf("entries[2] = {Name: %q, User: %q, ReadOnly: %v}, want {Name: \"zunesync.service\", User: \"xero\", ReadOnly: false}",
			entries[2].Name, entries[2].User, entries[2].ReadOnly)
	}

	// Check fourth entry (user service, readonly)
	if entries[3].Name != "backup.timer" || entries[3].User != "alice" || !entries[3].ReadOnly {
		t.Errorf("entries[3] = {Name: %q, User: %q, ReadOnly: %v}, want {Name: \"backup.timer\", User: \"alice\", ReadOnly: true}",
			entries[3].Name, entries[3].User, entries[3].ReadOnly)
	}
}

func TestWatchtowerConfig_HasWatchtower(t *testing.T) {
	tests := []struct {
		name     string
		host     HostConfig
		expected bool
	}{
		{
			name:     "nil watchtower config",
			host:     HostConfig{Name: "test"},
			expected: false,
		},
		{
			name: "zero port",
			host: HostConfig{
				Name:       "test",
				Watchtower: &WatchtowerConfig{Port: 0},
			},
			expected: false,
		},
		{
			name: "valid config",
			host: HostConfig{
				Name: "test",
				Watchtower: &WatchtowerConfig{
					Port:  8080,
					Token: "test-token",
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.host.HasWatchtower()
			if result != tt.expected {
				t.Errorf("HasWatchtower() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestWatchtowerConfig_GetWatchtowerPort(t *testing.T) {
	tests := []struct {
		name     string
		host     HostConfig
		expected int
	}{
		{
			name:     "nil config returns default",
			host:     HostConfig{Name: "test"},
			expected: 8080,
		},
		{
			name: "zero port returns default",
			host: HostConfig{
				Name:       "test",
				Watchtower: &WatchtowerConfig{Port: 0},
			},
			expected: 8080,
		},
		{
			name: "custom port",
			host: HostConfig{
				Name:       "test",
				Watchtower: &WatchtowerConfig{Port: 8023},
			},
			expected: 8023,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.host.GetWatchtowerPort()
			if result != tt.expected {
				t.Errorf("GetWatchtowerPort() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestWatchtowerConfig_GetWatchtowerUpdateTimeout(t *testing.T) {
	tests := []struct {
		name     string
		host     HostConfig
		expected int
	}{
		{
			name:     "nil config returns default",
			host:     HostConfig{Name: "test"},
			expected: 120,
		},
		{
			name: "zero timeout returns default",
			host: HostConfig{
				Name:       "test",
				Watchtower: &WatchtowerConfig{UpdateTimeout: 0},
			},
			expected: 120,
		},
		{
			name: "custom timeout",
			host: HostConfig{
				Name:       "test",
				Watchtower: &WatchtowerConfig{UpdateTimeout: 300},
			},
			expected: 300,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.host.GetWatchtowerUpdateTimeout()
			if result != tt.expected {
				t.Errorf("GetWatchtowerUpdateTimeout() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestWatchtowerConfig_GetWatchtowerToken(t *testing.T) {
	// Test with env var
	t.Run("env var takes precedence", func(t *testing.T) {
		t.Setenv("WATCHTOWER_TOKEN", "env-token")
		
		host := HostConfig{
			Name: "test",
			Watchtower: &WatchtowerConfig{
				Port:  8080,
				Token: "config-token",
			},
		}
		
		result := host.GetWatchtowerToken()
		if result != "env-token" {
			t.Errorf("GetWatchtowerToken() = %v, want env-token", result)
		}
	})

	// Test without env var (need separate test because t.Setenv persists)
	t.Run("falls back to config token", func(t *testing.T) {
		// Clear env var
		os.Unsetenv("WATCHTOWER_TOKEN")
		
		host := HostConfig{
			Name: "test",
			Watchtower: &WatchtowerConfig{
				Port:  8080,
				Token: "config-token",
			},
		}
		
		result := host.GetWatchtowerToken()
		if result != "config-token" {
			t.Errorf("GetWatchtowerToken() = %v, want config-token", result)
		}
	})

	t.Run("nil config returns empty", func(t *testing.T) {
		os.Unsetenv("WATCHTOWER_TOKEN")
		
		host := HostConfig{Name: "test"}
		result := host.GetWatchtowerToken()
		if result != "" {
			t.Errorf("GetWatchtowerToken() = %v, want empty string", result)
		}
	})
}

// TestHostConfig_GetSSHUser tests the SSH username helper method.
func TestHostConfig_GetSSHUser(t *testing.T) {
	tests := []struct {
		name     string
		host     HostConfig
		expected string
	}{
		{
			name:     "nil SSHConfig returns empty",
			host:     HostConfig{Name: "test"},
			expected: "",
		},
		{
			name: "empty username returns empty",
			host: HostConfig{
				Name:      "test",
				SSHConfig: &SSHConfig{Username: "", Port: 22},
			},
			expected: "",
		},
		{
			name: "returns configured username",
			host: HostConfig{
				Name:      "test",
				SSHConfig: &SSHConfig{Username: "root", Port: 22},
			},
			expected: "root",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.host.GetSSHUser(); got != tt.expected {
				t.Errorf("GetSSHUser() = %v, want %v", got, tt.expected)
			}
		})
	}
}

// TestHostConfig_GetSSHPort tests the SSH port helper method.
func TestHostConfig_GetSSHPort(t *testing.T) {
	tests := []struct {
		name     string
		host     HostConfig
		expected int
	}{
		{
			name:     "nil SSHConfig returns 0",
			host:     HostConfig{Name: "test"},
			expected: 0,
		},
		{
			name: "zero port returns 0",
			host: HostConfig{
				Name:      "test",
				SSHConfig: &SSHConfig{Username: "root", Port: 0},
			},
			expected: 0,
		},
		{
			name: "returns configured port",
			host: HostConfig{
				Name:      "test",
				SSHConfig: &SSHConfig{Username: "root", Port: 2222},
			},
			expected: 2222,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.host.GetSSHPort(); got != tt.expected {
				t.Errorf("GetSSHPort() = %v, want %v", got, tt.expected)
			}
		})
	}
}

// TestHostConfig_GetSSHTarget tests the SSH target string generation.
func TestHostConfig_GetSSHTarget(t *testing.T) {
	tests := []struct {
		name     string
		host     HostConfig
		expected string
	}{
		{
			name:     "no SSHConfig returns just address",
			host:     HostConfig{Name: "test", Address: "192.168.1.100"},
			expected: "192.168.1.100",
		},
		{
			name: "empty username returns just address",
			host: HostConfig{
				Name:      "test",
				Address:   "192.168.1.100",
				SSHConfig: &SSHConfig{Username: "", Port: 22},
			},
			expected: "192.168.1.100",
		},
		{
			name: "with username returns user@address",
			host: HostConfig{
				Name:      "test",
				Address:   "192.168.1.100",
				SSHConfig: &SSHConfig{Username: "root", Port: 22},
			},
			expected: "root@192.168.1.100",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.host.GetSSHTarget(); got != tt.expected {
				t.Errorf("GetSSHTarget() = %v, want %v", got, tt.expected)
			}
		})
	}
}

// TestHostConfig_GetSSHArgs tests the SSH arguments generation.
func TestHostConfig_GetSSHArgs(t *testing.T) {
	t.Run("no SSHConfig returns base args only", func(t *testing.T) {
		host := HostConfig{Name: "test", Address: "192.168.1.100"}
		args := host.GetSSHArgs()

		// Should have ConnectTimeout and StrictHostKeyChecking
		if len(args) != 4 {
			t.Errorf("GetSSHArgs() returned %d args, want 4", len(args))
		}
		// Should not have -p flag
		for _, arg := range args {
			if arg == "-p" {
				t.Error("GetSSHArgs() should not include -p when port is 0")
			}
		}
	})

	t.Run("with custom port includes -p flag", func(t *testing.T) {
		host := HostConfig{
			Name:      "test",
			Address:   "192.168.1.100",
			SSHConfig: &SSHConfig{Username: "root", Port: 2222},
		}
		args := host.GetSSHArgs()

		// Should have base args + -p + port number
		if len(args) != 6 {
			t.Errorf("GetSSHArgs() returned %d args, want 6", len(args))
		}

		// Find -p flag and check its value
		foundPort := false
		for i, arg := range args {
			if arg == "-p" && i+1 < len(args) && args[i+1] == "2222" {
				foundPort = true
				break
			}
		}
		if !foundPort {
			t.Error("GetSSHArgs() should include -p 2222")
		}
	})
}

// TestLoad_SSHConfig tests that SSH config is loaded from JSON.
func TestLoad_SSHConfig(t *testing.T) {
	configJSON := `{
		"hosts": [
			{
				"name": "remotehost",
				"address": "192.168.1.100",
				"ssh_config": {
					"username": "admin",
					"port": 2222
				},
				"systemd_services": ["docker.service"],
				"docker_compose_roots": []
			}
		]
	}`

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "services.json")
	if err := os.WriteFile(configPath, []byte(configJSON), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if len(cfg.Hosts) != 1 {
		t.Fatalf("expected 1 host, got %d", len(cfg.Hosts))
	}

	host := cfg.Hosts[0]
	if host.SSHConfig == nil {
		t.Fatal("SSHConfig is nil")
	}
	if host.SSHConfig.Username != "admin" {
		t.Errorf("SSHConfig.Username = %v, want admin", host.SSHConfig.Username)
	}
	if host.SSHConfig.Port != 2222 {
		t.Errorf("SSHConfig.Port = %v, want 2222", host.SSHConfig.Port)
	}
}
