// Package main tests for the Home Server Dashboard bootstrap.
package main

import (
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
