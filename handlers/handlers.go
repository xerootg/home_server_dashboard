// Package handlers provides HTTP handlers for the dashboard API.
package handlers

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/gomarkdown/markdown"
	"github.com/gomarkdown/markdown/html"
	"github.com/gomarkdown/markdown/parser"

	"home_server_dashboard/config"
	"home_server_dashboard/query"
	"home_server_dashboard/services"
	"home_server_dashboard/services/docker"
	"home_server_dashboard/services/systemd"
	"home_server_dashboard/services/traefik"
)

// getAllServices collects services from all configured providers.
func getAllServices(ctx context.Context, cfg *config.Config) ([]services.ServiceInfo, error) {
	var allServices []services.ServiceInfo

	// Build a map of host names to their private IPs for quick lookup
	hostIPMap := make(map[string]string)
	for _, host := range cfg.Hosts {
		hostIPMap[host.Name] = host.GetPrivateIP()
	}

	// Get Docker services from localhost
	localHostName := cfg.GetLocalHostName()
	dockerProvider, err := docker.NewProvider(localHostName)
	if err != nil {
		log.Printf("Warning: failed to create Docker provider: %v", err)
	} else {
		defer dockerProvider.Close()
		dockerServices, err := dockerProvider.GetServices(ctx)
		if err != nil {
			log.Printf("Warning: failed to get Docker services: %v", err)
		} else {
			// Set HostIP for each Docker service
			for i := range dockerServices {
				dockerServices[i].HostIP = hostIPMap[dockerServices[i].Host]
			}
			allServices = append(allServices, dockerServices...)
		}
	}

	// Get systemd services from each configured host
	for _, host := range cfg.Hosts {
		if len(host.SystemdServices) == 0 {
			continue
		}

		systemdProvider := systemd.NewProvider(host.Name, host.Address, host.SystemdServices)
		systemdServices, err := systemdProvider.GetServices(ctx)
		if err != nil {
			log.Printf("Warning: failed to get systemd services from %s: %v", host.Name, err)
			continue
		}
		// Set HostIP for each systemd service
		for i := range systemdServices {
			systemdServices[i].HostIP = hostIPMap[systemdServices[i].Host]
		}
		allServices = append(allServices, systemdServices...)
	}

	// Enrich services with Traefik hostnames
	allServices = enrichWithTraefikURLs(ctx, cfg, allServices)

	return allServices, nil
}

// enrichWithTraefikURLs adds Traefik-exposed URLs to services.
// It queries each host's Traefik API for router information and matches
// services by their name.
func enrichWithTraefikURLs(ctx context.Context, cfg *config.Config, svcList []services.ServiceInfo) []services.ServiceInfo {
	// Collect service->hostname mappings from all hosts with Traefik enabled
	// Key: service name, Value: list of hostnames
	traefikMappings := make(map[string][]string)

	for _, host := range cfg.Hosts {
		if !host.Traefik.Enabled {
			continue
		}

		client := traefik.NewClient(host.Name, host.Address, host.Traefik.APIPort)
		defer client.Close()

		mappings, err := client.GetServiceHostMappings(ctx)
		if err != nil {
			log.Printf("Warning: failed to get Traefik mappings from %s: %v", host.Name, err)
			continue
		}

		// Merge mappings (a service could be exposed via multiple hosts/routers)
		for svcName, hostnames := range mappings {
			existing := traefikMappings[svcName]
			for _, h := range hostnames {
				// Avoid duplicates
				found := false
				for _, e := range existing {
					if e == h {
						found = true
						break
					}
				}
				if !found {
					existing = append(existing, h)
				}
			}
			traefikMappings[svcName] = existing
		}
	}

	// Apply Traefik URLs to services
	for i := range svcList {
		svc := &svcList[i]

		// Try to match by service name (the Name field is the service name for Docker Compose)
		hostnames := traefikMappings[svc.Name]

		// Traefik often names services as "servicename-projectname", so try that pattern
		if len(hostnames) == 0 && svc.Project != "" && svc.Project != "systemd" {
			traefikServiceName := svc.Name + "-" + svc.Project
			hostnames = traefikMappings[traefikServiceName]
		}

		// Also try with container name for containers that might use that
		if len(hostnames) == 0 && svc.ContainerName != "" {
			hostnames = traefikMappings[svc.ContainerName]
		}

		// Convert hostnames to full URLs (https by default since Traefik usually terminates TLS)
		for _, hostname := range hostnames {
			url := "https://" + hostname
			svc.TraefikURLs = append(svc.TraefikURLs, url)
		}
	}

	return svcList
}

// ServicesHandler handles GET /api/services requests.
func ServicesHandler(w http.ResponseWriter, r *http.Request) {
	cfg := config.Get()
	if cfg == nil {
		http.Error(w, "Configuration not loaded", http.StatusInternalServerError)
		return
	}

	svcList, err := getAllServices(r.Context(), cfg)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error getting services: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(svcList)
}

