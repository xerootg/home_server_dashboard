package monitor

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"home_server_dashboard/config"
	"home_server_dashboard/events"
)

func TestNew(t *testing.T) {
	cfg := &config.Config{
		Hosts: []config.HostConfig{
			{Name: "localhost", Address: "localhost"},
		},
	}
	bus := events.NewBus(false)

	m := New(cfg, bus)
	if m == nil {
		t.Fatal("expected non-nil monitor")
	}
	if m.pollInterval != 60*time.Second {
		t.Errorf("expected default pollInterval 60s, got %v", m.pollInterval)
	}
	if !m.skipFirstEvent {
		t.Error("expected skipFirstEvent to be true by default")
	}
}

func TestNewWithOptions(t *testing.T) {
	cfg := &config.Config{}
	bus := events.NewBus(false)

	m := New(cfg, bus,
		WithPollInterval(5*time.Second),
		WithSkipFirstEvent(false),
	)

	if m.pollInterval != 5*time.Second {
		t.Errorf("expected pollInterval 5s, got %v", m.pollInterval)
	}
	if m.skipFirstEvent {
		t.Error("expected skipFirstEvent to be false")
	}
}

func TestServiceStateTracking(t *testing.T) {
	cfg := &config.Config{}
	bus := events.NewBus(false)
	m := New(cfg, bus)

	// Initially no services
	if m.ServiceCount() != 0 {
		t.Errorf("expected 0 services, got %d", m.ServiceCount())
	}

	// Add some state manually (simulating poll)
	m.mu.Lock()
	m.serviceStates["nas:traefik"] = ServiceState{State: "running", Status: "Up"}
	m.serviceStates["nas:nginx"] = ServiceState{State: "stopped", Status: "Exited (0)"}
	m.mu.Unlock()

	if m.ServiceCount() != 2 {
		t.Errorf("expected 2 services, got %d", m.ServiceCount())
	}

	// Get service state
	state, exists := m.GetServiceState("nas", "traefik")
	if !exists {
		t.Error("expected service to exist")
	}
	if state.State != "running" {
		t.Errorf("expected state 'running', got '%s'", state.State)
	}

	// Non-existent service
	_, exists = m.GetServiceState("nas", "nonexistent")
	if exists {
		t.Error("expected service to not exist")
	}
}

func TestHostStateTracking(t *testing.T) {
	cfg := &config.Config{}
	bus := events.NewBus(false)
	m := New(cfg, bus)

	// Initially no hosts
	if m.HostCount() != 0 {
		t.Errorf("expected 0 hosts, got %d", m.HostCount())
	}

	// Add host states manually
	m.mu.Lock()
	m.hostStates["nas"] = HostState{Reachable: true}
	m.hostStates["remote"] = HostState{Reachable: false, LastError: "timeout"}
	m.mu.Unlock()

	if m.HostCount() != 2 {
		t.Errorf("expected 2 hosts, got %d", m.HostCount())
	}

	// Get host state
	state, exists := m.GetHostState("nas")
	if !exists {
		t.Error("expected host to exist")
	}
	if !state.Reachable {
		t.Error("expected host to be reachable")
	}

	state, exists = m.GetHostState("remote")
	if !exists {
		t.Error("expected host to exist")
	}
	if state.Reachable {
		t.Error("expected host to be unreachable")
	}
	if state.LastError != "timeout" {
		t.Errorf("expected error 'timeout', got '%s'", state.LastError)
	}
}

func TestEventEmissionOnStateChange(t *testing.T) {
	cfg := &config.Config{}
	bus := events.NewBus(false)
	m := New(cfg, bus, WithSkipFirstEvent(false)) // Enable events immediately

	var receivedEvents []events.Event
	var mu sync.Mutex

	bus.Subscribe(events.ServiceStateChanged, func(event events.Event) {
		mu.Lock()
		receivedEvents = append(receivedEvents, event)
		mu.Unlock()
	})

	// Simulate first state
	m.mu.Lock()
	m.serviceStates["nas:traefik"] = ServiceState{State: "running", Status: "Up"}
	m.mu.Unlock()

	// Simulate state change (would happen via updateServiceState in real poll)
	// We need to manually trigger the event emission logic
	m.mu.Lock()
	oldState := m.serviceStates["nas:traefik"]
	newState := ServiceState{State: "stopped", Status: "Exited (1)"}
	m.serviceStates["nas:traefik"] = newState

	event := events.NewServiceStateChangedEvent(
		"nas", "traefik", "docker",
		oldState.State, newState.State, newState.Status,
	)
	m.mu.Unlock()

	bus.Publish(event)

	mu.Lock()
	defer mu.Unlock()
	if len(receivedEvents) != 1 {
		t.Fatalf("expected 1 event, got %d", len(receivedEvents))
	}

	stateEvent, ok := receivedEvents[0].(*events.ServiceStateChangedEvent)
	if !ok {
		t.Fatal("expected ServiceStateChangedEvent")
	}
	if stateEvent.PreviousState != "running" {
		t.Errorf("expected previous state 'running', got '%s'", stateEvent.PreviousState)
	}
	if stateEvent.CurrentState != "stopped" {
		t.Errorf("expected current state 'stopped', got '%s'", stateEvent.CurrentState)
	}
}

