// Package handlers provides HTTP handlers for the dashboard API.
package handlers

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
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

	"home_server_dashboard/auth"
	"home_server_dashboard/config"
	"home_server_dashboard/query"
	"home_server_dashboard/services"
	"home_server_dashboard/services/docker"
	"home_server_dashboard/services/homeassistant"
	"home_server_dashboard/services/systemd"
	"home_server_dashboard/services/traefik"
)

// Embedded filesystems (set by server package)
var (
	embeddedStaticFS fs.FS
	embeddedDocsFS   fs.FS
)

// SetEmbeddedFS sets the embedded filesystems for serving static content.
func SetEmbeddedFS(staticFS, docsFS fs.FS) {
	embeddedStaticFS = staticFS
	embeddedDocsFS = docsFS
}

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

	// Get Home Assistant services from each configured host
	for _, host := range cfg.Hosts {
		if !host.HasHomeAssistant() {
			continue
		}

		haProvider, err := homeassistant.NewProvider(&host)
		if err != nil {
			log.Printf("Warning: failed to create Home Assistant provider for %s: %v", host.Name, err)
			continue
		}
		if haProvider == nil {
			continue
		}
		defer haProvider.Close()

		haServices, err := haProvider.GetServices(ctx)
		if err != nil {
			log.Printf("Warning: failed to get Home Assistant services from %s: %v", host.Name, err)
			continue
		}
		// Set HostIP for each HA service
		for i := range haServices {
			haServices[i].HostIP = hostIPMap[haServices[i].Host]
		}
		allServices = append(allServices, haServices...)
	}

	// Apply port remapping (move ports from source services to target services)
	allServices = applyPortRemaps(allServices, allPortRemaps)

	// Enrich services with Traefik hostnames
	allServices = enrichWithTraefikURLs(ctx, cfg, allServices)

	// Get Traefik-only services (services registered in Traefik but not in Docker/systemd)
	// Build a set of existing service names to filter out duplicates
	// We need to track multiple possible names that Traefik might use:
	// - The actual service name
	// - The TraefikServiceName from Docker labels
	// - Common Traefik naming patterns like servicename-projectname
	// - Normalized names (underscores to hyphens, as Traefik does)
	existingServices := make(map[string]bool)
	for _, svc := range allServices {
		existingServices[svc.Name] = true
		// Also add normalized name (Traefik converts underscores to hyphens)
		normalizedName := normalizeForTraefik(svc.Name)
		if normalizedName != svc.Name {
			existingServices[normalizedName] = true
		}
		// Also add TraefikServiceName if present (from Docker labels)
		if svc.TraefikServiceName != "" {
			existingServices[svc.TraefikServiceName] = true
		}
		// Add common Traefik naming pattern: servicename-projectname (used by Docker provider)
		if svc.Project != "" && svc.Project != "systemd" && svc.Project != "traefik" {
			existingServices[svc.Name+"-"+svc.Project] = true
			// Also add normalized version
			existingServices[normalizedName+"-"+svc.Project] = true
		}
		// Also add container name as a possible match
		if svc.ContainerName != "" {
			existingServices[svc.ContainerName] = true
			// And normalized container name
			normalizedContainerName := normalizeForTraefik(svc.ContainerName)
			if normalizedContainerName != svc.ContainerName {
				existingServices[normalizedContainerName] = true
			}
		}
	}

	// Get Traefik services from each host with Traefik enabled
	for _, host := range cfg.Hosts {
		if !host.Traefik.Enabled {
			continue
		}

		traefikProvider := traefik.NewProvider(host.Name, host.Address, host.Traefik.APIPort)
		defer traefikProvider.Close()

		traefikServices, err := traefikProvider.GetServices(ctx, existingServices)
		if err != nil {
			log.Printf("Warning: failed to get Traefik services from %s: %v", host.Name, err)
			continue
		}

		// Set HostIP for each Traefik service
		for i := range traefikServices {
			traefikServices[i].HostIP = hostIPMap[traefikServices[i].Host]
		}

		allServices = append(allServices, traefikServices...)

		// Add these services to existing set to avoid duplicates from other hosts
		for _, svc := range traefikServices {
			existingServices[svc.Name] = true
		}
	}

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

