// Package gotify provides a Gotify notification implementation using the official Gotify API client.
package gotify

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gotify/go-api-client/v2/auth"
	"github.com/gotify/go-api-client/v2/client"
	"github.com/gotify/go-api-client/v2/client/message"
	"github.com/gotify/go-api-client/v2/gotify"
	"github.com/gotify/go-api-client/v2/models"

	"home_server_dashboard/config"
	"home_server_dashboard/events"
)

// Priority levels for Gotify messages.
const (
	PriorityMin    = 0  // Minimum priority (no notification)
	PriorityLow    = 2  // Low priority
	PriorityNormal = 5  // Normal priority
	PriorityHigh   = 8  // High priority (notification sound)
	PriorityMax    = 10 // Maximum priority (persistent notification)
)

// Message represents a Gotify message (used for internal formatting).
type Message struct {
	Title    string
	Message  string
	Priority int
}

// Notifier implements the notifiers.Notifier interface for Gotify.
type Notifier struct {
	client   *client.GotifyREST
	token    string
	hostname string // kept for testing/logging
}

// New creates a new Gotify notifier from configuration.
// Returns nil if Gotify is not configured or disabled.
func New(cfg *config.GotifyConfig) *Notifier {
	if cfg == nil || !cfg.IsValid() {
		return nil
	}

	hostname := strings.TrimSuffix(cfg.Hostname, "/")
	parsedURL, err := url.Parse(hostname)
	if err != nil {
		log.Printf("Gotify: failed to parse URL: %v", err)
		return nil
	}

	httpClient := &http.Client{
		Timeout: 10 * time.Second,
	}

	return &Notifier{
		client:   gotify.NewClient(parsedURL, httpClient),
		token:    cfg.Token,
		hostname: hostname,
	}
}

// Name returns the notifier's name.
func (n *Notifier) Name() string {
	return "gotify"
}

// Notify sends a notification for the given event.
func (n *Notifier) Notify(event events.Event) error {
	msg := n.formatEvent(event)
	if msg == nil {
		// Event type not supported for notification
		return nil
	}

	return n.send(msg)
}

// formatEvent converts an event into a Gotify message.
// Returns nil for events that shouldn't generate notifications.
func (n *Notifier) formatEvent(event events.Event) *Message {
	switch e := event.(type) {
	case *events.ServiceStateChangedEvent:
		return n.formatServiceStateChanged(e)
	case *events.HostUnreachableEvent:
		return n.formatHostUnreachable(e)
	case *events.HostRecoveredEvent:
		return n.formatHostRecovered(e)
	default:
		return nil
	}
}

// formatServiceStateChanged formats a service state change event.
func (n *Notifier) formatServiceStateChanged(e *events.ServiceStateChangedEvent) *Message {
	var priority int
	var emoji string

	switch {
	case e.CurrentState == "running" && e.PreviousState == "stopped":
		// Service started - informational
		priority = PriorityNormal
		emoji = "🟢"
	case e.CurrentState == "stopped" && e.PreviousState == "running":
		// Service stopped - important
		priority = PriorityHigh
		emoji = "🔴"
	case e.CurrentState == "stopped":
		// Service is stopped but wasn't running before (first detection)
		priority = PriorityNormal
		emoji = "🔴"
	default:
		// Other state changes
		priority = PriorityLow
		emoji = "🔄"
	}

	title := fmt.Sprintf("%s %s on %s", emoji, e.ServiceName, e.Host)
	message := fmt.Sprintf("%s → %s\n%s (%s)",
		e.PreviousState, e.CurrentState, e.Status, e.Source)

	return &Message{
		Title:    title,
		Message:  message,
		Priority: priority,
	}
}

// formatHostUnreachable formats a host unreachable event.
func (n *Notifier) formatHostUnreachable(e *events.HostUnreachableEvent) *Message {
	return &Message{
		Title:    fmt.Sprintf("🚨 Host %s Unreachable", e.Host),
		Message:  fmt.Sprintf("Cannot connect to host: %s", e.Reason),
		Priority: PriorityMax,
	}
}

// formatHostRecovered formats a host recovered event.
func (n *Notifier) formatHostRecovered(e *events.HostRecoveredEvent) *Message {
	return &Message{
		Title:    fmt.Sprintf("✅ Host %s Recovered", e.Host),
		Message:  "Host is now reachable",
		Priority: PriorityHigh,
	}
}

// send sends a message to Gotify using the official API client.
func (n *Notifier) send(msg *Message) error {
	params := message.NewCreateMessageParams()
	params.Body = &models.MessageExternal{
		Title:    msg.Title,
		Message:  msg.Message,
		Priority: msg.Priority,
	}

	_, err := n.client.Message.CreateMessage(params, auth.TokenAuth(n.token))
	if err != nil {
		log.Printf("Gotify notification failed: %v", err)
		return fmt.Errorf("failed to send notification: %w", err)
	}

	return nil
}

// Close releases resources held by the notifier.
func (n *Notifier) Close() error {
	// HTTP client doesn't need explicit cleanup
	return nil
}

// SendTest sends a test notification to verify connectivity.
func (n *Notifier) SendTest() error {
	msg := &Message{
		Title:    "🔔 Home Server Dashboard",
		Message:  "Test notification - Gotify is configured correctly!",
		Priority: PriorityNormal,
	}
	return n.send(msg)
}
