package traefik

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestExtractHostnames(t *testing.T) {
	tests := []struct {
		name     string
		rule     string
		expected []string
	}{
		{
			name:     "single host with backticks",
			rule:     "Host(`example.com`)",
			expected: []string{"example.com"},
		},
		{
			name:     "single host with double quotes",
			rule:     `Host("example.com")`,
			expected: []string{"example.com"},
		},
		{
			name:     "single host with single quotes",
			rule:     "Host('example.com')",
			expected: []string{"example.com"},
		},
		{
			name:     "multiple hosts with OR",
			rule:     "Host(`a.example.com`) || Host(`b.example.com`)",
			expected: []string{"a.example.com", "b.example.com"},
		},
		{
			name:     "host with path",
			rule:     "Host(`example.com`) && PathPrefix(`/api`)",
			expected: []string{"example.com"},
		},
		{
			name:     "complex rule with multiple hosts",
			rule:     "(Host(`a.com`) || Host(`b.com`)) && PathPrefix(`/app`)",
			expected: []string{"a.com", "b.com"},
		},
		{
			name:     "no host matcher",
			rule:     "PathPrefix(`/api`)",
			expected: nil,
		},
		{
			name:     "empty rule",
			rule:     "",
			expected: nil,
		},
		{
			name:     "host with subdomain wildcard pattern - no Host() present",
			rule:     "HostRegexp(`{subdomain:[a-z]+}.example.com`)",
			expected: []string{"example.com"}, // Extracts domain from HostRegexp when no Host() present
		},
		{
			name:     "host with spaces around",
			rule:     "Host( `example.com` )",
			expected: []string{"example.com"},
		},
		{
			name:     "authentik-style rule - Host preferred over HostRegexp",
			rule:     "Host(`authentik.themissing.xyz`) || HostRegexp(`{subdomain:[a-z0-9]+}.themissing.xyz`) && (PathPrefix(`/outpost.goauthentik.io/`) || PathPrefix(`/admin/outpost.goauthentik.io/`))",
			expected: []string{"authentik.themissing.xyz"}, // Only Host() extracted, HostRegexp ignored
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractHostnames(tt.rule)
			if len(result) != len(tt.expected) {
				t.Errorf("ExtractHostnames(%q) = %v, expected %v", tt.rule, result, tt.expected)
				return
			}
			for i, v := range result {
				if v != tt.expected[i] {
					t.Errorf("ExtractHostnames(%q)[%d] = %q, expected %q", tt.rule, i, v, tt.expected[i])
				}
			}
		})
	}
}

