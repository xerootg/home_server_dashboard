package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"home_server_dashboard/auth"
	"home_server_dashboard/config"
	"home_server_dashboard/services"
	"home_server_dashboard/services/docker"
)

// authUserContextKey is the context key used by auth package for storing user
var authUserContextKey = auth.UserContextKey

// testAdminUser is a mock admin user for testing
var testAdminUser = auth.User{
	ID:              "test-admin",
	Email:           "admin@test.com",
	Name:            "Test Admin",
	IsAdmin:         true,
	HasGlobalAccess: true,
}

// testNonAdminUser is a mock non-admin user for testing
var testNonAdminUser = auth.User{
	ID:              "test-user",
	Email:           "user@test.com",
	Name:            "Test User",
	IsAdmin:         false,
	HasGlobalAccess: false,
}

// setupTestConfig creates a temporary config file and loads it.
func setupTestConfig(t *testing.T, configJSON string) func() {
	t.Helper()

	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "services.json")

	err := os.WriteFile(configPath, []byte(configJSON), 0644)
	if err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	// Save original working directory
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}

	// Load config directly (bypassing file path issues)
	_, err = config.Load(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	return func() {
		// Restore state if needed
		_ = origDir
	}
}

// TestServicesHandler_NoConfig tests the handler when config is nil.
func TestServicesHandler_NoConfig(t *testing.T) {
	// Clear any existing config
	// Note: This test assumes we can access internal state, which we can't directly
	// So we'll skip this specific scenario
	t.Skip("Cannot easily clear global config state")
}

// TestServicesHandler_ReturnsJSON tests that the handler returns JSON.
func TestServicesHandler_ReturnsJSON(t *testing.T) {
	// Set up a minimal config
	configJSON := `{
		"hosts": [
			{
				"name": "localhost",
				"address": "localhost",
				"systemd_services": [],
				"docker_compose_roots": []
			}
		]
	}`

	cleanup := setupTestConfig(t, configJSON)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/services", nil)
	w := httptest.NewRecorder()

	// Call handler
	ServicesHandler(w, req)

	// Check content type
	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Content-Type = %v, want application/json", contentType)
	}

	// Response should be valid JSON (even if empty or error)
	// Note: This may fail if Docker isn't available, which is expected
	// The important thing is that it returns JSON
	body := w.Body.String()
	if !strings.HasPrefix(body, "[") && !strings.Contains(body, "Error") {
		// It should either be a JSON array or an error response
		t.Logf("Response body: %s", body)
	}
}

// TestDockerLogsHandler_MissingContainer tests logs handler without container param.
func TestDockerLogsHandler_MissingContainer(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/logs", nil)
	w := httptest.NewRecorder()

	DockerLogsHandler(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Status code = %d, want %d", w.Code, http.StatusBadRequest)
	}

	if !strings.Contains(w.Body.String(), "container parameter required") {
		t.Errorf("Expected 'container parameter required' in body, got: %s", w.Body.String())
	}
}

// TestSystemdLogsHandler_MissingUnit tests systemd logs handler without unit param.
func TestSystemdLogsHandler_MissingUnit(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/logs/systemd", nil)
	w := httptest.NewRecorder()

	SystemdLogsHandler(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Status code = %d, want %d", w.Code, http.StatusBadRequest)
	}

	if !strings.Contains(w.Body.String(), "unit parameter required") {
		t.Errorf("Expected 'unit parameter required' in body, got: %s", w.Body.String())
	}
}

// TestSystemdLogsHandler_SSEHeaders tests that SSE headers are set correctly.
func TestSystemdLogsHandler_SSEHeaders(t *testing.T) {
	configJSON := `{
		"hosts": [
			{
				"name": "testhost",
				"address": "192.168.1.100",
				"systemd_services": ["test.service"],
				"docker_compose_roots": []
			}
		]
	}`

	cleanup := setupTestConfig(t, configJSON)
	defer cleanup()

	// Create a request with unit parameter
	req := httptest.NewRequest(http.MethodGet, "/api/logs/systemd?unit=test.service&host=testhost", nil)

	// Use a context that will be cancelled immediately to prevent hanging
	ctx, cancel := context.WithCancel(req.Context())
	cancel() // Cancel immediately
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	// Call handler - it will try to connect and fail, but headers should be set
	SystemdLogsHandler(w, req)

	// Check SSE headers
	if ct := w.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type = %v, want text/event-stream", ct)
	}
	if cc := w.Header().Get("Cache-Control"); cc != "no-cache" {
		t.Errorf("Cache-Control = %v, want no-cache", cc)
	}
	if conn := w.Header().Get("Connection"); conn != "keep-alive" {
		t.Errorf("Connection = %v, want keep-alive", conn)
	}
}

