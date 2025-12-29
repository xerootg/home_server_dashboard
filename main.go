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

func main() {
	// Parse command line flags
	generateSudoersFlag := flag.Bool("generate-sudoers", false, "Generate sudoers configuration for systemd services and exit")
	sudoersUser := flag.String("sudoers-user", "", "Username for sudoers file (defaults to current user)")
	flag.Parse()

	// Load configuration
	cfg, err := config.Load("services.json")
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
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

	log.Printf("Loaded config with %d hosts", len(cfg.Hosts))

	// Create and start server
	srv := server.New(server.DefaultConfig())
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
