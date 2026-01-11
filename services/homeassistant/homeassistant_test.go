package homeassistant

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"home_server_dashboard/config"
)

// TestNewProvider tests the NewProvider function with various configurations.
func TestNewProvider(t *testing.T) {
	tests := []struct {
		name       string
		hostConfig *config.HostConfig
		wantNil    bool
		wantErr    bool
	}{
		{
			name: "valid config",
			hostConfig: &config.HostConfig{
				Name:    "testhost",
				Address: "192.168.1.100",
				HomeAssistant: &config.HomeAssistantConfig{
					Port:           8123,
					UseHTTPS:       false,
					LongLivedToken: "test-token",
				},
			},
			wantNil: false,
			wantErr: false,
		},
		{
			name: "no homeassistant config",
			hostConfig: &config.HostConfig{
				Name:          "testhost",
				Address:       "192.168.1.100",
				HomeAssistant: nil,
			},
			wantNil: true,
			wantErr: false,
		},
		{
			name: "empty token",
			hostConfig: &config.HostConfig{
				Name:    "testhost",
				Address: "192.168.1.100",
				HomeAssistant: &config.HomeAssistantConfig{
					Port:           8123,
					LongLivedToken: "",
				},
			},
			wantNil: true,
			wantErr: false,
		},
		{
			name: "https with ignore errors",
			hostConfig: &config.HostConfig{
				Name:    "testhost",
				Address: "192.168.1.100",
				HomeAssistant: &config.HomeAssistantConfig{
					Port:              8123,
					UseHTTPS:          true,
					IgnoreHTTPSErrors: true,
					LongLivedToken:    "test-token",
				},
			},
			wantNil: false,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, err := NewProvider(tt.hostConfig)

			if tt.wantErr && err == nil {
				t.Errorf("NewProvider() expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("NewProvider() unexpected error: %v", err)
			}
			if tt.wantNil && provider != nil {
				t.Errorf("NewProvider() expected nil, got provider")
			}
			if !tt.wantNil && !tt.wantErr && provider == nil {
				t.Errorf("NewProvider() expected provider, got nil")
			}
		})
	}
}

// TestProviderName tests the Name method.
func TestProviderName(t *testing.T) {
	hostConfig := &config.HostConfig{
		Name:    "testhost",
		Address: "192.168.1.100",
		HomeAssistant: &config.HomeAssistantConfig{
			Port:           8123,
			LongLivedToken: "test-token",
		},
	}

	provider, err := NewProvider(hostConfig)
	if err != nil {
		t.Fatalf("NewProvider() error: %v", err)
	}

	if provider.Name() != "homeassistant" {
		t.Errorf("Name() = %q, want %q", provider.Name(), "homeassistant")
	}
}

// TestServiceMethods tests the Service interface implementation.
func TestServiceMethods(t *testing.T) {
	hostConfig := &config.HostConfig{
		Name:    "testhost",
		Address: "192.168.1.100",
		HomeAssistant: &config.HomeAssistantConfig{
			Port:           8123,
			LongLivedToken: "test-token",
		},
	}

	provider, err := NewProvider(hostConfig)
	if err != nil {
		t.Fatalf("NewProvider() error: %v", err)
	}

	svc, err := provider.GetService("homeassistant")
	if err != nil {
		t.Fatalf("GetService() error: %v", err)
	}

	// Test GetName
	if svc.GetName() != "homeassistant" {
		t.Errorf("GetName() = %q, want %q", svc.GetName(), "homeassistant")
	}

	// Test GetHost
	if svc.GetHost() != "testhost" {
		t.Errorf("GetHost() = %q, want %q", svc.GetHost(), "testhost")
	}

	// Test GetSource
	if svc.GetSource() != "homeassistant" {
		t.Errorf("GetSource() = %q, want %q", svc.GetSource(), "homeassistant")
	}

	// Test Start returns error
	ctx := context.Background()
	if err := svc.Start(ctx); err == nil {
		t.Error("Start() expected error, got nil")
	}

	// Test Stop returns error
	if err := svc.Stop(ctx); err == nil {
		t.Error("Stop() expected error, got nil")
	}
}

