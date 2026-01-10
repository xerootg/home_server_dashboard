package gotify

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/gotify/go-api-client/v2/gotify"
	"github.com/gotify/go-api-client/v2/models"

	"home_server_dashboard/config"
	"home_server_dashboard/events"
)

// newTestNotifier creates a Notifier connected to a test server.
// The handler receives the MessageExternal that was sent.
func newTestNotifier(t *testing.T, handler func(*models.MessageExternal)) (*Notifier, *httptest.Server) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Gotify client uses POST /message with X-Gotify-Key header
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/message" {
			t.Errorf("expected /message path, got %s", r.URL.Path)
		}
		if r.Header.Get("X-Gotify-Key") != "test-token" {
			t.Errorf("expected X-Gotify-Key header 'test-token', got '%s'", r.Header.Get("X-Gotify-Key"))
		}

		var msg models.MessageExternal
		if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
			t.Errorf("failed to decode message: %v", err)
		}

		if handler != nil {
			handler(&msg)
		}

		// Return a valid response
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(&models.MessageExternal{ID: 1})
	}))

	parsedURL, _ := url.Parse(server.URL)
	notifier := &Notifier{
		client:   gotify.NewClient(parsedURL, server.Client()),
		token:    "test-token",
		hostname: server.URL,
	}

	return notifier, server
}

func TestNew_NilConfig(t *testing.T) {
	n := New(nil)
	if n != nil {
		t.Error("expected nil notifier for nil config")
	}
}

func TestNew_DisabledConfig(t *testing.T) {
	cfg := &config.GotifyConfig{
		Enabled:  false,
		Hostname: "https://gotify.example.com",
		Token:    "test-token",
	}
	n := New(cfg)
	if n != nil {
		t.Error("expected nil notifier for disabled config")
	}
}

func TestNew_IncompleteConfig(t *testing.T) {
	tests := []struct {
		name string
		cfg  *config.GotifyConfig
	}{
		{
			name: "missing hostname",
			cfg:  &config.GotifyConfig{Enabled: true, Token: "token"},
		},
		{
			name: "missing token",
			cfg:  &config.GotifyConfig{Enabled: true, Hostname: "https://gotify.example.com"},
		},
		{
			name: "empty config",
			cfg:  &config.GotifyConfig{Enabled: true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := New(tt.cfg)
			if n != nil {
				t.Error("expected nil notifier for incomplete config")
			}
		})
	}
}

func TestNew_ValidConfig(t *testing.T) {
	cfg := &config.GotifyConfig{
		Enabled:  true,
		Hostname: "https://gotify.example.com",
		Token:    "test-token",
	}
	n := New(cfg)
	if n == nil {
		t.Fatal("expected non-nil notifier for valid config")
	}
	if n.Name() != "gotify" {
		t.Errorf("expected name 'gotify', got '%s'", n.Name())
	}
}

func TestNew_TrailingSlashRemoved(t *testing.T) {
	cfg := &config.GotifyConfig{
		Enabled:  true,
		Hostname: "https://gotify.example.com/",
		Token:    "test-token",
	}
	n := New(cfg)
	if n == nil {
		t.Fatal("expected non-nil notifier")
	}
	if n.hostname != "https://gotify.example.com" {
		t.Errorf("expected trailing slash removed, got '%s'", n.hostname)
	}
}

func TestNotify_ServiceStateChanged(t *testing.T) {
	var receivedMsg *models.MessageExternal

	n, server := newTestNotifier(t, func(msg *models.MessageExternal) {
		receivedMsg = msg
	})
	defer server.Close()

	event := events.NewServiceStateChangedEvent("nas", "traefik", "docker", "stopped", "running", "Up 5 minutes")
	err := n.Notify(event)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if receivedMsg == nil {
		t.Fatal("expected message to be sent")
	}
	if receivedMsg.Title == "" {
		t.Error("expected non-empty title")
	}
	if receivedMsg.Message == "" {
		t.Error("expected non-empty message")
	}
}

func TestNotify_ServiceStopped_HighPriority(t *testing.T) {
	var receivedMsg *models.MessageExternal

	n, server := newTestNotifier(t, func(msg *models.MessageExternal) {
		receivedMsg = msg
	})
	defer server.Close()

	// Service went from running to stopped - should be high priority
	event := events.NewServiceStateChangedEvent("nas", "traefik", "docker", "running", "stopped", "Exited (1)")
	n.Notify(event)

	if receivedMsg == nil {
		t.Fatal("expected message to be sent")
	}
	if receivedMsg.Priority != PriorityHigh {
		t.Errorf("expected priority %d for service stopped, got %d", PriorityHigh, receivedMsg.Priority)
	}
}

func TestNotify_HostUnreachable(t *testing.T) {
	var receivedMsg *models.MessageExternal

	n, server := newTestNotifier(t, func(msg *models.MessageExternal) {
		receivedMsg = msg
	})
	defer server.Close()

	event := events.NewHostUnreachableEvent("remote-host", "connection refused")
	err := n.Notify(event)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if receivedMsg == nil {
		t.Fatal("expected message to be sent")
	}
	if receivedMsg.Priority != PriorityMax {
		t.Errorf("expected priority %d for host unreachable, got %d", PriorityMax, receivedMsg.Priority)
	}
}

func TestNotify_HostRecovered(t *testing.T) {
	var receivedMsg *models.MessageExternal

	n, server := newTestNotifier(t, func(msg *models.MessageExternal) {
		receivedMsg = msg
	})
	defer server.Close()

	event := events.NewHostRecoveredEvent("remote-host")
	err := n.Notify(event)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if receivedMsg == nil {
		t.Fatal("expected message to be sent")
	}
	if receivedMsg.Priority != PriorityHigh {
		t.Errorf("expected priority %d for host recovered, got %d", PriorityHigh, receivedMsg.Priority)
	}
}

func TestNotify_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{"error": "internal error"})
	}))
	defer server.Close()

	parsedURL, _ := url.Parse(server.URL)
	n := &Notifier{
		client:   gotify.NewClient(parsedURL, server.Client()),
		token:    "test-token",
		hostname: server.URL,
	}

	event := events.NewServiceStateChangedEvent("nas", "traefik", "docker", "stopped", "running", "Up")
	err := n.Notify(event)
	if err == nil {
		t.Error("expected error for server error response")
	}
}

func TestNotify_ConnectionError(t *testing.T) {
	// Create a notifier pointing to a non-existent server
	parsedURL, _ := url.Parse("http://localhost:99999")
	n := &Notifier{
		client:   gotify.NewClient(parsedURL, http.DefaultClient),
		token:    "test-token",
		hostname: "http://localhost:99999",
	}

	event := events.NewServiceStateChangedEvent("nas", "traefik", "docker", "stopped", "running", "Up")
	err := n.Notify(event)
	if err == nil {
		t.Error("expected error for connection failure")
	}
}

func TestClose(t *testing.T) {
	cfg := &config.GotifyConfig{
		Enabled:  true,
		Hostname: "https://gotify.example.com",
		Token:    "test-token",
	}
	n := New(cfg)

	// Close should succeed without error
	if err := n.Close(); err != nil {
		t.Errorf("unexpected error from Close: %v", err)
	}
}

func TestSendTest(t *testing.T) {
	var receivedMsg *models.MessageExternal

	n, server := newTestNotifier(t, func(msg *models.MessageExternal) {
		receivedMsg = msg
	})
	defer server.Close()

	err := n.SendTest()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if receivedMsg == nil {
		t.Fatal("expected test message to be sent")
	}
	if receivedMsg.Title == "" {
		t.Error("expected non-empty title in test message")
	}
}
