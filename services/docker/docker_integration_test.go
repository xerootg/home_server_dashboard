//go:build integration

// Package docker provides Docker container service management.
// This file contains integration tests that require a running Docker daemon.
// Run with: go test -tags=integration ./services/docker/...
package docker

import (
	"context"
	"testing"
	"time"
)

// TestDockerIntegration_NewProvider tests creating a real Docker provider.
func TestDockerIntegration_NewProvider(t *testing.T) {
	provider, err := NewProvider("localhost")
	if err != nil {
		t.Skipf("Docker not available: %v", err)
	}
	defer provider.Close()

	if provider.client == nil {
		t.Error("Expected non-nil Docker client")
	}

	if provider.hostName != "localhost" {
		t.Errorf("hostName = %v, want localhost", provider.hostName)
	}
}

// TestDockerIntegration_GetServices tests listing Docker containers.
func TestDockerIntegration_GetServices(t *testing.T) {
	provider, err := NewProvider("localhost")
	if err != nil {
		t.Skipf("Docker not available: %v", err)
	}
	defer provider.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	services, err := provider.GetServices(ctx)
	if err != nil {
		t.Fatalf("GetServices() error = %v", err)
	}

	t.Logf("Found %d Docker Compose services", len(services))

	for _, svc := range services {
		// Validate service fields
		if svc.Source != "docker" {
			t.Errorf("Service %s has source %s, want docker", svc.Name, svc.Source)
		}
		if svc.Host != "localhost" {
			t.Errorf("Service %s has host %s, want localhost", svc.Name, svc.Host)
		}
		// Docker containers can have various states
		validStates := map[string]bool{
			"running":    true,
			"stopped":    true,
			"created":    true,
			"exited":     true,
			"restarting": true,
			"paused":     true,
			"dead":       true,
			"removing":   true,
		}
		if !validStates[svc.State] {
			t.Errorf("Service %s has unexpected state: %s", svc.Name, svc.State)
		}
		if svc.Project == "" {
			t.Errorf("Service %s has empty project (should be Docker Compose project)", svc.Name)
		}

		t.Logf("  - %s/%s: %s (%s)", svc.Project, svc.Name, svc.State, svc.Status)
	}
}

// TestDockerIntegration_GetService tests getting a specific service.
func TestDockerIntegration_GetService(t *testing.T) {
	provider, err := NewProvider("localhost")
	if err != nil {
		t.Skipf("Docker not available: %v", err)
	}
	defer provider.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// First get the list of services to find a real container
	services, err := provider.GetServices(ctx)
	if err != nil {
		t.Fatalf("GetServices() error = %v", err)
	}

	if len(services) == 0 {
		t.Skip("No Docker Compose containers running to test GetService")
	}

	// Test with the first container
	containerName := services[0].ContainerName
	t.Logf("Testing GetService with container: %s", containerName)

	svc, err := provider.GetService(containerName)
	if err != nil {
		t.Fatalf("GetService(%s) error = %v", containerName, err)
	}

	info, err := svc.GetInfo(ctx)
	if err != nil {
		t.Fatalf("GetInfo() error = %v", err)
	}

	if info.ContainerName != containerName {
		t.Errorf("ContainerName = %v, want %v", info.ContainerName, containerName)
	}

	t.Logf("Container info: %s state=%s status=%s", info.ContainerName, info.State, info.Status)
}

// TestDockerIntegration_GetLogs tests streaming container logs.
func TestDockerIntegration_GetLogs(t *testing.T) {
	provider, err := NewProvider("localhost")
	if err != nil {
		t.Skipf("Docker not available: %v", err)
	}
	defer provider.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// First get the list of services to find a real container
	services, err := provider.GetServices(ctx)
	if err != nil {
		t.Fatalf("GetServices() error = %v", err)
	}

	// Find a running container
	var containerName string
	for _, svc := range services {
		if svc.State == "running" {
			containerName = svc.ContainerName
			break
		}
	}

	if containerName == "" {
		t.Skip("No running Docker containers to test logs")
	}

	t.Logf("Testing logs for container: %s", containerName)

	// Get logs without following (just tail)
	logs, err := provider.GetLogs(ctx, containerName, 10, false)
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
		if n > 100 {
			t.Logf("Log sample: %s...", string(buf[:100]))
		} else if n > 0 {
			t.Logf("Log sample: %s", string(buf[:n]))
		}
	}
}

// TestDockerIntegration_ServiceInterface tests the Service interface methods.
func TestDockerIntegration_ServiceInterface(t *testing.T) {
	provider, err := NewProvider("localhost")
	if err != nil {
		t.Skipf("Docker not available: %v", err)
	}
	defer provider.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	services, err := provider.GetServices(ctx)
	if err != nil {
		t.Fatalf("GetServices() error = %v", err)
	}

	if len(services) == 0 {
		t.Skip("No Docker Compose containers to test")
	}

	containerName := services[0].ContainerName
	svc, err := provider.GetService(containerName)
	if err != nil {
		t.Fatalf("GetService() error = %v", err)
	}

	// Test interface methods
	if name := svc.GetName(); name != containerName {
		t.Errorf("GetName() = %v, want %v", name, containerName)
	}

	if host := svc.GetHost(); host != "localhost" {
		t.Errorf("GetHost() = %v, want localhost", host)
	}

	if source := svc.GetSource(); source != "docker" {
		t.Errorf("GetSource() = %v, want docker", source)
	}
}
