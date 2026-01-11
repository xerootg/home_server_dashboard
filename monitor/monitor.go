// Package monitor provides service monitoring and state change detection.
// It uses native event sources (Docker events API, systemd D-Bus signals) where
// available, with polling as a fallback for remote hosts and Home Assistant.
package monitor

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/coreos/go-systemd/v22/dbus"
	dockerEvents "github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	containerAPI "github.com/docker/docker/api/types/container"

	"home_server_dashboard/config"
	"home_server_dashboard/events"
	"home_server_dashboard/services"
	"home_server_dashboard/services/homeassistant"
	"home_server_dashboard/services/systemd"
)

// ServiceState tracks the last known state of a service.
type ServiceState struct {
	State  string // "running", "stopped", "unknown"
	Status string // Human-readable status
}

// HostState tracks whether a host is reachable.
type HostState struct {
	Reachable bool
	LastError string
}

// Monitor watches services and emits events when states change.
// It uses Docker events API and systemd D-Bus signals for local services,
// and falls back to polling for remote hosts.
type Monitor struct {
	cfg            *config.Config
	bus            *events.Bus
	pollInterval   time.Duration
	serviceStates  map[string]ServiceState // key: "host:servicename"
	hostStates     map[string]HostState    // key: hostname
	mu             sync.RWMutex
	stopCh         chan struct{}
	wg             sync.WaitGroup
	running        bool
	skipFirstEvent bool // Don't emit events for initial state discovery

	// Event source connections
	dockerClient *client.Client
	dbusConn     *dbus.Conn
}

// Option is a functional option for configuring the monitor.
type Option func(*Monitor)

// WithPollInterval sets the polling interval for remote hosts.
func WithPollInterval(d time.Duration) Option {
	return func(m *Monitor) {
		m.pollInterval = d
	}
}

// WithSkipFirstEvent configures whether to skip events on initial state discovery.
// When true (default), initial discovery will not emit events.
func WithSkipFirstEvent(skip bool) Option {
	return func(m *Monitor) {
		m.skipFirstEvent = skip
	}
}

// New creates a new service monitor.
func New(cfg *config.Config, bus *events.Bus, opts ...Option) *Monitor {
	m := &Monitor{
		cfg:            cfg,
		bus:            bus,
		pollInterval:   60 * time.Second, // Polling fallback for remote hosts
		serviceStates:  make(map[string]ServiceState),
		hostStates:     make(map[string]HostState),
		stopCh:         make(chan struct{}),
		skipFirstEvent: true, // Don't alert on initial discovery
	}

	for _, opt := range opts {
		opt(m)
	}

	return m
}

// Start begins monitoring services in the background.
func (m *Monitor) Start() {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return
	}
	m.running = true
	m.mu.Unlock()

	// Initialize event sources
	m.initDockerEvents()
	m.initSystemdEvents()

	// Start event watchers
	m.wg.Add(1)
	go m.watchDockerEvents()

	m.wg.Add(1)
	go m.watchSystemdEvents()

	// Start polling for remote hosts (no native events available via SSH)
	if m.hasRemoteHosts() {
		m.wg.Add(1)
		go m.pollRemoteHosts()
	}

	// Start polling for Home Assistant instances
	if m.hasHomeAssistantHosts() {
		m.wg.Add(1)
		go m.pollHomeAssistantHosts()
	}

	log.Printf("Service monitor started (Docker events: %v, systemd D-Bus: %v, remote polling: %v, HA polling: %v)",
		m.dockerClient != nil, m.dbusConn != nil, m.hasRemoteHosts(), m.hasHomeAssistantHosts())
}

// Stop stops the monitor and waits for it to finish.
func (m *Monitor) Stop() {
	m.mu.Lock()
	if !m.running {
		m.mu.Unlock()
		return
	}
	m.running = false
	m.mu.Unlock()

	close(m.stopCh)
	m.wg.Wait()

	// Clean up connections
	if m.dockerClient != nil {
		m.dockerClient.Close()
		m.dockerClient = nil
	}
	if m.dbusConn != nil {
		m.dbusConn.Close()
		m.dbusConn = nil
	}

	log.Printf("Service monitor stopped")
}

// initDockerEvents initializes the Docker client for event watching.
func (m *Monitor) initDockerEvents() {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		log.Printf("Monitor: failed to create Docker client for events: %v", err)
		return
	}

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := cli.Ping(ctx); err != nil {
		log.Printf("Monitor: Docker not available for events: %v", err)
		cli.Close()
		return
	}

	m.dockerClient = cli
}