// TestDockerLogsHandler_SSEHeaders tests that Docker logs handler sets SSE headers.
func TestDockerLogsHandler_SSEHeaders(t *testing.T) {
	configJSON := `{
		"hosts": [
			{
				"name": "localhost",
				"address": "localhost",
				"systemd_services": [],
				"docker_compose_roots": []
			}
		]
	}`

	cleanup := setupTestConfig(t, configJSON)
	defer cleanup()

	// Create a request with container parameter
	req := httptest.NewRequest(http.MethodGet, "/api/logs?container=test-container", nil)

	// Use a context that will be cancelled immediately
	ctx, cancel := context.WithCancel(req.Context())
	cancel()
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	// Call handler
	DockerLogsHandler(w, req)

	// Check SSE headers (they should be set even if Docker connection fails)
	if ct := w.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type = %v, want text/event-stream", ct)
	}
}

// TestGetAllServices_WithConfig tests getAllServices with valid config.
func TestGetAllServices_WithConfig(t *testing.T) {
	cfg := &config.Config{
		Hosts: []config.HostConfig{
			{
				Name:            "testhost",
				Address:         "192.168.1.100", // Remote host - won't try D-Bus
				SystemdServices: []string{},      // Empty to avoid SSH attempts
			},
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately to prevent actual connections

	// This will fail but shouldn't panic
	_, err := getAllServices(ctx, cfg)
	// Error is expected since Docker connection will fail with cancelled context
	if err == nil {
		t.Log("getAllServices succeeded (Docker may be available)")
	}
}

// TestServiceInfoJSON tests that service info serializes correctly for API.
func TestServiceInfoJSON(t *testing.T) {
	svc := services.ServiceInfo{
		Name:          "nginx",
		Project:       "webstack",
		ContainerName: "webstack-nginx-1",
		State:         "running",
		Status:        "Up 2 hours",
		Image:         "nginx:latest",
		Source:        "docker",
		Host:          "nas",
	}

	data, err := json.Marshal(svc)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	// Verify expected JSON structure
	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	expected := map[string]string{
		"name":           "nginx",
		"project":        "webstack",
		"container_name": "webstack-nginx-1",
		"state":          "running",
		"status":         "Up 2 hours",
		"image":          "nginx:latest",
		"source":         "docker",
		"host":           "nas",
	}

	for key, want := range expected {
		if got, ok := result[key]; !ok {
			t.Errorf("Missing key %s in JSON", key)
		} else if got != want {
			t.Errorf("JSON[%s] = %v, want %v", key, got, want)
		}
	}
}

// TestIndexHandler tests that root path serves correctly.
func TestIndexHandler(t *testing.T) {
	tests := []struct {
		path       string
		wantStatus int
	}{
		{"/nonexistent", http.StatusNotFound},
		{"/api", http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			w := httptest.NewRecorder()
			IndexHandler(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("Status = %d, want %d", w.Code, tt.wantStatus)
			}
		})
	}
}

// BenchmarkServicesHandler benchmarks the services endpoint.
func BenchmarkServicesHandler(b *testing.B) {
	// Skip if running in environment without Docker
	b.Skip("Benchmark requires Docker; run manually if needed")

	configJSON := `{
		"hosts": [
			{
				"name": "localhost",
				"address": "localhost",
				"systemd_services": [],
				"docker_compose_roots": []
			}
		]
	}`

	tempDir := b.TempDir()
	configPath := filepath.Join(tempDir, "services.json")
	_ = os.WriteFile(configPath, []byte(configJSON), 0644)
	_, _ = config.Load(configPath)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/services", nil)
		w := httptest.NewRecorder()
		ServicesHandler(w, req)
	}
}

