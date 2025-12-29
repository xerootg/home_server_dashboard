// Package main tests for the Home Server Dashboard bootstrap.
package main

import (
	"os"
	"testing"

	"home_server_dashboard/config"
	"home_server_dashboard/server"
)

// TestConfigPackageImport verifies config package is accessible from main.
func TestConfigPackageImport(t *testing.T) {
	cfg := config.Default()
	if cfg == nil {
		t.Fatal("Expected non-nil default config")
	}
	if len(cfg.Hosts) == 0 {
		t.Error("Expected default config to have at least one host")
	}
}

// TestServerPackageImport verifies server package is accessible from main.
func TestServerPackageImport(t *testing.T) {
	cfg := server.DefaultConfig()
	if cfg == nil {
		t.Fatal("Expected non-nil default server config")
	}
	if cfg.Port != ":9001" {
		t.Errorf("Port = %v, want :9001", cfg.Port)
	}
}

// TestServerCreation verifies server can be created.
func TestServerCreation(t *testing.T) {
	srv := server.New(nil)
	if srv == nil {
		t.Fatal("Expected non-nil server")
	}
	handler := srv.Handler()
	if handler == nil {
		t.Fatal("Expected non-nil handler")
	}
}

// TestGetConfigPath verifies config path resolution from environment variable.
func TestGetConfigPath(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		want     string
	}{
		{
			name:     "default when env not set",
			envValue: "",
			want:     "services.json",
		},
		{
			name:     "uses env value when set",
			envValue: "/etc/nas_dashboard/services.json",
			want:     "/etc/nas_dashboard/services.json",
		},
		{
			name:     "uses custom path",
			envValue: "/custom/path/config.json",
			want:     "/custom/path/config.json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save and restore original env value
			original := os.Getenv("CONFIG_PATH")
			defer os.Setenv("CONFIG_PATH", original)

			if tt.envValue == "" {
				os.Unsetenv("CONFIG_PATH")
			} else {
				os.Setenv("CONFIG_PATH", tt.envValue)
			}

			got := getConfigPath()
			if got != tt.want {
				t.Errorf("getConfigPath() = %q, want %q", got, tt.want)
			}
		})
	}
}
