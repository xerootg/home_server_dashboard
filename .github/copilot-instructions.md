# Home Server Dashboard

REQUIRED: Update this document with any architectural or design information about the project.
REQUIRED: If you are working on a todo and find an issue, add it to the bottom of the todo list. the status should be "needs triage" and not checkmark or empty checkbox.

A lightweight Go web dashboard for monitoring Docker Compose services and systemd services across multiple hosts.

## Architecture

```
home_server_dashboard/
├── main.go              # Go HTTP server + Docker/systemd integration
├── services.json        # Configuration: hosts, systemd units to monitor
├── static/
│   ├── index.html       # Dashboard HTML structure (Bootstrap 5)
│   ├── style.css        # Custom dark theme styling
│   └── app.js           # Client-side logic, SSE handling, sorting/filtering
├── go.mod
└── go.sum
```

## Configuration (services.json)

Defines which hosts and services to monitor:

```json
{
  "hosts": [
    {
      "name": "nas",                    // Display name
      "address": "localhost",           // "localhost" uses D-Bus, others use SSH
      "systemd_services": ["docker.service"],
      "docker_compose_roots": ["/home/xero/nas/"]
    },
    {
      // another host
    }
  ]
}
```

## Backend (main.go)

**Server:** Standard library `net/http` on port 9001

**Endpoints:**
- `GET /` — Serves `static/index.html`
- `GET /static/*` — Static file server for CSS/JS
- `GET /api/services` — Returns JSON array of all services (Docker + systemd)
- `GET /api/logs?container=<name>` — SSE stream of Docker container logs

**Service Types:**

| Source | Method | Library/Tool |
|--------|--------|--------------|
| Docker | Docker API via socket | `github.com/docker/docker/client` |
| Systemd (local) | D-Bus | `github.com/coreos/go-systemd/v22/dbus` |
| Systemd (remote) | SSH + systemctl | `os/exec` with `ssh` command |

**ServiceInfo struct:**
```go
type ServiceInfo struct {
    Name          string `json:"name"`           // Service/unit name
    Project       string `json:"project"`        // Docker project or "systemd"
    ContainerName string `json:"container_name"` // Container name or unit name
    State         string `json:"state"`          // "running" or "stopped"
    Status        string `json:"status"`         // Human-readable status
    Image         string `json:"image"`          // Docker image or "-"
    Source        string `json:"source"`         // "docker" or "systemd"
    Host          string `json:"host"`           // Host name from config
}
```

**Docker Integration:**
- Queries containers via Docker socket on localhost
- Filters by `com.docker.compose.project` and `com.docker.compose.service` labels
- Streams logs using `ContainerLogs()` with multiplexed stdout/stderr

**Systemd Integration:**
- Local: Uses D-Bus via `dbus.NewSystemConnectionContext()` and `ListUnitsContext()`
- Remote: Uses SSH to run `systemctl show <unit> --property=ActiveState,SubState,LoadState`
- Filters units by exact name match from config

## Frontend

**Tech Stack:** Bootstrap 5.3 + Bootstrap Icons + vanilla JavaScript

**Data Flow:**
1. Page loads → `loadServices()` fetches `/api/services`
2. JavaScript renders table rows with source icons, host badges, status badges
3. Click row → `toggleLogs()` inserts inline logs row below
4. For Docker: SSE connection streams real-time logs
5. For Systemd: Shows journalctl command (logs not streamed)
6. Click again or ✕ → closes logs and disconnects SSE

**UI Features:**
- Clickable stat cards to filter by status (running/stopped)
- Sortable columns (click header to sort, click again to reverse)
- Source icons: gear (systemd) vs box (Docker)
- Host badges showing which host the service runs on

**Status Colors:**
- 🟢 Green (`badge-running`): Running/active
- 🟠 Orange (`badge-unhealthy`): Running but unhealthy
- 🔴 Red (`badge-stopped`): Stopped/exited/inactive

## Key Design Decisions

- **No frameworks:** Pure Go stdlib + vanilla JS for simplicity
- **Docker API over YAML parsing:** Query running state directly from Docker
- **D-Bus for local systemd:** Direct communication, no parsing needed
- **SSH for remote systemd:** Simple, uses existing SSH keys, no agent needed on remote
- **SSE for logs:** Lightweight real-time streaming without WebSocket complexity
- **Inline log expansion:** Logs appear directly below the clicked service row
- **Bootstrap for UI:** Consistent dark theme, responsive, minimal custom CSS
- **Unified API:** Single `/api/services` endpoint returns all service types
