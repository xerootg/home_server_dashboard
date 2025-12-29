// Package sudoers generates sudoers configuration for systemd service control.
package sudoers

import (
	"fmt"
	"strings"
)

// HostServices represents a host and its systemd services.
type HostServices struct {
	Name     string
	Services []string
}

// Generate creates a sudoers configuration string for the given hosts and username.
// The output can be installed to /etc/sudoers.d/home-server-dashboard.
func Generate(hosts []HostServices, username string) string {
	var b strings.Builder

	b.WriteString("# Home Server Dashboard sudoers configuration\n")
	b.WriteString("# Generated from services.json\n")
	b.WriteString("#\n")
	b.WriteString("# Install with:\n")
	b.WriteString("#   sudo visudo -f /etc/sudoers.d/home-server-dashboard\n")
	b.WriteString("# Then paste this content.\n")
	b.WriteString("#\n")
	b.WriteString("# For remote hosts, copy the relevant lines to each remote machine.\n")
	b.WriteString("\n")

	// Filter to only hosts with services
	hasServices := false
	for _, host := range hosts {
		if len(host.Services) > 0 {
			hasServices = true
			break
		}
	}

	if !hasServices {
		b.WriteString("# No systemd services configured in services.json\n")
		return b.String()
	}

	// Print configuration grouped by host
	for _, host := range hosts {
		if len(host.Services) == 0 {
			continue
		}
		b.WriteString(fmt.Sprintf("# Host: %s\n", host.Name))
		for _, svc := range host.Services {
			b.WriteString(fmt.Sprintf("%s ALL=(ALL) NOPASSWD: /usr/bin/systemctl start %s\n", username, svc))
			b.WriteString(fmt.Sprintf("%s ALL=(ALL) NOPASSWD: /usr/bin/systemctl stop %s\n", username, svc))
			b.WriteString(fmt.Sprintf("%s ALL=(ALL) NOPASSWD: /usr/bin/systemctl restart %s\n", username, svc))
		}
		b.WriteString("\n")
	}

	return b.String()
}
