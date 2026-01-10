// Package sudoers generates sudoers configuration for systemd service control.
package sudoers

import (
	"fmt"
	"strings"
)

// HostServices represents a host and its systemd services.
type HostServices struct {
	Name     string
	Address  string
	Services []string
}

// IsLocal returns true if the host is localhost.
func (h HostServices) IsLocal() bool {
	return h.Address == "localhost" || h.Address == "127.0.0.1"
}

// Generate creates a sudoers configuration string for the given hosts and username.
// The output can be installed to /etc/sudoers.d/home-server-dashboard.
// NOTE: For local hosts, use polkit rules instead (see polkit package).
// Sudoers is only needed for remote hosts where SSH + sudo is used.
func Generate(hosts []HostServices, username string) string {
	var b strings.Builder

	b.WriteString("# Home Server Dashboard sudoers configuration\n")
	b.WriteString("# Generated from services.json\n")
	b.WriteString("#\n")
	b.WriteString("# NOTE: This is only needed for REMOTE hosts.\n")
	b.WriteString("# Local hosts use D-Bus + polkit for authorization.\n")
	b.WriteString("#\n")
	b.WriteString("# For remote hosts, copy the relevant lines to each remote machine\n")
	b.WriteString("# and install with:\n")
	b.WriteString("#   sudo visudo -f /etc/sudoers.d/home-server-dashboard\n")
	b.WriteString("\n")

	// Filter to only remote hosts with services
	hasRemoteServices := false
	for _, host := range hosts {
		if !host.IsLocal() && len(host.Services) > 0 {
			hasRemoteServices = true
			break
		}
	}

	if !hasRemoteServices {
		b.WriteString("# No remote systemd services configured in services.json\n")
		return b.String()
	}

	// Print configuration grouped by host (remote only)
	for _, host := range hosts {
		if host.IsLocal() || len(host.Services) == 0 {
			continue
		}
		b.WriteString(fmt.Sprintf("# Host: %s (%s)\n", host.Name, host.Address))
		for _, svc := range host.Services {
			b.WriteString(fmt.Sprintf("%s ALL=(ALL) NOPASSWD: /usr/bin/systemctl start %s\n", username, svc))
			b.WriteString(fmt.Sprintf("%s ALL=(ALL) NOPASSWD: /usr/bin/systemctl stop %s\n", username, svc))
			b.WriteString(fmt.Sprintf("%s ALL=(ALL) NOPASSWD: /usr/bin/systemctl restart %s\n", username, svc))
		}
		b.WriteString("\n")
	}

	return b.String()
}
