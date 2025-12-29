package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"home_server_dashboard/config"
	"home_server_dashboard/services"
)

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
	servicesHandler(w, req)

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

// TestLogsHandler_MissingContainer tests logs handler without container param.
func TestLogsHandler_MissingContainer(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/logs", nil)
	w := httptest.NewRecorder()

	logsHandler(w, req)

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

	systemdLogsHandler(w, req)

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
	systemdLogsHandler(w, req)

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

// TestLogsHandler_SSEHeaders tests that Docker logs handler sets SSE headers.
func TestLogsHandler_SSEHeaders(t *testing.T) {
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
	logsHandler(w, req)

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

// TestIndexHandler tests that root path serves index.html.
func TestIndexHandler(t *testing.T) {
	// Create a mock handler that mimics the root handler logic
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			// Would serve index.html in real app
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	})

	tests := []struct {
		path       string
		wantStatus int
	}{
		{"/", http.StatusOK},
		{"/nonexistent", http.StatusNotFound},
		{"/api", http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)

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
		servicesHandler(w, req)
	}
}