// SystemdLogsHandler handles GET /api/logs/systemd requests for streaming systemd logs.
func SystemdLogsHandler(w http.ResponseWriter, r *http.Request) {
	unitName := r.URL.Query().Get("unit")
	hostName := r.URL.Query().Get("host")
	if unitName == "" {
		http.Error(w, "unit parameter required", http.StatusBadRequest)
		return
	}

	// Find the host config to get the address
	cfg := config.Get()
	hostAddress := "localhost"
	if cfg != nil {
		if host := cfg.GetHostByName(hostName); host != nil {
			hostAddress = host.Address
		}
	}

	// Set headers for SSE
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	ctx := r.Context()

	// Create systemd provider for this host
	systemdProvider := systemd.NewProvider(hostName, hostAddress, []string{unitName})
	logs, err := systemdProvider.GetLogs(ctx, unitName, 100, true)
	if err != nil {
		fmt.Fprintf(w, "data: Error starting journalctl: %v\n\n", err)
		flusher.Flush()
		return
	}
	defer logs.Close()

	reader := bufio.NewReader(logs)
	for {
		select {
		case <-ctx.Done():
			return
		default:
			line, err := reader.ReadString('\n')
			if err != nil {
				if err == io.EOF {
					select {
					case <-ctx.Done():
						return
					default:
						continue
					}
				}
				return
			}

			escaped := strings.TrimSpace(line)
			if escaped != "" {
				fmt.Fprintf(w, "data: %s\n\n", escaped)
				flusher.Flush()
			}
		}
	}
}

// DockerLogsHandler handles GET /api/logs requests for streaming Docker container logs.
func DockerLogsHandler(w http.ResponseWriter, r *http.Request) {
	containerName := r.URL.Query().Get("container")
	if containerName == "" {
		http.Error(w, "container parameter required", http.StatusBadRequest)
		return
	}

	// Set headers for SSE
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	cfg := config.Get()
	localHostName := "localhost"
	if cfg != nil {
		localHostName = cfg.GetLocalHostName()
	}

	dockerProvider, err := docker.NewProvider(localHostName)
	if err != nil {
		fmt.Fprintf(w, "data: Error: %v\n\n", err)
		flusher.Flush()
		return
	}
	defer dockerProvider.Close()

	ctx := r.Context()

	logs, err := dockerProvider.GetLogs(ctx, containerName, 100, true)
	if err != nil {
		fmt.Fprintf(w, "data: Error: %v\n\n", err)
		flusher.Flush()
		return
	}
	defer logs.Close()

	reader := bufio.NewReader(logs)
	for {
		select {
		case <-ctx.Done():
			return
		default:
			line, err := reader.ReadBytes('\n')
			if err != nil {
				if err == io.EOF {
					continue
				}
				return
			}

			// Escape for SSE and send
			escaped := strings.ReplaceAll(string(line), "\n", "")
			escaped = strings.ReplaceAll(escaped, "\r", "")
			if escaped != "" {
				fmt.Fprintf(w, "data: %s\n\n", escaped)
				flusher.Flush()
			}
		}
	}
}

// IndexHandler serves the main dashboard page.
func IndexHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/" {
		http.ServeFile(w, r, "static/index.html")
		return
	}
	http.NotFound(w, r)
}

// BangAndPipeHandler handles GET /api/bangAndPipeToRegex requests.
// It compiles a bang-and-pipe expression into an AST for client-side evaluation.
func BangAndPipeHandler(w http.ResponseWriter, r *http.Request) {
	expr := r.URL.Query().Get("expr")
	
	result := query.Compile(expr)
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// BangAndPipeDocsHandler handles GET /api/docs/bangandpipe requests.
// It renders the BangAndPipe query language documentation as HTML.
func BangAndPipeDocsHandler(w http.ResponseWriter, r *http.Request) {
	// Read the markdown file
	mdContent, err := os.ReadFile("docs/bangandpipe-query-language.md")
	if err != nil {
		http.Error(w, "Documentation not found", http.StatusNotFound)
		return
	}

	// Create markdown parser with extensions
	extensions := parser.CommonExtensions | parser.AutoHeadingIDs | parser.NoEmptyLineBeforeBlock
	p := parser.NewWithExtensions(extensions)
	doc := p.Parse(mdContent)

	// Create HTML renderer with options
	htmlFlags := html.CommonFlags | html.HrefTargetBlank
	opts := html.RendererOptions{Flags: htmlFlags}
	renderer := html.NewRenderer(opts)

	// Render to HTML
	htmlContent := markdown.Render(doc, renderer)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(htmlContent)
}
