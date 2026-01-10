package polkit

import (
	"strings"
	"testing"
)

func TestGeneratePolkitRules_NoLocalServices(t *testing.T) {
	hosts := []HostServices{
		{Name: "remote", Address: "192.168.1.100", Services: []string{"nginx.service"}},
	}

	result := GeneratePolkitRules(hosts, "testuser")

	if !strings.Contains(result, "// No local systemd services configured") {
		t.Error("Expected 'no local services' message in output")
	}
	if strings.Contains(result, "polkit.addRule") {
		t.Error("Should not have polkit rule when no local services")
	}
}

func TestGeneratePolkitRules_EmptyHosts(t *testing.T) {
	result := GeneratePolkitRules(nil, "testuser")

	if !strings.Contains(result, "// No local systemd services configured") {
		t.Error("Expected 'no local services' message in output")
	}
}

func TestGeneratePolkitRules_WithLocalServices(t *testing.T) {
	hosts := []HostServices{
		{Name: "nas", Address: "localhost", Services: []string{"docker.service", "ollama.service"}},
	}

	result := GeneratePolkitRules(hosts, "myuser")

	// Check header
	if !strings.Contains(result, "// Home Server Dashboard polkit rules") {
		t.Error("Expected header in output")
	}

	// Check user check
	if !strings.Contains(result, `subject.user !== "myuser"`) {
		t.Error("Expected user check for myuser")
	}

	// Check action check
	if !strings.Contains(result, `action.id === "org.freedesktop.systemd1.manage-units"`) {
		t.Error("Expected action check for manage-units")
	}

	// Check service list
	if !strings.Contains(result, `"docker.service"`) {
		t.Error("Expected docker.service in allowed units")
	}
	if !strings.Contains(result, `"ollama.service"`) {
		t.Error("Expected ollama.service in allowed units")
	}

	// Check return YES
	if !strings.Contains(result, "polkit.Result.YES") {
		t.Error("Expected YES result for allowed units")
	}
}

func TestGeneratePolkitRules_MixedHosts(t *testing.T) {
	hosts := []HostServices{
		{Name: "local", Address: "localhost", Services: []string{"local.service"}},
		{Name: "remote", Address: "192.168.1.100", Services: []string{"remote.service"}},
		{Name: "local2", Address: "127.0.0.1", Services: []string{"local2.service"}},
	}

	result := GeneratePolkitRules(hosts, "admin")

	// Should have local services
	if !strings.Contains(result, `"local.service"`) {
		t.Error("Expected local.service in allowed units")
	}
	if !strings.Contains(result, `"local2.service"`) {
		t.Error("Expected local2.service in allowed units")
	}

	// Should NOT have remote services
	if strings.Contains(result, `"remote.service"`) {
		t.Error("Should not have remote.service in allowed units")
	}
}

func TestGeneratePolkitRules_LocalHostVariants(t *testing.T) {
	tests := []struct {
		address string
		isLocal bool
	}{
		{"localhost", true},
		{"127.0.0.1", true},
		{"192.168.1.1", false},
		{"nas.local", false},
	}

	for _, tt := range tests {
		h := HostServices{Address: tt.address}
		if h.IsLocal() != tt.isLocal {
			t.Errorf("Address %q: expected IsLocal()=%v, got %v", tt.address, tt.isLocal, h.IsLocal())
		}
	}
}