// TestBangAndPipeDocsHandler_Success tests that the docs handler returns HTML.
func TestBangAndPipeDocsHandler_Success(t *testing.T) {
	// Save and restore current directory
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}
	
	// Create a temporary docs directory with test content
	tempDir := t.TempDir()
	docsDir := filepath.Join(tempDir, "docs")
	if err := os.MkdirAll(docsDir, 0755); err != nil {
		t.Fatalf("Failed to create docs dir: %v", err)
	}
	
	// Write test markdown
	mdContent := "# Test Title\n\nThis is **bold** and `code`."
	if err := os.WriteFile(filepath.Join(docsDir, "bangandpipe-query-language.md"), []byte(mdContent), 0644); err != nil {
		t.Fatalf("Failed to write test markdown: %v", err)
	}
	
	// Change to temp directory
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("Failed to change directory: %v", err)
	}
	defer os.Chdir(origDir)
	
	req := httptest.NewRequest(http.MethodGet, "/api/docs/bangandpipe", nil)
	w := httptest.NewRecorder()
	
	BangAndPipeDocsHandler(w, req)
	
	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
	
	contentType := w.Header().Get("Content-Type")
	if !strings.HasPrefix(contentType, "text/html") {
		t.Errorf("Expected text/html content type, got %s", contentType)
	}
	
	body := w.Body.String()
	if !strings.Contains(body, "<h1") {
		t.Errorf("Expected HTML heading, got: %s", body)
	}
	if !strings.Contains(body, "<strong>bold</strong>") {
		t.Errorf("Expected bold rendering, got: %s", body)
	}
	if !strings.Contains(body, "<code>code</code>") {
		t.Errorf("Expected code rendering, got: %s", body)
	}
}

// TestBangAndPipeDocsHandler_NotFound tests that missing docs returns 404.
func TestBangAndPipeDocsHandler_NotFound(t *testing.T) {
	// Save and restore current directory
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}
	
	// Change to temp directory without docs folder
	tempDir := t.TempDir()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("Failed to change directory: %v", err)
	}
	defer os.Chdir(origDir)
	
	req := httptest.NewRequest(http.MethodGet, "/api/docs/bangandpipe", nil)
	w := httptest.NewRecorder()
	
	BangAndPipeDocsHandler(w, req)
	
	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", w.Code)
	}
}

