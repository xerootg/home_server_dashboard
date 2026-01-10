// Package main bootstraps the Home Server Dashboard application.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/user"
	"time"

	"home_server_dashboard/auth"
	"home_server_dashboard/config"
	"home_server_dashboard/polkit"
	"home_server_dashboard/server"
	"home_server_dashboard/sudoers"
)

// getConfigPath returns the configuration file path.
// Priority: CONFIG_PATH env var > default "services.json" in current directory
func getConfigPath() string {
	if path := os.Getenv("CONFIG_PATH"); path != "" {
		return path
	}
	return "services.json"
}

func main() {
	// Parse command line flags
	generateSudoersFlag := flag.Bool("generate-sudoers", false, "Generate sudoers configuration for remote systemd services and exit")
	generatePolkitFlag := flag.Bool("generate-polkit", false, "Generate polkit rules for local systemd services and exit")
	authUser := flag.String("user", "", "Username for sudoers/polkit files (defaults to current user)")
	// Keep old flag name for backwards compatibility
	sudoersUser := flag.String("sudoers-user", "", "Deprecated: use -user instead")
	flag.Parse()

	// Load configuration
	configPath := getConfigPath()
	cfg, err := config.Load(configPath)
	if err != nil {
		log.Fatalf("Failed to load configuration from %s: %v", configPath, err)
	}

	// Determine username for auth files
	username := *authUser
	if username == "" {
		username = *sudoersUser // Fallback to old flag
	}
	if username == "" {
		currentUser, err := user.Current()
		if err != nil {
			log.Fatalf("Failed to get current user: %v", err)
		}
		username = currentUser.Username
	}

	// Handle polkit generation
	if *generatePolkitFlag {
		// Convert config hosts to polkit.HostServices
		hosts := make([]polkit.HostServices, len(cfg.Hosts))
		for i, host := range cfg.Hosts {
			hosts[i] = polkit.HostServices{
				Name:     host.Name,
				Address:  host.Address,
				Services: host.SystemdServices,
			}
		}

		fmt.Print(polkit.GeneratePolkitRules(hosts, username))
		os.Exit(0)
	}

	// Handle sudoers generation
	if *generateSudoersFlag {
		// Convert config hosts to sudoers.HostServices
		hosts := make([]sudoers.HostServices, len(cfg.Hosts))
		for i, host := range cfg.Hosts {
			hosts[i] = sudoers.HostServices{
				Name:     host.Name,
				Address:  host.Address,
				Services: host.SystemdServices,
			}
		}

		fmt.Print(sudoers.Generate(hosts, username))
		os.Exit(0)
	}

	log.Printf("Loaded config from %s with %d hosts", configPath, len(cfg.Hosts))

	// Validate group configurations (log warnings for non-existent services)
	cfg.ValidateGroupConfigs()

	// Create server config with embedded filesystems
	serverCfg := server.DefaultConfig()
	staticFS, err := getStaticFS()
	if err != nil {
		log.Fatalf("Failed to get embedded static filesystem: %v", err)
	}
	docsFS, err := getDocsFS()
	if err != nil {
		log.Fatalf("Failed to get embedded docs filesystem: %v", err)
	}
	serverCfg.StaticFS = staticFS
	serverCfg.DocsFS = docsFS

	// Initialize OIDC authentication if configured
	if cfg.IsOIDCEnabled() {
		log.Printf("OIDC authentication enabled, connecting to provider...")
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		authProvider, err := auth.NewProvider(ctx, cfg.OIDC, cfg.Local)
		if err != nil {
			log.Fatalf("Failed to initialize OIDC provider: %v", err)
		}
		serverCfg.AuthProvider = authProvider
		log.Printf("OIDC authentication initialized successfully")
		if cfg.Local != nil && cfg.Local.Admins != "" {
			log.Printf("Local authentication configured for admins: %s", cfg.Local.Admins)
		}
	} else {
		log.Printf("OIDC authentication not configured, running without authentication")
	}

	// Create and start server
	srv := server.New(serverCfg)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
