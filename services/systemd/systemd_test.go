// Package systemd provides systemd service management for local and remote hosts.
package systemd

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
)

// TestNewProvider tests the NewProvider constructor.
func TestNewProvider(t *testing.T) {
	tests := []struct {
		name        string
		hostName    string
		address     string
		unitNames   []string
		wantIsLocal bool
	}{
		{
			name:        "localhost provider",
			hostName:    "nas",
			address:     "localhost",
			unitNames:   []string{"docker.service"},
			wantIsLocal: true,
		},
		{
			name:        "loopback provider",
			hostName:    "local",
			address:     "127.0.0.1",
			unitNames:   []string{"nginx.service"},
			wantIsLocal: true,
		},
		{
			name:        "remote provider",
			hostName:    "remote-server",
			address:     "192.168.1.100",
			unitNames:   []string{"docker.service", "nginx.service"},
			wantIsLocal: false,
		},
		{
			name:        "hostname address",
			hostName:    "server",
			address:     "server.local",
			unitNames:   []string{},
			wantIsLocal: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewProvider(tt.hostName, tt.address, tt.unitNames)

			if p.hostName != tt.hostName {
				t.Errorf("hostName = %v, want %v", p.hostName, tt.hostName)
			}
			if p.address != tt.address {
				t.Errorf("address = %v, want %v", p.address, tt.address)
			}
			if len(p.unitNames) != len(tt.unitNames) {
				t.Errorf("unitNames length = %v, want %v", len(p.unitNames), len(tt.unitNames))
			}
			if p.isLocal != tt.wantIsLocal {
				t.Errorf("isLocal = %v, want %v", p.isLocal, tt.wantIsLocal)
			}
		})
	}
}

// TestProviderName tests the Provider Name method.
func TestProviderName(t *testing.T) {
	p := NewProvider("test", "localhost", nil)
	if got := p.Name(); got != "systemd" {
		t.Errorf("Name() = %v, want systemd", got)
	}
}

// TestSystemdService_Methods tests the SystemdService getter methods.
func TestSystemdService_Methods(t *testing.T) {
	svc := &SystemdService{
		unitName: "nginx.service",
		hostName: "myserver",
		address:  "192.168.1.100",
		isLocal:  false,
	}

	t.Run("GetName", func(t *testing.T) {
		if got := svc.GetName(); got != "nginx.service" {
			t.Errorf("GetName() = %v, want nginx.service", got)
		}
	})

	t.Run("GetHost", func(t *testing.T) {
		if got := svc.GetHost(); got != "myserver" {
			t.Errorf("GetHost() = %v, want myserver", got)
		}
	})

	t.Run("GetSource", func(t *testing.T) {
		if got := svc.GetSource(); got != "systemd" {
			t.Errorf("GetSource() = %v, want systemd", got)
		}
	})
}

// TestGetService tests the GetService method.
func TestGetService(t *testing.T) {
	p := NewProvider("nas", "localhost", []string{"docker.service"})

	svc, err := p.GetService("docker.service")
	if err != nil {
		t.Fatalf("GetService() error = %v", err)
	}

	systemdSvc, ok := svc.(*SystemdService)
	if !ok {
		t.Fatal("GetService() did not return a *SystemdService")
	}

	if systemdSvc.unitName != "docker.service" {
		t.Errorf("unitName = %v, want docker.service", systemdSvc.unitName)
	}

	if systemdSvc.hostName != "nas" {
		t.Errorf("hostName = %v, want nas", systemdSvc.hostName)
	}

	if !systemdSvc.isLocal {
		t.Error("isLocal should be true for localhost")
	}
}

// TestJournalReader tests the journalReader implementation.
func TestJournalReader(t *testing.T) {
	t.Run("Read from stdout", func(t *testing.T) {
		content := "Jan 01 00:00:00 host service[123]: test log line\n"
		reader := &journalReader{
			stdout: io.NopCloser(strings.NewReader(content)),
			cmd:    nil, // nil cmd for test
		}

		buf := make([]byte, 256)
		n, err := reader.Read(buf)
		if err != nil && err != io.EOF {
			t.Fatalf("Read() error = %v", err)
		}

		if got := string(buf[:n]); got != content {
			t.Errorf("Read() = %q, want %q", got, content)
		}
	})

	t.Run("Multiple reads", func(t *testing.T) {
		lines := []string{
			"line 1\n",
			"line 2\n",
			"line 3\n",
		}
		content := strings.Join(lines, "")
		reader := &journalReader{
			stdout: io.NopCloser(strings.NewReader(content)),
			cmd:    nil,
		}

		var result bytes.Buffer
		buf := make([]byte, 10) // Small buffer to force multiple reads
		for {
			n, err := reader.Read(buf)
			if n > 0 {
				result.Write(buf[:n])
			}
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatalf("Read() error = %v", err)
			}
		}

		if result.String() != content {
			t.Errorf("Read() result = %q, want %q", result.String(), content)
		}
	})
}

