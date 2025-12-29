# Home Server Dashboard

REQUIRED: Update this document with any architectural or design information about the project.
REQUIRED: If you are working on a todo and find an issue, add it to the bottom of the todo list. the status should be "needs triage" and not checkmark or empty checkbox.
REQUIRED: All tests must pass, including integration tests, before any item in todo can be marked done. if there are no tests for the feature, add them, and they must pass.

A lightweight Go web dashboard for monitoring Docker Compose services and systemd services across multiple hosts.

## Architecture

```
home_server_dashboard/
├── main.go                        # HTTP server, routes, and request handlers
├── main_test.go                   # HTTP handler tests
├── services.json                  # Configuration: hosts, systemd units to monitor
├── config/
│   ├── config.go                  # Shared configuration loading and types
│   └── config_test.go             # Config loading and helper tests
├── services/
│   ├── service.go                 # Common Service interface and ServiceInfo type
│   ├── service_test.go            # ServiceInfo serialization tests
│   ├── docker/
│   │   ├── docker.go              # Docker provider and service implementation
│   │   ├── docker_test.go         # Unit tests (mocked, no Docker required)
│   │   └── docker_integration_test.go  # Integration tests (requires Docker)
│   └── systemd/
│       ├── systemd.go             # Systemd provider and service implementation
│       ├── systemd_test.go        # Unit tests (mocked, no D-Bus required)
│       └── systemd_integration_test.go # Integration tests (requires systemd)
├── static/
│   ├── index.html                 # Dashboard HTML structure (Bootstrap 5)
│   ├── style.css                  # Custom dark theme styling
│   └── app.js                     # Client-side logic, SSE handling, sorting/filtering
├── go.mod
└── go.sum
```

## Package Structure

### `config` Package
- **Purpose:** Shared configuration loading from `services.json`
- **Key Types:**
  - `HostConfig` — Single host configuration with helper methods like `IsLocal()`
  - `Config` — Complete configuration with helper methods like `GetLocalHostName()`, `GetHostByName()`
- **Functions:** `Load()`, `Get()`, `Default()`

### `services` Package
- **Purpose:** Defines common interface and types for all service providers
- **Key Types:**
  - `ServiceInfo` — Status information struct (JSON serializable)
  - `Service` — Interface for individual service control (GetInfo, GetLogs, Start, Stop, Restart)
  - `Provider` — Interface for service discovery (GetServices, GetService, GetLogs)

### `services/docker` Package
- **Purpose:** Docker container management via Docker API
- **Key Types:**
  - `Provider` — Implements `services.Provider` for Docker containers
  - `DockerService` — Implements `services.Service` for individual containers
- **Features:**
  - Connects via Docker socket
  - Filters by Docker Compose labels
  - Streams logs with 8-byte header demultiplexing

### `services/systemd` Package
- **Purpose:** Systemd unit management via D-Bus (local) or SSH (remote)
- **Key Types:**
  - `Provider` — Implements `services.Provider` for systemd units
  - `SystemdService` — Implements `services.Service` for individual units
- **Features:**
  - Auto-detects local vs remote based on address
  - Uses D-Bus for localhost, SSH for remote hosts
  - Streams logs via journalctl

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
- `GET /api/logs/systemd?unit=<name>&host=<host>` — SSE stream of systemd unit logs

**Service Types:**

| Source | Method | Library/Tool |
|--------|--------|--------------|
| Docker | Docker API via socket | `github.com/docker/docker/client` |
| Systemd (local) | D-Bus | `github.com/coreos/go-systemd/v22/dbus` |
| Systemd (remote) | SSH + systemctl | `os/exec` with `ssh` command |

**ServiceInfo struct (in `services/service.go`):**
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

**Service Interface (in `services/service.go`):**
```go
type Service interface {
    GetInfo(ctx context.Context) (ServiceInfo, error)
    GetLogs(ctx context.Context, tailLines int, follow bool) (io.ReadCloser, error)
    Start(ctx context.Context) error
    Stop(ctx context.Context) error
    Restart(ctx context.Context) error
    GetName() string
    GetHost() string
    GetSource() string
}
```

**Docker Integration (`services/docker/docker.go`):**
- Queries containers via Docker socket on localhost
- Filters by `com.docker.compose.project` and `com.docker.compose.service` labels
- Streams logs using `ContainerLogs()` with multiplexed stdout/stderr

**Systemd Integration (`services/systemd/systemd.go`):**
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
5. For Systemd: SSE connection streams journalctl logs
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

## Testing

The project uses Go's built-in testing framework with two categories of tests:

### Unit Tests (run without external dependencies)
```bash
go test ./...
```

Unit tests mock system dependencies and can run on any machine:
- **config/** — Config loading, parsing, helper methods
- **services/** — ServiceInfo JSON serialization
- **services/docker/** — Log reader header stripping, provider methods
- **services/systemd/** — Provider creation, systemctl output parsing
- **main.go** — HTTP handler validation, SSE headers, error responses

### Integration Tests (require Docker/systemd)
```bash
go test -tags=integration ./...
```

Integration tests use the `//go:build integration` build tag and test real system interactions:
- **services/docker/** — Real Docker API calls, container listing, log streaming
- **services/systemd/** — Real D-Bus connections, journalctl log streaming

| Package | Unit Tests | Integration Tests |
|---------|------------|-------------------|
| config | ✅ | — |
| services | ✅ | — |
| services/docker | ✅ | ✅ |
| services/systemd | ✅ | ✅ |
| main | ✅ | — |
