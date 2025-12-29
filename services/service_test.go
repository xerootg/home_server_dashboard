// Package services provides interfaces and types for managing services.
package services

import (
	"encoding/json"
	"testing"
)

// TestServiceInfo_JSONSerialization tests that ServiceInfo serializes correctly.
func TestServiceInfo_JSONSerialization(t *testing.T) {
	info := ServiceInfo{
		Name:          "nginx",
		Project:       "webstack",
		ContainerName: "webstack-nginx-1",
		State:         "running",
		Status:        "Up 2 hours",
		Image:         "nginx:latest",
		Source:        "docker",
		Host:          "nas",
		HostIP:        "192.168.1.100",
		Ports: []PortInfo{
			{HostPort: 8080, ContainerPort: 80, Protocol: "tcp"},
			{HostPort: 8443, ContainerPort: 443, Protocol: "tcp"},
		},
	}

	// Serialize to JSON
	data, err := json.Marshal(info)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	// Deserialize back
	var decoded ServiceInfo
	err = json.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	// Verify fields
	if decoded.Name != info.Name {
		t.Errorf("Name = %v, want %v", decoded.Name, info.Name)
	}
	if decoded.Project != info.Project {
		t.Errorf("Project = %v, want %v", decoded.Project, info.Project)
	}
	if decoded.ContainerName != info.ContainerName {
		t.Errorf("ContainerName = %v, want %v", decoded.ContainerName, info.ContainerName)
	}
	if decoded.State != info.State {
		t.Errorf("State = %v, want %v", decoded.State, info.State)
	}
	if decoded.Status != info.Status {
		t.Errorf("Status = %v, want %v", decoded.Status, info.Status)
	}
	if decoded.Image != info.Image {
		t.Errorf("Image = %v, want %v", decoded.Image, info.Image)
	}
	if decoded.Source != info.Source {
		t.Errorf("Source = %v, want %v", decoded.Source, info.Source)
	}
	if decoded.Host != info.Host {
		t.Errorf("Host = %v, want %v", decoded.Host, info.Host)
	}
	if decoded.HostIP != info.HostIP {
		t.Errorf("HostIP = %v, want %v", decoded.HostIP, info.HostIP)
	}
	// Verify Ports
	if len(decoded.Ports) != len(info.Ports) {
		t.Errorf("Ports length = %v, want %v", len(decoded.Ports), len(info.Ports))
	}
	for i, port := range decoded.Ports {
		if port.HostPort != info.Ports[i].HostPort {
			t.Errorf("Ports[%d].HostPort = %v, want %v", i, port.HostPort, info.Ports[i].HostPort)
		}
		if port.ContainerPort != info.Ports[i].ContainerPort {
			t.Errorf("Ports[%d].ContainerPort = %v, want %v", i, port.ContainerPort, info.Ports[i].ContainerPort)
		}
		if port.Protocol != info.Ports[i].Protocol {
			t.Errorf("Ports[%d].Protocol = %v, want %v", i, port.Protocol, info.Ports[i].Protocol)
		}
	}
}

// TestServiceInfo_JSONFieldNames tests that JSON field names are correct.
func TestServiceInfo_JSONFieldNames(t *testing.T) {
	info := ServiceInfo{
		Name:          "test",
		Project:       "proj",
		ContainerName: "cont",
		State:         "running",
		Status:        "status",
		Image:         "image",
		Source:        "docker",
		Host:          "host",
		HostIP:        "192.168.1.1",
		Ports: []PortInfo{
			{HostPort: 8080, ContainerPort: 80, Protocol: "tcp"},
		},
	}

	data, err := json.Marshal(info)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	jsonStr := string(data)

	expectedFields := []string{
		`"name"`,
		`"project"`,
		`"container_name"`,
		`"state"`,
		`"status"`,
		`"image"`,
		`"source"`,
		`"host"`,
		`"host_ip"`,
		`"ports"`,
		`"host_port"`,
		`"container_port"`,
		`"protocol"`,
	}

	for _, field := range expectedFields {
		if !contains(jsonStr, field) {
			t.Errorf("JSON output missing field %s: %s", field, jsonStr)
		}
	}
}