// initSystemdEvents initializes the D-Bus connection for systemd event watching.
func (m *Monitor) initSystemdEvents() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := dbus.NewSystemConnectionContext(ctx)
	if err != nil {
		log.Printf("Monitor: failed to connect to systemd D-Bus for events: %v", err)
		return
	}

	m.dbusConn = conn
}

// watchDockerEvents watches Docker events and emits state change events.
func (m *Monitor) watchDockerEvents() {
	defer m.wg.Done()

	if m.dockerClient == nil {
		log.Printf("Monitor: Docker events not available, skipping Docker watch")
		return
	}

	localHostName := m.cfg.GetLocalHostName()

	// Create filter for container events
	filterArgs := filters.NewArgs()
	filterArgs.Add("type", "container")
	filterArgs.Add("event", "start")
	filterArgs.Add("event", "stop")
	filterArgs.Add("event", "die")
	filterArgs.Add("event", "kill")
	filterArgs.Add("event", "pause")
	filterArgs.Add("event", "unpause")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start listening for events
	eventsChan, errChan := m.dockerClient.Events(ctx, dockerEvents.ListOptions{Filters: filterArgs})

	log.Printf("Monitor: watching Docker events for container state changes")

	// Do initial discovery
	m.discoverDockerServices(localHostName)

	for {
		select {
		case <-m.stopCh:
			return
		case err := <-errChan:
			if err != nil {
				log.Printf("Monitor: Docker events error: %v", err)
				m.handleHostError(localHostName, "Docker events: "+err.Error())
				// Try to reconnect after a delay
				select {
				case <-m.stopCh:
					return
				case <-time.After(10 * time.Second):
					m.initDockerEvents()
					if m.dockerClient != nil {
						eventsChan, errChan = m.dockerClient.Events(ctx, dockerEvents.ListOptions{Filters: filterArgs})
						m.handleHostSuccess(localHostName)
					}
				}
			}
		case event := <-eventsChan:
			m.handleDockerEvent(localHostName, event)
		}
	}
}

// discoverDockerServices does initial discovery of Docker services.
func (m *Monitor) discoverDockerServices(hostName string) {
	if m.dockerClient == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	containers, err := m.dockerClient.ContainerList(ctx, containerAPI.ListOptions{All: true})
	if err != nil {
		log.Printf("Monitor: failed to list Docker containers: %v", err)
		m.handleHostError(hostName, "Docker list: "+err.Error())
		return
	}

	m.handleHostSuccess(hostName)

	for _, container := range containers {
		// Get service name from Docker Compose labels
		serviceName := container.Labels["com.docker.compose.service"]
		if serviceName == "" {
			continue // Skip non-compose containers
		}

		state := "stopped"
		if container.State == "running" {
			state = "running"
		}

		m.updateServiceState(services.ServiceInfo{
			Name:   serviceName,
			Host:   hostName,
			Source: "docker",
			State:  state,
			Status: container.Status,
		})
	}

	m.markDiscoveryComplete()
}

// handleDockerEvent processes a Docker event and emits state change events.
func (m *Monitor) handleDockerEvent(hostName string, event dockerEvents.Message) {
	// Get service name from labels
	serviceName := event.Actor.Attributes["com.docker.compose.service"]
	if serviceName == "" {
		return // Skip non-compose containers
	}

	var newState string
	switch event.Action {
	case "start", "unpause":
		newState = "running"
	case "stop", "die", "kill", "pause":
		newState = "stopped"
	default:
		return // Ignore other events
	}

	status := string(event.Action)

	m.updateServiceState(services.ServiceInfo{
		Name:   serviceName,
		Host:   hostName,
		Source: "docker",
		State:  newState,
		Status: status,
	})
}

// watchSystemdEvents watches systemd D-Bus signals for unit state changes.
func (m *Monitor) watchSystemdEvents() {
	defer m.wg.Done()

	if m.dbusConn == nil {
		log.Printf("Monitor: systemd D-Bus not available, skipping systemd watch")
		return
	}

	// Get local host config
	localHost := m.getLocalHostConfig()
	if localHost == nil || len(localHost.SystemdServices) == 0 {
		log.Printf("Monitor: no local systemd services configured")
		return
	}

	// Build set of units to watch
	watchUnits := make(map[string]bool)
	for _, unit := range localHost.SystemdServices {
		watchUnits[unit] = true
	}

	// Do initial discovery
	m.discoverSystemdServices(localHost.Name, watchUnits)

	// Subscribe to unit state changes
	// The D-Bus API gives us property changes which include state transitions
	err := m.dbusConn.Subscribe()
	if err != nil {
		log.Printf("Monitor: failed to subscribe to systemd signals: %v", err)
		return
	}
	defer m.dbusConn.Unsubscribe()

	// Create channel for unit status updates
	updateCh := make(chan *dbus.SubStateUpdate, 64)
	errCh := make(chan error, 1)
	m.dbusConn.SetSubStateSubscriber(updateCh, errCh)

	log.Printf("Monitor: watching systemd D-Bus signals for %d units", len(watchUnits))

	for {
		select {
		case <-m.stopCh:
			return
		case err := <-errCh:
			if err != nil {
				log.Printf("Monitor: systemd D-Bus error: %v", err)
			}
		case update := <-updateCh:
			if update == nil {
				continue
			}
			// Check if this is a unit we care about
			if !watchUnits[update.UnitName] {
				continue
			}
			m.handleSystemdUpdate(localHost.Name, update)
		}
	}
}

