package watchtower

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"home_server_dashboard/config"
)

const sampleMetrics = `# HELP watchtower_containers_scanned Number of containers scanned for changes by watchtower during the last scan
# TYPE watchtower_containers_scanned gauge
watchtower_containers_scanned 15
# HELP watchtower_containers_updated Number of containers updated by watchtower during the last scan
# TYPE watchtower_containers_updated gauge
watchtower_containers_updated 2
# HELP watchtower_containers_failed Number of containers where update failed during the last scan
# TYPE watchtower_containers_failed gauge
watchtower_containers_failed 1
# HELP watchtower_containers_restarted Number of containers restarted due to linked dependencies during the last scan
# TYPE watchtower_containers_restarted gauge
watchtower_containers_restarted 3
# HELP watchtower_scans_total Number of scans since the watchtower started
# TYPE watchtower_scans_total counter
watchtower_scans_total 42
`

func TestParsePrometheusMetrics(t *testing.T) {
	metrics, err := parsePrometheusMetrics(sampleMetrics)
	if err != nil {
		t.Fatalf("parsePrometheusMetrics failed: %v", err)
	}

	tests := []struct {
		name     string
		got      int
		expected int
	}{
		{"ContainersScanned", metrics.ContainersScanned, 15},
		{"ContainersUpdated", metrics.ContainersUpdated, 2},
		{"ContainersFailed", metrics.ContainersFailed, 1},
		{"ContainersRestarted", metrics.ContainersRestarted, 3},
		{"ScansTotal", metrics.ScansTotal, 42},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.expected {
				t.Errorf("got %d, expected %d", tt.got, tt.expected)
			}
		})
	}
}

func TestParsePrometheusMetrics_Empty(t *testing.T) {
	metrics, err := parsePrometheusMetrics("")
	if err != nil {
		t.Fatalf("parsePrometheusMetrics failed: %v", err)
	}

	if metrics.ContainersScanned != 0 {
		t.Errorf("expected 0, got %d", metrics.ContainersScanned)
	}
	if metrics.ScansTotal != 0 {
		t.Errorf("expected 0, got %d", metrics.ScansTotal)
	}
}

func TestNewClient_NilConfig(t *testing.T) {
	client := NewClient(nil)
	if client != nil {
		t.Error("expected nil client for nil config")
	}
}

func TestNewClient_NoWatchtower(t *testing.T) {
	hostCfg := &config.HostConfig{
		Name:    "test",
		Address: "localhost",
	}
	client := NewClient(hostCfg)
	if client != nil {
		t.Error("expected nil client when Watchtower is not configured")
	}
}

func TestNewClient_NoToken(t *testing.T) {
	hostCfg := &config.HostConfig{
		Name:    "test",
		Address: "localhost",
		Watchtower: &config.WatchtowerConfig{
			Port: 8080,
			// No token
		},
	}
	client := NewClient(hostCfg)
	if client != nil {
		t.Error("expected nil client when token is missing")
	}
}

func TestNewClient_ValidConfig(t *testing.T) {
	// Set env variable for token
	t.Setenv("WATCHTOWER_TOKEN", "test-token")

	hostCfg := &config.HostConfig{
		Name:    "test",
		Address: "localhost",
		Watchtower: &config.WatchtowerConfig{
			Port: 8080,
		},
	}
	client := NewClient(hostCfg)
	if client == nil {
		t.Fatal("expected non-nil client")
	}

	if client.BaseURL() != "http://localhost:8080" {
		t.Errorf("unexpected base URL: %s", client.BaseURL())
	}
}

func TestClient_GetMetrics(t *testing.T) {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check authorization
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte(sampleMetrics))
	}))
	defer server.Close()

	client := &Client{
		baseURL: server.URL,
		token:   "test-token",
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}

	ctx := context.Background()
	metrics, err := client.GetMetrics(ctx)
	if err != nil {
		t.Fatalf("GetMetrics failed: %v", err)
	}

	if metrics.ContainersScanned != 15 {
		t.Errorf("expected 15 containers scanned, got %d", metrics.ContainersScanned)
	}
	if metrics.ScansTotal != 42 {
		t.Errorf("expected 42 total scans, got %d", metrics.ScansTotal)
	}
}

func TestClient_GetMetrics_Unauthorized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte("unauthorized"))
	}))
	defer server.Close()

	client := &Client{
		baseURL: server.URL,
		token:   "wrong-token",
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}

	ctx := context.Background()
	_, err := client.GetMetrics(ctx)
	if err == nil {
		t.Fatal("expected error for unauthorized request")
	}
}

func TestClient_IsUpdateInProgress(t *testing.T) {
	scanCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		scanCount++
		w.Header().Set("Content-Type", "text/plain")
		// First call: no active scan, scans_total = 10
		// Second call: active scan, scans_total = 11
		if scanCount == 1 {
			w.Write([]byte(`watchtower_containers_scanned 0
watchtower_scans_total 10`))
		} else {
			w.Write([]byte(`watchtower_containers_scanned 5
watchtower_scans_total 11`))
		}
	}))
	defer server.Close()

	client := &Client{
		baseURL: server.URL,
		token:   "test-token",
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}

	ctx := context.Background()

	// First call - should initialize
	inProgress, err := client.IsUpdateInProgress(ctx)
	if err != nil {
		t.Fatalf("IsUpdateInProgress failed: %v", err)
	}
	if inProgress {
		t.Error("expected no update in progress on first call")
	}

	// Second call - scan count increased and containers being scanned
	inProgress, err = client.IsUpdateInProgress(ctx)
	if err != nil {
		t.Fatalf("IsUpdateInProgress failed: %v", err)
	}
	if !inProgress {
		t.Error("expected update in progress when scan count increased")
	}
}

func TestClient_WasRecentlyUpdated(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte(`watchtower_containers_scanned 0
watchtower_scans_total 10`))
	}))
	defer server.Close()

	client := &Client{
		baseURL: server.URL,
		token:   "test-token",
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}

	ctx := context.Background()

	// First call
	updated, err := client.WasRecentlyUpdated(ctx, time.Minute)
	if err != nil {
		t.Fatalf("WasRecentlyUpdated failed: %v", err)
	}
	if updated {
		t.Error("expected no recent update on first call")
	}
}

func TestClient_GetLastMetrics(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte(sampleMetrics))
	}))
	defer server.Close()

	client := &Client{
		baseURL: server.URL,
		token:   "test-token",
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}

	// Before any call
	if client.GetLastMetrics() != nil {
		t.Error("expected nil before first GetMetrics call")
	}

	// After GetMetrics call
	ctx := context.Background()
	_, _ = client.GetMetrics(ctx)

	cached := client.GetLastMetrics()
	if cached == nil {
		t.Fatal("expected cached metrics")
	}
	if cached.ContainersScanned != 15 {
		t.Errorf("expected 15, got %d", cached.ContainersScanned)
	}
}