// TestGetServiceNotFound tests GetService with invalid name.
func TestGetServiceNotFound(t *testing.T) {
	hostConfig := &config.HostConfig{
		Name:    "testhost",
		Address: "192.168.1.100",
		HomeAssistant: &config.HomeAssistantConfig{
			Port:           8123,
			LongLivedToken: "test-token",
		},
	}

	provider, err := NewProvider(hostConfig)
	if err != nil {
		t.Fatalf("NewProvider() error: %v", err)
	}

	_, err = provider.GetService("invalid-service")
	if err == nil {
		t.Error("GetService() expected error for invalid service name, got nil")
	}
}

// TestGetLogs tests that GetLogs returns a stub message.
func TestGetLogs(t *testing.T) {
	hostConfig := &config.HostConfig{
		Name:    "testhost",
		Address: "192.168.1.100",
		HomeAssistant: &config.HomeAssistantConfig{
			Port:           8123,
			LongLivedToken: "test-token",
		},
	}

	provider, err := NewProvider(hostConfig)
	if err != nil {
		t.Fatalf("NewProvider() error: %v", err)
	}

	ctx := context.Background()
	reader, err := provider.GetLogs(ctx, "homeassistant", 100, false)
	if err != nil {
		t.Fatalf("GetLogs() error: %v", err)
	}
	defer reader.Close()

	buf := make([]byte, 1024)
	n, err := reader.Read(buf)
	if err != nil {
		t.Fatalf("Read() error: %v", err)
	}

	content := string(buf[:n])
	if !strings.Contains(content, "Logs are not available") {
		t.Errorf("GetLogs() expected stub message, got: %s", content)
	}
}

// TestConfigEndpoint tests the GetHomeAssistantEndpoint helper.
func TestConfigEndpoint(t *testing.T) {
	tests := []struct {
		name     string
		config   *config.HostConfig
		expected string
	}{
		{
			name: "http default port",
			config: &config.HostConfig{
				Address: "192.168.1.100",
				HomeAssistant: &config.HomeAssistantConfig{
					Port:           0, // should default to 8123
					UseHTTPS:       false,
					LongLivedToken: "token",
				},
			},
			expected: "http://192.168.1.100:8123/api/",
		},
		{
			name: "https custom port",
			config: &config.HostConfig{
				Address: "ha.example.com",
				HomeAssistant: &config.HomeAssistantConfig{
					Port:           443,
					UseHTTPS:       true,
					LongLivedToken: "token",
				},
			},
			expected: "https://ha.example.com:443/api/",
		},
		{
			name: "no config",
			config: &config.HostConfig{
				Address:       "192.168.1.100",
				HomeAssistant: nil,
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.config.GetHomeAssistantEndpoint()
			if result != tt.expected {
				t.Errorf("GetHomeAssistantEndpoint() = %q, want %q", result, tt.expected)
			}
		})
	}
}

