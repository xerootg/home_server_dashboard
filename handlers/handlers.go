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
)

// getAllServices collects services from all configured providers.
func getAllServices(ctx context.Context, cfg *config.Config) ([]services.ServiceInfo, error) {
	var allServices []services.ServiceInfo

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
		allServices = append(allServices, systemdServices...)
	}

	return allServices, nil
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