// discoverSystemdServices does initial discovery of systemd services.
func (m *Monitor) discoverSystemdServices(hostName string, watchUnits map[string]bool) {
	if m.dbusConn == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	units, err := m.dbusConn.ListUnitsContext(ctx)
	if err != nil {
		log.Printf("Monitor: failed to list systemd units: %v", err)
		m.handleHostError(hostName, "systemd list: "+err.Error())
		return
	}

	m.handleHostSuccess(hostName)

	for _, unit := range units {
		if !watchUnits[unit.Name] {
			continue
		}

		state := "stopped"
		if unit.ActiveState == "active" {
			state = "running"
		}

		m.updateServiceState(services.ServiceInfo{
			Name:   unit.Name,
			Host:   hostName,
			Source: "systemd",
			State:  state,
			Status: unit.ActiveState + " (" + unit.SubState + ")",
		})
	}

	m.markDiscoveryComplete()
}

// handleSystemdUpdate processes a systemd unit state update.
// SubState values for services: running, dead, exited, failed, auto-restart, etc.
func (m *Monitor) handleSystemdUpdate(hostName string, update *dbus.SubStateUpdate) {
	state := "stopped"
	// SubState "running" means the service is active and running
	if update.SubState == "running" {
		state = "running"
	}

	m.updateServiceState(services.ServiceInfo{
		Name:   update.UnitName,
		Host:   hostName,
		Source: "systemd",
		State:  state,
		Status: update.SubState,
	})
}

// pollRemoteHosts polls remote hosts that don't support native events.
func (m *Monitor) pollRemoteHosts() {
	defer m.wg.Done()

	// Wait for initial discovery to complete before polling
	time.Sleep(2 * time.Second)

	ticker := time.NewTicker(m.pollInterval)
	defer ticker.Stop()

	// Initial poll
	m.pollRemote()

	for {
		select {
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.pollRemote()
		}
	}
}

// pollRemote fetches current service states from remote hosts.
func (m *Monitor) pollRemote() {
	ctx, cancel := context.WithTimeout(context.Background(), m.pollInterval/2)
	defer cancel()

	for _, host := range m.cfg.Hosts {
		// Skip local host - it uses native events
		if host.IsLocal() {
			continue
		}

		if len(host.SystemdServices) == 0 {
			continue
		}

		systemdProvider := systemd.NewProvider(host.Name, host.Address, host.SystemdServices)
		systemdServices, err := systemdProvider.GetServices(ctx)
		if err != nil {
			log.Printf("Monitor: failed to poll remote host %s: %v", host.Name, err)
			m.handleHostError(host.Name, err.Error())
			continue
		}

		m.handleHostSuccess(host.Name)

		for _, svc := range systemdServices {
			m.updateServiceState(svc)
		}
	}

	m.markDiscoveryComplete()
}

// hasRemoteHosts returns true if there are remote hosts configured.
func (m *Monitor) hasRemoteHosts() bool {
	for _, host := range m.cfg.Hosts {
		if !host.IsLocal() && len(host.SystemdServices) > 0 {
			return true
		}
	}
	return false
}

// getLocalHostConfig returns the local host configuration.
func (m *Monitor) getLocalHostConfig() *config.HostConfig {
	for i := range m.cfg.Hosts {
		if m.cfg.Hosts[i].IsLocal() {
			return &m.cfg.Hosts[i]
		}
	}
	return nil
}

// markDiscoveryComplete marks initial discovery as complete.
func (m *Monitor) markDiscoveryComplete() {
	m.mu.Lock()
	if m.skipFirstEvent {
		m.skipFirstEvent = false
		log.Printf("Monitor: initial state captured for %d services", len(m.serviceStates))
	}
	m.mu.Unlock()
}

