// Package server provides HTTP server setup and routing.
package server

import (
	"log"
	"net/http"

	"home_server_dashboard/handlers"
)

// Config holds server configuration options.
type Config struct {
	Port       string
	StaticDir  string
	ConfigPath string
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
	// Serve static files
	fs := http.FileServer(http.Dir(s.config.StaticDir))
	s.mux.Handle("/static/", http.StripPrefix("/static/", fs))

	// Serve index.html at root
	s.mux.HandleFunc("/", handlers.IndexHandler)

	// API endpoints
	s.mux.HandleFunc("/api/services", handlers.ServicesHandler)
	s.mux.HandleFunc("/api/logs", handlers.DockerLogsHandler)
	s.mux.HandleFunc("/api/logs/systemd", handlers.SystemdLogsHandler)
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
