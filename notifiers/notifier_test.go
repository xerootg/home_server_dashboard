package notifiers

import (
	"sync/atomic"
	"testing"

	"home_server_dashboard/events"
)

// mockNotifier is a test notifier that counts calls.
type mockNotifier struct {
	name      string
	callCount int32
	lastEvent events.Event
	closeErr  error
}

func (m *mockNotifier) Name() string { return m.name }

func (m *mockNotifier) Notify(event events.Event) error {
	atomic.AddInt32(&m.callCount, 1)
	m.lastEvent = event
	return nil
}

func (m *mockNotifier) Close() error { return m.closeErr }

func TestManagerRegister(t *testing.T) {
	bus := events.NewBus(false)
	manager := NewManager(bus)
	defer manager.Close()

	if manager.NotifierCount() != 0 {
		t.Errorf("expected 0 notifiers, got %d", manager.NotifierCount())
	}

	mock := &mockNotifier{name: "test"}
	manager.Register(mock)

	if manager.NotifierCount() != 1 {
		t.Errorf("expected 1 notifier, got %d", manager.NotifierCount())
	}
}

func TestManagerRoutesEventsToNotifiers(t *testing.T) {
	bus := events.NewBus(false)
	manager := NewManager(bus)
	defer manager.Close()

	mock1 := &mockNotifier{name: "mock1"}
	mock2 := &mockNotifier{name: "mock2"}
	manager.Register(mock1)
	manager.Register(mock2)

	// Publish an event
	event := events.NewServiceStateChangedEvent("nas", "traefik", "docker", "stopped", "running", "Up")
	bus.Publish(event)

	// Both notifiers should receive the event
	if atomic.LoadInt32(&mock1.callCount) != 1 {
		t.Errorf("mock1: expected 1 call, got %d", mock1.callCount)
	}
	if atomic.LoadInt32(&mock2.callCount) != 1 {
		t.Errorf("mock2: expected 1 call, got %d", mock2.callCount)
	}
}

func TestManagerReceivesAllEventTypes(t *testing.T) {
	bus := events.NewBus(false)
	manager := NewManager(bus)
	defer manager.Close()

	mock := &mockNotifier{name: "test"}
	manager.Register(mock)

	// Publish different event types
	bus.Publish(events.NewServiceStateChangedEvent("nas", "traefik", "docker", "stopped", "running", "Up"))
	bus.Publish(events.NewHostUnreachableEvent("remote", "timeout"))
	bus.Publish(events.NewHostRecoveredEvent("remote"))

	if atomic.LoadInt32(&mock.callCount) != 3 {
		t.Errorf("expected 3 calls for all event types, got %d", mock.callCount)
	}
}

func TestManagerCloseUnsubscribes(t *testing.T) {
	bus := events.NewBus(false)
	manager := NewManager(bus)

	mock := &mockNotifier{name: "test"}
	manager.Register(mock)

	// Close should unsubscribe from events
	manager.Close()

	// Events after close should not reach the notifier
	bus.Publish(events.NewServiceStateChangedEvent("nas", "traefik", "docker", "stopped", "running", "Up"))

	if atomic.LoadInt32(&mock.callCount) != 0 {
		t.Errorf("expected 0 calls after close, got %d", mock.callCount)
	}
}