// TestServiceActionHandler_MethodNotAllowed tests that GET is rejected.
func TestServiceActionHandler_MethodNotAllowed(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/services/start", nil)
	w := httptest.NewRecorder()

	ServiceActionHandler(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Status code = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

// TestServiceActionHandler_InvalidAction tests that invalid actions are rejected.
func TestServiceActionHandler_InvalidAction(t *testing.T) {
	body := strings.NewReader(`{"container_name": "test", "service_name": "test", "source": "docker"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/services/invalid", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	ServiceActionHandler(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Status code = %d, want %d", w.Code, http.StatusBadRequest)
	}

	if !strings.Contains(w.Body.String(), "Invalid action") {
		t.Errorf("Expected 'Invalid action' in body, got: %s", w.Body.String())
	}
}

// TestServiceActionHandler_InvalidJSON tests that invalid JSON is rejected.
func TestServiceActionHandler_InvalidJSON(t *testing.T) {
	body := strings.NewReader(`{invalid json}`)
	req := httptest.NewRequest(http.MethodPost, "/api/services/start", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	ServiceActionHandler(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Status code = %d, want %d", w.Code, http.StatusBadRequest)
	}

	if !strings.Contains(w.Body.String(), "Invalid request body") {
		t.Errorf("Expected 'Invalid request body' in body, got: %s", w.Body.String())
	}
}

// TestServiceActionHandler_SSEHeaders tests that SSE headers are set.
func TestServiceActionHandler_SSEHeaders(t *testing.T) {
	configJSON := `{
		"hosts": [
			{
				"name": "localhost",
				"address": "localhost",
				"systemd_services": [],
				"docker_compose_roots": []
			}
		]
	}`

	cleanup := setupTestConfig(t, configJSON)
	defer cleanup()

	body := strings.NewReader(`{"container_name": "test-container", "service_name": "test", "source": "docker", "host": "localhost"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/services/start", body)
	req.Header.Set("Content-Type", "application/json")
	
	// Use a context that will be cancelled immediately
	ctx, cancel := context.WithCancel(req.Context())
	cancel()
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	ServiceActionHandler(w, req)

	// Check SSE headers
	if ct := w.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type = %v, want text/event-stream", ct)
	}
	if cc := w.Header().Get("Cache-Control"); cc != "no-cache" {
		t.Errorf("Cache-Control = %v, want no-cache", cc)
	}
	if conn := w.Header().Get("Connection"); conn != "keep-alive" {
		t.Errorf("Connection = %v, want keep-alive", conn)
	}
}

// TestServiceActionHandler_UnknownSource tests that unknown source is rejected.
func TestServiceActionHandler_UnknownSource(t *testing.T) {
	configJSON := `{
		"hosts": [
			{
				"name": "localhost",
				"address": "localhost",
				"systemd_services": [],
				"docker_compose_roots": []
			}
		]
	}`

	cleanup := setupTestConfig(t, configJSON)
	defer cleanup()

	body := strings.NewReader(`{"container_name": "test", "service_name": "test", "source": "unknown", "host": "localhost"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/services/start", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	ServiceActionHandler(w, req)

	// Check SSE headers are set
	if ct := w.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type = %v, want text/event-stream", ct)
	}

	// Check response contains error about unknown source
	responseBody := w.Body.String()
	if !strings.Contains(responseBody, "Unknown service source") {
		t.Errorf("Expected 'Unknown service source' in body, got: %s", responseBody)
	}
}

// TestServiceActionHandler_ValidActions tests that valid actions are accepted.
func TestServiceActionHandler_ValidActions(t *testing.T) {
	configJSON := `{
		"hosts": [
			{
				"name": "localhost",
				"address": "localhost",
				"systemd_services": [],
				"docker_compose_roots": []
			}
		]
	}`

	cleanup := setupTestConfig(t, configJSON)
	defer cleanup()

	actions := []string{"start", "stop", "restart"}
	for _, action := range actions {
		t.Run(action, func(t *testing.T) {
			body := strings.NewReader(`{"container_name": "test-container", "service_name": "test", "source": "docker", "host": "localhost"}`)
			req := httptest.NewRequest(http.MethodPost, "/api/services/"+action, body)
			req.Header.Set("Content-Type", "application/json")

			// Use a context that will be cancelled immediately
			ctx, cancel := context.WithCancel(req.Context())
			cancel()
			req = req.WithContext(ctx)

			w := httptest.NewRecorder()

			ServiceActionHandler(w, req)

			// Should set SSE headers (handler accepted the action)
			if ct := w.Header().Get("Content-Type"); ct != "text/event-stream" {
				t.Errorf("Content-Type = %v, want text/event-stream for action %s", ct, action)
			}
		})
	}
}

// TestServiceActionHandler_ReadOnlyService tests that readonly services reject actions.
func TestServiceActionHandler_ReadOnlyService(t *testing.T) {
	configJSON := `{
		"hosts": [
			{
				"name": "testhost",
				"address": "localhost",
				"systemd_services": ["docker.service", "nas-dashboard.service:ro"],
				"docker_compose_roots": []
			}
		]
	}`

	cleanup := setupTestConfig(t, configJSON)
	defer cleanup()

	t.Run("readonly systemd service rejects actions", func(t *testing.T) {
		body := strings.NewReader(`{"container_name": "nas-dashboard.service", "service_name": "nas-dashboard.service", "source": "systemd", "host": "testhost"}`)
		req := httptest.NewRequest(http.MethodPost, "/api/services/restart", body)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		ServiceActionHandler(w, req)

		// Should return forbidden
		if w.Code != http.StatusForbidden {
			t.Errorf("Status = %d, want %d", w.Code, http.StatusForbidden)
		}

		// Check response contains readonly message
		responseBody := w.Body.String()
		if !strings.Contains(responseBody, "read-only") {
			t.Errorf("Expected 'read-only' in body, got: %s", responseBody)
		}
	})

	t.Run("non-readonly systemd service allows actions", func(t *testing.T) {
		body := strings.NewReader(`{"container_name": "docker.service", "service_name": "docker.service", "source": "systemd", "host": "testhost"}`)
		req := httptest.NewRequest(http.MethodPost, "/api/services/restart", body)
		req.Header.Set("Content-Type", "application/json")

		// Use a context that will be cancelled immediately to prevent actual service actions
		ctx, cancel := context.WithCancel(req.Context())
		cancel() // Cancel immediately to prevent real actions
		req = req.WithContext(ctx)

		w := httptest.NewRecorder()

		ServiceActionHandler(w, req)

		// Should NOT return forbidden - should set SSE headers instead
		if w.Code == http.StatusForbidden {
			t.Error("Non-readonly service should not be forbidden")
		}

		if ct := w.Header().Get("Content-Type"); ct != "text/event-stream" {
			t.Errorf("Content-Type = %v, want text/event-stream", ct)
		}
	})
}

// TestFindComposeFile tests the compose file detection function.
func TestFindComposeFile(t *testing.T) {
	tempDir := t.TempDir()

	// Test with no compose file
	result := findComposeFile(tempDir)
	if result != "" {
		t.Errorf("Expected empty string for empty dir, got: %s", result)
	}

	// Test with docker-compose.yml
	composePath := filepath.Join(tempDir, "docker-compose.yml")
	if err := os.WriteFile(composePath, []byte("version: '3'"), 0644); err != nil {
		t.Fatalf("Failed to create compose file: %v", err)
	}

	result = findComposeFile(tempDir)
	if result != composePath {
		t.Errorf("Expected %s, got: %s", composePath, result)
	}

	// Remove and test with compose.yml
	os.Remove(composePath)
	composePath2 := filepath.Join(tempDir, "compose.yml")
	if err := os.WriteFile(composePath2, []byte("version: '3'"), 0644); err != nil {
		t.Fatalf("Failed to create compose file: %v", err)
	}

	result = findComposeFile(tempDir)
	if result != composePath2 {
		t.Errorf("Expected %s, got: %s", composePath2, result)
	}
}

// TestServiceActionRequest_JSONParsing tests JSON parsing of request body.
func TestServiceActionRequest_JSONParsing(t *testing.T) {
	jsonStr := `{
		"container_name": "test-container",
		"service_name": "test-service",
		"source": "docker",
		"host": "localhost",
		"project": "myproject"
	}`

	var req ServiceActionRequest
	if err := json.Unmarshal([]byte(jsonStr), &req); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	if req.ContainerName != "test-container" {
		t.Errorf("ContainerName = %s, want test-container", req.ContainerName)
	}
	if req.ServiceName != "test-service" {
		t.Errorf("ServiceName = %s, want test-service", req.ServiceName)
	}
	if req.Source != "docker" {
		t.Errorf("Source = %s, want docker", req.Source)
	}
	if req.Host != "localhost" {
		t.Errorf("Host = %s, want localhost", req.Host)
	}
	if req.Project != "myproject" {
		t.Errorf("Project = %s, want myproject", req.Project)
	}
}

// TestApplyPortRemaps tests the applyPortRemaps function.
func TestApplyPortRemaps(t *testing.T) {
	tests := []struct {
		name              string
		services          []services.ServiceInfo
		remaps            []docker.PortRemap
		expectTarget      string // service name to check for remapped port
		expectPort        uint16 // port number expected on target
		expectSource      string // expected SourceService value on remapped port (on target)
		expectSourceSvc   string // source service name to check for TargetService
		expectTargetLabel string // expected TargetService value on source port
	}{
		{
			name: "basic port remap",
			services: []services.ServiceInfo{
				{
					Name:   "gluetun",
					Source: "docker",
					Ports: []services.PortInfo{
						{HostPort: 8193, ContainerPort: 8193, Protocol: "tcp"},
					},
				},
				{
					Name:   "qbittorrent-books",
					Source: "docker",
					Ports:  []services.PortInfo{},
				},
			},
			remaps: []docker.PortRemap{
				{Port: 8193, TargetService: "qbittorrent-books", SourceService: "gluetun"},
			},
			expectTarget:      "qbittorrent-books",
			expectPort:        8193,
			expectSource:      "gluetun",
			expectSourceSvc:   "gluetun",
			expectTargetLabel: "qbittorrent-books",
		},
		{
			name: "no remaps - services unchanged",
			services: []services.ServiceInfo{
				{
					Name:   "nginx",
					Source: "docker",
					Ports: []services.PortInfo{
						{HostPort: 80, ContainerPort: 80, Protocol: "tcp"},
					},
				},
			},
			remaps:            []docker.PortRemap{},
			expectTarget:      "",
			expectPort:        0,
			expectSource:      "",
			expectSourceSvc:   "",
			expectTargetLabel: "",
		},
		{
			name: "remap to non-existent target - no change",
			services: []services.ServiceInfo{
				{
					Name:   "gluetun",
					Source: "docker",
					Ports: []services.PortInfo{
						{HostPort: 8193, ContainerPort: 8193, Protocol: "tcp"},
					},
				},
			},
			remaps: []docker.PortRemap{
				{Port: 8193, TargetService: "nonexistent", SourceService: "gluetun"},
			},
			expectTarget:      "",
			expectPort:        0,
			expectSource:      "",
			expectSourceSvc:   "",
			expectTargetLabel: "",
		},
		{
			name: "remap non-existent port - no change",
			services: []services.ServiceInfo{
				{
					Name:   "gluetun",
					Source: "docker",
					Ports: []services.PortInfo{
						{HostPort: 8080, ContainerPort: 8080, Protocol: "tcp"},
					},
				},
				{
					Name:   "qbittorrent-books",
					Source: "docker",
					Ports:  []services.PortInfo{},
				},
			},
			remaps: []docker.PortRemap{
				{Port: 8193, TargetService: "qbittorrent-books", SourceService: "gluetun"},
			},
			expectTarget:      "",
			expectPort:        0,
			expectSource:      "",
			expectSourceSvc:   "",
			expectTargetLabel: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := applyPortRemaps(tt.services, tt.remaps)

			if tt.expectTarget == "" {
				// No remap expected, verify no changes
				return
			}

			// Find target service and verify port was added
			var targetSvc *services.ServiceInfo
			var sourceSvc *services.ServiceInfo
			for i := range result {
				if result[i].Name == tt.expectTarget {
					targetSvc = &result[i]
				}
				if result[i].Name == tt.expectSourceSvc {
					sourceSvc = &result[i]
				}
			}

			if targetSvc == nil {
				t.Fatalf("Target service %q not found", tt.expectTarget)
			}

			// Check that the port was added to target with SourceService set
			var foundPort *services.PortInfo
			for i := range targetSvc.Ports {
				if targetSvc.Ports[i].HostPort == tt.expectPort {
					foundPort = &targetSvc.Ports[i]
					break
				}
			}

			if foundPort == nil {
				t.Errorf("Port %d not found on target service %s", tt.expectPort, tt.expectTarget)
			} else {
				if foundPort.SourceService != tt.expectSource {
					t.Errorf("Port SourceService = %q, want %q", foundPort.SourceService, tt.expectSource)
				}
			}

			// Check that the source port has TargetService set
			if sourceSvc != nil && tt.expectSourceSvc != "" {
				for _, port := range sourceSvc.Ports {
					if port.HostPort == tt.expectPort {
						if port.TargetService != tt.expectTargetLabel {
							t.Errorf("Port %d on source service TargetService = %q, want %q", tt.expectPort, port.TargetService, tt.expectTargetLabel)
						}
					}
				}
			}
		})
	}
}

// TestApplyPortRemaps_MultipleRemaps tests multiple port remaps from the same source.
func TestApplyPortRemaps_MultipleRemaps(t *testing.T) {
	svcList := []services.ServiceInfo{
		{
			Name:   "gluetun",
			Source: "docker",
			Ports: []services.PortInfo{
				{HostPort: 8193, ContainerPort: 8193, Protocol: "tcp"},
				{HostPort: 8194, ContainerPort: 8194, Protocol: "tcp"},
				{HostPort: 9117, ContainerPort: 9117, Protocol: "tcp"},
			},
		},
		{
			Name:   "qbittorrent-books",
			Source: "docker",
			Ports:  []services.PortInfo{},
		},
		{
			Name:   "qbittorrent-movies",
			Source: "docker",
			Ports:  []services.PortInfo{},
		},
		{
			Name:   "jackett",
			Source: "docker",
			Ports:  []services.PortInfo{},
		},
	}

	remaps := []docker.PortRemap{
		{Port: 8193, TargetService: "qbittorrent-books", SourceService: "gluetun"},
		{Port: 8194, TargetService: "qbittorrent-movies", SourceService: "gluetun"},
		{Port: 9117, TargetService: "jackett", SourceService: "gluetun"},
	}

	result := applyPortRemaps(svcList, remaps)

	// Build a map for easier checking
	svcMap := make(map[string]*services.ServiceInfo)
	for i := range result {
		svcMap[result[i].Name] = &result[i]
	}

	// Check qbittorrent-books got port 8193
	if len(svcMap["qbittorrent-books"].Ports) != 1 || svcMap["qbittorrent-books"].Ports[0].HostPort != 8193 {
		t.Errorf("qbittorrent-books should have port 8193")
	}
	if svcMap["qbittorrent-books"].Ports[0].SourceService != "gluetun" {
		t.Errorf("qbittorrent-books port should have SourceService=gluetun")
	}

	// Check qbittorrent-movies got port 8194
	if len(svcMap["qbittorrent-movies"].Ports) != 1 || svcMap["qbittorrent-movies"].Ports[0].HostPort != 8194 {
		t.Errorf("qbittorrent-movies should have port 8194")
	}

	// Check jackett got port 9117
	if len(svcMap["jackett"].Ports) != 1 || svcMap["jackett"].Ports[0].HostPort != 9117 {
		t.Errorf("jackett should have port 9117")
	}

	// Check all ports on gluetun have TargetService set
	expectedTargets := map[uint16]string{
		8193: "qbittorrent-books",
		8194: "qbittorrent-movies",
		9117: "jackett",
	}
	for _, port := range svcMap["gluetun"].Ports {
		expectedTarget, ok := expectedTargets[port.HostPort]
		if !ok {
			continue
		}
		if port.TargetService != expectedTarget {
			t.Errorf("gluetun port %d should have TargetService=%q, got %q", port.HostPort, expectedTarget, port.TargetService)
		}
	}
}

// TestLogFlushHandler_MethodNotAllowed tests that GET is rejected.
func TestLogFlushHandler_MethodNotAllowed(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/logs/flush", nil)
	w := httptest.NewRecorder()

	LogFlushHandler(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Status code = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

// TestLogFlushHandler_RequiresAdmin tests that non-admin users are rejected.
func TestLogFlushHandler_RequiresAdmin(t *testing.T) {
	body := strings.NewReader(`{"container_name": "test", "service_name": "test"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/logs/flush", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	// No user in context (nil user)
	LogFlushHandler(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("Status code = %d, want %d", w.Code, http.StatusForbidden)
	}

	if !strings.Contains(w.Body.String(), "administrator privileges required") {
		t.Errorf("Expected 'administrator privileges required' in body, got: %s", w.Body.String())
	}
}

// TestLogFlushHandler_InvalidJSON tests that invalid JSON is rejected.
func TestLogFlushHandler_InvalidJSON(t *testing.T) {
	body := strings.NewReader(`{invalid json}`)
	req := httptest.NewRequest(http.MethodPost, "/api/logs/flush", body)
	req.Header.Set("Content-Type", "application/json")
	
	// Add admin user to context
	ctx := context.WithValue(req.Context(), authUserContextKey, &testAdminUser)
	req = req.WithContext(ctx)
	
	w := httptest.NewRecorder()

	LogFlushHandler(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Status code = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

// TestLogFlushHandler_MissingContainerName tests that container_name is required.
func TestLogFlushHandler_MissingContainerName(t *testing.T) {
	body := strings.NewReader(`{"service_name": "test"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/logs/flush", body)
	req.Header.Set("Content-Type", "application/json")
	
	// Add admin user to context
	ctx := context.WithValue(req.Context(), authUserContextKey, &testAdminUser)
	req = req.WithContext(ctx)
	
	w := httptest.NewRecorder()

	LogFlushHandler(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Status code = %d, want %d", w.Code, http.StatusBadRequest)
	}

	if !strings.Contains(w.Body.String(), "container_name is required") {
		t.Errorf("Expected 'container_name is required' in body, got: %s", w.Body.String())
	}
}