func TestHostUnreachableEvent(t *testing.T) {
	cfg := &config.Config{}
	bus := events.NewBus(false)
	m := New(cfg, bus, WithSkipFirstEvent(false))

	var eventCount int32

	bus.Subscribe(events.HostUnreachable, func(event events.Event) {
		atomic.AddInt32(&eventCount, 1)
	})

	// Simulate host going from reachable to unreachable
	m.mu.Lock()
	m.hostStates["remote"] = HostState{Reachable: true}
	m.mu.Unlock()

	// Now mark as unreachable
	m.handleHostError("remote", "connection refused")

	if atomic.LoadInt32(&eventCount) != 1 {
		t.Errorf("expected 1 host unreachable event, got %d", eventCount)
	}
}

func TestHostRecoveredEvent(t *testing.T) {
	cfg := &config.Config{}
	bus := events.NewBus(false)
	m := New(cfg, bus, WithSkipFirstEvent(false))

	var eventCount int32

	bus.Subscribe(events.HostRecovered, func(event events.Event) {
		atomic.AddInt32(&eventCount, 1)
	})

	// Simulate host going from unreachable to reachable
	m.mu.Lock()
	m.hostStates["remote"] = HostState{Reachable: false, LastError: "timeout"}
	m.mu.Unlock()

	// Now mark as reachable
	m.handleHostSuccess("remote")

	if atomic.LoadInt32(&eventCount) != 1 {
		t.Errorf("expected 1 host recovered event, got %d", eventCount)
	}
}

func TestSkipFirstEvent(t *testing.T) {
	cfg := &config.Config{}
	bus := events.NewBus(false)
	m := New(cfg, bus) // skipFirstEvent is true by default

	var eventCount int32

	bus.Subscribe(events.HostUnreachable, func(event events.Event) {
		atomic.AddInt32(&eventCount, 1)
	})

	// During initial discovery, events should be skipped
	m.handleHostError("remote", "timeout")

	if atomic.LoadInt32(&eventCount) != 0 {
		t.Errorf("expected 0 events during initial discovery, got %d", eventCount)
	}

	// After skipFirstEvent is disabled, events should be emitted
	m.mu.Lock()
	m.skipFirstEvent = false
	m.hostStates["remote"] = HostState{Reachable: true} // Reset to reachable
	m.mu.Unlock()

	m.handleHostError("remote", "timeout")

	if atomic.LoadInt32(&eventCount) != 1 {
		t.Errorf("expected 1 event after discovery phase, got %d", eventCount)
	}
}

func TestStartStop(t *testing.T) {
	cfg := &config.Config{
		Hosts: []config.HostConfig{
			{Name: "localhost", Address: "localhost"},
		},
	}
	bus := events.NewBus(false)
	m := New(cfg, bus, WithPollInterval(100*time.Millisecond))

	// Start monitor
	m.Start()

	// Verify it's running
	m.mu.RLock()
	running := m.running
	m.mu.RUnlock()
	if !running {
		t.Error("expected monitor to be running")
	}

	// Stop monitor
	m.Stop()

	// Verify it stopped
	m.mu.RLock()
	running = m.running
	m.mu.RUnlock()
	if running {
		t.Error("expected monitor to be stopped")
	}
}

func TestMultipleStartsAreSafe(t *testing.T) {
	cfg := &config.Config{}
	bus := events.NewBus(false)
	m := New(cfg, bus)

	// Multiple starts should be safe
	m.Start()
	m.Start()
	m.Start()

	m.Stop()
}

func TestMultipleStopsAreSafe(t *testing.T) {
	cfg := &config.Config{}
	bus := events.NewBus(false)
	m := New(cfg, bus)

	m.Start()

	// Multiple stops should be safe
	m.Stop()
	m.Stop()
	m.Stop()
}