// normalizeForTraefik converts a name to match Traefik's naming convention.
// Traefik converts underscores to hyphens in service names from Docker.
func normalizeForTraefik(name string) string {
	return strings.ReplaceAll(name, "_", "-")
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

		// Try with normalized name (underscores to hyphens, as Traefik does)
		if len(hostnames) == 0 {
			normalizedName := normalizeForTraefik(svc.Name)
			if normalizedName != svc.Name {
				hostnames = traefikMappings[normalizedName]
			}
		}

		// Traefik often names services as "servicename-projectname", so try that pattern
		if len(hostnames) == 0 && svc.Project != "" && svc.Project != "systemd" {
			traefikServiceName := svc.Name + "-" + svc.Project
			hostnames = traefikMappings[traefikServiceName]
		}

		// Try the same pattern but with normalized name (underscores to hyphens)
		if len(hostnames) == 0 && svc.Project != "" && svc.Project != "systemd" {
			traefikServiceName := normalizeForTraefik(svc.Name) + "-" + svc.Project
			hostnames = traefikMappings[traefikServiceName]
		}

		// Also try with container name for containers that might use that
		if len(hostnames) == 0 && svc.ContainerName != "" {
			hostnames = traefikMappings[svc.ContainerName]
		}

		// Try normalized container name
		if len(hostnames) == 0 && svc.ContainerName != "" {
			normalizedContainerName := normalizeForTraefik(svc.ContainerName)
			if normalizedContainerName != svc.ContainerName {
				hostnames = traefikMappings[normalizedContainerName]
			}
		}

		// Convert hostnames to full URLs (https by default since Traefik usually terminates TLS)
		for _, hostname := range hostnames {
			url := "https://" + hostname
			svc.TraefikURLs = append(svc.TraefikURLs, url)
		}
	}

	return svcList
}

