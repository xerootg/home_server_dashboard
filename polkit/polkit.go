// Package polkit generates polkit rules for systemd service control via D-Bus.
package polkit

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

// GeneratePolkitRules creates a polkit rules file for D-Bus systemd control.
// This allows the specified user to start/stop/restart the configured units
// without requiring sudo (which fails with NoNewPrivileges=true).
// The output should be installed to /etc/polkit-1/rules.d/50-home-server-dashboard.rules
func GeneratePolkitRules(hosts []HostServices, username string) string {
	var b strings.Builder

	b.WriteString("// Home Server Dashboard polkit rules\n")
	b.WriteString("// Generated from services.json\n")
	b.WriteString("//\n")
	b.WriteString("// Install with:\n")
	b.WriteString("//   sudo cp this-file /etc/polkit-1/rules.d/50-home-server-dashboard.rules\n")
	b.WriteString("//\n")
	b.WriteString("// This allows the dashboard to control systemd units via D-Bus\n")
	b.WriteString("// without requiring sudo (which fails with NoNewPrivileges=true).\n")
	b.WriteString("\n")

	// Collect all local services
	var localServices []string
	for _, host := range hosts {
		if host.IsLocal() && len(host.Services) > 0 {
			localServices = append(localServices, host.Services...)
		}
	}

	if len(localServices) == 0 {
		b.WriteString("// No local systemd services configured in services.json\n")
		return b.String()
	}

	b.WriteString("polkit.addRule(function(action, subject) {\n")
	b.WriteString(fmt.Sprintf("    if (subject.user !== \"%s\") {\n", username))
	b.WriteString("        return polkit.Result.NOT_HANDLED;\n")
	b.WriteString("    }\n")
	b.WriteString("\n")
	b.WriteString("    // Allow managing systemd units\n")
	b.WriteString("    if (action.id === \"org.freedesktop.systemd1.manage-units\") {\n")
	b.WriteString("        var unit = action.lookup(\"unit\");\n")
	b.WriteString("        var allowedUnits = [\n")

	for i, svc := range localServices {
		comma := ","
		if i == len(localServices)-1 {
			comma = ""
		}
		b.WriteString(fmt.Sprintf("            \"%s\"%s\n", svc, comma))
	}

	b.WriteString("        ];\n")
	b.WriteString("\n")
	b.WriteString("        if (allowedUnits.indexOf(unit) >= 0) {\n")
	b.WriteString("            return polkit.Result.YES;\n")
	b.WriteString("        }\n")
	b.WriteString("    }\n")
	b.WriteString("\n")
	b.WriteString("    return polkit.Result.NOT_HANDLED;\n")
	b.WriteString("});\n")

	return b.String()
}
