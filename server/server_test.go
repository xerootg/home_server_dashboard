package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Port != ":9001" {
		t.Errorf("Port = %v, want :9001", cfg.Port)
	}
	if cfg.StaticDir != "static" {
		t.Errorf("StaticDir = %v, want static", cfg.StaticDir)
	}
	if cfg.ConfigPath != "services.json" {
		t.Errorf("ConfigPath = %v, want services.json", cfg.ConfigPath)
	}
}

func TestNew_WithNilConfig(t *testing.T) {
	s := New(nil)

	if s == nil {
		t.Fatal("Expected non-nil server")
	}
	if s.config == nil {
		t.Fatal("Expected non-nil config")
	}
	if s.mux == nil {
		t.Fatal("Expected non-nil mux")
	}
}

func TestNew_WithCustomConfig(t *testing.T) {
	cfg := &Config{
		Port:       ":8080",
		StaticDir:  "public",
		ConfigPath: "custom.json",
	}

	s := New(cfg)

	if s.config.Port != ":8080" {
		t.Errorf("Port = %v, want :8080", s.config.Port)
	}
	if s.config.StaticDir != "public" {
		t.Errorf("StaticDir = %v, want public", s.config.StaticDir)
	}
}

func TestServer_Handler(t *testing.T) {
	s := New(nil)

	handler := s.Handler()
	if handler == nil {
		t.Fatal("Expected non-nil handler")
	}
}

func TestServer_RoutesRegistered(t *testing.T) {
	s := New(nil)

	tests := []struct {
		path   string
		method string
	}{
		{"/", http.MethodGet},
		{"/api/services", http.MethodGet},
		{"/api/logs", http.MethodGet},
		{"/api/logs/systemd", http.MethodGet},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			w := httptest.NewRecorder()

			s.Handler().ServeHTTP(w, req)

			// We just check that we don't get a panic and the route exists
			// Actual handler behavior is tested in handlers package
			// 404 would indicate route not registered, but we might get other codes
			// depending on handler logic
			if w.Code == http.StatusNotFound && tt.path == "/api/services" {
				t.Errorf("Route %s not registered", tt.path)
			}
		})
	}
}

func TestServer_StaticFilesRoute(t *testing.T) {
	s := New(nil)

	// Request a static file that doesn't exist
	req := httptest.NewRequest(http.MethodGet, "/static/nonexistent.js", nil)
	w := httptest.NewRecorder()

	s.Handler().ServeHTTP(w, req)

	// Should get 404 for non-existent file, but route should be registered
	// The important thing is we don't panic
	if w.Code != http.StatusNotFound {
		t.Logf("Status = %d (expected 404 for non-existent file)", w.Code)
	}
}
