package sudoers

import (
	"strings"
	"testing"
)

func TestGenerate_NoServices(t *testing.T) {
	hosts := []HostServices{
		{Name: "host1", Services: nil},
		{Name: "host2", Services: []string{}},
	}

	result := Generate(hosts, "testuser")

	if !strings.Contains(result, "# No systemd services configured") {
		t.Error("Expected 'no services' message in output")
	}
}

func TestGenerate_EmptyHosts(t *testing.T) {
	result := Generate(nil, "testuser")

	if !strings.Contains(result, "# No systemd services configured") {
		t.Error("Expected 'no services' message in output")
	}
}

func TestGenerate_WithServices(t *testing.T) {
	hosts := []HostServices{
		{Name: "nas", Services: []string{"docker.service", "ollama.service"}},
		{Name: "remote", Services: []string{"nginx.service"}},
	}

	result := Generate(hosts, "myuser")

	// Check header
	if !strings.Contains(result, "# Home Server Dashboard sudoers configuration") {
		t.Error("Expected header in output")
	}

	// Check host comments
	if !strings.Contains(result, "# Host: nas") {
		t.Error("Expected host comment for nas")
	}
	if !strings.Contains(result, "# Host: remote") {
		t.Error("Expected host comment for remote")
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
		{Name: "with-services", Services: []string{"foo.service"}},
		{Name: "no-services", Services: nil},
		{Name: "also-services", Services: []string{"bar.service"}},
	}

	result := Generate(hosts, "admin")

	// Should have rules for hosts with services
	if !strings.Contains(result, "# Host: with-services") {
		t.Error("Expected host comment for with-services")
	}
	if !strings.Contains(result, "# Host: also-services") {
		t.Error("Expected host comment for also-services")
	}

	// Should NOT have comment for host without services
	if strings.Contains(result, "# Host: no-services") {
		t.Error("Should not have host comment for no-services")
	}

	// Should have the service rules
	if !strings.Contains(result, "admin ALL=(ALL) NOPASSWD: /usr/bin/systemctl start foo.service") {
		t.Error("Expected start rule for foo.service")
	}
	if !strings.Contains(result, "admin ALL=(ALL) NOPASSWD: /usr/bin/systemctl start bar.service") {
		t.Error("Expected start rule for bar.service")
	}
}