// TestHasHomeAssistant tests the HasHomeAssistant helper.
func TestHasHomeAssistant(t *testing.T) {
	tests := []struct {
		name     string
		config   *config.HostConfig
		expected bool
	}{
		{
			name: "has config with token",
			config: &config.HostConfig{
				HomeAssistant: &config.HomeAssistantConfig{
					LongLivedToken: "token",
				},
			},
			expected: true,
		},
		{
			name: "has config without token",
			config: &config.HostConfig{
				HomeAssistant: &config.HomeAssistantConfig{
					LongLivedToken: "",
				},
			},
			expected: false,
		},
		{
			name: "no config",
			config: &config.HostConfig{
				HomeAssistant: nil,
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.config.HasHomeAssistant()
			if result != tt.expected {
				t.Errorf("HasHomeAssistant() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// mockHAServer creates a mock Home Assistant server for testing.
func mockHAServer(t *testing.T, healthResponse string, statusCode int) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check authorization header
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		switch r.URL.Path {
		case "/api/":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(statusCode)
			json.NewEncoder(w).Encode(map[string]string{"message": healthResponse})
		case "/api/services/homeassistant/restart":
			if r.Method == "POST" {
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode([]map[string]interface{}{})
			} else {
				w.WriteHeader(http.StatusMethodNotAllowed)
			}
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

// mockSupervisorServer creates a mock Supervisor API server for testing.
func mockSupervisorServer(t *testing.T) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check authorization header
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		switch r.URL.Path {
		case "/addons":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"result": "ok",
				"data": map[string]interface{}{
					"addons": []map[string]interface{}{
						{
							"slug":        "esphome",
							"name":        "ESPHome",
							"description": "ESPHome addon for Home Assistant",
							"state":       "started",
							"version":     "2024.1.0",
							"installed":   true,
							"available":   true,
						},
						{
							"slug":        "ssh",
							"name":        "SSH & Web Terminal",
							"description": "SSH server addon",
							"state":       "stopped",
							"version":     "9.9.0",
							"installed":   true,
							"available":   true,
						},
						{
							"slug":        "notinstalled",
							"name":        "Not Installed",
							"description": "Not installed addon",
							"state":       "unknown",
							"version":     "1.0.0",
							"installed":   false,
							"available":   true,
						},
					},
				},
			})
		case "/addons/esphome/logs":
			w.Write([]byte("ESPHome log line 1\nESPHome log line 2\n"))
		case "/addons/esphome/logs/follow":
			w.Write([]byte("ESPHome streaming log\n"))
		case "/addons/esphome/restart", "/addons/esphome/start", "/addons/esphome/stop":
			if r.Method == "POST" {
				json.NewEncoder(w).Encode(map[string]string{"result": "ok"})
			} else {
				w.WriteHeader(http.StatusMethodNotAllowed)
			}
		case "/core/start", "/core/stop", "/core/restart":
			if r.Method == "POST" {
				json.NewEncoder(w).Encode(map[string]string{"result": "ok"})
			} else {
				w.WriteHeader(http.StatusMethodNotAllowed)
			}
		case "/core/logs":
			w.Write([]byte("Core log line 1\nCore log line 2\n"))
		case "/supervisor/logs":
			w.Write([]byte("Supervisor log line 1\n"))
		case "/host/logs":
			w.Write([]byte("Host log line 1\n"))
		case "/supervisor/info":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"result": "ok",
				"data": map[string]interface{}{
					"version": "2024.01.0",
					"healthy": true,
					"channel": "stable",
				},
			})
		case "/core/info":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"result": "ok",
				"data": map[string]interface{}{
					"version": "2024.1.0",
					"state":   "running",
				},
			})
		case "/host/info":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"result": "ok",
				"data": map[string]interface{}{
					"hostname":         "homeassistant",
					"operating_system": "Home Assistant OS 11.0",
					"kernel":           "6.1.0",
				},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

// TestHasSupervisorAPI tests the HasSupervisorAPI helper.
func TestHasSupervisorAPI(t *testing.T) {
	tests := []struct {
		name     string
		config   *config.HostConfig
		expected bool
	}{
		{
			name: "has HAOS config with SSH addon port",
			config: &config.HostConfig{
				HomeAssistant: &config.HomeAssistantConfig{
					LongLivedToken:    "token",
					IsHomeAssistantOS: true,
					SSHAddonPort:      22,
				},
			},
			expected: true,
		},
		{
			name: "has HAOS config without SSH addon port",
			config: &config.HostConfig{
				HomeAssistant: &config.HomeAssistantConfig{
					LongLivedToken:    "token",
					IsHomeAssistantOS: true,
					SSHAddonPort:      0,
				},
			},
			expected: false,
		},
		{
			name: "not HAOS",
			config: &config.HostConfig{
				HomeAssistant: &config.HomeAssistantConfig{
					LongLivedToken:    "token",
					IsHomeAssistantOS: false,
				},
			},
			expected: false,
		},
		{
			name: "no HA config",
			config: &config.HostConfig{
				HomeAssistant: nil,
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.config.HasSupervisorAPI()
			if result != tt.expected {
				t.Errorf("HasSupervisorAPI() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// TestGetSSHAddonPort tests the GetSSHAddonPort helper.
func TestGetSSHAddonPort(t *testing.T) {
	tests := []struct {
		name     string
		config   *config.HostConfig
		expected int
	}{
		{
			name: "has HAOS config with default port",
			config: &config.HostConfig{
				Address: "192.168.1.100",
				HomeAssistant: &config.HomeAssistantConfig{
					LongLivedToken:    "token",
					IsHomeAssistantOS: true,
					SSHAddonPort:      0, // should default to 22
				},
			},
			expected: 22,
		},
		{
			name: "has HAOS config with custom port",
			config: &config.HostConfig{
				Address: "192.168.1.100",
				HomeAssistant: &config.HomeAssistantConfig{
					LongLivedToken:    "token",
					IsHomeAssistantOS: true,
					SSHAddonPort:      2222,
				},
			},
			expected: 2222,
		},
		{
			name: "no HA config",
			config: &config.HostConfig{
				Address:       "192.168.1.100",
				HomeAssistant: nil,
			},
			expected: 22,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.config.GetSSHAddonPort()
			if result != tt.expected {
				t.Errorf("GetSSHAddonPort() = %d, want %d", result, tt.expected)
			}
		})
	}
}

// TestAddonStateToServiceState tests the addon state conversion.
func TestAddonStateToServiceState(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"started", "running"},
		{"STARTED", "running"},
		{"Started", "running"},
		{"stopped", "stopped"},
		{"error", "stopped"},
		{"unknown", "stopped"},
		{"invalid", "stopped"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := addonStateToServiceState(tt.input)
			if result != tt.expected {
				t.Errorf("addonStateToServiceState(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

// TestGetServiceHAOS tests GetService with HAOS service types.
// Note: Without SUPERVISOR_TOKEN env var and SSH connection, supervisor/host/addon
// services won't be available. This test verifies the behavior in both scenarios.
func TestGetServiceHAOS(t *testing.T) {
	hostConfig := &config.HostConfig{
		Name:    "testhost",
		Address: "192.168.1.100",
		HomeAssistant: &config.HomeAssistantConfig{
			Port:              8123,
			LongLivedToken:    "test-token",
			IsHomeAssistantOS: true,
			SSHAddonPort:      22,
		},
	}

	provider, err := NewProvider(hostConfig)
	if err != nil {
		t.Fatalf("NewProvider() error: %v", err)
	}

	hasSupervisorAPI := provider.HasSupervisorAPI()

	tests := []struct {
		name             string
		serviceName      string
		wantErr          bool
		wantErrNoSuper   bool // Expected error when no supervisor access
		expectType       string
		requireSupervisor bool
	}{
		{"core service", "homeassistant", false, false, "core", false},
		{"ha-core service", "ha-core", false, false, "core", false},
		{"supervisor service", "ha-supervisor", false, true, "supervisor", true},
		{"host service", "ha-host", false, true, "host", true},
		{"addon service", "addon-esphome", false, true, "addon", true},
		{"invalid service", "invalid", true, true, "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, err := provider.GetService(tt.serviceName)
			
			// Check error expectations based on supervisor access
			expectedErr := tt.wantErr
			if !hasSupervisorAPI && tt.wantErrNoSuper {
				expectedErr = true
			}
			
			if expectedErr {
				if err == nil {
					t.Error("GetService() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("GetService() unexpected error: %v", err)
				return
			}

			// Check GetSource
			s := svc.(*Service)
			if tt.expectType == "addon" {
				if s.GetSource() != "homeassistant-addon" {
					t.Errorf("GetSource() = %q, want %q", s.GetSource(), "homeassistant-addon")
				}
			} else {
				if s.GetSource() != "homeassistant" {
					t.Errorf("GetSource() = %q, want %q", s.GetSource(), "homeassistant")
				}
			}
		})
	}
}

// TestServiceGetName tests GetName for different service types.
// Note: Without SUPERVISOR_TOKEN env var and SSH connection, supervisor/host/addon
// services won't be available. This test only tests what's accessible.
func TestServiceGetName(t *testing.T) {
	hostConfig := &config.HostConfig{
		Name:    "testhost",
		Address: "192.168.1.100",
		HomeAssistant: &config.HomeAssistantConfig{
			Port:              8123,
			LongLivedToken:    "test-token",
			IsHomeAssistantOS: true,
			SSHAddonPort:      22,
		},
	}

	provider, err := NewProvider(hostConfig)
	if err != nil {
		t.Fatalf("NewProvider() error: %v", err)
	}

	hasSupervisorAPI := provider.HasSupervisorAPI()

	tests := []struct {
		serviceName       string
		expectName        string
		requireSupervisor bool
	}{
		{"homeassistant", "homeassistant", false},
		{"ha-supervisor", "ha-supervisor", true},
		{"ha-host", "ha-host", true},
		{"addon-esphome", "addon-esphome", true},
	}

	for _, tt := range tests {
		t.Run(tt.serviceName, func(t *testing.T) {
			if tt.requireSupervisor && !hasSupervisorAPI {
				t.Skip("Skipping test: requires Supervisor API access (SUPERVISOR_TOKEN + SSH)")
			}
			svc, err := provider.GetService(tt.serviceName)
			if err != nil {
				t.Fatalf("GetService() error: %v", err)
			}
			if svc.GetName() != tt.expectName {
				t.Errorf("GetName() = %q, want %q", svc.GetName(), tt.expectName)
			}
		})
	}
}

// createMockSupervisorProvider creates a Provider with a mock supervisor client for testing.
func createMockSupervisorProvider(t *testing.T, server *httptest.Server) *Provider {
	hostConfig := &config.HostConfig{
		Name:    "testhost",
		Address: "192.168.1.100",
		HomeAssistant: &config.HomeAssistantConfig{
			Port:              8123,
			LongLivedToken:    "test-token",
			IsHomeAssistantOS: true,
			SSHAddonPort:      22,
		},
	}

	provider, err := NewProvider(hostConfig)
	if err != nil {
		t.Fatalf("NewProvider() error: %v", err)
	}

	// Replace the supervisor client with a mock that points to our test server
	// We need to create a custom transport that rewrites URLs to the test server
	provider.supervisorClient = &http.Client{
		Transport: &mockSupervisorTransport{
			testServerURL: server.URL,
		},
	}
	provider.supervisorToken = "mock-supervisor-token"

	return provider
}

// mockSupervisorTransport rewrites Supervisor API requests to a test server.
type mockSupervisorTransport struct {
	testServerURL string
}

func (t *mockSupervisorTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Rewrite the URL to point to the test server
	newURL := t.testServerURL + req.URL.Path
	newReq, err := http.NewRequestWithContext(req.Context(), req.Method, newURL, req.Body)
	if err != nil {
		return nil, err
	}
	// Copy headers
	for k, v := range req.Header {
		newReq.Header[k] = v
	}
	return http.DefaultClient.Do(newReq)
}

// TestCoreControl tests the CoreControl method via mock Supervisor API.
func TestCoreControl(t *testing.T) {
	server := mockSupervisorServer(t)
	defer server.Close()

	provider := createMockSupervisorProvider(t, server)

	tests := []struct {
		name    string
		action  string
		wantErr bool
	}{
		{"start", "start", false},
		{"stop", "stop", false},
		{"restart", "restart", false},
		{"invalid action", "invalid", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := provider.CoreControl(context.Background(), tt.action)
			if tt.wantErr {
				if err == nil {
					t.Error("CoreControl() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("CoreControl() unexpected error: %v", err)
				}
			}
		})
	}
}

// TestAddonControl tests the AddonControl method via mock Supervisor API.
func TestAddonControl(t *testing.T) {
	server := mockSupervisorServer(t)
	defer server.Close()

	provider := createMockSupervisorProvider(t, server)

	tests := []struct {
		name    string
		slug    string
		action  string
		wantErr bool
	}{
		{"start addon", "esphome", "start", false},
		{"stop addon", "esphome", "stop", false},
		{"restart addon", "esphome", "restart", false},
		{"invalid action", "esphome", "invalid", true},
		{"unknown addon", "unknown", "start", true}, // Server will return 404
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := provider.AddonControl(context.Background(), tt.slug, tt.action)
			if tt.wantErr {
				if err == nil {
					t.Error("AddonControl() expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("AddonControl() unexpected error: %v", err)
				}
			}
		})
	}
}

// TestServiceStartStopRestart tests the Service.Start/Stop/Restart methods.
func TestServiceStartStopRestart(t *testing.T) {
	server := mockSupervisorServer(t)
	defer server.Close()

	provider := createMockSupervisorProvider(t, server)

	tests := []struct {
		name        string
		serviceName string
		action      string
		wantErr     bool
	}{
		// Core service tests (HAOS with Supervisor API)
		{"core start", "homeassistant", "start", false},
		{"core stop", "homeassistant", "stop", false},
		{"core restart", "homeassistant", "restart", false},

		// Addon service tests
		{"addon start", "addon-esphome", "start", false},
		{"addon stop", "addon-esphome", "stop", false},
		{"addon restart", "addon-esphome", "restart", false},

		// Supervisor and Host don't support control
		{"supervisor restart", "ha-supervisor", "restart", true},
		{"host restart", "ha-host", "restart", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, err := provider.GetService(tt.serviceName)
			if err != nil {
				t.Fatalf("GetService() error: %v", err)
			}

			var actionErr error
			switch tt.action {
			case "start":
				actionErr = svc.Start(context.Background())
			case "stop":
				actionErr = svc.Stop(context.Background())
			case "restart":
				actionErr = svc.Restart(context.Background())
			}

			if tt.wantErr {
				if actionErr == nil {
					t.Errorf("%s() expected error, got nil", tt.action)
				}
			} else {
				if actionErr != nil {
					t.Errorf("%s() unexpected error: %v", tt.action, actionErr)
				}
			}
		})
	}
}

// TestCoreControlWithoutSupervisorAPI tests Core control on non-HAOS installations.
func TestCoreControlWithoutSupervisorAPI(t *testing.T) {
	// Create a provider without Supervisor API access (non-HAOS)
	hostConfig := &config.HostConfig{
		Name:    "testhost",
		Address: "192.168.1.100",
		HomeAssistant: &config.HomeAssistantConfig{
			Port:              8123,
			LongLivedToken:    "test-token",
			IsHomeAssistantOS: false, // Not HAOS
		},
	}

	provider, err := NewProvider(hostConfig)
	if err != nil {
		t.Fatalf("NewProvider() error: %v", err)
	}

	svc, err := provider.GetService("homeassistant")
	if err != nil {
		t.Fatalf("GetService() error: %v", err)
	}

	ctx := context.Background()

	// Start should fail on non-HAOS
	if err := svc.Start(ctx); err == nil {
		t.Error("Start() expected error on non-HAOS, got nil")
	}

	// Stop should fail on non-HAOS
	if err := svc.Stop(ctx); err == nil {
		t.Error("Stop() expected error on non-HAOS, got nil")
	}

	// Restart will attempt to use HA REST API (may fail due to no server, but should not error with "not supported")
	err = svc.Restart(ctx)
	if err != nil {
		// Expected: connection refused or similar, not "not supported"
		if strings.Contains(err.Error(), "not supported") {
			t.Errorf("Restart() should attempt HA REST API, got: %v", err)
		}
	}
}
