package sudoers

import (
	"strings"
	"testing"
)

func TestGenerate_NoServices(t *testing.T) {
	hosts := []HostServices{
		{Name: "host1", Address: "192.168.1.1", Services: nil},
		{Name: "host2", Address: "192.168.1.2", Services: []string{}},
	}

	result := Generate(hosts, "testuser")

	if !strings.Contains(result, "# No remote systemd services configured") {
		t.Error("Expected 'no remote services' message in output")
	}
}

func TestGenerate_EmptyHosts(t *testing.T) {
	result := Generate(nil, "testuser")

	if !strings.Contains(result, "# No remote systemd services configured") {
		t.Error("Expected 'no remote services' message in output")
	}
}

func TestGenerate_OnlyLocalServices(t *testing.T) {
	hosts := []HostServices{
		{Name: "nas", Address: "localhost", Services: []string{"docker.service"}},
		{Name: "local", Address: "127.0.0.1", Services: []string{"nginx.service"}},
	}

	result := Generate(hosts, "myuser")

	// Local services should not generate sudoers rules
	if !strings.Contains(result, "# No remote systemd services configured") {
		t.Error("Expected 'no remote services' message when only local hosts")
	}
	if strings.Contains(result, "myuser ALL=") {
		t.Error("Should not generate sudoers rules for local hosts")
	}
}

func TestGenerate_WithRemoteServices(t *testing.T) {
	hosts := []HostServices{
		{Name: "remote1", Address: "192.168.1.100", Services: []string{"docker.service", "ollama.service"}},
		{Name: "remote2", Address: "nas.local", Services: []string{"nginx.service"}},
	}

	result := Generate(hosts, "myuser")

	// Check header
	if !strings.Contains(result, "# Home Server Dashboard sudoers configuration") {
		t.Error("Expected header in output")
	}

	// Check note about remote only
	if !strings.Contains(result, "only needed for REMOTE hosts") {
		t.Error("Expected note about remote hosts")
	}

	// Check host comments include address
	if !strings.Contains(result, "# Host: remote1 (192.168.1.100)") {
		t.Error("Expected host comment with address for remote1")
	}
	if !strings.Contains(result, "# Host: remote2 (nas.local)") {
		t.Error("Expected host comment with address for remote2")
	}

	// Check service rules for docker.service
	if !strings.Contains(result, "myuser ALL=(ALL) NOPASSWD: /usr/bin/systemctl start docker.service") {
		t.Error("Expected start rule for docker.service")
	}
	if !strings.Contains(result, "myuser ALL=(ALL) NOPASSWD: /usr/bin/systemctl stop docker.service") {
		t.Error("Expected stop rule for docker.service")
	}
	if !strings.Contains(result, "myuser ALL=(ALL) NOPASSWD: /usr/bin/systemctl restart docker.service") {
		t.Error("Expected restart rule for docker.service")
	}

	// Check service rules for nginx.service
	if !strings.Contains(result, "myuser ALL=(ALL) NOPASSWD: /usr/bin/systemctl start nginx.service") {
		t.Error("Expected start rule for nginx.service")
	}
}

func TestGenerate_MixedHosts(t *testing.T) {
	hosts := []HostServices{
		{Name: "local", Address: "localhost", Services: []string{"local.service"}},
		{Name: "remote", Address: "192.168.1.100", Services: []string{"remote.service"}},
		{Name: "local2", Address: "127.0.0.1", Services: []string{"local2.service"}},
	}

	result := Generate(hosts, "admin")

	// Should have rules for remote host
	if !strings.Contains(result, "# Host: remote (192.168.1.100)") {
		t.Error("Expected host comment for remote")
	}
	if !strings.Contains(result, "admin ALL=(ALL) NOPASSWD: /usr/bin/systemctl start remote.service") {
		t.Error("Expected start rule for remote.service")
	}

	// Should NOT have rules for local hosts
	if strings.Contains(result, "# Host: local") {
		t.Error("Should not have host comment for local")
	}
	if strings.Contains(result, "local.service") {
		t.Error("Should not have rules for local.service")
	}
	if strings.Contains(result, "local2.service") {
		t.Error("Should not have rules for local2.service")
	}
}

func TestIsLocal(t *testing.T) {
	tests := []struct {
		address string
		isLocal bool
	}{
		{"localhost", true},
		{"127.0.0.1", true},
		{"192.168.1.1", false},
		{"nas.local", false},
		{"remote.example.com", false},
	}

	for _, tt := range tests {
		h := HostServices{Address: tt.address}
		if h.IsLocal() != tt.isLocal {
			t.Errorf("Address %q: expected IsLocal()=%v, got %v", tt.address, tt.isLocal, h.IsLocal())
		}
	}
}
