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
			if len(p.entries) != len(tt.unitNames) {
				t.Errorf("entries length = %v, want %v", len(p.entries), len(tt.unitNames))
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
		name               string
		output             string
		wantActive         string
		wantSub            string
		wantLoad           string
		wantState          string
		wantStatusContains string
		wantDescription    string
	}{
		{
			name: "running service",
			output: `ActiveState=active
SubState=running
LoadState=loaded
Description=Docker Application Container Engine`,
			wantActive:         "active",
			wantSub:            "running",
			wantLoad:           "loaded",
			wantState:          "running",
			wantStatusContains: "active",
			wantDescription:    "Docker Application Container Engine",
		},
		{
			name: "stopped service",
			output: `ActiveState=inactive
SubState=dead
LoadState=loaded
Description=OpenSSH Server`,
			wantActive:         "inactive",
			wantSub:            "dead",
			wantLoad:           "loaded",
			wantState:          "stopped",
			wantStatusContains: "inactive",
			wantDescription:    "OpenSSH Server",
		},
		{
			name: "failed service",
			output: `ActiveState=failed
SubState=failed
LoadState=loaded
Description=My Custom Service`,
			wantActive:         "failed",
			wantSub:            "failed",
			wantLoad:           "loaded",
			wantState:          "stopped",
			wantStatusContains: "failed",
			wantDescription:    "My Custom Service",
		},
		{
			name: "not found service",
			output: `ActiveState=inactive
SubState=dead
LoadState=not-found
Description=nginx.service`,
			wantActive:         "inactive",
			wantSub:            "dead",
			wantLoad:           "not-found",
			wantState:          "stopped",
			wantStatusContains: "not found",
			wantDescription:    "nginx.service",
		},
		{
			name: "service with no description",
			output: `ActiveState=active
SubState=running
LoadState=loaded`,
			wantActive:         "active",
			wantSub:            "running",
			wantLoad:           "loaded",
			wantState:          "running",
			wantStatusContains: "active",
			wantDescription:    "",
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
			description := props["Description"]

			if activeState != tt.wantActive {
				t.Errorf("ActiveState = %v, want %v", activeState, tt.wantActive)
			}
			if subState != tt.wantSub {
				t.Errorf("SubState = %v, want %v", subState, tt.wantSub)
			}
			if loadState != tt.wantLoad {
				t.Errorf("LoadState = %v, want %v", loadState, tt.wantLoad)
			}
			if description != tt.wantDescription {
				t.Errorf("Description = %v, want %v", description, tt.wantDescription)
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

	if len(p.entries) != 0 {
		t.Errorf("entries should be empty, got %v", p.entries)
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

// TestNewProviderWithEntries_ReadOnly tests that read-only entries are preserved.
func TestNewProviderWithEntries_ReadOnly(t *testing.T) {
	entries := []ServiceEntry{
		{Name: "docker.service", ReadOnly: false},
		{Name: "nas-dashboard.service", ReadOnly: true},
		{Name: "nginx.service", ReadOnly: false},
	}

	p := NewProviderWithEntries("testhost", "localhost", entries)

	if len(p.entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(p.entries))
	}

	// Verify read-only flags are preserved
	for _, entry := range p.entries {
		switch entry.Name {
		case "docker.service":
			if entry.ReadOnly {
				t.Error("docker.service should not be read-only")
			}
		case "nas-dashboard.service":
			if !entry.ReadOnly {
				t.Error("nas-dashboard.service should be read-only")
			}
		case "nginx.service":
			if entry.ReadOnly {
				t.Error("nginx.service should not be read-only")
			}
		}
	}
}

// TestNewProvider_AlwaysNonReadOnly tests that NewProvider creates non-read-only entries.
func TestNewProvider_AlwaysNonReadOnly(t *testing.T) {
	// Even if the unit name contains ":ro", NewProvider should not parse it
	// (that's the config package's job)
	p := NewProvider("testhost", "localhost", []string{"docker.service", "test.service"})

	for _, entry := range p.entries {
		if entry.ReadOnly {
			t.Errorf("entry %s should not be read-only when using NewProvider", entry.Name)
		}
	}
}

// TestGetService_ReadOnlyFlagPreserved tests that read-only flag is passed to the provider.
func TestGetService_ReadOnlyFlagPreserved(t *testing.T) {
	entries := []ServiceEntry{
		{Name: "readonly.service", ReadOnly: true},
		{Name: "normal.service", ReadOnly: false},
	}

	p := NewProviderWithEntries("testhost", "192.168.1.100", entries)

	// Verify the entries are stored correctly in the provider
	foundReadonly := false
	foundNormal := false
	for _, entry := range p.entries {
		if entry.Name == "readonly.service" {
			foundReadonly = true
			if !entry.ReadOnly {
				t.Error("readonly.service should have ReadOnly=true in provider entries")
			}
		}
		if entry.Name == "normal.service" {
			foundNormal = true
			if entry.ReadOnly {
				t.Error("normal.service should have ReadOnly=false in provider entries")
			}
		}
	}

	if !foundReadonly {
		t.Error("readonly.service not found in provider entries")
	}
	if !foundNormal {
		t.Error("normal.service not found in provider entries")
	}
}

// TestNewProviderWithEntries_UserServices tests that user services are handled correctly.
func TestNewProviderWithEntries_UserServices(t *testing.T) {
	entries := []ServiceEntry{
		{Name: "docker.service", User: "", ReadOnly: false},
		{Name: "zunesync.service", User: "xero", ReadOnly: false},
		{Name: "backup.timer", User: "alice", ReadOnly: true},
	}

	p := NewProviderWithEntries("testhost", "localhost", entries)

	if len(p.entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(p.entries))
	}

	// Verify user and read-only flags are preserved
	tests := []struct {
		name     string
		user     string
		readOnly bool
	}{
		{"docker.service", "", false},
		{"zunesync.service", "xero", false},
		{"backup.timer", "alice", true},
	}

	for i, tt := range tests {
		entry := p.entries[i]
		if entry.Name != tt.name {
			t.Errorf("entries[%d].Name = %q, want %q", i, entry.Name, tt.name)
		}
		if entry.User != tt.user {
			t.Errorf("entries[%d].User = %q, want %q", i, entry.User, tt.user)
		}
		if entry.ReadOnly != tt.readOnly {
			t.Errorf("entries[%d].ReadOnly = %v, want %v", i, entry.ReadOnly, tt.readOnly)
		}
	}
}

// TestGetService_UserFieldPreserved tests that user field is passed to SystemdService.
func TestGetService_UserFieldPreserved(t *testing.T) {
	entries := []ServiceEntry{
		{Name: "system.service", User: "", ReadOnly: false},
		{Name: "user.service", User: "testuser", ReadOnly: false},
	}

	p := NewProviderWithEntries("testhost", "192.168.1.100", entries)

	t.Run("system service has no user", func(t *testing.T) {
		svc, err := p.GetService("system.service")
		if err != nil {
			t.Fatalf("GetService failed: %v", err)
		}
		systemdSvc := svc.(*SystemdService)
		if systemdSvc.user != "" {
			t.Errorf("user = %q, want empty string", systemdSvc.user)
		}
	})

	t.Run("user service has user field set", func(t *testing.T) {
		svc, err := p.GetService("user.service")
		if err != nil {
			t.Fatalf("GetService failed: %v", err)
		}
		systemdSvc := svc.(*SystemdService)
		if systemdSvc.user != "testuser" {
			t.Errorf("user = %q, want %q", systemdSvc.user, "testuser")
		}
	})
}

// TestFindEntry tests the findEntry helper method.
func TestFindEntry(t *testing.T) {
	entries := []ServiceEntry{
		{Name: "docker.service", User: "", ReadOnly: false},
		{Name: "zunesync.service", User: "xero", ReadOnly: true},
	}

	p := NewProviderWithEntries("testhost", "localhost", entries)

	t.Run("find existing system entry", func(t *testing.T) {
		entry, found := p.findEntry("docker.service")
		if !found {
			t.Error("expected to find docker.service")
		}
		if entry.Name != "docker.service" {
			t.Errorf("Name = %q, want %q", entry.Name, "docker.service")
		}
		if entry.User != "" {
			t.Errorf("User = %q, want empty", entry.User)
		}
	})

	t.Run("find existing user entry", func(t *testing.T) {
		entry, found := p.findEntry("zunesync.service")
		if !found {
			t.Error("expected to find zunesync.service")
		}
		if entry.Name != "zunesync.service" {
			t.Errorf("Name = %q, want %q", entry.Name, "zunesync.service")
		}
		if entry.User != "xero" {
			t.Errorf("User = %q, want %q", entry.User, "xero")
		}
		if !entry.ReadOnly {
			t.Error("ReadOnly = false, want true")
		}
	})

	t.Run("entry not found", func(t *testing.T) {
		_, found := p.findEntry("nonexistent.service")
		if found {
			t.Error("expected not to find nonexistent.service")
		}
	})
}
