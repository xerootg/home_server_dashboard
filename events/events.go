// Package events provides an event-driven architecture for service monitoring.
// It implements a publish-subscribe pattern for decoupling event producers
// (like service monitors) from event consumers (like notifiers).
package events

import (
	"sync"
	"time"
)

// EventType represents the type of an event.
type EventType string

const (
	// ServiceStateChanged is emitted when a service changes state (running/stopped).
	ServiceStateChanged EventType = "service_state_changed"
	// HostUnreachable is emitted when a host cannot be contacted.
	HostUnreachable EventType = "host_unreachable"
	// HostRecovered is emitted when a previously unreachable host becomes reachable.
	HostRecovered EventType = "host_recovered"
)

// Event represents something that happened in the system.
type Event interface {
	// Type returns the event type.
	Type() EventType
	// Timestamp returns when the event occurred.
	Timestamp() time.Time
}

// baseEvent provides common event fields.
type baseEvent struct {
	eventType EventType
	timestamp time.Time
}

func (e *baseEvent) Type() EventType     { return e.eventType }
func (e *baseEvent) Timestamp() time.Time { return e.timestamp }

// ServiceStateChangedEvent is emitted when a service changes state.
type ServiceStateChangedEvent struct {
	baseEvent
	Host         string // Host name where the service runs
	ServiceName  string // Name of the service
	Source       string // "docker" or "systemd" or "traefik"
	PreviousState string // Previous state (e.g., "running", "stopped", "unknown")
	CurrentState  string // Current state
	Status        string // Human-readable status message
}

// NewServiceStateChangedEvent creates a new service state changed event.
func NewServiceStateChangedEvent(host, serviceName, source, previousState, currentState, status string) *ServiceStateChangedEvent {
	return &ServiceStateChangedEvent{
		baseEvent: baseEvent{
			eventType: ServiceStateChanged,
			timestamp: time.Now(),
		},
		Host:          host,
		ServiceName:   serviceName,
		Source:        source,
		PreviousState: previousState,
		CurrentState:  currentState,
		Status:        status,
	}
}

// HostUnreachableEvent is emitted when a host cannot be contacted.
type HostUnreachableEvent struct {
	baseEvent
	Host   string // Host name
	Reason string // Why the host is unreachable
}

// NewHostUnreachableEvent creates a new host unreachable event.
func NewHostUnreachableEvent(host, reason string) *HostUnreachableEvent {
	return &HostUnreachableEvent{
		baseEvent: baseEvent{
			eventType: HostUnreachable,
			timestamp: time.Now(),
		},
		Host:   host,
		Reason: reason,
	}
}

// HostRecoveredEvent is emitted when a previously unreachable host becomes reachable.
type HostRecoveredEvent struct {
	baseEvent
	Host string // Host name
}

// NewHostRecoveredEvent creates a new host recovered event.
func NewHostRecoveredEvent(host string) *HostRecoveredEvent {
	return &HostRecoveredEvent{
		baseEvent: baseEvent{
			eventType: HostRecovered,
			timestamp: time.Now(),
		},
		Host: host,
	}
}

// Handler is a function that handles an event.
type Handler func(event Event)

// Subscription represents a subscription to events.
type Subscription struct {
	id        int
	eventType EventType
	handler   Handler
	bus       *Bus
}

// Unsubscribe removes this subscription from the event bus.
func (s *Subscription) Unsubscribe() {
	s.bus.unsubscribe(s)
}

// Bus is a thread-safe event bus for publishing and subscribing to events.
type Bus struct {
	mu           sync.RWMutex
	handlers     map[EventType]map[int]*Subscription
	nextID       int
	asyncPublish bool // If true, handlers are called in goroutines
}

// NewBus creates a new event bus.
// If asyncPublish is true, event handlers are called asynchronously in goroutines.
func NewBus(asyncPublish bool) *Bus {
	return &Bus{
		handlers:     make(map[EventType]map[int]*Subscription),
		asyncPublish: asyncPublish,
	}
}

// Subscribe registers a handler for a specific event type.
// Returns a Subscription that can be used to unsubscribe.
func (b *Bus) Subscribe(eventType EventType, handler Handler) *Subscription {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.handlers[eventType] == nil {
		b.handlers[eventType] = make(map[int]*Subscription)
	}

	b.nextID++
	sub := &Subscription{
		id:        b.nextID,
		eventType: eventType,
		handler:   handler,
		bus:       b,
	}
	b.handlers[eventType][sub.id] = sub
	return sub
}

// SubscribeAll registers a handler for all event types.
// The handler will be called for every published event.
func (b *Bus) SubscribeAll(handler Handler) []*Subscription {
	eventTypes := []EventType{ServiceStateChanged, HostUnreachable, HostRecovered}
	subs := make([]*Subscription, len(eventTypes))
	for i, et := range eventTypes {
		subs[i] = b.Subscribe(et, handler)
	}
	return subs
}

// unsubscribe removes a subscription from the bus.
func (b *Bus) unsubscribe(sub *Subscription) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if handlers, ok := b.handlers[sub.eventType]; ok {
		delete(handlers, sub.id)
	}
}

// Publish sends an event to all subscribed handlers.
func (b *Bus) Publish(event Event) {
	b.mu.RLock()
	handlers := b.handlers[event.Type()]
	// Make a copy of handlers to release the lock quickly
	handlersCopy := make([]Handler, 0, len(handlers))
	for _, sub := range handlers {
		handlersCopy = append(handlersCopy, sub.handler)
	}
	b.mu.RUnlock()

	// Call handlers
	for _, handler := range handlersCopy {
		if b.asyncPublish {
			go handler(event)
		} else {
			handler(event)
		}
	}
}

// HandlerCount returns the total number of subscribed handlers.
func (b *Bus) HandlerCount() int {
	b.mu.RLock()
	defer b.mu.RUnlock()

	count := 0
	for _, handlers := range b.handlers {
		count += len(handlers)
	}
	return count
}