// filterServicesForUser returns only the services the user is allowed to access.
func filterServicesForUser(svcList []services.ServiceInfo, user *auth.User) []services.ServiceInfo {
	// If user has global access, return all services
	if user == nil || user.HasGlobalAccess {
		return svcList
	}

	// Filter services based on user's allowed services
	var filtered []services.ServiceInfo
	for _, svc := range svcList {
		if user.CanAccessService(svc.Host, svc.Name) {
			filtered = append(filtered, svc)
		}
	}
	return filtered
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

	// Filter services based on user permissions
	user := auth.GetUserFromContext(r.Context())
	svcList = filterServicesForUser(svcList, user)

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

	// Check user permissions
	user := auth.GetUserFromContext(r.Context())
	if user != nil && !user.CanAccessService(hostName, unitName) {
		http.Error(w, "Access denied: you do not have permission to view logs for this service", http.StatusForbidden)
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

// TraefikLogsHandler handles GET /api/logs/traefik requests.
// Traefik services don't support log streaming, so this returns a stub message.
func TraefikLogsHandler(w http.ResponseWriter, r *http.Request) {
	serviceName := r.URL.Query().Get("service")
	hostName := r.URL.Query().Get("host")
	if serviceName == "" {
		http.Error(w, "service parameter required", http.StatusBadRequest)
		return
	}

	// Check user permissions
	user := auth.GetUserFromContext(r.Context())
	if user != nil && !user.CanAccessService(hostName, serviceName) {
		http.Error(w, "Access denied: you do not have permission to view logs for this service", http.StatusForbidden)
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

	// Send a message explaining that logs are not supported for Traefik services
	fmt.Fprintf(w, "data: ═══════════════════════════════════════════════════════════════\n\n")
	flusher.Flush()
	fmt.Fprintf(w, "data: Logs are not supported for Traefik services.\n\n")
	flusher.Flush()
	fmt.Fprintf(w, "data: \n\n")
	flusher.Flush()
	fmt.Fprintf(w, "data: Traefik services are external services registered in Traefik's\n\n")
	flusher.Flush()
	fmt.Fprintf(w, "data: routing configuration. They may be running on external hosts,\n\n")
	flusher.Flush()
	fmt.Fprintf(w, "data: load balancers, or other infrastructure not managed by this\n\n")
	flusher.Flush()
	fmt.Fprintf(w, "data: dashboard.\n\n")
	flusher.Flush()
	fmt.Fprintf(w, "data: \n\n")
	flusher.Flush()
	fmt.Fprintf(w, "data: To view logs for this service, please check the host where the\n\n")
	flusher.Flush()
	fmt.Fprintf(w, "data: actual service is running.\n\n")
	flusher.Flush()
	fmt.Fprintf(w, "data: ═══════════════════════════════════════════════════════════════\n\n")
	flusher.Flush()

	// Keep connection open briefly so client receives all messages
	<-r.Context().Done()
}

// HomeAssistantLogsHandler handles GET /api/logs/homeassistant requests.
// For HAOS installations with Supervisor API access, streams logs from Core, Supervisor, Host, or Addons.
// For standard HA installations, returns a stub message.
func HomeAssistantLogsHandler(w http.ResponseWriter, r *http.Request) {
	serviceName := r.URL.Query().Get("service")
	hostName := r.URL.Query().Get("host")
	if serviceName == "" {
		serviceName = "homeassistant"
	}

	// Check user permissions
	user := auth.GetUserFromContext(r.Context())
	if user != nil && !user.CanAccessService(hostName, serviceName) {
		http.Error(w, "Access denied: you do not have permission to view logs for this service", http.StatusForbidden)
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

	// Find the Home Assistant provider for this host
	cfg := config.Get()
	var provider *homeassistant.Provider
	for _, host := range cfg.Hosts {
		if host.Name == hostName && host.HasHomeAssistant() {
			var err error
			provider, err = homeassistant.NewProvider(&host)
			if err != nil {
				log.Printf("Failed to create HA provider for %s: %v", hostName, err)
			}
			break
		}
	}
	if provider != nil {
		defer provider.Close()
	}

	// Get logs from the provider
	if provider != nil {
		logs, err := provider.GetLogs(r.Context(), serviceName, 100, true)
		if err != nil {
			log.Printf("Failed to get logs for %s: %v", serviceName, err)
			fmt.Fprintf(w, "data: Error getting logs: %v\n\n", err)
			flusher.Flush()
		} else {
			defer logs.Close()
			scanner := bufio.NewScanner(logs)
			for scanner.Scan() {
				select {
				case <-r.Context().Done():
					return
				default:
					line := scanner.Text()
					fmt.Fprintf(w, "data: %s\n\n", line)
					flusher.Flush()
				}
			}
			if err := scanner.Err(); err != nil {
				log.Printf("Error scanning logs: %v", err)
			}
			return
		}
	}

	// Fallback: Send a message explaining that logs are not supported
	fmt.Fprintf(w, "data: ═══════════════════════════════════════════════════════════════\n\n")
	flusher.Flush()
	fmt.Fprintf(w, "data: Logs are not available for Home Assistant via this dashboard.\n\n")
	flusher.Flush()
	fmt.Fprintf(w, "data: \n\n")
	flusher.Flush()
	fmt.Fprintf(w, "data: Home Assistant logs can be viewed through:\n\n")
	flusher.Flush()
	fmt.Fprintf(w, "data: - The Home Assistant web UI: Settings → System → Logs\n\n")
	flusher.Flush()
	fmt.Fprintf(w, "data: - The Home Assistant CLI: ha core logs\n\n")
	flusher.Flush()
	fmt.Fprintf(w, "data: - Direct file access: /config/home-assistant.log\n\n")
	flusher.Flush()
	fmt.Fprintf(w, "data: \n\n")
	flusher.Flush()
	fmt.Fprintf(w, "data: ═══════════════════════════════════════════════════════════════\n\n")
	flusher.Flush()

	// Keep connection open briefly so client receives all messages
	<-r.Context().Done()
}

// DockerLogsHandler handles GET /api/logs requests for streaming Docker container logs.
func DockerLogsHandler(w http.ResponseWriter, r *http.Request) {
	containerName := r.URL.Query().Get("container")
	serviceName := r.URL.Query().Get("service")
	if containerName == "" {
		http.Error(w, "container parameter required", http.StatusBadRequest)
		return
	}

	cfg := config.Get()
	localHostName := "localhost"
	if cfg != nil {
		localHostName = cfg.GetLocalHostName()
	}

	// Check user permissions - use service name if provided, otherwise container name
	checkName := serviceName
	if checkName == "" {
		checkName = containerName
	}
	user := auth.GetUserFromContext(r.Context())
	if user != nil && !user.CanAccessService(localHostName, checkName) {
		http.Error(w, "Access denied: you do not have permission to view logs for this service", http.StatusForbidden)
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
		// Try embedded filesystem first
		if embeddedStaticFS != nil {
			content, err := fs.ReadFile(embeddedStaticFS, "index.html")
			if err == nil {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.Write(content)
				return
			}
		}
		// Fallback to filesystem for development
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
	var mdContent []byte
	var err error

	// Try embedded filesystem first
	if embeddedDocsFS != nil {
		mdContent, err = fs.ReadFile(embeddedDocsFS, "bangandpipe-query-language.md")
	}

	// Fallback to filesystem for development
	if err != nil || embeddedDocsFS == nil {
		mdContent, err = os.ReadFile("docs/bangandpipe-query-language.md")
		if err != nil {
			http.Error(w, "Documentation not found", http.StatusNotFound)
			return
		}
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

	// Check user permissions
	user := auth.GetUserFromContext(r.Context())
	if user != nil && !user.CanAccessService(req.Host, req.ServiceName) {
		http.Error(w, "Access denied: you do not have permission to control this service", http.StatusForbidden)
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
	} else if req.Source == "traefik" {
		err = handleTraefikAction(ctx, cfg, req, action, sendEvent)
	} else if req.Source == "homeassistant" || req.Source == "homeassistant-addon" {
		err = handleHomeAssistantAction(ctx, cfg, req, action, sendEvent)
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

// handleTraefikAction handles actions for Traefik services (not supported).
func handleTraefikAction(ctx context.Context, cfg *config.Config, req ServiceActionRequest, action string, sendEvent func(string, string)) error {
	sendEvent("status", "Traefik services cannot be controlled from this dashboard.")
	sendEvent("status", "")
	sendEvent("status", "Traefik services are external services registered in Traefik's")
	sendEvent("status", "routing configuration. They may be running on external hosts,")
	sendEvent("status", "load balancers, or other infrastructure not managed by this dashboard.")
	sendEvent("status", "")
	sendEvent("status", fmt.Sprintf("To %s this service, please access the host where the", action))
	sendEvent("status", "actual service is running.")

	return fmt.Errorf("%s is not supported for Traefik services - these are external services managed outside this dashboard", action)
}

// handleHomeAssistantAction handles actions for Home Assistant services.
// Supports: homeassistant core (restart only), ha-supervisor (no actions), ha-host (no actions), addon-* (start/stop/restart)
func handleHomeAssistantAction(ctx context.Context, cfg *config.Config, req ServiceActionRequest, action string, sendEvent func(string, string)) error {
	if cfg == nil {
		return fmt.Errorf("configuration not loaded")
	}

	// Find the host config
	host := cfg.GetHostByName(req.Host)
	if host == nil {
		return fmt.Errorf("host not found: %s", req.Host)
	}

	if !host.HasHomeAssistant() {
		return fmt.Errorf("Home Assistant not configured for host: %s", req.Host)
	}

	haProvider, err := homeassistant.NewProvider(host)
	if err != nil {
		return fmt.Errorf("failed to create Home Assistant provider: %w", err)
	}
	if haProvider == nil {
		return fmt.Errorf("Home Assistant provider is nil for host: %s", req.Host)
	}
	defer haProvider.Close()

	// Handle addon services
	if strings.HasPrefix(req.ServiceName, "addon-") {
		if !haProvider.HasSupervisorAPI() {
			return fmt.Errorf("addon control requires HAOS with Supervisor API access")
		}

		slug := strings.TrimPrefix(req.ServiceName, "addon-")
		sendEvent("status", fmt.Sprintf("Executing %s on addon %s...", action, slug))

		if err := haProvider.AddonControl(ctx, slug, action); err != nil {
			return fmt.Errorf("failed to %s addon %s: %w", action, slug, err)
		}

		sendEvent("status", fmt.Sprintf("Addon %s %s command sent successfully", slug, action))
		return nil
	}

	// Handle supervisor and host services (no actions supported)
	if req.ServiceName == "ha-supervisor" {
		return fmt.Errorf("%s is not supported for Supervisor - it is managed by HAOS", action)
	}
	if req.ServiceName == "ha-host" {
		return fmt.Errorf("%s is not supported for Host - use HAOS interface for host control", action)
	}

	// Handle core Home Assistant service
	switch action {
	case "restart":
		sendEvent("status", "Triggering Home Assistant restart...")
		sendEvent("status", "")
		sendEvent("status", "Note: Home Assistant will restart and briefly become unavailable.")
		sendEvent("status", "This typically takes 30-60 seconds to complete.")
		sendEvent("status", "")

		if err := haProvider.Restart(ctx); err != nil {
			return fmt.Errorf("failed to restart Home Assistant: %w", err)
		}

		sendEvent("status", "Restart command sent successfully.")
		sendEvent("status", "Home Assistant is now restarting...")
		return nil

	case "start":
		sendEvent("status", "Start is not supported for Home Assistant.")
		sendEvent("status", "")
		sendEvent("status", "If Home Assistant is down, check the host where it's running.")
		sendEvent("status", "You may need to SSH into the host or check the hardware.")
		return fmt.Errorf("start is not supported for Home Assistant - if it's down, check the host")

	case "stop":
		sendEvent("status", "Stop is not supported for Home Assistant via this dashboard.")
		sendEvent("status", "")
		sendEvent("status", "Stopping Home Assistant would disable home automation.")
		sendEvent("status", "If you need to stop it, use the HA web UI or CLI:")
		sendEvent("status", "  ha core stop")
		return fmt.Errorf("stop is not supported for Home Assistant - use the HA web UI if needed")

	default:
		return fmt.Errorf("unknown action: %s", action)
	}
}

// LogFlushRequest represents the request body for flushing logs.
type LogFlushRequest struct {
	ContainerName string `json:"container_name"`
	ServiceName   string `json:"service_name"`
	Host          string `json:"host"`
}

// LogFlushHandler handles POST /api/logs/flush requests for truncating Docker logs.
// Only administrators can flush logs.
func LogFlushHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Check user permissions - only admins can flush logs
	user := auth.GetUserFromContext(r.Context())
	if user == nil || !user.IsAdmin {
		http.Error(w, "Access denied: administrator privileges required to flush logs", http.StatusForbidden)
		return
	}

	// Parse request body
	var req LogFlushRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.ContainerName == "" {
		http.Error(w, "container_name is required", http.StatusBadRequest)
		return
	}

	cfg := config.Get()
	localHostName := "localhost"
	if cfg != nil {
		localHostName = cfg.GetLocalHostName()
	}

	// Create Docker provider
	dockerProvider, err := docker.NewProvider(localHostName)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to create Docker provider: %v", err), http.StatusInternalServerError)
		return
	}
	defer dockerProvider.Close()

	// Truncate logs
	err = dockerProvider.TruncateLogs(r.Context(), req.ContainerName)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to flush logs: %v", err), http.StatusInternalServerError)
		return
	}

	log.Printf("Admin %s flushed logs for container %s", user.Email, req.ContainerName)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "success",
		"message": fmt.Sprintf("Logs flushed for %s", req.ContainerName),
	})
}
