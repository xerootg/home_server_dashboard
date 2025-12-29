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
	"os/exec"
	"path/filepath"
	"strings"
	"time"

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
	var allPortRemaps []docker.PortRemap

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
		dockerServices, portRemaps := dockerProvider.GetServicesWithRemaps(ctx)
		// Set HostIP for each Docker service
		for i := range dockerServices {
			dockerServices[i].HostIP = hostIPMap[dockerServices[i].Host]
		}
		allServices = append(allServices, dockerServices...)
		allPortRemaps = append(allPortRemaps, portRemaps...)
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

	// Apply port remapping (move ports from source services to target services)
	allServices = applyPortRemaps(allServices, allPortRemaps)

	// Enrich services with Traefik hostnames
	allServices = enrichWithTraefikURLs(ctx, cfg, allServices)

	return allServices, nil
}

// applyPortRemaps moves ports from source services to target services based on remap labels.
// This is used when services run in another container's network namespace (e.g., qbittorrent in gluetun).
// The remapped ports will have SourceService set on the target to indicate which service exposes the port,
// and TargetService set on the source to indicate where the port is remapped to.
func applyPortRemaps(svcList []services.ServiceInfo, remaps []docker.PortRemap) []services.ServiceInfo {
	if len(remaps) == 0 {
		return svcList
	}

	// Build a map of service name to index for quick lookup
	svcIndex := make(map[string]int)
	for i, svc := range svcList {
		svcIndex[svc.Name] = i
	}

	// Process each remap
	for _, remap := range remaps {
		sourceIdx, sourceExists := svcIndex[remap.SourceService]
		targetIdx, targetExists := svcIndex[remap.TargetService]

		if !sourceExists || !targetExists {
			log.Printf("Warning: port remap failed - source=%s (exists=%v) target=%s (exists=%v)",
				remap.SourceService, sourceExists, remap.TargetService, targetExists)
			continue
		}

		// Find the port in the source service
		sourceSvc := &svcList[sourceIdx]
		targetSvc := &svcList[targetIdx]

		var portIdx int = -1
		for i, port := range sourceSvc.Ports {
			if port.HostPort == remap.Port {
				portIdx = i
				break
			}
		}

		if portIdx == -1 {
			log.Printf("Warning: port remap failed - port %d not found on service %s",
				remap.Port, remap.SourceService)
			continue
		}

		// Create a copy of the port with SourceService set for the target
		remappedPort := sourceSvc.Ports[portIdx]
		remappedPort.SourceService = remap.SourceService

		// Add to target service
		targetSvc.Ports = append(targetSvc.Ports, remappedPort)

		// Mark source port with TargetService to show where it's remapped to
		sourceSvc.Ports[portIdx].TargetService = remap.TargetService

		log.Printf("Port %d remapped from %s to %s", remap.Port, remap.SourceService, remap.TargetService)
	}

	return svcList
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

		// Try to match by Traefik service name if explicitly defined in labels
		var hostnames []string
		if svc.TraefikServiceName != "" {
			hostnames = traefikMappings[svc.TraefikServiceName]
		}

		// Fall back to matching by service name (the Name field is the service name for Docker Compose)
		if len(hostnames) == 0 {
			hostnames = traefikMappings[svc.Name]
		}

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

// ServiceActionRequest represents the request body for service actions.
type ServiceActionRequest struct {
	ContainerName string `json:"container_name"`
	ServiceName   string `json:"service_name"`
	Source        string `json:"source"`
	Host          string `json:"host"`
	Project       string `json:"project"`
}

// ServiceActionHandler handles POST /api/services/action requests for start/stop/restart.
// It streams status updates via SSE.
func ServiceActionHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse action from URL path
	path := r.URL.Path
	action := strings.TrimPrefix(path, "/api/services/")
	if action != "start" && action != "stop" && action != "restart" {
		http.Error(w, "Invalid action. Must be start, stop, or restart", http.StatusBadRequest)
		return
	}

	// Parse request body
	var req ServiceActionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
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

	ctx := r.Context()

	// Helper to send SSE events
	sendEvent := func(eventType, message string) {
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", eventType, message)
		flusher.Flush()
	}

	sendEvent("status", fmt.Sprintf("Starting %s action on %s...", action, req.ServiceName))

	cfg := config.Get()
	var err error

	if req.Source == "docker" {
		err = handleDockerAction(ctx, cfg, req, action, sendEvent)
	} else if req.Source == "systemd" {
		err = handleSystemdAction(ctx, cfg, req, action, sendEvent)
	} else {
		sendEvent("error", "Unknown service source: "+req.Source)
		sendEvent("complete", "failed")
		return
	}

	if err != nil {
		log.Printf("Service action failed: action=%s service=%s source=%s host=%s error=%v",
			action, req.ServiceName, req.Source, req.Host, err)
		sendEvent("error", err.Error())
		sendEvent("complete", "failed")
		return
	}

	sendEvent("status", fmt.Sprintf("Action '%s' completed successfully", action))
	sendEvent("complete", "success")
}

