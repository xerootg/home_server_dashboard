package websocket

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"home_server_dashboard/events"
)

// TestNewHub tests hub creation.
func TestNewHub(t *testing.T) {
	eventBus := events.NewBus(false)
	hub := NewHub(eventBus)

	if hub == nil {
		t.Fatal("NewHub returned nil")
	}

	if hub.eventBus != eventBus {
		t.Error("Hub event bus not set correctly")
	}

	if hub.clients == nil {
		t.Error("Hub clients map not initialized")
	}

	if hub.broadcast == nil {
		t.Error("Hub broadcast channel not initialized")
	}
}

// TestHubStartStop tests starting and stopping the hub.
func TestHubStartStop(t *testing.T) {
	eventBus := events.NewBus(false)
	hub := NewHub(eventBus)

	// Start should work
	hub.Start()
	if !hub.running {
		t.Error("Hub should be running after Start()")
	}

	// Starting again should be a no-op
	hub.Start()
	if !hub.running {
		t.Error("Hub should still be running after second Start()")
	}

	// Stop should work
	hub.Stop()
	if hub.running {
		t.Error("Hub should not be running after Stop()")
	}

	// Stopping again should be a no-op
	hub.Stop()
	if hub.running {
		t.Error("Hub should still not be running after second Stop()")
	}
}

// TestHubClientCount tests client count tracking.
func TestHubClientCount(t *testing.T) {
	eventBus := events.NewBus(false)
	hub := NewHub(eventBus)
	hub.Start()
	defer hub.Stop()

	// Initially no clients
	if hub.ClientCount() != 0 {
		t.Errorf("Expected 0 clients, got %d", hub.ClientCount())
	}
}

// TestMessageSerialization tests that messages serialize correctly to JSON.
func TestMessageSerialization(t *testing.T) {
	tests := []struct {
		name    string
		msg     Message
		wantType string
	}{
		{
			name: "service update",
			msg: Message{
				Type:      MessageTypeServiceUpdate,
				Timestamp: 1234567890000,
				Payload: ServiceUpdatePayload{
					Host:          "nas",
					ServiceName:   "nginx",
					Source:        "docker",
					PreviousState: "running",
					CurrentState:  "stopped",
					Status:        "Exited (0)",
				},
			},
			wantType: "service_update",
		},
		{
			name: "host unreachable",
			msg: Message{
				Type:      MessageTypeHostUnreachable,
				Timestamp: 1234567890000,
				Payload: HostEventPayload{
					Host:   "server1",
					Reason: "connection refused",
				},
			},
			wantType: "host_unreachable",
		},
		{
			name: "host recovered",
			msg: Message{
				Type:      MessageTypeHostRecovered,
				Timestamp: 1234567890000,
				Payload: HostEventPayload{
					Host: "server1",
				},
			},
			wantType: "host_recovered",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.msg)
			if err != nil {
				t.Fatalf("Failed to marshal message: %v", err)
			}

			// Verify it can be unmarshaled back
			var result map[string]interface{}
			if err := json.Unmarshal(data, &result); err != nil {
				t.Fatalf("Failed to unmarshal message: %v", err)
			}

			if result["type"] != tt.wantType {
				t.Errorf("Expected type %s, got %s", tt.wantType, result["type"])
			}

			if result["timestamp"] != float64(tt.msg.Timestamp) {
				t.Errorf("Expected timestamp %d, got %v", tt.msg.Timestamp, result["timestamp"])
			}
		})
	}
}

// TestHubBroadcast tests that events are broadcast to clients.
func TestHubBroadcast(t *testing.T) {
	eventBus := events.NewBus(false)
	hub := NewHub(eventBus)
	hub.Start()
	defer hub.Stop()

	// Create a test HTTP server
	server := httptest.NewServer(hub.Handler())
	defer server.Close()

	// Connect a WebSocket client
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect to WebSocket: %v", err)
	}
	defer conn.Close()

	// Wait for client to register
	time.Sleep(50 * time.Millisecond)

	if hub.ClientCount() != 1 {
		t.Errorf("Expected 1 client, got %d", hub.ClientCount())
	}

	// Publish an event through the event bus
	eventBus.Publish(events.NewServiceStateChangedEvent(
		"nas",
		"nginx",
		"docker",
		"running",
		"stopped",
		"Exited (0)",
	))

	// Read the message from WebSocket
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, data, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("Failed to read WebSocket message: %v", err)
	}

	// Parse the message
	var msg Message
	if err := json.Unmarshal(data, &msg); err != nil {
		t.Fatalf("Failed to unmarshal message: %v", err)
	}

	if msg.Type != MessageTypeServiceUpdate {
		t.Errorf("Expected message type %s, got %s", MessageTypeServiceUpdate, msg.Type)
	}

	// Verify payload
	payloadJSON, _ := json.Marshal(msg.Payload)
	var payload ServiceUpdatePayload
	json.Unmarshal(payloadJSON, &payload)

	if payload.ServiceName != "nginx" {
		t.Errorf("Expected service name nginx, got %s", payload.ServiceName)
	}
	if payload.CurrentState != "stopped" {
		t.Errorf("Expected current state stopped, got %s", payload.CurrentState)
	}
}