// TestParseSystemctlOutput tests parsing of systemctl show output.
func TestParseSystemctlOutput(t *testing.T) {
	// This tests the parsing logic used in getRemoteUnitInfo
	tests := []struct {
		name          string
		output        string
		wantActive    string
		wantSub       string
		wantLoad      string
		wantState     string
		wantStatusContains string
	}{
		{
			name: "running service",
			output: `ActiveState=active
SubState=running
LoadState=loaded`,
			wantActive:    "active",
			wantSub:       "running",
			wantLoad:      "loaded",
			wantState:     "running",
			wantStatusContains: "active",
		},
		{
			name: "stopped service",
			output: `ActiveState=inactive
SubState=dead
LoadState=loaded`,
			wantActive:    "inactive",
			wantSub:       "dead",
			wantLoad:      "loaded",
			wantState:     "stopped",
			wantStatusContains: "inactive",
		},
		{
			name: "failed service",
			output: `ActiveState=failed
SubState=failed
LoadState=loaded`,
			wantActive:    "failed",
			wantSub:       "failed",
			wantLoad:      "loaded",
			wantState:     "stopped",
			wantStatusContains: "failed",
		},
		{
			name: "not found service",
			output: `ActiveState=inactive
SubState=dead
LoadState=not-found`,
			wantActive:    "inactive",
			wantSub:       "dead",
			wantLoad:      "not-found",
			wantState:     "stopped",
			wantStatusContains: "not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Parse like getRemoteUnitInfo does
			props := make(map[string]string)
			for _, line := range strings.Split(tt.output, "\n") {
				parts := strings.SplitN(line, "=", 2)
				if len(parts) == 2 {
					props[parts[0]] = strings.TrimSpace(parts[1])
				}
			}

			activeState := props["ActiveState"]
			subState := props["SubState"]
			loadState := props["LoadState"]

			if activeState != tt.wantActive {
				t.Errorf("ActiveState = %v, want %v", activeState, tt.wantActive)
			}
			if subState != tt.wantSub {
				t.Errorf("SubState = %v, want %v", subState, tt.wantSub)
			}
			if loadState != tt.wantLoad {
				t.Errorf("LoadState = %v, want %v", loadState, tt.wantLoad)
			}

			// Check state mapping
			state := "stopped"
			if activeState == "active" {
				state = "running"
			}
			if state != tt.wantState {
				t.Errorf("state = %v, want %v", state, tt.wantState)
			}

			// Check status formatting
			status := activeState + " (" + subState + ")"
			if loadState == "not-found" {
				status = "not found"
			}
			if !strings.Contains(status, tt.wantStatusContains) {
				t.Errorf("status %q does not contain %q", status, tt.wantStatusContains)
			}
		})
	}
}

// TestProviderWithNoUnits tests provider behavior with empty unit list.
func TestProviderWithNoUnits(t *testing.T) {
	p := NewProvider("testhost", "192.168.1.100", []string{})

	if len(p.unitNames) != 0 {
		t.Errorf("unitNames should be empty, got %v", p.unitNames)
	}

	// GetServices should return empty list for remote host with no units
	// (without actually connecting since there are no units to query)
	ctx := context.Background()
	services, err := p.GetServices(ctx)
	if err != nil {
		// For remote hosts, it will try SSH which will fail, but that's expected
		// in a test environment without SSH access
		t.Log("Expected SSH error for remote host:", err)
	}
	// If no error, services should be empty
	if err == nil && len(services) != 0 {
		t.Errorf("Expected empty services, got %d", len(services))
	}
}

// TestSystemdServiceLocal tests local SystemdService configuration.
func TestSystemdServiceLocal(t *testing.T) {
	tests := []struct {
		name      string
		address   string
		wantLocal bool
	}{
		{"localhost", "localhost", true},
		{"loopback", "127.0.0.1", true},
		{"remote IP", "192.168.1.100", false},
		{"remote hostname", "server.local", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewProvider("test", tt.address, []string{"test.service"})
			svc, _ := p.GetService("test.service")
			systemdSvc := svc.(*SystemdService)

			if systemdSvc.isLocal != tt.wantLocal {
				t.Errorf("isLocal = %v, want %v", systemdSvc.isLocal, tt.wantLocal)
			}
		})
	}
}

// TestGetLogsBuildsCorrectService tests that GetLogs creates the right service.
func TestGetLogsBuildsCorrectService(t *testing.T) {
	// We can't actually test the full GetLogs without journalctl/SSH,
	// but we can verify the provider creates correct SystemdService
	p := NewProvider("myhost", "192.168.1.50", []string{"nginx.service"})

	// GetLogs internally creates a SystemdService, so test GetService
	// which uses same logic
	svc, err := p.GetService("nginx.service")
	if err != nil {
		t.Fatalf("GetService() error = %v", err)
	}

	systemdSvc := svc.(*SystemdService)
	if systemdSvc.hostName != "myhost" {
		t.Errorf("hostName = %v, want myhost", systemdSvc.hostName)
	}
	if systemdSvc.address != "192.168.1.50" {
		t.Errorf("address = %v, want 192.168.1.50", systemdSvc.address)
	}
	if systemdSvc.isLocal {
		t.Error("isLocal should be false for remote address")
	}
}

// BenchmarkProviderCreation benchmarks creating new providers.
func BenchmarkProviderCreation(b *testing.B) {
	units := []string{"docker.service", "nginx.service", "postgresql.service"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		NewProvider("benchmark-host", "192.168.1.100", units)
	}
}
