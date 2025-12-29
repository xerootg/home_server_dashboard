// Package main bootstraps the Home Server Dashboard application.
package main

import (
	"log"

	"home_server_dashboard/config"
	"home_server_dashboard/server"
)

func main() {
	// Load configuration
	cfg, err := config.Load("services.json")
	if err != nil {
		log.Printf("Warning: %v - using defaults", err)
		cfg = config.Default()
	}
	log.Printf("Loaded config with %d hosts", len(cfg.Hosts))

	// Create and start server
	srv := server.New(server.DefaultConfig())
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
