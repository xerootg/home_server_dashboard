// Package docker provides Docker container service management.
package docker

import (
	"bufio"
	"bytes"
	"io"
	"testing"

	"github.com/docker/docker/api/types/container"

	"home_server_dashboard/services"
)

// TestExtractExposedPorts tests the extractExposedPorts function.
func TestExtractExposedPorts(t *testing.T) {
	tests := []struct {
		name     string
		ports    []container.Port
		expected []services.PortInfo
	}{
		{
			name:     "empty ports",
			ports:    []container.Port{},
			expected: nil,
		},
		{
			name: "skip localhost-only bindings",
			ports: []container.Port{
				{IP: "127.0.0.1", PrivatePort: 80, PublicPort: 8080, Type: "tcp"},
			},
			expected: nil,
		},
		{
			name: "include 0.0.0.0 bindings",
			ports: []container.Port{
				{IP: "0.0.0.0", PrivatePort: 80, PublicPort: 8080, Type: "tcp"},
			},
			expected: []services.PortInfo{
				{HostPort: 8080, ContainerPort: 80, Protocol: "tcp"},
			},
		},
		{
			name: "include empty IP bindings (all interfaces)",
			ports: []container.Port{
				{IP: "", PrivatePort: 443, PublicPort: 8443, Type: "tcp"},
			},
			expected: []services.PortInfo{
				{HostPort: 8443, ContainerPort: 443, Protocol: "tcp"},
			},
		},
		{
			name: "skip ports without public port",
			ports: []container.Port{
				{IP: "0.0.0.0", PrivatePort: 80, PublicPort: 0, Type: "tcp"},
			},
			expected: nil,
		},
		{
			name: "mixed bindings",
			ports: []container.Port{
				{IP: "127.0.0.1", PrivatePort: 3000, PublicPort: 3000, Type: "tcp"}, // skip
				{IP: "0.0.0.0", PrivatePort: 80, PublicPort: 8080, Type: "tcp"},     // include
				{IP: "", PrivatePort: 443, PublicPort: 8443, Type: "tcp"},           // include
				{IP: "0.0.0.0", PrivatePort: 53, PublicPort: 5353, Type: "udp"},     // include
			},
			expected: []services.PortInfo{
				{HostPort: 8080, ContainerPort: 80, Protocol: "tcp"},
				{HostPort: 8443, ContainerPort: 443, Protocol: "tcp"},
				{HostPort: 5353, ContainerPort: 53, Protocol: "udp"},
			},
		},
		{
			name: "include specific non-localhost IP",
			ports: []container.Port{
				{IP: "192.168.1.100", PrivatePort: 80, PublicPort: 80, Type: "tcp"},
			},
			expected: []services.PortInfo{
				{HostPort: 80, ContainerPort: 80, Protocol: "tcp"},
			},
		},
		{
			name: "deduplicate same port on IPv4 and IPv6",
			ports: []container.Port{
				{IP: "0.0.0.0", PrivatePort: 80, PublicPort: 8080, Type: "tcp"},
				{IP: "::", PrivatePort: 80, PublicPort: 8080, Type: "tcp"},
			},
			expected: []services.PortInfo{
				{HostPort: 8080, ContainerPort: 80, Protocol: "tcp"},
			},
		},
		{
			name: "deduplicate same port multiple bindings",
			ports: []container.Port{
				{IP: "0.0.0.0", PrivatePort: 80, PublicPort: 8080, Type: "tcp"},
				{IP: "", PrivatePort: 80, PublicPort: 8080, Type: "tcp"},
				{IP: "192.168.1.100", PrivatePort: 80, PublicPort: 8080, Type: "tcp"},
			},
			expected: []services.PortInfo{
				{HostPort: 8080, ContainerPort: 80, Protocol: "tcp"},
			},
		},
		{
			name: "different protocols not deduplicated",
			ports: []container.Port{
				{IP: "0.0.0.0", PrivatePort: 53, PublicPort: 53, Type: "tcp"},
				{IP: "0.0.0.0", PrivatePort: 53, PublicPort: 53, Type: "udp"},
			},
			expected: []services.PortInfo{
				{HostPort: 53, ContainerPort: 53, Protocol: "tcp"},
				{HostPort: 53, ContainerPort: 53, Protocol: "udp"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractExposedPorts(tt.ports, nil)

			if len(result) != len(tt.expected) {
				t.Fatalf("extractExposedPorts() returned %d ports, want %d", len(result), len(tt.expected))
			}

			for i, port := range result {
				if port.HostPort != tt.expected[i].HostPort {
					t.Errorf("port[%d].HostPort = %v, want %v", i, port.HostPort, tt.expected[i].HostPort)
				}
				if port.ContainerPort != tt.expected[i].ContainerPort {
					t.Errorf("port[%d].ContainerPort = %v, want %v", i, port.ContainerPort, tt.expected[i].ContainerPort)
				}
				if port.Protocol != tt.expected[i].Protocol {
					t.Errorf("port[%d].Protocol = %v, want %v", i, port.Protocol, tt.expected[i].Protocol)
				}
			}
		})
	}
}

// TestParsePort tests the parsePort helper function.
func TestParsePort(t *testing.T) {
	tests := []struct {
		input    string
		expected uint16
	}{
		{"80", 80},
		{"8080", 8080},
		{"443", 443},
		{"65535", 65535},
		{"0", 0},
		{"", 0},
		{"invalid", 0},
		{"-1", 0},
		{"65536", 0},
		{"123abc", 0},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := parsePort(tt.input)
			if result != tt.expected {
				t.Errorf("parsePort(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

// TestDockerService_Methods tests the DockerService getter methods.
func TestDockerService_Methods(t *testing.T) {
	svc := &DockerService{
		containerName: "test-container",
		hostName:      "myhost",
		client:        nil, // nil client since we're just testing getters
	}

	t.Run("GetName", func(t *testing.T) {
		if got := svc.GetName(); got != "test-container" {
			t.Errorf("GetName() = %v, want test-container", got)
		}
	})

	t.Run("GetHost", func(t *testing.T) {
		if got := svc.GetHost(); got != "myhost" {
			t.Errorf("GetHost() = %v, want myhost", got)
		}
	})

	t.Run("GetSource", func(t *testing.T) {
		if got := svc.GetSource(); got != "docker" {
			t.Errorf("GetSource() = %v, want docker", got)
		}
	})
}

// TestProviderName tests the Provider Name method.
func TestProviderName(t *testing.T) {
	p := &Provider{
		hostName: "test",
		client:   nil,
	}

	if got := p.Name(); got != "docker" {
		t.Errorf("Name() = %v, want docker", got)
	}
}

// TestDockerLogReader tests the log reader that strips Docker's 8-byte header.
func TestDockerLogReader(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected string
	}{
		{
			name: "strips 8-byte header from stdout",
			// Docker log format: [8-byte header][payload]
			// Header: stream type (1 byte), 3 zero bytes, size (4 bytes big-endian)
			input:    append([]byte{1, 0, 0, 0, 0, 0, 0, 11}, []byte("hello world\n")...),
			expected: "hello world\n",
		},
		{
			name: "strips 8-byte header from stderr",
			// Stream type 2 = stderr
			input:    append([]byte{2, 0, 0, 0, 0, 0, 0, 6}, []byte("error!\n")...),
			expected: "error!\n",
		},
		{
			name:     "short line without header passthrough",
			input:    []byte("short\n"),
			expected: "short\n",
		},
		{
			name:     "exactly 8 bytes treated as header",
			input:    append([]byte{1, 0, 0, 0, 0, 0, 0, 5}, []byte("data\n")...),
			expected: "data\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := &dockerLogReader{
				reader: bufio.NewReader(bytes.NewReader(tt.input)),
				closer: io.NopCloser(bytes.NewReader(nil)),
			}

			buf := make([]byte, 256)
			n, err := reader.Read(buf)
			if err != nil && err != io.EOF {
				t.Fatalf("Read() error = %v", err)
			}

			got := string(buf[:n])
			if got != tt.expected {
				t.Errorf("Read() = %q, want %q", got, tt.expected)
			}
		})
	}
}

// TestDockerLogReader_Close tests that Close properly closes the underlying stream.
func TestDockerLogReader_Close(t *testing.T) {
	closeCalled := false
	mockCloser := &mockReadCloser{
		closeFunc: func() error {
			closeCalled = true
			return nil
		},
	}

	reader := &dockerLogReader{
		reader: bufio.NewReader(bytes.NewReader([]byte("test\n"))),
		closer: mockCloser,
	}

	err := reader.Close()
	if err != nil {
		t.Errorf("Close() error = %v", err)
	}

	if !closeCalled {
		t.Error("Close() did not call underlying closer")
	}
}

// mockReadCloser is a mock implementation of io.ReadCloser for testing.
type mockReadCloser struct {
	readFunc  func(p []byte) (n int, err error)
	closeFunc func() error
}

func (m *mockReadCloser) Read(p []byte) (n int, err error) {
	if m.readFunc != nil {
		return m.readFunc(p)
	}
	return 0, io.EOF
}

func (m *mockReadCloser) Close() error {
	if m.closeFunc != nil {
		return m.closeFunc()
	}
	return nil
}

// TestGetService tests the GetService method returns a properly configured DockerService.
func TestGetService(t *testing.T) {
	p := &Provider{
		hostName: "testhost",
		client:   nil,
	}

	svc, err := p.GetService("my-container")
	if err != nil {
		t.Fatalf("GetService() error = %v", err)
	}

	dockerSvc, ok := svc.(*DockerService)
	if !ok {
		t.Fatal("GetService() did not return a *DockerService")
	}

	if dockerSvc.containerName != "my-container" {
		t.Errorf("containerName = %v, want my-container", dockerSvc.containerName)
	}

	if dockerSvc.hostName != "testhost" {
		t.Errorf("hostName = %v, want testhost", dockerSvc.hostName)
	}
}

// TestDockerLogReader_MultipleReads tests reading multiple log lines.
func TestDockerLogReader_MultipleReads(t *testing.T) {
	// Two log lines with Docker headers
	line1 := append([]byte{1, 0, 0, 0, 0, 0, 0, 7}, []byte("line 1\n")...)
	line2 := append([]byte{1, 0, 0, 0, 0, 0, 0, 7}, []byte("line 2\n")...)
	input := append(line1, line2...)

	reader := &dockerLogReader{
		reader: bufio.NewReader(bytes.NewReader(input)),
		closer: io.NopCloser(bytes.NewReader(nil)),
	}

	// Read first line
	buf := make([]byte, 256)
	n, err := reader.Read(buf)
	if err != nil {
		t.Fatalf("First Read() error = %v", err)
	}
	if got := string(buf[:n]); got != "line 1\n" {
		t.Errorf("First Read() = %q, want %q", got, "line 1\n")
	}

	// Read second line
	n, err = reader.Read(buf)
	if err != nil {
		t.Fatalf("Second Read() error = %v", err)
	}
	if got := string(buf[:n]); got != "line 2\n" {
		t.Errorf("Second Read() = %q, want %q", got, "line 2\n")
	}
}

// TestClose tests the Provider Close method.
func TestClose(t *testing.T) {
	p := &Provider{
		hostName: "test",
		client:   nil, // nil client should not cause panic
	}

	err := p.Close()
	if err != nil {
		t.Errorf("Close() with nil client error = %v", err)
	}
}

// BenchmarkDockerLogReader benchmarks the log reader performance.
func BenchmarkDockerLogReader(b *testing.B) {
	// Create a realistic log line with header
	logLine := append([]byte{1, 0, 0, 0, 0, 0, 0, 100}, bytes.Repeat([]byte("x"), 100)...)
	logLine = append(logLine, '\n')

	// Create many lines
	var input []byte
	for i := 0; i < 1000; i++ {
		input = append(input, logLine...)
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		reader := &dockerLogReader{
			reader: bufio.NewReader(bytes.NewReader(input)),
			closer: io.NopCloser(bytes.NewReader(nil)),
		}

		buf := make([]byte, 256)
		for {
			_, err := reader.Read(buf)
			if err != nil {
				break
			}
		}
	}
}

// TestDescriptionLabelKey verifies the description label constant is correct.
func TestDescriptionLabelKey(t *testing.T) {
	// This test documents the expected label key for service descriptions
	expectedLabelKey := "home.server.dashboard.description"

	// Simulate extracting description from labels (mirrors the logic in GetServices and GetInfo)
	labels := map[string]string{
		"com.docker.compose.project":          "testproject",
		"com.docker.compose.service":          "testservice",
		"home.server.dashboard.description":   "My custom service description",
	}

	description := labels[expectedLabelKey]
	if description != "My custom service description" {
		t.Errorf("Description extraction failed, got %q, want %q", description, "My custom service description")
	}

	// Test with empty description
	emptyLabels := map[string]string{
		"com.docker.compose.project": "testproject",
		"com.docker.compose.service": "testservice",
	}

	emptyDescription := emptyLabels[expectedLabelKey]
	if emptyDescription != "" {
		t.Errorf("Expected empty description for labels without description key, got %q", emptyDescription)
	}
}

// TestIsLabelTrue tests the isLabelTrue helper function.
func TestIsLabelTrue(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"true", true},
		{"True", true},
		{"TRUE", true},
		{"1", true},
		{"yes", true},
		{"Yes", true},
		{"YES", true},
		{"false", false},
		{"0", false},
		{"no", false},
		{"", false},
		{"random", false},
		{" true ", true},  // With whitespace
		{" 1 ", true},     // With whitespace
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := isLabelTrue(tt.input)
			if result != tt.expected {
				t.Errorf("isLabelTrue(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

// TestParseHiddenPorts tests the parseHiddenPorts helper function.
func TestParseHiddenPorts(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected map[uint16]bool
	}{
		{
			name:     "empty string",
			input:    "",
			expected: map[uint16]bool{},
		},
		{
			name:     "single port",
			input:    "8080",
			expected: map[uint16]bool{8080: true},
		},
		{
			name:  "multiple ports",
			input: "80,443,8080",
			expected: map[uint16]bool{
				80:   true,
				443:  true,
				8080: true,
			},
		},
		{
			name:  "ports with spaces",
			input: " 80 , 443 , 8080 ",
			expected: map[uint16]bool{
				80:   true,
				443:  true,
				8080: true,
			},
		},
		{
			name:  "ignores invalid values",
			input: "80,invalid,443,65536,-1",
			expected: map[uint16]bool{
				80:  true,
				443: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseHiddenPorts(tt.input)
			if len(result) != len(tt.expected) {
				t.Fatalf("parseHiddenPorts(%q) returned %d ports, want %d", tt.input, len(result), len(tt.expected))
			}
			for port, expected := range tt.expected {
				if result[port] != expected {
					t.Errorf("parseHiddenPorts(%q)[%d] = %v, want %v", tt.input, port, result[port], expected)
				}
			}
		})
	}
}

// TestGetPortLabel tests the getPortLabel helper function.
func TestGetPortLabel(t *testing.T) {
	labels := map[string]string{
		"home.server.dashboard.ports.8080.label": "Admin",
		"home.server.dashboard.ports.443.label":  "HTTPS",
		"home.server.dashboard.ports.80.label":   "",
	}

	tests := []struct {
		port     uint16
		expected string
	}{
		{8080, "Admin"},
		{443, "HTTPS"},
		{80, ""},
		{9000, ""},  // Not in labels
	}

	for _, tt := range tests {
		t.Run(string(rune(tt.port)), func(t *testing.T) {
			result := getPortLabel(labels, tt.port)
			if result != tt.expected {
				t.Errorf("getPortLabel(labels, %d) = %q, want %q", tt.port, result, tt.expected)
			}
		})
	}
}

// TestIsPortHiddenByLabel tests the isPortHiddenByLabel helper function.
func TestIsPortHiddenByLabel(t *testing.T) {
	labels := map[string]string{
		"home.server.dashboard.ports.8080.hidden": "true",
		"home.server.dashboard.ports.443.hidden":  "false",
		"home.server.dashboard.ports.80.hidden":   "1",
		"home.server.dashboard.ports.9000.hidden": "yes",
	}

	tests := []struct {
		port     uint16
		expected bool
	}{
		{8080, true},
		{443, false},
		{80, true},
		{9000, true},
		{3000, false},  // Not in labels
	}

	for _, tt := range tests {
		t.Run(string(rune(tt.port)), func(t *testing.T) {
			result := isPortHiddenByLabel(labels, tt.port)
			if result != tt.expected {
				t.Errorf("isPortHiddenByLabel(labels, %d) = %v, want %v", tt.port, result, tt.expected)
			}
		})
	}
}

// TestExtractExposedPortsWithLabels tests extractExposedPorts with label customizations.
func TestExtractExposedPortsWithLabels(t *testing.T) {
	tests := []struct {
		name     string
		ports    []container.Port
		labels   map[string]string
		expected []services.PortInfo
	}{
		{
			name: "port with custom label",
			ports: []container.Port{
				{IP: "0.0.0.0", PrivatePort: 80, PublicPort: 8080, Type: "tcp"},
			},
			labels: map[string]string{
				"home.server.dashboard.ports.8080.label": "Admin Panel",
			},
			expected: []services.PortInfo{
				{HostPort: 8080, ContainerPort: 80, Protocol: "tcp", Label: "Admin Panel", Hidden: false},
			},
		},
		{
			name: "port hidden by ports.hidden list",
			ports: []container.Port{
				{IP: "0.0.0.0", PrivatePort: 80, PublicPort: 8080, Type: "tcp"},
				{IP: "0.0.0.0", PrivatePort: 443, PublicPort: 8443, Type: "tcp"},
			},
			labels: map[string]string{
				"home.server.dashboard.ports.hidden": "8080",
			},
			expected: []services.PortInfo{
				{HostPort: 8080, ContainerPort: 80, Protocol: "tcp", Label: "", Hidden: true},
				{HostPort: 8443, ContainerPort: 443, Protocol: "tcp", Label: "", Hidden: false},
			},
		},
		{
			name: "port hidden by individual label",
			ports: []container.Port{
				{IP: "0.0.0.0", PrivatePort: 80, PublicPort: 8080, Type: "tcp"},
			},
			labels: map[string]string{
				"home.server.dashboard.ports.8080.hidden": "true",
			},
			expected: []services.PortInfo{
				{HostPort: 8080, ContainerPort: 80, Protocol: "tcp", Label: "", Hidden: true},
			},
		},
		{
			name: "combined label and hidden",
			ports: []container.Port{
				{IP: "0.0.0.0", PrivatePort: 80, PublicPort: 8080, Type: "tcp"},
				{IP: "0.0.0.0", PrivatePort: 443, PublicPort: 8443, Type: "tcp"},
			},
			labels: map[string]string{
				"home.server.dashboard.ports.8080.label":  "Admin",
				"home.server.dashboard.ports.8443.hidden": "true",
			},
			expected: []services.PortInfo{
				{HostPort: 8080, ContainerPort: 80, Protocol: "tcp", Label: "Admin", Hidden: false},
				{HostPort: 8443, ContainerPort: 443, Protocol: "tcp", Label: "", Hidden: true},
			},
		},
		{
			name: "nil labels work",
			ports: []container.Port{
				{IP: "0.0.0.0", PrivatePort: 80, PublicPort: 8080, Type: "tcp"},
			},
			labels: nil,
			expected: []services.PortInfo{
				{HostPort: 8080, ContainerPort: 80, Protocol: "tcp", Label: "", Hidden: false},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractExposedPorts(tt.ports, tt.labels)

			if len(result) != len(tt.expected) {
				t.Fatalf("extractExposedPorts() returned %d ports, want %d", len(result), len(tt.expected))
			}

			for i, port := range result {
				if port.HostPort != tt.expected[i].HostPort {
					t.Errorf("port[%d].HostPort = %v, want %v", i, port.HostPort, tt.expected[i].HostPort)
				}
				if port.ContainerPort != tt.expected[i].ContainerPort {
					t.Errorf("port[%d].ContainerPort = %v, want %v", i, port.ContainerPort, tt.expected[i].ContainerPort)
				}
				if port.Protocol != tt.expected[i].Protocol {
					t.Errorf("port[%d].Protocol = %v, want %v", i, port.Protocol, tt.expected[i].Protocol)
				}
				if port.Label != tt.expected[i].Label {
					t.Errorf("port[%d].Label = %q, want %q", i, port.Label, tt.expected[i].Label)
				}
				if port.Hidden != tt.expected[i].Hidden {
					t.Errorf("port[%d].Hidden = %v, want %v", i, port.Hidden, tt.expected[i].Hidden)
				}
			}
		})
	}
}

// TestLabelConstants verifies the label constants are correct.
func TestLabelConstants(t *testing.T) {
	if LabelPrefix != "home.server.dashboard" {
		t.Errorf("LabelPrefix = %q, want %q", LabelPrefix, "home.server.dashboard")
	}
	if LabelDescription != "home.server.dashboard.description" {
		t.Errorf("LabelDescription = %q, want %q", LabelDescription, "home.server.dashboard.description")
	}
	if LabelHidden != "home.server.dashboard.hidden" {
		t.Errorf("LabelHidden = %q, want %q", LabelHidden, "home.server.dashboard.hidden")
	}
	if LabelPortsPrefix != "home.server.dashboard.ports" {
		t.Errorf("LabelPortsPrefix = %q, want %q", LabelPortsPrefix, "home.server.dashboard.ports")
	}
	if LabelPortsHidden != "home.server.dashboard.ports.hidden" {
		t.Errorf("LabelPortsHidden = %q, want %q", LabelPortsHidden, "home.server.dashboard.ports.hidden")
	}
	if LabelRemapPortPrefix != "home.server.dashboard.remapport" {
		t.Errorf("LabelRemapPortPrefix = %q, want %q", LabelRemapPortPrefix, "home.server.dashboard.remapport")
	}
}

// TestParsePortRemaps tests the parsePortRemaps function.
func TestParsePortRemaps(t *testing.T) {
	tests := []struct {
		name          string
		labels        map[string]string
		sourceService string
		expected      []PortRemap
	}{
		{
			name:          "no remap labels",
			labels:        map[string]string{},
			sourceService: "gluetun",
			expected:      nil,
		},
		{
			name: "single port remap",
			labels: map[string]string{
				"home.server.dashboard.remapport.8193": "qbittorrent-books",
			},
			sourceService: "gluetun",
			expected: []PortRemap{
				{Port: 8193, TargetService: "qbittorrent-books", SourceService: "gluetun"},
			},
		},
		{
			name: "multiple port remaps",
			labels: map[string]string{
				"home.server.dashboard.remapport.8193": "qbittorrent-books",
				"home.server.dashboard.remapport.8194": "qbittorrent-movies",
				"home.server.dashboard.remapport.9117": "jackett",
			},
			sourceService: "gluetun",
			expected: []PortRemap{
				{Port: 8193, TargetService: "qbittorrent-books", SourceService: "gluetun"},
				{Port: 8194, TargetService: "qbittorrent-movies", SourceService: "gluetun"},
				{Port: 9117, TargetService: "jackett", SourceService: "gluetun"},
			},
		},
		{
			name: "ignore non-remap labels",
			labels: map[string]string{
				"home.server.dashboard.remapport.8193": "qbittorrent-books",
				"home.server.dashboard.description":    "VPN container",
				"home.server.dashboard.ports.8080.label": "Web UI",
			},
			sourceService: "gluetun",
			expected: []PortRemap{
				{Port: 8193, TargetService: "qbittorrent-books", SourceService: "gluetun"},
			},
		},
		{
			name: "ignore invalid port numbers",
			labels: map[string]string{
				"home.server.dashboard.remapport.abc":    "service1",
				"home.server.dashboard.remapport.99999":  "service2",
				"home.server.dashboard.remapport.-1":     "service3",
				"home.server.dashboard.remapport.8193":   "valid-service",
			},
			sourceService: "gluetun",
			expected: []PortRemap{
				{Port: 8193, TargetService: "valid-service", SourceService: "gluetun"},
			},
		},
		{
			name: "ignore empty target service",
			labels: map[string]string{
				"home.server.dashboard.remapport.8193": "",
				"home.server.dashboard.remapport.8194": "   ",
				"home.server.dashboard.remapport.8195": "valid-service",
			},
			sourceService: "gluetun",
			expected: []PortRemap{
				{Port: 8195, TargetService: "valid-service", SourceService: "gluetun"},
			},
		},
		{
			name: "trim whitespace from target service",
			labels: map[string]string{
				"home.server.dashboard.remapport.8193": "  qbittorrent-books  ",
			},
			sourceService: "gluetun",
			expected: []PortRemap{
				{Port: 8193, TargetService: "qbittorrent-books", SourceService: "gluetun"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parsePortRemaps(tt.labels, tt.sourceService)

			if len(tt.expected) == 0 && len(result) == 0 {
				return // Both are empty, test passes
			}

			// Check that all expected remaps are present (order may vary due to map iteration)
			if len(result) != len(tt.expected) {
				t.Fatalf("parsePortRemaps() returned %d remaps, want %d", len(result), len(tt.expected))
			}

			// Create a map for easier lookup
			resultMap := make(map[uint16]PortRemap)
			for _, r := range result {
				resultMap[r.Port] = r
			}

			for _, exp := range tt.expected {
				got, ok := resultMap[exp.Port]
				if !ok {
					t.Errorf("expected remap for port %d not found", exp.Port)
					continue
				}
				if got.TargetService != exp.TargetService {
					t.Errorf("port %d: TargetService = %q, want %q", exp.Port, got.TargetService, exp.TargetService)
				}
				if got.SourceService != exp.SourceService {
					t.Errorf("port %d: SourceService = %q, want %q", exp.Port, got.SourceService, exp.SourceService)
				}
			}
		})
	}
}