// updateServiceState checks if a service state changed and emits an event if so.
func (m *Monitor) updateServiceState(svc services.ServiceInfo) {
	key := svc.Host + ":" + svc.Name

	m.mu.Lock()
	defer m.mu.Unlock()

	oldState, exists := m.serviceStates[key]
	newState := ServiceState{
		State:  svc.State,
		Status: svc.Status,
	}

	// Update stored state
	m.serviceStates[key] = newState

	// Check if state changed
	if exists && oldState.State != newState.State {
		// Don't emit events during initial discovery
		if !m.skipFirstEvent {
			event := events.NewServiceStateChangedEvent(
				svc.Host,
				svc.Name,
				svc.Source,
				oldState.State,
				newState.State,
				newState.Status,
			)
			m.bus.Publish(event)
			log.Printf("Monitor: service state change - %s on %s: %s → %s",
				svc.Name, svc.Host, oldState.State, newState.State)
		}
	} else if !exists {
		// First time seeing this service
		log.Printf("Monitor: discovered service %s on %s (state: %s)", svc.Name, svc.Host, svc.State)
	}
}

// handleHostError handles a host becoming unreachable.
func (m *Monitor) handleHostError(host, reason string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	oldState, exists := m.hostStates[host]
	m.hostStates[host] = HostState{Reachable: false, LastError: reason}

	if exists && oldState.Reachable && !m.skipFirstEvent {
		event := events.NewHostUnreachableEvent(host, reason)
		m.bus.Publish(event)
		log.Printf("Monitor: host unreachable - %s: %s", host, reason)
	}
}

// handleHostSuccess handles a host becoming reachable.
func (m *Monitor) handleHostSuccess(host string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	oldState, exists := m.hostStates[host]
	m.hostStates[host] = HostState{Reachable: true}

	if exists && !oldState.Reachable && !m.skipFirstEvent {
		event := events.NewHostRecoveredEvent(host)
		m.bus.Publish(event)
		log.Printf("Monitor: host recovered - %s", host)
	}
}

// GetServiceState returns the current known state of a service.
func (m *Monitor) GetServiceState(host, serviceName string) (ServiceState, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	state, exists := m.serviceStates[host+":"+serviceName]
	return state, exists
}

// GetHostState returns the current known state of a host.
func (m *Monitor) GetHostState(host string) (HostState, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	state, exists := m.hostStates[host]
	return state, exists
}

// ServiceCount returns the number of tracked services.
func (m *Monitor) ServiceCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.serviceStates)
}

// HostCount returns the number of tracked hosts.
func (m *Monitor) HostCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.hostStates)
}

// hasHomeAssistantHosts returns true if there are Home Assistant hosts configured.
func (m *Monitor) hasHomeAssistantHosts() bool {
	for _, host := range m.cfg.Hosts {
		if host.HasHomeAssistant() {
			return true
		}
	}
	return false
}

// pollHomeAssistantHosts polls Home Assistant instances for health status.
func (m *Monitor) pollHomeAssistantHosts() {
	defer m.wg.Done()

	// Wait for initial discovery to complete before polling
	time.Sleep(2 * time.Second)

	ticker := time.NewTicker(m.pollInterval)
	defer ticker.Stop()

	// Initial poll
	m.pollHomeAssistant()

	for {
		select {
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.pollHomeAssistant()
		}
	}
}

// pollHomeAssistant fetches current health status from Home Assistant instances.
func (m *Monitor) pollHomeAssistant() {
	ctx, cancel := context.WithTimeout(context.Background(), m.pollInterval/2)
	defer cancel()

	for _, host := range m.cfg.Hosts {
		if !host.HasHomeAssistant() {
			continue
		}

		haProvider, err := homeassistant.NewProvider(&host)
		if err != nil {
			log.Printf("Monitor: failed to create Home Assistant provider for %s: %v", host.Name, err)
			m.handleHostError(host.Name, err.Error())
			continue
		}
		if haProvider == nil {
			continue
		}

		state, status, err := haProvider.CheckHealth(ctx)
		if err != nil {
			log.Printf("Monitor: Home Assistant on %s is unreachable: %v", host.Name, err)
			m.handleHostError(host.Name, err.Error())
		} else {
			m.handleHostSuccess(host.Name)
		}

		// Update service state
		svc := services.ServiceInfo{
			Name:   "homeassistant",
			Host:   host.Name,
			State:  state,
			Status: status,
			Source: "homeassistant",
		}
		m.updateServiceState(svc)
	}

	m.markDiscoveryComplete()
}