// handleDockerAction performs Docker container actions.
// For restart, it uses docker-compose down/up instead of simple restart.
func handleDockerAction(ctx context.Context, cfg *config.Config, req ServiceActionRequest, action string, sendEvent func(string, string)) error {
	localHostName := "localhost"
	if cfg != nil {
		localHostName = cfg.GetLocalHostName()
	}

	// For restart, use docker-compose down/up
	if action == "restart" {
		return handleDockerComposeRestart(ctx, cfg, req, sendEvent)
	}

	// For start/stop, use Docker API
	dockerProvider, err := docker.NewProvider(localHostName)
	if err != nil {
		return fmt.Errorf("failed to create Docker provider: %w", err)
	}
	defer dockerProvider.Close()

	svc, err := dockerProvider.GetService(req.ContainerName)
	if err != nil {
		return fmt.Errorf("failed to get service: %w", err)
	}

	sendEvent("status", fmt.Sprintf("Executing %s on container %s...", action, req.ContainerName))

	switch action {
	case "start":
		err = svc.Start(ctx)
	case "stop":
		err = svc.Stop(ctx)
	}

	if err != nil {
		return fmt.Errorf("failed to %s container: %w", action, err)
	}

	return nil
}

// handleDockerComposeRestart performs docker-compose down/up for a service.
func handleDockerComposeRestart(ctx context.Context, cfg *config.Config, req ServiceActionRequest, sendEvent func(string, string)) error {
	if cfg == nil {
		return fmt.Errorf("configuration not loaded")
	}

	// Find the compose root for this project
	var composeRoot string
	for _, host := range cfg.Hosts {
		if !host.IsLocal() {
			continue
		}
		for _, root := range host.DockerComposeRoots {
			// Check if this root contains the project
			// Docker Compose project name is typically the directory name
			// or specified in compose file
			testPath := filepath.Join(root, req.Project)
			if _, err := os.Stat(testPath); err == nil {
				composeRoot = testPath
				break
			}
			// Also check if the root itself is the project directory
			if filepath.Base(root) == req.Project || strings.TrimSuffix(filepath.Base(root), "/") == req.Project {
				composeRoot = root
				break
			}
		}
		if composeRoot != "" {
			break
		}
	}

	// If we couldn't find a specific project directory, try to find compose file
	// by checking each compose root for a docker-compose.yml that contains the service
	if composeRoot == "" {
		for _, host := range cfg.Hosts {
			if !host.IsLocal() {
				continue
			}
			for _, root := range host.DockerComposeRoots {
				composeFile := findComposeFile(root)
				if composeFile != "" {
					composeRoot = filepath.Dir(composeFile)
					break
				}
			}
			if composeRoot != "" {
				break
			}
		}
	}

	if composeRoot == "" {
		// Fall back to simple docker restart if we can't find compose root
		sendEvent("status", "Could not find docker-compose root, falling back to simple restart...")
		return handleDockerSimpleRestart(ctx, cfg, req, sendEvent)
	}

	sendEvent("status", fmt.Sprintf("Found compose root: %s", composeRoot))

	// Run docker-compose down for the specific service
	sendEvent("status", fmt.Sprintf("Running docker compose down for %s...", req.ServiceName))
	
	downCmd := exec.CommandContext(ctx, "docker", "compose", "down", req.ServiceName)
	downCmd.Dir = composeRoot
	downOutput, err := downCmd.CombinedOutput()
	if err != nil {
		// Log but don't fail - service might not be running
		sendEvent("status", fmt.Sprintf("Down output: %s", strings.TrimSpace(string(downOutput))))
	} else if len(downOutput) > 0 {
		sendEvent("status", strings.TrimSpace(string(downOutput)))
	}

	// Brief pause to ensure cleanup
	time.Sleep(500 * time.Millisecond)

	// Run docker-compose up for the specific service
	sendEvent("status", fmt.Sprintf("Running docker compose up -d for %s...", req.ServiceName))
	
	upCmd := exec.CommandContext(ctx, "docker", "compose", "up", "-d", req.ServiceName)
	upCmd.Dir = composeRoot
	upOutput, err := upCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker compose up failed: %s - %w", strings.TrimSpace(string(upOutput)), err)
	}
	if len(upOutput) > 0 {
		sendEvent("status", strings.TrimSpace(string(upOutput)))
	}

	return nil
}

