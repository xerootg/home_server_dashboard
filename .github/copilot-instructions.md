# Home Server Dashboard

REQUIRED: Update this document with any architectural or design information about the project.
REQUIRED: If you are working on a todo and find an issue, add it to the bottom of the todo list. the status should be "needs triage" and not checkmark or empty checkbox.
REQUIRED: All tests must pass, including integration tests, before any item in todo can be marked done. if there are no tests for the feature, add them, and they must pass. The ONLY exception to this rule is for html/css/js files that are purely frontend and have no backend component.

A lightweight Go web dashboard for monitoring Docker Compose services and systemd services across multiple hosts.

## Architecture

```
home_server_dashboard/
├── main.go                        # Application bootstrap (config loading, server start)
├── main_test.go                   # Bootstrap integration tests
├── services.json                  # Configuration: hosts, systemd units to monitor
├── auth/
│   ├── auth.go                    # OIDC authentication provider, session management, middleware
│   └── auth_test.go               # Auth unit tests (session store, claim checking)
├── handlers/
│   ├── handlers.go                # HTTP request handlers (services, logs, index)
│   └── handlers_test.go           # Handler unit tests
├── server/
│   ├── server.go                  # HTTP server setup, routing, configuration
│   └── server_test.go             # Server configuration and routing tests
├── config/
│   ├── config.go                  # Shared configuration loading and types
│   └── config_test.go             # Config loading and helper tests
├── query/
│   ├── query.go                   # Bang & Pipe expression compiler (types, Compile)
│   ├── lexer.go                   # Tokenizer for expression parsing
│   ├── parser.go                  # Recursive descent parser for expressions
│   └── query_test.go              # Comprehensive parser tests
├── polkit/
│   ├── polkit.go                  # Polkit rules generator for local systemd control
│   └── polkit_test.go             # Polkit generator tests
├── sudoers/
│   ├── sudoers.go                 # Sudoers config generator for remote systemd control
│   └── sudoers_test.go            # Sudoers generator tests
├── services/
│   ├── service.go                 # Common Service interface and ServiceInfo type
│   ├── service_test.go            # ServiceInfo serialization tests
│   ├── docker/
│   │   ├── docker.go              # Docker provider and service implementation
│   │   ├── docker_test.go         # Unit tests (mocked, no Docker required)
│   │   └── docker_integration_test.go  # Integration tests (requires Docker)
│   ├── systemd/
│   │   ├── systemd.go             # Systemd provider and service implementation
│   │   ├── systemd_test.go        # Unit tests (mocked, no D-Bus required)
│   │   └── systemd_integration_test.go # Integration tests (requires systemd)
│   └── traefik/
│       ├── traefik.go             # Traefik API client for hostname lookup
│       ├── traefik_test.go        # Unit tests for Traefik client
│       ├── service.go             # Traefik service provider and service implementation
│       ├── service_test.go        # Unit tests for Traefik service provider
│       ├── matcher.go             # MatcherLookupService for hostname extraction with state tracking
│       └── matcher_test.go        # Unit tests for matcher lookup service
├── static/
│   ├── index.html                 # Dashboard HTML structure (Bootstrap 5)
│   ├── style.css                  # Custom dark theme styling
│   └── app.js                     # Client-side logic, SSE handling, sorting/filtering
├── tests/
│   └── js/
│       └── search_test.js         # JavaScript unit tests for search functionality
├── go.mod
└── go.sum
```

## Package Structure

### `main` Package
- **Purpose:** Application entry point and bootstrap
- **Responsibilities:**
  - Load configuration from `services.json`
  - Initialize OIDC authentication provider (if configured)
  - Create and start HTTP server
- **Files:** `main.go`, `main_test.go`

### `auth` Package
- **Purpose:** OIDC authentication and session management
- **Key Types:**
  - `Provider` — OIDC authentication provider with session store and group configurations
  - `User` — Authenticated user information (ID, Email, Name, Groups, IsAdmin, HasGlobalAccess, AllowedServices)
  - `Session` — User session with expiry
  - `SessionStore` — Thread-safe in-memory session storage
  - `StateStore` — OIDC state token management
