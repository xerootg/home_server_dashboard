package events

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewServiceStateChangedEvent(t *testing.T) {
	event := NewServiceStateChangedEvent("nas", "traefik", "docker", "stopped", "running", "Up 5 minutes")

	if event.Type() != ServiceStateChanged {
		t.Errorf("expected type %s, got %s", ServiceStateChanged, event.Type())
	}
	if event.Host != "nas" {
		t.Errorf("expected host 'nas', got '%s'", event.Host)
	}
	if event.ServiceName != "traefik" {
		t.Errorf("expected service 'traefik', got '%s'", event.ServiceName)
	}
	if event.Source != "docker" {
		t.Errorf("expected source 'docker', got '%s'", event.Source)
	}
	if event.PreviousState != "stopped" {
		t.Errorf("expected previous state 'stopped', got '%s'", event.PreviousState)
	}
	if event.CurrentState != "running" {
		t.Errorf("expected current state 'running', got '%s'", event.CurrentState)
	}
	if event.Status != "Up 5 minutes" {
		t.Errorf("expected status 'Up 5 minutes', got '%s'", event.Status)
	}
	if event.Timestamp().IsZero() {
		t.Error("expected non-zero timestamp")
	}
}

func TestNewHostUnreachableEvent(t *testing.T) {
	event := NewHostUnreachableEvent("remote-host", "connection refused")

	if event.Type() != HostUnreachable {
		t.Errorf("expected type %s, got %s", HostUnreachable, event.Type())
	}
	if event.Host != "remote-host" {
		t.Errorf("expected host 'remote-host', got '%s'", event.Host)
	}
	if event.Reason != "connection refused" {
		t.Errorf("expected reason 'connection refused', got '%s'", event.Reason)
	}
}

func TestNewHostRecoveredEvent(t *testing.T) {
	event := NewHostRecoveredEvent("remote-host")

	if event.Type() != HostRecovered {
		t.Errorf("expected type %s, got %s", HostRecovered, event.Type())
	}
	if event.Host != "remote-host" {
		t.Errorf("expected host 'remote-host', got '%s'", event.Host)
	}
}

func TestBusSubscribeAndPublish(t *testing.T) {
	bus := NewBus(false) // synchronous for testing

	var receivedEvents []Event
	var mu sync.Mutex

	bus.Subscribe(ServiceStateChanged, func(event Event) {
		mu.Lock()
		receivedEvents = append(receivedEvents, event)
		mu.Unlock()
	})

	event := NewServiceStateChangedEvent("nas", "traefik", "docker", "stopped", "running", "Up")
	bus.Publish(event)

	mu.Lock()
	defer mu.Unlock()
	if len(receivedEvents) != 1 {
		t.Fatalf("expected 1 event, got %d", len(receivedEvents))
	}
	if receivedEvents[0].Type() != ServiceStateChanged {
		t.Errorf("expected event type %s, got %s", ServiceStateChanged, receivedEvents[0].Type())
	}
}

func TestBusOnlyReceivesSubscribedEvents(t *testing.T) {
	bus := NewBus(false)

	var hostEventCount int
	var serviceEventCount int

	bus.Subscribe(HostUnreachable, func(event Event) {
		hostEventCount++
	})
	bus.Subscribe(ServiceStateChanged, func(event Event) {
		serviceEventCount++
	})

	// Publish a service state changed event
	bus.Publish(NewServiceStateChangedEvent("nas", "traefik", "docker", "stopped", "running", "Up"))
	// Publish a host unreachable event
	bus.Publish(NewHostUnreachableEvent("remote", "timeout"))

	if hostEventCount != 1 {
		t.Errorf("expected host event count 1, got %d", hostEventCount)
	}
	if serviceEventCount != 1 {
		t.Errorf("expected service event count 1, got %d", serviceEventCount)
	}
}

func TestBusUnsubscribe(t *testing.T) {
	bus := NewBus(false)

	var count int
	sub := bus.Subscribe(ServiceStateChanged, func(event Event) {
		count++
	})

	// First publish should be received
	bus.Publish(NewServiceStateChangedEvent("nas", "traefik", "docker", "stopped", "running", "Up"))
	if count != 1 {
		t.Errorf("expected count 1, got %d", count)
	}

	// Unsubscribe and publish again
	sub.Unsubscribe()
	bus.Publish(NewServiceStateChangedEvent("nas", "traefik", "docker", "running", "stopped", "Down"))
	if count != 1 {
		t.Errorf("expected count still 1 after unsubscribe, got %d", count)
	}
}

func TestBusSubscribeAll(t *testing.T) {
	bus := NewBus(false)

	var count int
	subs := bus.SubscribeAll(func(event Event) {
		count++
	})

	if len(subs) != 3 {
		t.Fatalf("expected 3 subscriptions, got %d", len(subs))
	}

	// Publish different event types
	bus.Publish(NewServiceStateChangedEvent("nas", "traefik", "docker", "stopped", "running", "Up"))
	bus.Publish(NewHostUnreachableEvent("remote", "timeout"))
	bus.Publish(NewHostRecoveredEvent("remote"))

	if count != 3 {
		t.Errorf("expected count 3, got %d", count)
	}
}

func TestBusAsyncPublish(t *testing.T) {
	bus := NewBus(true) // async mode

	var count int32
	done := make(chan struct{})

	bus.Subscribe(ServiceStateChanged, func(event Event) {
		atomic.AddInt32(&count, 1)
		select {
		case done <- struct{}{}:
		default:
		}
	})

	bus.Publish(NewServiceStateChangedEvent("nas", "traefik", "docker", "stopped", "running", "Up"))

	// Wait for async handler with timeout
	select {
	case <-done:
		// Handler was called
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for async handler")
	}

	if atomic.LoadInt32(&count) != 1 {
		t.Errorf("expected count 1, got %d", count)
	}
}

func TestBusMultipleHandlersSameEvent(t *testing.T) {
	bus := NewBus(false)

	var count1, count2 int

	bus.Subscribe(ServiceStateChanged, func(event Event) {
		count1++
	})
	bus.Subscribe(ServiceStateChanged, func(event Event) {
		count2++
	})

	bus.Publish(NewServiceStateChangedEvent("nas", "traefik", "docker", "stopped", "running", "Up"))

	if count1 != 1 {
		t.Errorf("expected count1 1, got %d", count1)
	}
	if count2 != 1 {
		t.Errorf("expected count2 1, got %d", count2)
	}
}

func TestBusHandlerCount(t *testing.T) {
	bus := NewBus(false)

	if bus.HandlerCount() != 0 {
		t.Errorf("expected 0 handlers, got %d", bus.HandlerCount())
	}

	sub1 := bus.Subscribe(ServiceStateChanged, func(event Event) {})
	sub2 := bus.Subscribe(HostUnreachable, func(event Event) {})
	bus.Subscribe(ServiceStateChanged, func(event Event) {})

	if bus.HandlerCount() != 3 {
		t.Errorf("expected 3 handlers, got %d", bus.HandlerCount())
	}

	sub1.Unsubscribe()
	if bus.HandlerCount() != 2 {
		t.Errorf("expected 2 handlers after unsubscribe, got %d", bus.HandlerCount())
	}

	sub2.Unsubscribe()
	if bus.HandlerCount() != 1 {
		t.Errorf("expected 1 handler, got %d", bus.HandlerCount())
	}
}
