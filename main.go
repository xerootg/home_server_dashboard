// Package main bootstraps the Home Server Dashboard application.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/user"

	"home_server_dashboard/config"
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
	generateSudoersFlag := flag.Bool("generate-sudoers", false, "Generate sudoers configuration for systemd services and exit")
	sudoersUser := flag.String("sudoers-user", "", "Username for sudoers file (defaults to current user)")
	flag.Parse()

	// Load configuration
	configPath := getConfigPath()
	cfg, err := config.Load(configPath)
	if err != nil {
		log.Fatalf("Failed to load configuration from %s: %v", configPath, err)
	}

	// Handle sudoers generation
	if *generateSudoersFlag {
		username := *sudoersUser
		if username == "" {
			currentUser, err := user.Current()
			if err != nil {
				log.Fatalf("Failed to get current user: %v", err)
			}
			username = currentUser.Username
		}

		// Convert config hosts to sudoers.HostServices
		hosts := make([]sudoers.HostServices, len(cfg.Hosts))
		for i, host := range cfg.Hosts {
			hosts[i] = sudoers.HostServices{
				Name:     host.Name,
				Services: host.SystemdServices,
			}
		}

		fmt.Print(sudoers.Generate(hosts, username))
		os.Exit(0)
	}

	log.Printf("Loaded config from %s with %d hosts", configPath, len(cfg.Hosts))

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

	// Create and start server
	srv := server.New(serverCfg)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