- **Key Functions:**
  - `NewProvider(ctx, cfg, localCfg)` — Creates OIDC provider from config
  - `Middleware(next)` — HTTP middleware requiring authentication
  - `LoginHandler` — Initiates OIDC login flow
  - `CallbackHandler` — Handles OIDC callback, validates tokens
  - `LogoutHandler` — Clears session and redirects to login
  - `StatusHandler` — Returns JSON with auth status
  - `GetUserFromContext(ctx)` — Retrieves authenticated user from request context
- **User Methods:**
  - `CanAccessService(host, serviceName)` — Checks if user can access a specific service
  - `HasAnyAccess()` — Returns true if user has global access or any allowed services
- **Features:**
  - OIDC discovery via `.well-known/openid-configuration`
  - Secure session cookies (HttpOnly, SameSite)
  - Admin claim checking (configurable, defaults to "groups" containing "admin")
  - **Group-based access control:** OIDC groups can grant access to specific services on specific hosts
  - **Additive permissions:** Users in multiple groups get combined permissions from all groups
  - Automatic session cleanup
  - **Local access detection:** If Host header differs from `service_url`, uses Basic Auth against `local.admins` list (with global access)
- **Files:** `auth/auth.go`, `auth/auth_test.go`

### `handlers` Package
- **Purpose:** HTTP request handlers for all API endpoints
- **Key Functions:**
  - `ServicesHandler` — Returns JSON array of services (filtered by user permissions)
  - `DockerLogsHandler` — SSE stream for Docker container logs (checks permissions)
  - `SystemdLogsHandler` — SSE stream for systemd unit logs (checks permissions)
  - `IndexHandler` — Serves the main dashboard page
  - `ServiceActionHandler` — Handles start/stop/restart actions with SSE status updates (checks permissions)
- **Key Types:**
  - `ServiceActionRequest` — Request body for service control actions
- **Internal:** `getAllServices()` aggregates services from all providers, `filterServicesForUser()` applies permission filtering

### `server` Package
- **Purpose:** HTTP server configuration and routing
- **Key Types:**
  - `Config` — Server configuration (port, static dir, config path, auth provider)
  - `Server` — HTTP server with routing setup
- **Functions:** `New()`, `DefaultConfig()`, `ListenAndServe()`, `Handler()`

### `config` Package
- **Purpose:** Shared configuration loading from `services.json`
- **Key Types:**
  - `HostConfig` — Single host configuration with helper methods like `IsLocal()`, `GetPrivateIP()`
  - `OIDCConfig` — OIDC authentication settings (ServiceURL, Callback, ConfigURL, ClientID, ClientSecret, GroupsClaim, AdminGroup, Groups)
  - `OIDCGroupConfig` — Group-based access control configuration (Services map)
  - `LocalConfig` — Local authentication settings (Admins)
  - `Config` — Complete configuration with helper methods like `GetLocalHostName()`, `GetHostByName()`, `IsOIDCEnabled()`
- **Functions:** `Load()`, `Get()`, `Default()`, `isPrivateIP()`

### `query` Package
- **Purpose:** Compiles "Bang & Pipe" search expressions into ASTs for client-side evaluation
- **Key Types:**
  - `NodeType` — Enum: `pattern`, `or`, `and`, `not`
  - `Node` — AST node with Type, Pattern, Regex, Children, Child fields
  - `CompileError` — Parse error with Message, Position, Length
  - `CompileResult` — Result with Valid, AST, Error fields
- **Key Functions:**
  - `Compile(expr string)` — Parses expression and returns AST or error
  - `tokenize()` (internal) — Lexer for tokenizing input
- **Grammar:**
  - Operators: `|` (OR), `&` (AND), `!` (NOT), `()` (grouping)
  - Literals: `"quoted string"` or unquoted terms
  - Precedence: NOT > AND > OR
- **Files:** `query.go`, `lexer.go`, `parser.go`, `query_test.go`

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
  - Extracts exposed ports bound to non-localhost addresses (0.0.0.0 or specific IPs)

### `services/systemd` Package
- **Purpose:** Systemd unit management via D-Bus (local) or SSH (remote)
- **Key Types:**
  - `Provider` — Implements `services.Provider` for systemd units
  - `SystemdService` — Implements `services.Service` for individual units