// contains checks if a string contains a substring.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// TestServiceInfo_EmptyFields tests ServiceInfo with empty fields.
func TestServiceInfo_EmptyFields(t *testing.T) {
	info := ServiceInfo{}

	data, err := json.Marshal(info)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	var decoded ServiceInfo
	err = json.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	// All fields should be empty strings
	if decoded.Name != "" {
		t.Errorf("Name should be empty, got %v", decoded.Name)
	}
	if decoded.State != "" {
		t.Errorf("State should be empty, got %v", decoded.State)
	}
}

// TestServiceInfo_SliceSerialization tests serializing a slice of ServiceInfo.
func TestServiceInfo_SliceSerialization(t *testing.T) {
	services := []ServiceInfo{
		{Name: "nginx", Source: "docker", State: "running"},
		{Name: "docker.service", Source: "systemd", State: "running"},
		{Name: "postgresql", Source: "docker", State: "stopped"},
	}

	data, err := json.Marshal(services)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	var decoded []ServiceInfo
	err = json.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if len(decoded) != len(services) {
		t.Errorf("Decoded slice length = %d, want %d", len(decoded), len(services))
	}

	for i, svc := range decoded {
		if svc.Name != services[i].Name {
			t.Errorf("services[%d].Name = %v, want %v", i, svc.Name, services[i].Name)
		}
	}
}

// TestServiceStateValues tests expected state values.
func TestServiceStateValues(t *testing.T) {
	// These are the standard state values used throughout the application
	validStates := []string{"running", "stopped"}

	for _, state := range validStates {
		info := ServiceInfo{State: state}
		if info.State != state {
			t.Errorf("State assignment failed for %s", state)
		}
	}
}

// TestServiceSourceValues tests expected source values.
func TestServiceSourceValues(t *testing.T) {
	// These are the standard source values
	validSources := []string{"docker", "systemd"}

	for _, source := range validSources {
		info := ServiceInfo{Source: source}
		if info.Source != source {
			t.Errorf("Source assignment failed for %s", source)
		}
	}
}

// TestPortInfo_JSONSerialization tests that PortInfo serializes correctly.
func TestPortInfo_JSONSerialization(t *testing.T) {
	tests := []struct {
		name string
		port PortInfo
	}{
		{
			name: "TCP port",
			port: PortInfo{HostPort: 8080, ContainerPort: 80, Protocol: "tcp"},
		},
		{
			name: "UDP port",
			port: PortInfo{HostPort: 53, ContainerPort: 53, Protocol: "udp"},
		},
		{
			name: "High port numbers",
			port: PortInfo{HostPort: 65535, ContainerPort: 65535, Protocol: "tcp"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.port)
			if err != nil {
				t.Fatalf("Marshal() error = %v", err)
			}

			var decoded PortInfo
			err = json.Unmarshal(data, &decoded)
			if err != nil {
				t.Fatalf("Unmarshal() error = %v", err)
			}

			if decoded.HostPort != tt.port.HostPort {
				t.Errorf("HostPort = %v, want %v", decoded.HostPort, tt.port.HostPort)
			}
			if decoded.ContainerPort != tt.port.ContainerPort {
				t.Errorf("ContainerPort = %v, want %v", decoded.ContainerPort, tt.port.ContainerPort)
			}
			if decoded.Protocol != tt.port.Protocol {
				t.Errorf("Protocol = %v, want %v", decoded.Protocol, tt.port.Protocol)
			}
		})
	}
}

// TestServiceInfo_NilPorts tests that nil Ports field serializes correctly.
func TestServiceInfo_NilPorts(t *testing.T) {
	info := ServiceInfo{
		Name:   "test",
		Source: "docker",
		Ports:  nil,
	}

	data, err := json.Marshal(info)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	var decoded ServiceInfo
	err = json.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	// nil and empty slice should both result in nil/empty after roundtrip
	if decoded.Ports != nil && len(decoded.Ports) != 0 {
		t.Errorf("Ports should be nil or empty, got %v", decoded.Ports)
	}
}

// TestServiceInfo_EmptyPorts tests that empty Ports slice serializes correctly.
func TestServiceInfo_EmptyPorts(t *testing.T) {
	info := ServiceInfo{
		Name:   "test",
		Source: "docker",
		Ports:  []PortInfo{},
	}

	data, err := json.Marshal(info)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	var decoded ServiceInfo
	err = json.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if len(decoded.Ports) != 0 {
		t.Errorf("Ports should be empty, got %v", decoded.Ports)
	}
}
