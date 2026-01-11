# Home Server Dashboard

REQUIRED: Update this document with any architectural or design information about the project.
REQUIRED: If you are working on a todo and find an issue, add it to the bottom of the todo list. the status should be "needs triage" and not checkmark or empty checkbox.
REQUIRED: All tests must pass, including integration tests, before any item in todo can be marked done. if there are no tests for the feature, add them, and they must pass.

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
├── events/
│   ├── events.go                  # Event types and event bus for pub/sub
│   └── events_test.go             # Event bus unit tests
├── monitor/
│   ├── monitor.go                 # Service state monitoring with polling
│   └── monitor_test.go            # Monitor unit tests
├── notifiers/
│   ├── notifier.go                # Notifier interface and manager
│   ├── notifier_test.go           # Notifier manager tests
│   └── gotify/
│       ├── gotify.go              # Gotify notification implementation
│       └── gotify_test.go         # Gotify notifier tests
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
│   ├── traefik/
│   │   ├── traefik.go             # Traefik API client for hostname lookup
│   │   ├── traefik_test.go        # Unit tests for Traefik client
│   │   ├── service.go             # Traefik service provider and service implementation
│   │   ├── service_test.go        # Unit tests for Traefik service provider
│   │   ├── matcher.go             # MatcherLookupService for hostname extraction with state tracking
│   │   └── matcher_test.go        # Unit tests for matcher lookup service
│   └── homeassistant/
│       ├── homeassistant.go       # Home Assistant provider and service implementation
│       └── homeassistant_test.go  # Unit tests for Home Assistant provider
├── frontend/                      # Frontend source and tests (JSX/ES6 modules)
│   ├── jsx.js                     # Minimal JSX runtime (h, Fragment, raw)
│   ├── main.jsx                   # Entry point, DOMContentLoaded, window.__dashboard global
│   ├── utils.js                   # Pure utility functions (escapeHtml, getStatusClass)
│   ├── state.js                   # Centralized state management (logsState, servicesState, etc.)
│   ├── search-core.js             # Unified search functions (evaluateAST, textMatches, etc.)
│   ├── services.js                # Service lookup helpers (getServiceHostIP, scrollToService)
│   ├── render.js                  # Rendering functions (renderPorts, renderServices, etc.)
│   ├── filter.js                  # Filtering/sorting (sortServices, toggleFilter, applyFilter)
│   ├── logs.js                    # Logs viewer functionality (toggleLogs, SSE, log search)
│   ├── table-search.js            # Table search UI functions
│   ├── actions.js                 # Service action modal (confirmServiceAction, executeServiceAction)
│   ├── api.js                     # API/auth functions (loadServices, checkAuthStatus)
│   ├── help.js                    # Help modal (showHelpModal)
│   ├── websocket.js               # WebSocket client for real-time updates
│   ├── run-tests.mjs              # Test runner entry point
│   ├── test-utils.mjs             # Test framework (describe, it, assert)
│   ├── utils.test.mjs             # Tests for utils.js
│   ├── state.test.mjs             # Tests for state.js
│   ├── search-core.test.mjs       # Tests for search-core.js
│   ├── filter.test.mjs            # Tests for filter.js
│   ├── render.test.mjs            # Tests for render.js
│   ├── websocket.test.mjs         # Tests for websocket.js
│   └── window-exports.test.mjs    # Validates onclick handlers match exported functions
├── websocket/                     # WebSocket server for real-time updates
│   ├── websocket.go               # Hub, Client, Message types, event broadcasting
│   └── websocket_test.go          # WebSocket unit tests
├── static/                        # Static assets (embedded into binary)
│   ├── index.html                 # Dashboard HTML structure (Bootstrap 5)
│   ├── style.css                  # Custom dark theme styling
│   ├── app.js                     # Bundled JavaScript (generated by esbuild, gitignored)
│   └── app.js.map                 # Source map for debugging (generated, gitignored)
├── package.json                   # npm config with esbuild dependency
├── build.mjs                      # esbuild build script with JSX support
├── go.mod
└── go.sum
```

## Package Structure

### `main` Package
- **Purpose:** Application entry point and bootstrap
- **Responsibilities:**
  - Load configuration from `services.json`
  - Initialize OIDC authentication provider (if configured)
  - Initialize event bus, monitor, notifiers, and WebSocket hub
  - Create and start HTTP server
  - Handle graceful shutdown
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
  - `GotifyConfig` — Gotify notification settings (Enabled, Hostname, Token)
  - `Config` — Complete configuration with helper methods like `GetLocalHostName()`, `GetHostByName()`, `IsOIDCEnabled()`
- **Functions:** `Load()`, `Get()`, `Default()`, `isPrivateIP()`

### `events` Package
- **Purpose:** Event-driven architecture for service monitoring pub/sub
- **Key Types:**
  - `EventType` — Enum: `ServiceStateChanged`, `HostUnreachable`, `HostRecovered`
  - `Event` — Interface with `Type()` and `Timestamp()` methods
  - `ServiceStateChangedEvent` — Emitted when a service changes state (running/stopped)
  - `HostUnreachableEvent` — Emitted when a host cannot be contacted
  - `HostRecoveredEvent` — Emitted when a previously unreachable host recovers
  - `Bus` — Thread-safe event bus for publish/subscribe
  - `Subscription` — Subscription handle with `Unsubscribe()` method
  - `Handler` — Function type for event handlers
- **Key Functions:**
  - `NewBus(asyncPublish bool)` — Creates event bus (async mode calls handlers in goroutines)
  - `NewServiceStateChangedEvent(...)` — Creates service state change event
  - `NewHostUnreachableEvent(host, reason)` — Creates host unreachable event
  - `NewHostRecoveredEvent(host)` — Creates host recovered event
- **Usage Pattern:**
  ```go
  bus := events.NewBus(true) // async dispatch
  sub := bus.Subscribe(events.ServiceStateChanged, func(e events.Event) {
      // handle event
  })
  bus.Publish(events.NewServiceStateChangedEvent(...))
  sub.Unsubscribe()
  ```

### `monitor` Package
- **Purpose:** Service state monitoring using native event sources (Docker Events API, systemd D-Bus signals)
- **Key Types:**
  - `Monitor` — Watches services and emits events on state changes
  - `ServiceState` — Tracks last known state of a service
  - `HostState` — Tracks whether a host is reachable
  - `Option` — Functional options for configuration
- **Key Functions:**
  - `New(cfg, bus, opts...)` — Creates monitor with config and event bus
  - `WithPollInterval(duration)` — Sets polling interval for remote hosts (default 60s)
  - `WithSkipFirstEvent(bool)` — Skip events during initial discovery (default true)
  - `Start()` — Begins background monitoring
  - `Stop()` — Stops monitoring and waits for cleanup
- **Features:**
  - **Native Docker Events:** Uses Docker Events API with filters for container state changes (start/stop/die/kill/pause/unpause)
  - **Native systemd D-Bus signals:** Uses `Subscribe()` and `SetSubStateSubscriber()` for real-time unit state changes
  - **Remote host polling:** Falls back to polling for remote hosts (SSH-based systemd) at configurable interval
  - Emits `ServiceStateChanged` events when service state changes
  - Emits `HostUnreachable`/`HostRecovered` events for host connectivity
  - Skips initial discovery to avoid startup notification spam
  - Thread-safe state tracking

### `notifiers` Package
- **Purpose:** Notification delivery for events (extensible for multiple backends)
- **Key Types:**
  - `Notifier` — Interface: `Name()`, `Notify(event)`, `Close()`
  - `Manager` — Manages multiple notifiers, routes events to all registered notifiers
- **Key Functions:**
  - `NewManager(bus)` — Creates manager subscribed to all event types
  - `Register(notifier)` — Adds a notifier to receive events
  - `Close()` — Unsubscribes from events and closes all notifiers
- **Design:** Notifiers are best-effort; failures are logged but don't stop other notifiers

### `notifiers/gotify` Package
- **Purpose:** Gotify push notification implementation using the official Gotify API client
- **Key Types:**
  - `Notifier` — Implements `notifiers.Notifier` for Gotify
  - `Message` — Internal message struct with Title, Message, Priority (used for formatting)
- **Key Functions:**
  - `New(cfg)` — Creates notifier from config (returns nil if disabled/invalid)
  - `Notify(event)` — Sends notification for event
  - `SendTest()` — Sends test notification to verify connectivity
- **Dependencies:**
  - `github.com/gotify/go-api-client/v2` — Official Gotify Go API client
- **Priority Levels:**
  - `PriorityMax (10)` — Host unreachable (persistent notification)
  - `PriorityHigh (8)` — Service stopped, host recovered
  - `PriorityNormal (5)` — Service started, test messages
  - `PriorityLow (2)` — Other state changes

### `websocket` Package
- **Purpose:** WebSocket server for real-time service state updates to browser clients
- **Key Types:**
  - `Hub` — Manages connected WebSocket clients and broadcasts messages
  - `Client` — Individual WebSocket connection with read/write pumps
  - `Message` — WebSocket message with Type, Timestamp, Payload
  - `MessageType` — Enum: `service_update`, `host_unreachable`, `host_recovered`, `ping`
  - `ServiceUpdatePayload` — Service state change details
  - `HostEventPayload` — Host event details
- **Key Functions:**
  - `NewHub(eventBus)` — Creates hub subscribed to event bus
  - `Start()` — Begins hub's main loop and event subscriptions
  - `Stop()` — Gracefully shuts down hub and closes connections
  - `Handler()` — Returns HTTP handler for WebSocket upgrade
  - `ClientCount()` — Returns number of connected clients
- **Features:**
  - Subscribes to event bus (ServiceStateChanged, HostUnreachable, HostRecovered)
  - Broadcasts events to all connected WebSocket clients
  - Ping/pong keepalive with configurable timeouts
  - Automatic reconnection support on client side
  - JSON message format for easy client-side parsing
- **Dependencies:**
  - `github.com/gorilla/websocket` — WebSocket implementation

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

### `services/homeassistant` Package
- **Purpose:** Home Assistant API client for health monitoring and service control. For HAOS installations, also provides addon discovery and control via the Supervisor API.
- **Key Types:**
  - `Provider` — Implements `services.Provider` for Home Assistant instances
  - `Service` — Implements `services.Service` for HA core, supervisor, host, and addon control
  - `Addon` — Represents a Home Assistant addon from the Supervisor API
  - `AddonsResponse`, `SupervisorInfo`, `CoreInfo`, `HostInfo` — API response types
- **Key Functions:**
  - `NewProvider(hostConfig)` — Creates provider from host config (returns nil if HA not configured)
  - `GetServices(ctx)` — For HAOS: returns Core, Supervisor, Host, and all addons; otherwise just HA core
  - `CheckHealth(ctx)` — Returns state ("running"/"stopped") and status message
  - `Restart(ctx)` — Calls `homeassistant.restart` service via HA REST API (fallback for non-HAOS)
  - `CoreControl(ctx, action)` — Start/stop/restart HA Core via Supervisor API (HAOS only)
  - `GetAddons(ctx)` — Returns list of installed addons (HAOS only)
  - `GetAddonLogs(ctx, slug, follow)` — Streams addon logs (HAOS only)
  - `GetCoreLogs(ctx, follow)` — Streams HA Core logs (HAOS only)
  - `GetSupervisorLogs(ctx, follow)` — Streams Supervisor logs (HAOS only)
  - `GetHostLogs(ctx, follow)` — Streams Host OS logs (HAOS only)
  - `AddonControl(ctx, slug, action)` — Start/stop/restart an addon (HAOS only)
  - `HasSupervisorAPI()` — Returns true if Supervisor API is available
- **Dependencies:**
  - `github.com/mutablelogic/go-client/pkg/homeassistant` — Official HA Go client
  - `golang.org/x/crypto/ssh` — SSH client for Supervisor API tunneling
- **Features:**
  - Uses Home Assistant REST API with long-lived access tokens
  - Supports HTTPS with optional certificate verification skip (for self-signed certs)
  - Health status based on API `/api/` endpoint response
  - **HAOS Support:** When `is_homeassistant_operatingsystem` is true and `ssh_addon_port` is configured:
    - Discovers all installed addons as separate services
    - Provides log streaming for Core, Supervisor, Host, and individual addons
    - Supports start/stop/restart for addons via Supervisor API (`POST /addons/<slug>/start|stop|restart`)
    - Supports start/stop/restart for HA Core via Supervisor API (`POST /core/start|stop|restart`)
    - Uses SSH addon for tunneling to internal Supervisor API (`http://supervisor`)
    - Automatically fetches `SUPERVISOR_TOKEN` from SSH addon container at `/run/s6/container_environment/SUPERVISOR_TOKEN`
  - **Non-HAOS Support:** Only restart is supported for HA Core via HA REST API (`homeassistant.restart` service)
  - Monitored for state changes and emits Gotify notifications

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
      "name": "homeassistant",          // Home Assistant host
      "address": "192.168.1.50",        // IP or hostname of HA instance
      "homeassistant": {
        "port": 8123,                   // HA API port (default 8123)
        "use_https": true,              // Use HTTPS for API connection
        "ignore_https_errors": true,    // Skip TLS verification (for self-signed certs)
        "longlivedtoken": "your-token", // Long-lived access token from HA
        "is_homeassistant_operatingsystem": true,  // Enable HAOS addon discovery
        "ssh_addon_port": 22            // SSH addon port for Supervisor API tunneling (default 22)
      },
      "systemd_services": [],
      "docker_compose_roots": []
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
  },
  "gotify": {                           // Optional: Gotify push notifications
    "enabled": true,                    // Enable/disable Gotify notifications
    "hostname": "https://gotify.example.com", // Gotify server URL
    "token": "your-app-token"           // Application token from Gotify
  }
}
```

### Gotify Notifications

The dashboard can send push notifications to a Gotify server when service states change or hosts become unreachable. This is **optional** — the dashboard works fine without Gotify configured.

**Configuration:**
```json
"gotify": {
  "enabled": true,
  "hostname": "https://gotify.example.com",
  "token": "your-app-token"
}
```

**Events that trigger notifications:**
| Event | Priority | Description |
|-------|----------|-------------|
| Service started | Normal (5) | 🟢 Service went from stopped to running |
| Service stopped | High (8) | 🔴 Service went from running to stopped |
| Host unreachable | Max (10) | 🚨 Cannot connect to a configured host |
| Host recovered | High (8) | ✅ Previously unreachable host is now reachable |

**Startup behavior:** The monitor skips event emission during initial service discovery to avoid notification spam on dashboard startup/restart.

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
- `GET /ws` — WebSocket endpoint for real-time service state updates (protected)

**Application Layers:**
| Layer | Package | Responsibility |
|-------|---------|----------------|
| Bootstrap | `main` | Config loading, server initialization |
| Routing | `server` | HTTP routes, static files, server config |
| Auth | `auth` | OIDC authentication, session management |
| Handlers | `handlers` | Request processing, response generation |
| Services | `services/*` | Docker/systemd/Traefik provider implementations |
| WebSocket | `websocket` | Real-time updates via WebSocket |
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
- **Tristate filters**: All filter cards (status, source, host) support tristate cycling by clicking:
  - **Include** (blue border): Show matching items (first click)
  - **Exclude** (red border): Hide matching items (second click)
  - **Exclusive** (green border): Show ONLY matching items (third click)
  - Fourth click clears the filter (back to no filter)
- Clickable stat cards to filter by status (running/stopped) with tristate support
- **Host filter row**: Dynamic row of host badges below the status/source cards, showing all hosts with service counts
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
  - **Sticky behavior**: Floats at top of screen when scrolled out of view, with transparency until hover/focus
  - **Scroll position preservation**: Switching between Filter/Find modes preserves scroll position (scrolls to bottom if content shrinks)
  - Two modes: Filter mode (hides non-matching) and Find mode (navigate between matches)
  - Mode toggle button switches between funnel (filter) and search (find) icons
  - Find mode: up/down navigation buttons, Enter/Shift+Enter keyboard shortcuts, current match highlighting
  - Filters across all columns (name, project, host, container, status, image, source)
  - Supports plain text, regex (with `!` prefix for inverse), and Bang & Pipe mode
  - Case sensitivity toggle
  - Match count display (filter: "X of Y", find: "1 of N")
  - Reuses the same AST evaluation as log search
- **Scroll-to-top button**: Floating button in lower right corner, appears when scrolled down, smooth scrolls to top
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
npm test
# or directly:
node frontend/run-tests.mjs
```

JavaScript tests cover the client-side functionality with modular test files:
- **frontend/utils.test.mjs** — escapeHtml, getStatusClass utilities
- **frontend/state.test.mjs** — State management and reset functions
- **frontend/search-core.test.mjs** — Text matching, regex parsing, AST evaluation
- **frontend/filter.test.mjs** — Service sorting functions
- **frontend/render.test.mjs** — Port rendering, Traefik URLs, source icons
- **frontend/window-exports.test.mjs** — Validates HTML onclick handlers reference exported window.__dashboard functions

| Package | Unit Tests | Integration Tests |
|---------|------------|-------------------|
| main | ✅ | — |
| handlers | ✅ | — |
| server | ✅ | — |
| config | ✅ | — |
| events | ✅ | — |
| monitor | ✅ | — |
| notifiers | ✅ | — |
| notifiers/gotify | ✅ | — |
| query | ✅ | — |
| services | ✅ | — |
| services/docker | ✅ | ✅ |
| services/systemd | ✅ | ✅ |
| services/traefik | ✅ | — |
| services/homeassistant | ✅ | — |
| frontend | ✅ (Node.js) | — |