- **Features:**
  - Auto-detects local vs remote based on address
  - Uses D-Bus for localhost, SSH for remote hosts
  - Streams logs via journalctl

### `services/traefik` Package
- **Purpose:** Traefik API client for hostname discovery and external service monitoring
- **Key Types:**
  - `Config` — Traefik API connection settings (Enabled, APIPort)
  - `Client` — HTTP client for querying Traefik API (includes MatcherLookupService)
  - `Router` — Represents a Traefik HTTP router
  - `TraefikAPIService` — Represents a service from the Traefik `/api/http/services` endpoint
  - `Provider` — Implements `services.Provider` for Traefik-only services (external services not backed by Docker/systemd)
  - `TraefikService` — Implements `services.Service` for individual Traefik services (stub implementation for logs/start/stop/restart)
  - `MatcherLookupService` — Stateful hostname extraction with change tracking
  - `MatcherInfo` — Detailed matcher information (type, hostname, exactness)
  - `MatcherType` — Enum: `MatcherTypeHost`, `MatcherTypeHostRegexp`
  - `RouterMatcherState` — Tracks rule changes and error states per router
- **Key Functions:**
  - `NewClient()` — Creates a new Traefik API client with embedded matcher service
  - `NewProvider()` — Creates a Traefik service provider for external service discovery
  - `GetRouters()` — Fetches all HTTP routers from Traefik API
  - `GetTraefikServices()` — Fetches all services from Traefik API `/api/http/services`
  - `GetServiceHostMappings()` — Returns map of service/router names to hostnames (uses matcher service, includes router-name-based mappings)
  - `GetClaimedBackendServices()` — Returns backend services "claimed" by routers owned by existing Docker/systemd services
  - `ExtractHostnames()` — Parses Host() and HostRegexp() matchers from Traefik rules
  - `ExtractMatchers()` — Returns detailed MatcherInfo for each hostname matcher
  - `NewMatcherLookupService()` — Creates a matcher service for state-tracked extraction
  - `ProcessRouter()` — Extracts hostnames with state tracking and logging
- **Features:**
  - Queries Traefik REST API at `/api/http/routers` and `/api/http/services`
  - **External Service Discovery:** Discovers services registered in Traefik that are not backed by Docker/systemd (e.g., reverse-proxied external hosts)
  - **Health Status:** Shows service health based on Traefik's server status (UP/DOWN)
  - Extracts hostnames from both `Host()` and `HostRegexp()` rule matchers
  - **Host() Preferred:** When both `Host()` and `HostRegexp()` are present, only exact `Host()` matches are used
  - Supports SSH tunneling for remote Traefik instances
  - Matches services by normalized name (strips `@provider` suffix)
  - **Filters Internal Services:** Excludes Traefik internal services (api@internal, dashboard@internal, etc.)
  - **Filters Claimed Backend Services:** When a Docker service creates a router pointing to a different backend (e.g., `jellyfin@docker` → `jellyfin-svc@file`), the backend service is filtered out since it's "owned" by the Docker service
  - **Router-Name-Based URL Mapping:** Docker services get Traefik URLs even when their router points to an external backend
  - **State Tracking:** Logs when router rules change between fetches
  - **Error Recovery:** Logs when a previously-failing router recovers
  - **HostRegexp Fallback:** Extracts domain suffixes from regex patterns only when no `Host()` is present (e.g., `{subdomain:[a-z]+}.example.com` → `example.com`)

## Configuration (services.json)