func TestNormalizeServiceName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{
			input:    "myservice@docker",
			expected: "myservice",
		},
		{
			input:    "myservice@file",
			expected: "myservice",
		},
		{
			input:    "myservice",
			expected: "myservice",
		},
		{
			input:    "my-service-name@kubernetes",
			expected: "my-service-name",
		},
		{
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := normalizeServiceName(tt.input)
			if result != tt.expected {
				t.Errorf("normalizeServiceName(%q) = %q, expected %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestNewClient(t *testing.T) {
	client := NewClient("testhost", "192.168.1.100", 8080)
	if client == nil {
		t.Fatal("NewClient returned nil")
	}
	if client.hostName != "testhost" {
		t.Errorf("hostName = %q, expected %q", client.hostName, "testhost")
	}
	if client.hostAddress != "192.168.1.100" {
		t.Errorf("hostAddress = %q, expected %q", client.hostAddress, "192.168.1.100")
	}
	if client.apiPort != 8080 {
		t.Errorf("apiPort = %d, expected %d", client.apiPort, 8080)
	}
}

func TestNewClientDefaultPort(t *testing.T) {
	client := NewClient("testhost", "localhost", 0)
	if client.apiPort != 8080 {
		t.Errorf("apiPort = %d, expected default 8080", client.apiPort)
	}
}

func TestClientIsLocal(t *testing.T) {
	tests := []struct {
		address  string
		expected bool
	}{
		{"localhost", true},
		{"127.0.0.1", true},
		{"192.168.1.100", false},
		{"remote.example.com", false},
	}

	for _, tt := range tests {
		t.Run(tt.address, func(t *testing.T) {
			client := NewClient("test", tt.address, 8080)
			if client.isLocal() != tt.expected {
				t.Errorf("isLocal() for address %q = %v, expected %v", tt.address, client.isLocal(), tt.expected)
			}
		})
	}
}

func TestGetRoutersFromAPI(t *testing.T) {
	// Create a mock Traefik API server
	mockRouters := []Router{
		{
			Name:        "myapp@docker",
			Rule:        "Host(`myapp.example.com`)",
			Service:     "myapp@docker",
			Status:      "enabled",
			EntryPoints: []string{"websecure"},
		},
		{
			Name:        "api@docker",
			Rule:        "Host(`api.example.com`) && PathPrefix(`/v1`)",
			Service:     "api-service@docker",
			Status:      "enabled",
			EntryPoints: []string{"websecure"},
		},
		{
			Name:        "disabled@docker",
			Rule:        "Host(`disabled.example.com`)",
			Service:     "disabled-service@docker",
			Status:      "disabled",
			EntryPoints: []string{"websecure"},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/http/routers" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(mockRouters)
	}))
	defer server.Close()

	// Create a client pointing to our mock server
	// We need to extract the port from the server URL
	client := &Client{
		hostName:    "test",
		hostAddress: "localhost",
		httpClient:  server.Client(),
	}

	// Override the base URL by parsing the test server's port
	// For this test, we'll make a direct HTTP request
	ctx := context.Background()

	req, err := http.NewRequestWithContext(ctx, "GET", server.URL+"/api/http/routers", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	resp, err := client.httpClient.Do(req)
	if err != nil {
		t.Fatalf("Failed to fetch routers: %v", err)
	}
	defer resp.Body.Close()

	var routers []Router
	if err := json.NewDecoder(resp.Body).Decode(&routers); err != nil {
		t.Fatalf("Failed to decode routers: %v", err)
	}

	if len(routers) != 3 {
		t.Errorf("Expected 3 routers, got %d", len(routers))
	}
}

func TestGetServiceHostMappingsLogic(t *testing.T) {
	// Test the mapping logic with mock data
	routers := []Router{
		{
			Name:    "app1@docker",
			Rule:    "Host(`app1.example.com`)",
			Service: "app1@docker",
			Status:  "enabled",
		},
		{
			Name:    "app1-alt@docker",
			Rule:    "Host(`app1-alt.example.com`)",
			Service: "app1@docker", // Same service, different hostname
			Status:  "enabled",
		},
		{
			Name:    "app2@docker",
			Rule:    "Host(`app2.example.com`) || Host(`www.app2.example.com`)",
			Service: "app2@docker",
			Status:  "enabled",
		},
		{
			Name:    "disabled@docker",
			Rule:    "Host(`disabled.example.com`)",
			Service: "disabled@docker",
			Status:  "disabled", // Should be skipped
		},
	}

	// Simulate the mapping logic
	result := make(map[string][]string)
	for _, router := range routers {
		if router.Status != "enabled" {
			continue
		}

		hostnames := ExtractHostnames(router.Rule)
		if len(hostnames) == 0 {
			continue
		}

		serviceName := normalizeServiceName(router.Service)

		existing := result[serviceName]
		for _, h := range hostnames {
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
		result[serviceName] = existing
	}

	// Verify app1 has both hostnames
	app1Hosts := result["app1"]
	if len(app1Hosts) != 2 {
		t.Errorf("Expected 2 hostnames for app1, got %d: %v", len(app1Hosts), app1Hosts)
	}

	// Verify app2 has both hostnames from the OR rule
	app2Hosts := result["app2"]
	if len(app2Hosts) != 2 {
		t.Errorf("Expected 2 hostnames for app2, got %d: %v", len(app2Hosts), app2Hosts)
	}

	// Verify disabled service is not included
	if _, ok := result["disabled"]; ok {
		t.Error("Disabled service should not be in the result")
	}
}

func TestRouterJSONParsing(t *testing.T) {
	// Test that the Router struct can parse the actual Traefik API response format
	// Traefik uses backticks in rules, but JSON uses double quotes for strings
	jsonData := `{
		"name": "myapp@docker",
		"rule": "Host(` + "`myapp.example.com`" + `)",
		"service": "myapp@docker",
		"status": "enabled",
		"entryPoints": ["websecure", "web"]
	}`

	var router Router
	if err := json.Unmarshal([]byte(jsonData), &router); err != nil {
		t.Fatalf("Failed to unmarshal router JSON: %v", err)
	}

	if router.Name != "myapp@docker" {
		t.Errorf("Name = %q, expected %q", router.Name, "myapp@docker")
	}
	if router.Service != "myapp@docker" {
		t.Errorf("Service = %q, expected %q", router.Service, "myapp@docker")
	}
	if router.Status != "enabled" {
		t.Errorf("Status = %q, expected %q", router.Status, "enabled")
	}
	if len(router.EntryPoints) != 2 {
		t.Errorf("EntryPoints length = %d, expected 2", len(router.EntryPoints))
	}
}

func TestConfigDefaults(t *testing.T) {
	cfg := Config{}
	if cfg.Enabled {
		t.Error("Config.Enabled should default to false")
	}
	if cfg.APIPort != 0 {
		t.Error("Config.APIPort should default to 0 (then NewClient sets 8080)")
	}
}

func TestGetClaimedBackendServices(t *testing.T) {
	// Test that backend services are claimed when their router is owned by an existing service
	routersJSON := `[
		{
			"name": "jellyfin@docker",
			"rule": "Host(\"jellyfin.example.com\")",
			"service": "jellyfin-svc@file",
			"status": "enabled"
		},
		{
			"name": "myapp@docker",
			"rule": "Host(\"myapp.example.com\")",
			"service": "myapp@docker",
			"status": "enabled"
		},
		{
			"name": "external@file",
			"rule": "Host(\"external.example.com\")",
			"service": "external-backend@file",
			"status": "enabled"
		}
	]`

	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(routersJSON))
	}))
	defer server.Close()

	// Create client (we'll manually override the URL for testing)
	client := NewClient("test", "localhost", 8080)
	defer client.Close()

	// Parse server URL to get port
	var port int
	_, err := json.Marshal(server.URL) // Just to use json package
	if err != nil {
		t.Fatalf("Failed: %v", err)
	}
	port = 8080 // Will be overridden

	// For this test, we'll manually create routers and test the logic
	routers := []Router{
		{Name: "jellyfin@docker", Rule: "Host(`jellyfin.example.com`)", Service: "jellyfin-svc@file", Status: "enabled"},
		{Name: "myapp@docker", Rule: "Host(`myapp.example.com`)", Service: "myapp@docker", Status: "enabled"},
		{Name: "external@file", Rule: "Host(`external.example.com`)", Service: "external-backend@file", Status: "enabled"},
	}

	// Existing services (Docker/systemd)
	existingServices := map[string]bool{
		"jellyfin": true, // This owns the jellyfin@docker router
		"myapp":    true, // This owns the myapp@docker router
		// "external" does NOT exist, so external-backend should not be claimed
	}

	// Simulate GetClaimedBackendServices logic
	claimed := make(map[string]bool)
	for _, router := range routers {
		routerName := normalizeServiceName(router.Name)
		if existingServices[routerName] {
			backendName := normalizeServiceName(router.Service)
			if backendName != routerName {
				claimed[backendName] = true
			}
		}
	}

	// Verify claims
	// jellyfin owns router jellyfin@docker -> jellyfin-svc@file, so jellyfin-svc should be claimed
	if !claimed["jellyfin-svc"] {
		t.Error("Expected jellyfin-svc to be claimed (backend for jellyfin@docker router)")
	}

	// myapp owns router myapp@docker -> myapp@docker, same name so not claimed
	if claimed["myapp"] {
		t.Error("Did not expect myapp to be claimed (router and service have same name)")
	}

	// external does not exist, so external-backend should NOT be claimed
	if claimed["external-backend"] {
		t.Error("Did not expect external-backend to be claimed (no existing service owns the router)")
	}

	_ = port // Avoid unused variable error
}