// handleDockerSimpleRestart falls back to Docker API restart if compose is not available.
func handleDockerSimpleRestart(ctx context.Context, cfg *config.Config, req ServiceActionRequest, sendEvent func(string, string)) error {
	localHostName := "localhost"
	if cfg != nil {
		localHostName = cfg.GetLocalHostName()
	}

	dockerProvider, err := docker.NewProvider(localHostName)
	if err != nil {
		return fmt.Errorf("failed to create Docker provider: %w", err)
	}
	defer dockerProvider.Close()

	svc, err := dockerProvider.GetService(req.ContainerName)
	if err != nil {
		return fmt.Errorf("failed to get service: %w", err)
	}

	sendEvent("status", fmt.Sprintf("Restarting container %s...", req.ContainerName))
	return svc.Restart(ctx)
}

// findComposeFile looks for docker-compose.yml or compose.yml in the given directory.
func findComposeFile(dir string) string {
	candidates := []string{
		filepath.Join(dir, "docker-compose.yml"),
		filepath.Join(dir, "docker-compose.yaml"),
		filepath.Join(dir, "compose.yml"),
		filepath.Join(dir, "compose.yaml"),
	}
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

// handleSystemdAction performs systemd service actions.
func handleSystemdAction(ctx context.Context, cfg *config.Config, req ServiceActionRequest, action string, sendEvent func(string, string)) error {
	// Find the host config
	hostAddress := "localhost"
	if cfg != nil {
		if host := cfg.GetHostByName(req.Host); host != nil {
			hostAddress = host.Address
		}
	}

	systemdProvider := systemd.NewProvider(req.Host, hostAddress, []string{req.ServiceName})
	svc, err := systemdProvider.GetService(req.ServiceName)
	if err != nil {
		return fmt.Errorf("failed to get service: %w", err)
	}

	sendEvent("status", fmt.Sprintf("Executing %s on unit %s...", action, req.ServiceName))

	switch action {
	case "start":
		err = svc.Start(ctx)
	case "stop":
		err = svc.Stop(ctx)
	case "restart":
		err = svc.Restart(ctx)
	}

	if err != nil {
		return fmt.Errorf("failed to %s unit: %w", action, err)
	}

	return nil
}
