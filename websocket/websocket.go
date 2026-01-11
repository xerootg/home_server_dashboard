// Package websocket provides WebSocket-based real-time updates for the dashboard.
// It implements a hub pattern where clients connect and receive state change events
// from the event bus.
package websocket

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"home_server_dashboard/events"
)

const (
	// Time allowed to write a message to the peer.
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer.
	pongWait = 60 * time.Second

	// Send pings to peer with this period. Must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10

	// Maximum message size allowed from peer.
	maxMessageSize = 512
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins for development
	},
}

// MessageType represents the type of WebSocket message.
type MessageType string

const (
	// MessageTypeServiceUpdate is sent when a service state changes.
	MessageTypeServiceUpdate MessageType = "service_update"
	// MessageTypeHostUnreachable is sent when a host becomes unreachable.
	MessageTypeHostUnreachable MessageType = "host_unreachable"
	// MessageTypeHostRecovered is sent when a host recovers.
	MessageTypeHostRecovered MessageType = "host_recovered"
	// MessageTypePing is a keepalive message.
	MessageTypePing MessageType = "ping"
)

// Message represents a WebSocket message sent to clients.
type Message struct {
	Type      MessageType `json:"type"`
	Timestamp int64       `json:"timestamp"` // Unix timestamp in milliseconds
	Payload   interface{} `json:"payload"`
}

// ServiceUpdatePayload contains information about a service state change.
type ServiceUpdatePayload struct {
	Host          string `json:"host"`
	ServiceName   string `json:"service_name"`
	Source        string `json:"source"`
	PreviousState string `json:"previous_state"`
	CurrentState  string `json:"current_state"`
	Status        string `json:"status"`
}

// HostEventPayload contains information about a host event.
type HostEventPayload struct {
	Host   string `json:"host"`
	Reason string `json:"reason,omitempty"`
}

// Client represents a connected WebSocket client.
type Client struct {
	hub  *Hub
	conn *websocket.Conn
	send chan []byte
}

// Hub maintains the set of active clients and broadcasts messages to them.
type Hub struct {
	clients    map[*Client]bool
	broadcast  chan []byte
	register   chan *Client
	unregister chan *Client
	mu         sync.RWMutex
	running    bool
	stopCh     chan struct{}
	wg         sync.WaitGroup

	// Event bus subscription
	eventBus      *events.Bus
	subscriptions []*events.Subscription
}

// NewHub creates a new WebSocket hub.
func NewHub(eventBus *events.Bus) *Hub {
	return &Hub{
		clients:    make(map[*Client]bool),
		broadcast:  make(chan []byte, 256),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		stopCh:     make(chan struct{}),
		eventBus:   eventBus,
	}
}

// Start begins the hub's main loop.
func (h *Hub) Start() {
	h.mu.Lock()
	if h.running {
		h.mu.Unlock()
		return
	}
	h.running = true
	h.mu.Unlock()

	// Subscribe to event bus
	h.subscribeToEvents()

	h.wg.Add(1)
	go h.run()

	log.Printf("WebSocket hub started")
}

// Stop gracefully shuts down the hub.
func (h *Hub) Stop() {
	h.mu.Lock()
	if !h.running {
		h.mu.Unlock()
		return
	}
	h.running = false
	h.mu.Unlock()

	// Unsubscribe from events
	for _, sub := range h.subscriptions {
		sub.Unsubscribe()
	}
	h.subscriptions = nil

	close(h.stopCh)
	h.wg.Wait()

	log.Printf("WebSocket hub stopped")
}

// subscribeToEvents subscribes to event bus events.
func (h *Hub) subscribeToEvents() {
	// Subscribe to service state changes
	h.subscriptions = append(h.subscriptions,
		h.eventBus.Subscribe(events.ServiceStateChanged, func(e events.Event) {
			evt := e.(*events.ServiceStateChangedEvent)
			h.broadcastMessage(Message{
				Type:      MessageTypeServiceUpdate,
				Timestamp: evt.Timestamp().UnixMilli(),
				Payload: ServiceUpdatePayload{
					Host:          evt.Host,
					ServiceName:   evt.ServiceName,
					Source:        evt.Source,
					PreviousState: evt.PreviousState,
					CurrentState:  evt.CurrentState,
					Status:        evt.Status,
				},
			})
		}),
	)

	// Subscribe to host unreachable events
	h.subscriptions = append(h.subscriptions,
		h.eventBus.Subscribe(events.HostUnreachable, func(e events.Event) {
			evt := e.(*events.HostUnreachableEvent)
			h.broadcastMessage(Message{
				Type:      MessageTypeHostUnreachable,
				Timestamp: evt.Timestamp().UnixMilli(),
				Payload: HostEventPayload{
					Host:   evt.Host,
					Reason: evt.Reason,
				},
			})
		}),
	)

	// Subscribe to host recovered events
	h.subscriptions = append(h.subscriptions,
		h.eventBus.Subscribe(events.HostRecovered, func(e events.Event) {
			evt := e.(*events.HostRecoveredEvent)
			h.broadcastMessage(Message{
				Type:      MessageTypeHostRecovered,
				Timestamp: evt.Timestamp().UnixMilli(),
				Payload: HostEventPayload{
					Host: evt.Host,
				},
			})
		}),
	)
}

// broadcastMessage serializes and broadcasts a message to all clients.
func (h *Hub) broadcastMessage(msg Message) {
	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("WebSocket: failed to marshal message: %v", err)
		return
	}

	select {
	case h.broadcast <- data:
	default:
		log.Printf("WebSocket: broadcast channel full, dropping message")
	}
}

// run is the main hub loop.
func (h *Hub) run() {
	defer h.wg.Done()

	for {
		select {
		case <-h.stopCh:
			// Close all client connections
			h.mu.Lock()
			for client := range h.clients {
				close(client.send)
				delete(h.clients, client)
			}
			h.mu.Unlock()
			return

		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			clientCount := len(h.clients)
			h.mu.Unlock()
			log.Printf("WebSocket: client connected (total: %d)", clientCount)

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}
			clientCount := len(h.clients)
			h.mu.Unlock()
			log.Printf("WebSocket: client disconnected (total: %d)", clientCount)

		case message := <-h.broadcast:
			h.mu.RLock()
			for client := range h.clients {
				select {
				case client.send <- message:
				default:
					// Client buffer full, close connection
					go func(c *Client) {
						h.unregister <- c
					}(client)
				}
			}
			h.mu.RUnlock()
		}
	}
}

// ClientCount returns the number of connected clients.
func (h *Hub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// Handler returns an HTTP handler for WebSocket connections.
func (h *Hub) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("WebSocket: upgrade error: %v", err)
			return
		}

		client := &Client{
			hub:  h,
			conn: conn,
			send: make(chan []byte, 256),
		}

		h.register <- client

		// Start read and write pumps
		go client.writePump()
		go client.readPump()
	}
}

// readPump pumps messages from the WebSocket connection to the hub.
// Currently, we don't process client messages, but we need to read
// to handle pong responses and detect connection close.
func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()

	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, _, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket: read error: %v", err)
			}
			break
		}
		// We don't process incoming messages currently
	}
}

// writePump pumps messages from the hub to the WebSocket connection.
func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// Hub closed the channel
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)

			// Add queued messages to the current write
			n := len(c.send)
			for i := 0; i < n; i++ {
				w.Write([]byte{'\n'})
				w.Write(<-c.send)
			}

			if err := w.Close(); err != nil {
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