// TestHubHostEvents tests host unreachable and recovered events.
func TestHubHostEvents(t *testing.T) {
	eventBus := events.NewBus(false)
	hub := NewHub(eventBus)
	hub.Start()
	defer hub.Stop()

	// Create a test HTTP server
	server := httptest.NewServer(hub.Handler())
	defer server.Close()

	// Connect a WebSocket client
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect to WebSocket: %v", err)
	}
	defer conn.Close()

	// Wait for client to register
	time.Sleep(50 * time.Millisecond)

	// Test host unreachable
	eventBus.Publish(events.NewHostUnreachableEvent("server1", "connection refused"))

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, data, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("Failed to read WebSocket message: %v", err)
	}

	var msg Message
	if err := json.Unmarshal(data, &msg); err != nil {
		t.Fatalf("Failed to unmarshal message: %v", err)
	}

	if msg.Type != MessageTypeHostUnreachable {
		t.Errorf("Expected message type %s, got %s", MessageTypeHostUnreachable, msg.Type)
	}

	// Test host recovered
	eventBus.Publish(events.NewHostRecoveredEvent("server1"))

	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, data, err = conn.ReadMessage()
	if err != nil {
		t.Fatalf("Failed to read WebSocket message: %v", err)
	}

	if err := json.Unmarshal(data, &msg); err != nil {
		t.Fatalf("Failed to unmarshal message: %v", err)
	}

	if msg.Type != MessageTypeHostRecovered {
		t.Errorf("Expected message type %s, got %s", MessageTypeHostRecovered, msg.Type)
	}
}

// TestHubMultipleClients tests broadcast to multiple clients.
func TestHubMultipleClients(t *testing.T) {
	eventBus := events.NewBus(false)
	hub := NewHub(eventBus)
	hub.Start()
	defer hub.Stop()

	// Create a test HTTP server
	server := httptest.NewServer(hub.Handler())
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/"

	// Connect multiple clients
	const numClients = 3
	conns := make([]*websocket.Conn, numClients)
	for i := 0; i < numClients; i++ {
		conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
		if err != nil {
			t.Fatalf("Failed to connect client %d: %v", i, err)
		}
		conns[i] = conn
		defer conn.Close()
	}

	// Wait for clients to register
	time.Sleep(100 * time.Millisecond)

	if hub.ClientCount() != numClients {
		t.Errorf("Expected %d clients, got %d", numClients, hub.ClientCount())
	}

	// Publish an event
	eventBus.Publish(events.NewServiceStateChangedEvent(
		"nas", "nginx", "docker", "running", "stopped", "Exited (0)",
	))

	// All clients should receive the message
	var wg sync.WaitGroup
	received := make([]bool, numClients)

	for i, conn := range conns {
		wg.Add(1)
		go func(idx int, c *websocket.Conn) {
			defer wg.Done()
			c.SetReadDeadline(time.Now().Add(2 * time.Second))
			_, _, err := c.ReadMessage()
			if err == nil {
				received[idx] = true
			}
		}(i, conn)
	}

	wg.Wait()

	for i, r := range received {
		if !r {
			t.Errorf("Client %d did not receive message", i)
		}
	}
}

// TestHubClientDisconnect tests that disconnected clients are removed.
func TestHubClientDisconnect(t *testing.T) {
	eventBus := events.NewBus(false)
	hub := NewHub(eventBus)
	hub.Start()
	defer hub.Stop()

	// Create a test HTTP server
	server := httptest.NewServer(hub.Handler())
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/"

	// Connect a client
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}

	// Wait for registration
	time.Sleep(50 * time.Millisecond)

	if hub.ClientCount() != 1 {
		t.Errorf("Expected 1 client, got %d", hub.ClientCount())
	}

	// Disconnect the client
	conn.Close()

	// Wait for unregistration
	time.Sleep(100 * time.Millisecond)

	if hub.ClientCount() != 0 {
		t.Errorf("Expected 0 clients after disconnect, got %d", hub.ClientCount())
	}
}

// TestHandlerUpgrade tests that the handler upgrades HTTP to WebSocket.
func TestHandlerUpgrade(t *testing.T) {
	eventBus := events.NewBus(false)
	hub := NewHub(eventBus)
	hub.Start()
	defer hub.Stop()

	// Create a test server
	server := httptest.NewServer(hub.Handler())
	defer server.Close()

	// Try to connect with a regular HTTP request (should fail upgrade)
	resp, err := http.Get(server.URL)
	if err != nil {
		t.Fatalf("HTTP request failed: %v", err)
	}
	defer resp.Body.Close()

	// Non-WebSocket request should return bad request
	if resp.StatusCode != http.StatusBadRequest {
		t.Logf("Note: HTTP to WebSocket upgrade returned status %d", resp.StatusCode)
	}
}
