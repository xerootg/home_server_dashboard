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
