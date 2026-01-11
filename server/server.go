// Package server provides HTTP server setup and routing.
package server

import (
	"io/fs"
	"log"
	"net/http"

	"home_server_dashboard/auth"
	"home_server_dashboard/handlers"
	"home_server_dashboard/websocket"
)

// Config holds server configuration options.
type Config struct {
	Port         string
	StaticDir    string           // Deprecated: use StaticFS instead
	ConfigPath   string
	StaticFS     fs.FS            // Embedded static filesystem
	DocsFS       fs.FS            // Embedded docs filesystem
	AuthProvider *auth.Provider   // OIDC auth provider (nil if auth disabled)
	WebSocketHub *websocket.Hub   // WebSocket hub for real-time updates
}

// DefaultConfig returns the default server configuration.
func DefaultConfig() *Config {
	return &Config{
		Port:       ":9001",
		StaticDir:  "static",
		ConfigPath: "services.json",
	}
}

// Server represents the HTTP server.
type Server struct {
	config *Config
	mux    *http.ServeMux
}

// New creates a new Server with the given configuration.
func New(cfg *Config) *Server {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	s := &Server{
		config: cfg,
		mux:    http.NewServeMux(),
	}

	s.setupRoutes()
	return s
}

// setupRoutes configures all HTTP routes.
func (s *Server) setupRoutes() {
	// Serve static files from embedded filesystem (always public for login page styling)
	if s.config.StaticFS != nil {
		fs := http.FileServer(http.FS(s.config.StaticFS))
		s.mux.Handle("/static/", http.StripPrefix("/static/", fs))
	} else {
		// Fallback to filesystem for development
		fs := http.FileServer(http.Dir(s.config.StaticDir))
		s.mux.Handle("/static/", http.StripPrefix("/static/", fs))
	}

	// Set the embedded filesystems for handlers
	handlers.SetEmbeddedFS(s.config.StaticFS, s.config.DocsFS)

	// Auth routes (always public)
	if s.config.AuthProvider != nil {
		s.mux.HandleFunc("/login", s.config.AuthProvider.LoginHandler)
		s.mux.HandleFunc("/oidc/callback", s.config.AuthProvider.CallbackHandler)
		s.mux.HandleFunc("/logout", s.config.AuthProvider.LogoutHandler)
		s.mux.HandleFunc("/auth/status", s.config.AuthProvider.StatusHandler)
	} else {
		// When auth is disabled, provide a status endpoint that says so
		s.mux.HandleFunc("/auth/status", auth.NoAuthStatusHandler)
	}

	// Create middleware wrapper for protected routes
	protect := func(h http.HandlerFunc) http.HandlerFunc {
		if s.config.AuthProvider != nil {
			return func(w http.ResponseWriter, r *http.Request) {
				s.config.AuthProvider.Middleware(http.HandlerFunc(h)).ServeHTTP(w, r)
			}
		}
		return h
	}

	// Serve index.html at root (protected)
	s.mux.HandleFunc("/", protect(handlers.IndexHandler))

	// API endpoints (protected)
	s.mux.HandleFunc("/api/services", protect(handlers.ServicesHandler))
	s.mux.HandleFunc("/api/logs", protect(handlers.DockerLogsHandler))
	s.mux.HandleFunc("/api/logs/systemd", protect(handlers.SystemdLogsHandler))
	s.mux.HandleFunc("/api/logs/traefik", protect(handlers.TraefikLogsHandler))
	s.mux.HandleFunc("/api/logs/homeassistant", protect(handlers.HomeAssistantLogsHandler))
	s.mux.HandleFunc("/api/logs/flush", protect(handlers.LogFlushHandler))
	s.mux.HandleFunc("/api/bangAndPipeToRegex", protect(handlers.BangAndPipeHandler))
	s.mux.HandleFunc("/api/docs/bangandpipe", protect(handlers.BangAndPipeDocsHandler))

	// Service control actions (start/stop/restart) (protected)
	s.mux.HandleFunc("/api/services/start", protect(handlers.ServiceActionHandler))
	s.mux.HandleFunc("/api/services/stop", protect(handlers.ServiceActionHandler))
	s.mux.HandleFunc("/api/services/restart", protect(handlers.ServiceActionHandler))

	// WebSocket endpoint for real-time updates (protected)
	if s.config.WebSocketHub != nil {
		s.mux.HandleFunc("/ws", protect(s.config.WebSocketHub.Handler()))
	}
}

// Handler returns the HTTP handler for the server.
func (s *Server) Handler() http.Handler {
	return s.mux
}

// ListenAndServe starts the HTTP server.
func (s *Server) ListenAndServe() error {
	log.Printf("Starting server on %s", s.config.Port)
	return http.ListenAndServe(s.config.Port, s.mux)
}