Defines which hosts and services to monitor. Supports JSON with comments (`//`, `/* */`) and trailing commas via [hujson](https://github.com/tailscale/hujson). **The service will fail to start if the config file cannot be parsed.**

```json
{
  "hosts": [
    {
      "name": "nas",                    // Display name
      "address": "localhost",           // "localhost" uses D-Bus, others use SSH
      "nic": ["ens10"],                 // NIC names to resolve private IP for port links
      "systemd_services": ["docker.service"],
      "docker_compose_roots": ["/home/xero/nas/"],
      "traefik": {
        "enabled": true,                // Enable Traefik hostname lookup
        "api_port": 8080                // Traefik API port (default 8080)
      }
    },
    {
      // another host
    }
  ],
  "oidc": {                             // Optional: OIDC authentication
    "service_url": "https://dashboard.example.com",  // Dashboard's public URL
    "callback": "/oidc/callback",       // Callback path for OIDC flow
    "config_url": "https://auth.example.com/.well-known/openid-configuration",
    "client_id": "your-client-id",
    "client_secret": "your-client-secret",
    "groups_claim": "groups",           // Optional: claim containing user groups (default: "groups")
    "admin_group": "admin",             // Optional: group name that grants admin access (default: "admin")
    "groups": {                         // Optional: group-based access control (OIDC only)
      "poweruser": {                    // OIDC group name
        "services": {                   // Services this group can access
          "nas": ["docker.service", "audiobookshelf", "traefik"],
          "anotherhost": ["ollama.service"]
        }
      },
      "bookreader": {
        "services": {
          "nas": ["audiobookshelf"]
        }
      }
    }
  },
  "local": {                            // Optional: Local authentication
    "admins": "user1,user2"             // Comma-separated usernames for local access (always have global access)
  }
}
```

### OIDC Group-Based Access Control

Group filtering applies **only to OIDC authentication** (not local/PAM users). It allows non-admin users to access a subset of services based on their OIDC group memberships.

**How it works:**
1. When a user logs in via OIDC, their group claims are extracted from the ID token
2. Each group the user belongs to is checked against the `oidc.groups` configuration
3. If a matching group is found, the user gains access to the services listed for that group
4. Permissions are **additive** — users in multiple groups get access to all services from all their groups (deduplicated)
5. Users with the configured `admin_group` (default: "admin") in their `groups_claim` have **global access** to all services
6. Local/PAM users always have **global access** regardless of group configuration

**Configuration:**
```json
"oidc": {
  // ... other OIDC settings ...
  "groups": {
    "<oidc-group-name>": {
      "services": {
        "<host-name>": ["<service1>", "<service2>", ...]
      }
    }
  }
}
```

- `<oidc-group-name>`: Must exactly match the group name from the OIDC provider's groups claim
- `<host-name>`: Must match a host's `name` field in the `hosts` array
- Services can be Docker service names (e.g., `audiobookshelf`) or systemd unit names (e.g., `docker.service`)

**Example:** A user in both `poweruser` and `bookreader` groups would have access to: `docker.service`, `audiobookshelf`, `traefik` on `nas`, and `ollama.service` on `anotherhost`.

## Backend

**Server:** Standard library `net/http` on port 9001, configured via `server` package

**Endpoints:**
- `GET /` — Serves `static/index.html` (protected when OIDC enabled)
- `GET /static/*` — Static file server for CSS/JS (always public)
- `GET /login` — Initiates OIDC login flow (redirects to provider)
- `GET /oidc/callback` — Handles OIDC callback, exchanges code for tokens
- `GET /logout` — Clears session and redirects to login
- `GET /auth/status` — Returns JSON with authentication status
- `GET /api/services` — Returns JSON array of all services (Docker + systemd + Traefik)
- `GET /api/logs?container=<name>` — SSE stream of Docker container logs
- `GET /api/logs/systemd?unit=<name>&host=<host>` — SSE stream of systemd unit logs
- `GET /api/logs/traefik?service=<name>&host=<host>` — SSE stream for Traefik (returns stub message, logs not supported)
- `GET /api/bangAndPipeToRegex?expr=<expr>` — Compiles Bang & Pipe expression to AST
- `GET /api/docs/bangandpipe` — Returns rendered HTML documentation for Bang & Pipe syntax
- `POST /api/services/start` — Start a service (SSE stream of status updates)
- `POST /api/services/stop` — Stop a service (SSE stream of status updates)
- `POST /api/services/restart` — Restart a service (Docker uses compose down/up, SSE stream of status updates)

**Application Layers:**
| Layer | Package | Responsibility |
|-------|---------|----------------|
| Bootstrap | `main` | Config loading, server initialization |
| Routing | `server` | HTTP routes, static files, server config |
| Auth | `auth` | OIDC authentication, session management |
| Handlers | `handlers` | Request processing, response generation |
| Services | `services/*` | Docker/systemd/Traefik provider implementations |
| Config | `config` | Configuration loading and access |
| Query | `query` | Bang & Pipe expression parsing |

**Service Types:**

| Source | Method | Library/Tool |
|--------|--------|--------------|
| Docker | Docker API via socket | `github.com/docker/docker/client` |
| Systemd (local) | D-Bus | `github.com/coreos/go-systemd/v22/dbus` |
| Systemd (remote) | SSH + systemctl | `os/exec` with `ssh` command |
| Traefik | Traefik REST API | Standard `net/http` client |

**ServiceInfo struct (in `services/service.go`):**
```go
type PortInfo struct {
    HostPort      uint16 `json:"host_port"`                 // Port exposed on the host
    ContainerPort uint16 `json:"container_port"`            // Port on the container
    Protocol      string `json:"protocol"`                  // "tcp" or "udp"
    Label         string `json:"label,omitempty"`           // Custom label for display (from Docker label)
    Hidden        bool   `json:"hidden,omitempty"`          // If true, port should be hidden from UI
    SourceService string `json:"source_service,omitempty"`  // Service that exposes this port (for remapped ports on target)
    TargetService string `json:"target_service,omitempty"`  // Service this port is remapped to (for remapped ports on source)
}

type ServiceInfo struct {
    Name          string     `json:"name"`                    // Service/unit name
    Project       string     `json:"project"`                 // Docker project or "systemd"
    ContainerName string     `json:"container_name"`          // Container name or unit name
    State         string     `json:"state"`                   // "running" or "stopped"
    Status        string     `json:"status"`                  // Human-readable status
    Image         string     `json:"image"`                   // Docker image or "-"
    Source        string     `json:"source"`                  // "docker" or "systemd"
    Host          string     `json:"host"`                    // Host name from config
    HostIP        string     `json:"host_ip"`                 // Private IP address for port links
    Ports         []PortInfo `json:"ports"`                   // Exposed ports (non-localhost bindings)
    TraefikURLs   []string   `json:"traefik_urls"`            // Traefik-exposed hostnames (as full URLs)
    TraefikServiceName string `json:"traefik_service_name,omitempty"` // Traefik service name from labels (if different from Name)
    Description   string     `json:"description"`             // Service description (from Docker label or systemd unit)
    Hidden        bool       `json:"hidden,omitempty"`        // If true, service should be hidden from UI
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
- Extracts custom description from `home.server.dashboard.description` label
- Streams logs using `ContainerLogs()` with multiplexed stdout/stderr

**Docker Dashboard Labels:**
The dashboard reads the following labels from Docker containers to customize visibility and display:

| Label | Values | Description |
|-------|--------|-------------|
| `home.server.dashboard.description` | Any string | Custom description displayed below service name |
| `home.server.dashboard.hidden` | `true`, `1`, `yes` | Hide entire service from dashboard |
| `home.server.dashboard.ports.hidden` | `port1,port2,...` | Comma-separated list of port numbers to hide |
| `home.server.dashboard.ports.<port>.label` | Any string | Custom label for a specific port (e.g., `home.server.dashboard.ports.8080.label=Admin`) |
| `home.server.dashboard.ports.<port>.hidden` | `true`, `1`, `yes` | Hide a specific port from display |
| `home.server.dashboard.remapport.<port>` | Service name | Remap a port to another service (for containers sharing network namespace) |

Example docker-compose.yml:
```yaml
services:
  myapp:
    image: myapp:latest
    labels:
      home.server.dashboard.description: "My awesome application"
      home.server.dashboard.ports.8080.label: "Admin"
      home.server.dashboard.ports.9000.hidden: "true"
      # Or hide multiple ports at once:
      # home.server.dashboard.ports.hidden: "9000,9001"

  # Example: VPN container exposing ports for services running in its network
  gluetun:
    image: qmcgaw/gluetun
    ports:
      - "8193:8193"  # qbittorrent-books web UI
    labels:
      # Remap port 8193 to appear on qbittorrent-books service
      home.server.dashboard.remapport.8193: qbittorrent-books
  
  qbittorrent-books:
    image: lscr.io/linuxserver/qbittorrent
    network_mode: "service:gluetun"  # Uses gluetun's network
    # Port 8193 will appear on both services:
    # - On gluetun: "→qbittorrent-books:8193" (de-emphasized, grey badge) - clicking scrolls to qbittorrent-books row
    # - On qbittorrent-books: "gluetun:8193" (normal info badge) - clicking opens URL using gluetun's IP:port
```

**Systemd Integration (`services/systemd/systemd.go`):**
- **Querying:** Local uses D-Bus via `dbus.NewSystemConnectionContext()` and `ListUnitsContext()`
- **Querying:** Remote uses SSH to run `systemctl show <unit> --property=ActiveState,SubState,LoadState,Description`
- **Service Control (Local):** Uses D-Bus `StartUnitContext()`, `StopUnitContext()`, `RestartUnitContext()` with polkit authorization
- **Service Control (Remote):** Uses SSH with sudo to run `systemctl start/stop/restart`
- Fetches unit description from systemd's `Description` property
- Filters units by exact name match from config

**Polkit Authorization (`polkit/polkit.go`):**
- Generates polkit rules for local systemd service control via D-Bus
- Required because the dashboard runs with `NoNewPrivileges=true` which prevents sudo
- Output installed to `/etc/polkit-1/rules.d/50-home-server-dashboard.rules`
- Allows the dashboard user to start/stop/restart configured units without sudo

**Sudoers Configuration (`sudoers/sudoers.go`):**
- Generates sudoers configuration for **remote** systemd service control only
- Local hosts use polkit + D-Bus instead (sudoers doesn't work with NoNewPrivileges)
- Output can be installed to `/etc/sudoers.d/home-server-dashboard` on remote hosts

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
- **Port links**: Clickable badges after service name showing exposed ports (non-localhost only), opens HTTP URL on click
- **Remapped port links**: For services sharing network namespaces (e.g., VPN containers):
  - On source service (e.g., gluetun): grey badge with arrow "→target:port" - clicking scrolls to target service row with highlight animation
  - On target service (e.g., qbittorrent): info badge "source:port" - clicking opens URL using the source service's IP
- **Traefik links**: Green clickable badges showing Traefik-exposed hostnames, opens HTTPS URL on click
- **Service descriptions**: Muted text below service name showing description (from Docker label `home.server.dashboard.description` or systemd unit description)
- **Table search**: VS Code-style search widget below filter cards
  - Filters across all columns (name, project, host, container, status, image, source)
  - Supports plain text, regex (with `!` prefix for inverse), and Bang & Pipe mode
  - Case sensitivity toggle
  - Match count display
  - Reuses the same AST evaluation as log search
- **Service control buttons**: Start/Stop/Restart buttons in Actions column
  - Shows confirmation modal before executing action
  - Real-time status updates via SSE during action execution
  - Docker restart uses `docker compose down/up` instead of simple restart
  - Auto-refreshes service data after successful action (5 second countdown)

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
- **handlers/** — HTTP handler validation, SSE headers, error responses
- **server/** — Server configuration, routing setup
- **services/** — ServiceInfo JSON serialization
- **services/docker/** — Log reader header stripping, provider methods
- **services/systemd/** — Provider creation, systemctl output parsing
- **query/** — Bang & Pipe expression lexer, parser, AST generation
- **main.go** — Bootstrap and package integration

### Integration Tests (require Docker/systemd)
```bash
go test -tags=integration ./...
```

Integration tests use the `//go:build integration` build tag and test real system interactions:
- **services/docker/** — Real Docker API calls, container listing, log streaming
- **services/systemd/** — Real D-Bus connections, journalctl log streaming
- **js_integration_test.go** — Runs JavaScript unit tests via Node.js

### JavaScript Tests
```bash
node tests/js/search_test.js
```

JavaScript tests cover the client-side search functionality:
- **tests/js/search_test.js** — Text matching, regex parsing, AST evaluation (47 tests)

| Package | Unit Tests | Integration Tests |
|---------|------------|-------------------|
| main | ✅ | — |
| handlers | ✅ | — |
| server | ✅ | — |
| config | ✅ | — |
| query | ✅ | — |
| services | ✅ | — |
| services/docker | ✅ | ✅ |
| services/systemd | ✅ | ✅ |
| services/traefik | ✅ | — |
| tests/js | ✅ (Node.js) | — |
