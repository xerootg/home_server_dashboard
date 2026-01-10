// Package notifiers provides notification implementations for the event system.
package notifiers

import (
	"home_server_dashboard/events"
)

// Notifier is the interface that all notification providers must implement.
type Notifier interface {
	// Name returns the notifier's name (e.g., "gotify", "email").
	Name() string

	// Notify sends a notification for the given event.
	// Returns an error if the notification fails.
	Notify(event events.Event) error

	// Close releases any resources held by the notifier.
	Close() error
}

// Manager manages multiple notifiers and routes events to them.
type Manager struct {
	notifiers []Notifier
	bus       *events.Bus
	subs      []*events.Subscription
}

// NewManager creates a new notifier manager that listens to the event bus.
func NewManager(bus *events.Bus) *Manager {
	m := &Manager{
		notifiers: make([]Notifier, 0),
		bus:       bus,
	}

	// Subscribe to all events
	m.subs = bus.SubscribeAll(m.handleEvent)

	return m
}

// Register adds a notifier to the manager.
func (m *Manager) Register(notifier Notifier) {
	m.notifiers = append(m.notifiers, notifier)
}

// handleEvent is called for each event and routes it to all registered notifiers.
func (m *Manager) handleEvent(event events.Event) {
	for _, notifier := range m.notifiers {
		if err := notifier.Notify(event); err != nil {
			// Log but don't fail - notifications are best-effort
			// We import log here to avoid circular dependencies
			// Individual notifiers should log their own errors
		}
	}
}

// Close unsubscribes from the event bus and closes all notifiers.
func (m *Manager) Close() error {
	// Unsubscribe from events
	for _, sub := range m.subs {
		sub.Unsubscribe()
	}

	// Close all notifiers
	var lastErr error
	for _, notifier := range m.notifiers {
		if err := notifier.Close(); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

// NotifierCount returns the number of registered notifiers.
func (m *Manager) NotifierCount() int {
	return len(m.notifiers)
}
