//go:build integration

// Package systemd provides systemd service management for local and remote hosts.
// This file contains integration tests that require a running systemd daemon.
// Run with: go test -tags=integration ./services/systemd/...
package systemd

import (
	"context"
	"testing"
	"time"
)

// TestSystemdIntegration_NewProvider tests creating a real systemd provider.
func TestSystemdIntegration_NewProvider(t *testing.T) {
	provider := NewProvider("localhost", "localhost", []string{"docker.service"})

	if provider.hostName != "localhost" {
		t.Errorf("hostName = %v, want localhost", provider.hostName)
	}

	if !provider.isLocal {
		t.Error("Expected isLocal to be true for localhost")
	}
}

// TestSystemdIntegration_GetServices tests listing systemd services via D-Bus.
func TestSystemdIntegration_GetServices(t *testing.T) {
	// Use a common service that should exist on most Linux systems
	unitNames := []string{"docker.service", "ssh.service", "sshd.service"}

	provider := NewProvider("localhost", "localhost", unitNames)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	services, err := provider.GetServices(ctx)
	if err != nil {
		t.Fatalf("GetServices() error = %v", err)
	}

	t.Logf("Found %d systemd services", len(services))

	for _, svc := range services {
		// Validate service fields
		if svc.Source != "systemd" {
			t.Errorf("Service %s has source %s, want systemd", svc.Name, svc.Source)
		}
		if svc.Host != "localhost" {
			t.Errorf("Service %s has host %s, want localhost", svc.Name, svc.Host)
		}
		if svc.Project != "systemd" {
			t.Errorf("Service %s has project %s, want systemd", svc.Name, svc.Project)
		}
		// State should be running, stopped, or not found
		validStates := map[string]bool{
			"running": true,
			"stopped": true,
		}
		if !validStates[svc.State] {
			t.Errorf("Service %s has unexpected state: %s", svc.Name, svc.State)
		}

		t.Logf("  - %s: %s (%s)", svc.Name, svc.State, svc.Status)
	}
}

// TestSystemdIntegration_GetService tests getting a specific systemd service.
func TestSystemdIntegration_GetService(t *testing.T) {
	provider := NewProvider("localhost", "localhost", []string{"docker.service"})

	svc, err := provider.GetService("docker.service")
	if err != nil {
		t.Fatalf("GetService() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	info, err := svc.GetInfo(ctx)
	if err != nil {
		t.Fatalf("GetInfo() error = %v", err)
	}

	if info.Name != "docker.service" {
		t.Errorf("Name = %v, want docker.service", info.Name)
	}

	if info.Source != "systemd" {
		t.Errorf("Source = %v, want systemd", info.Source)
	}

	t.Logf("Service info: %s state=%s status=%s", info.Name, info.State, info.Status)
}

// TestSystemdIntegration_GetLogs tests streaming systemd service logs.
func TestSystemdIntegration_GetLogs(t *testing.T) {
	provider := NewProvider("localhost", "localhost", []string{"docker.service"})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get logs without following (just tail)
	logs, err := provider.GetLogs(ctx, "docker.service", 10, false)
	if err != nil {
		t.Fatalf("GetLogs() error = %v", err)
	}
	defer logs.Close()

	// Read some log data
	buf := make([]byte, 4096)
	n, err := logs.Read(buf)
	if err != nil && n == 0 {
		t.Logf("No logs available or error reading: %v", err)
	} else {
		t.Logf("Read %d bytes of logs", n)
		if n > 200 {
			t.Logf("Log sample:\n%s...", string(buf[:200]))
		} else if n > 0 {
			t.Logf("Log sample:\n%s", string(buf[:n]))
		}
	}
}

// TestSystemdIntegration_ServiceInterface tests the Service interface methods.
func TestSystemdIntegration_ServiceInterface(t *testing.T) {
	provider := NewProvider("localhost", "localhost", []string{"docker.service"})

	svc, err := provider.GetService("docker.service")
	if err != nil {
		t.Fatalf("GetService() error = %v", err)
	}

	// Test interface methods
	if name := svc.GetName(); name != "docker.service" {
		t.Errorf("GetName() = %v, want docker.service", name)
	}

	if host := svc.GetHost(); host != "localhost" {
		t.Errorf("GetHost() = %v, want localhost", host)
	}

	if source := svc.GetSource(); source != "systemd" {
		t.Errorf("GetSource() = %v, want systemd", source)
	}
}

// TestSystemdIntegration_MultipleUnits tests querying multiple units at once.
func TestSystemdIntegration_MultipleUnits(t *testing.T) {
	// Test with multiple units, some may exist and some may not
	unitNames := []string{
		"docker.service",
		"nonexistent-unit-12345.service",
	}

	provider := NewProvider("localhost", "localhost", unitNames)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	services, err := provider.GetServices(ctx)
	if err != nil {
		t.Fatalf("GetServices() error = %v", err)
	}

	// Should return info for all requested units
	if len(services) != len(unitNames) {
		t.Errorf("Expected %d services, got %d", len(unitNames), len(services))
	}

	// Find the nonexistent unit and verify its status
	for _, svc := range services {
		t.Logf("  - %s: state=%s status=%s", svc.Name, svc.State, svc.Status)
		if svc.Name == "nonexistent-unit-12345.service" {
			if svc.State != "stopped" {
				t.Errorf("Nonexistent unit should have state 'stopped', got %s", svc.State)
			}
		}
	}
}

// TestSystemdIntegration_DBusConnection tests that D-Bus connection works.
func TestSystemdIntegration_DBusConnection(t *testing.T) {
	provider := NewProvider("localhost", "localhost", []string{"-.mount"})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// -.mount (root mount) should always exist on a Linux system
	services, err := provider.GetServices(ctx)
	if err != nil {
		t.Fatalf("D-Bus connection failed: %v", err)
	}

	if len(services) == 0 {
		t.Error("Expected at least one service (-.mount)")
	}

	for _, svc := range services {
		t.Logf("Root mount: %s state=%s", svc.Name, svc.State)
	}
}
