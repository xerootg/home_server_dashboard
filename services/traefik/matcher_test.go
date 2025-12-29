package traefik

import (
	"bytes"
	"errors"
	"log"
	"strings"
	"testing"
)

func TestExtractMatchers(t *testing.T) {
	tests := []struct {
		name     string
		rule     string
		expected []MatcherInfo
	}{
		{
			name: "single Host with backticks",
			rule: "Host(`example.com`)",
			expected: []MatcherInfo{
				{Type: MatcherTypeHost, Hostname: "example.com", OriginalPattern: "example.com", IsExact: true},
			},
		},
		{
			name: "single Host with double quotes",
			rule: `Host("example.com")`,
			expected: []MatcherInfo{
				{Type: MatcherTypeHost, Hostname: "example.com", OriginalPattern: "example.com", IsExact: true},
			},
		},
		{
			name: "multiple Hosts with OR",
			rule: "Host(`a.example.com`) || Host(`b.example.com`)",
			expected: []MatcherInfo{
				{Type: MatcherTypeHost, Hostname: "a.example.com", OriginalPattern: "a.example.com", IsExact: true},
				{Type: MatcherTypeHost, Hostname: "b.example.com", OriginalPattern: "b.example.com", IsExact: true},
			},
		},
		{
			name: "HostRegexp with simple escaped hostname",
			rule: "HostRegexp(`^example\\.com$`)",
			expected: []MatcherInfo{
				{Type: MatcherTypeHostRegexp, Hostname: "example.com", OriginalPattern: `^example\.com$`, IsExact: true},
			},
		},
		{
			name: "HostRegexp with variable subdomain",
			rule: "HostRegexp(`{subdomain:[a-z]+}.example.com`)",
			expected: []MatcherInfo{
				{Type: MatcherTypeHostRegexp, Hostname: "example.com", OriginalPattern: `{subdomain:[a-z]+}.example.com`, IsExact: false},
			},
		},
		{
			name: "HostRegexp with wildcard prefix",
			rule: "HostRegexp(`.*\\.example\\.com`)",
			expected: []MatcherInfo{
				{Type: MatcherTypeHostRegexp, Hostname: "example.com", OriginalPattern: `.*\.example\.com`, IsExact: false},
			},
		},
		{
			name: "mixed Host and HostRegexp - prefer Host",
			rule: "Host(`api.example.com`) || HostRegexp(`{subdomain:[a-z]+}.example.com`)",
			expected: []MatcherInfo{
				// Only Host() is returned when both are present
				{Type: MatcherTypeHost, Hostname: "api.example.com", OriginalPattern: "api.example.com", IsExact: true},
			},
		},
		{
			name: "Host with path prefix",
			rule: "Host(`example.com`) && PathPrefix(`/api`)",
			expected: []MatcherInfo{
				{Type: MatcherTypeHost, Hostname: "example.com", OriginalPattern: "example.com", IsExact: true},
			},
		},
		{
			name:     "no host matcher",
			rule:     "PathPrefix(`/api`)",
			expected: nil,
		},
		{
			name:     "empty rule",
			rule:     "",
			expected: nil,
		},
		{
			name: "Host with spaces",
			rule: "Host( `example.com` )",
			expected: []MatcherInfo{
				{Type: MatcherTypeHost, Hostname: "example.com", OriginalPattern: "example.com", IsExact: true},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractMatchers(tt.rule)
			if len(result) != len(tt.expected) {
				t.Errorf("ExtractMatchers(%q) returned %d matchers, expected %d", tt.rule, len(result), len(tt.expected))
				return
			}
			for i, m := range result {
				exp := tt.expected[i]
				if m.Type != exp.Type {
					t.Errorf("matcher[%d].Type = %v, expected %v", i, m.Type, exp.Type)
				}
				if m.Hostname != exp.Hostname {
					t.Errorf("matcher[%d].Hostname = %q, expected %q", i, m.Hostname, exp.Hostname)
				}
				if m.OriginalPattern != exp.OriginalPattern {
					t.Errorf("matcher[%d].OriginalPattern = %q, expected %q", i, m.OriginalPattern, exp.OriginalPattern)
				}
				if m.IsExact != exp.IsExact {
					t.Errorf("matcher[%d].IsExact = %v, expected %v", i, m.IsExact, exp.IsExact)
				}
			}
		})
	}
}

func TestParseHostRegexpPattern(t *testing.T) {
	tests := []struct {
		name     string
		pattern  string
		expected *MatcherInfo
	}{
		{
			name:    "simple anchored hostname",
			pattern: `^example\.com$`,
			expected: &MatcherInfo{
				Type:            MatcherTypeHostRegexp,
				Hostname:        "example.com",
				OriginalPattern: `^example\.com$`,
				IsExact:         true,
			},
		},
		{
			name:    "hostname without anchors",
			pattern: `example\.com`,
			expected: &MatcherInfo{
				Type:            MatcherTypeHostRegexp,
				Hostname:        "example.com",
				OriginalPattern: `example\.com`,
				IsExact:         true,
			},
		},
		{
			name:    "variable subdomain with named group",
			pattern: `{subdomain:[a-z]+}.example.com`,
			expected: &MatcherInfo{
				Type:            MatcherTypeHostRegexp,
				Hostname:        "example.com",
				OriginalPattern: `{subdomain:[a-z]+}.example.com`,
				IsExact:         false,
			},
		},
		{
			name:    "wildcard prefix",
			pattern: `.*\.example\.com`,
			expected: &MatcherInfo{
				Type:            MatcherTypeHostRegexp,
				Hostname:        "example.com",
				OriginalPattern: `.*\.example\.com`,
				IsExact:         false,
			},
		},
		{
			name:    "alternation prefix",
			pattern: `(foo|bar)\.example\.com`,
			expected: &MatcherInfo{
				Type:            MatcherTypeHostRegexp,
				Hostname:        "example.com",
				OriginalPattern: `(foo|bar)\.example\.com`,
				IsExact:         false,
			},
		},
		{
			name:    "subdomain with hyphen",
			pattern: `^sub-domain\.example\.com$`,
			expected: &MatcherInfo{
				Type:            MatcherTypeHostRegexp,
				Hostname:        "sub-domain.example.com",
				OriginalPattern: `^sub-domain\.example\.com$`,
				IsExact:         true,
			},
		},
		{
			name:     "complex regex with no extractable domain",
			pattern:  `^[a-z0-9]+$`,
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseHostRegexpPattern(tt.pattern)
			if tt.expected == nil {
				if result != nil {
					t.Errorf("parseHostRegexpPattern(%q) = %+v, expected nil", tt.pattern, result)
				}
				return
			}
			if result == nil {
				t.Errorf("parseHostRegexpPattern(%q) = nil, expected %+v", tt.pattern, tt.expected)
				return
			}
			if result.Type != tt.expected.Type {
				t.Errorf("Type = %v, expected %v", result.Type, tt.expected.Type)
			}
			if result.Hostname != tt.expected.Hostname {
				t.Errorf("Hostname = %q, expected %q", result.Hostname, tt.expected.Hostname)
			}
			if result.OriginalPattern != tt.expected.OriginalPattern {
				t.Errorf("OriginalPattern = %q, expected %q", result.OriginalPattern, tt.expected.OriginalPattern)
			}
			if result.IsExact != tt.expected.IsExact {
				t.Errorf("IsExact = %v, expected %v", result.IsExact, tt.expected.IsExact)
			}
		})
	}
}

func TestMatcherLookupServiceBasic(t *testing.T) {
	svc := NewMatcherLookupService("testhost")

	// First call should extract hostnames
	hostnames := svc.ProcessRouter("router1@docker", "Host(`example.com`)", nil)
	if len(hostnames) != 1 || hostnames[0] != "example.com" {
		t.Errorf("ProcessRouter returned %v, expected [example.com]", hostnames)
	}

	// Second call with same rule should return cached hostnames
	hostnames = svc.ProcessRouter("router1@docker", "Host(`example.com`)", nil)
	if len(hostnames) != 1 || hostnames[0] != "example.com" {
		t.Errorf("ProcessRouter second call returned %v, expected [example.com]", hostnames)
	}
}

func TestMatcherLookupServiceStateTracking(t *testing.T) {
	svc := NewMatcherLookupService("testhost")

	// Initial state
	svc.ProcessRouter("router1@docker", "Host(`old.example.com`)", nil)
	state := svc.GetState("router1@docker")
	if state == nil {
		t.Fatal("GetState returned nil after ProcessRouter")
	}
	if state.Rule != "Host(`old.example.com`)" {
		t.Errorf("state.Rule = %q, expected %q", state.Rule, "Host(`old.example.com`)")
	}
	if len(state.Hostnames) != 1 || state.Hostnames[0] != "old.example.com" {
		t.Errorf("state.Hostnames = %v, expected [old.example.com]", state.Hostnames)
	}
}

func TestMatcherLookupServiceRuleChange(t *testing.T) {
	// Capture log output
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(nil)

	svc := NewMatcherLookupService("testhost")

	// Initial state
	svc.ProcessRouter("router1@docker", "Host(`old.example.com`)", nil)

	// Rule change should log
	hostnames := svc.ProcessRouter("router1@docker", "Host(`new.example.com`)", nil)
	if len(hostnames) != 1 || hostnames[0] != "new.example.com" {
		t.Errorf("ProcessRouter after rule change returned %v, expected [new.example.com]", hostnames)
	}

	logOutput := buf.String()
	if !strings.Contains(logOutput, "rule changed") {
		t.Errorf("Expected log to contain 'rule changed', got: %s", logOutput)
	}
	if !strings.Contains(logOutput, "old.example.com") {
		t.Errorf("Expected log to contain old hostname, got: %s", logOutput)
	}
	if !strings.Contains(logOutput, "new.example.com") {
		t.Errorf("Expected log to contain new hostname, got: %s", logOutput)
	}
}

func TestMatcherLookupServiceErrorRecovery(t *testing.T) {
	// Capture log output
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(nil)

	svc := NewMatcherLookupService("testhost")

	testError := errors.New("connection failed")

	// First call with error
	hostnames := svc.ProcessRouter("router1@docker", "Host(`example.com`)", testError)
	if hostnames != nil {
		t.Errorf("ProcessRouter with error returned %v, expected nil", hostnames)
	}

	buf.Reset()

	// Recovery - same router now succeeds
	hostnames = svc.ProcessRouter("router1@docker", "Host(`example.com`)", nil)
	if len(hostnames) != 1 || hostnames[0] != "example.com" {
		t.Errorf("ProcessRouter after recovery returned %v, expected [example.com]", hostnames)
	}

	logOutput := buf.String()
	if !strings.Contains(logOutput, "recovered from error") {
		t.Errorf("Expected log to contain 'recovered from error', got: %s", logOutput)
	}
}

func TestMatcherLookupServiceHostRegexpLogging(t *testing.T) {
	// Capture log output
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(nil)

	svc := NewMatcherLookupService("testhost")

	// HostRegexp with variable pattern should log
	hostnames := svc.ProcessRouter("router1@docker", "HostRegexp(`{subdomain:[a-z]+}.example.com`)", nil)
	if len(hostnames) != 1 || hostnames[0] != "example.com" {
		t.Errorf("ProcessRouter with HostRegexp returned %v, expected [example.com]", hostnames)
	}

	logOutput := buf.String()
	if !strings.Contains(logOutput, "HostRegexp pattern") {
		t.Errorf("Expected log to contain 'HostRegexp pattern', got: %s", logOutput)
	}
	if !strings.Contains(logOutput, "extracted domain") {
		t.Errorf("Expected log to contain 'extracted domain', got: %s", logOutput)
	}
}

func TestMatcherLookupServiceClearState(t *testing.T) {
	svc := NewMatcherLookupService("testhost")

	svc.ProcessRouter("router1@docker", "Host(`example.com`)", nil)
	svc.ProcessRouter("router2@docker", "Host(`other.com`)", nil)

	// Clear single router
	svc.ClearState("router1@docker")
	if svc.GetState("router1@docker") != nil {
		t.Error("ClearState did not remove router1@docker state")
	}
	if svc.GetState("router2@docker") == nil {
		t.Error("ClearState removed router2@docker state (should not have)")
	}

	// Clear all
	svc.ClearAllState()
	if svc.GetState("router2@docker") != nil {
		t.Error("ClearAllState did not remove all states")
	}
}

func TestMatcherLookupServiceConcurrency(t *testing.T) {
	svc := NewMatcherLookupService("testhost")

	// Run concurrent calls
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(n int) {
			svc.ProcessRouter("router1@docker", "Host(`example.com`)", nil)
			svc.GetState("router1@docker")
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify state is consistent
	state := svc.GetState("router1@docker")
	if state == nil {
		t.Error("State should not be nil after concurrent access")
	}
	if len(state.Hostnames) != 1 || state.Hostnames[0] != "example.com" {
		t.Errorf("Unexpected hostnames after concurrent access: %v", state.Hostnames)
	}
}

func TestExtractHostnamesBackwardsCompatibility(t *testing.T) {
	// Test that ExtractHostnames still works as the original function
	tests := []struct {
		rule     string
		expected []string
	}{
		{"Host(`example.com`)", []string{"example.com"}},
		{"Host(`a.com`) || Host(`b.com`)", []string{"a.com", "b.com"}},
		{"Host(`example.com`) && PathPrefix(`/api`)", []string{"example.com"}},
		{"PathPrefix(`/api`)", nil},
		{"", nil},
		// Now also works with HostRegexp (only when no Host() present)
		{"HostRegexp(`^example\\.com$`)", []string{"example.com"}},
		// When Host() is present, HostRegexp is ignored
		{"Host(`exact.com`) || HostRegexp(`{sub:[a-z]+}.wildcard.com`)", []string{"exact.com"}},
	}

	for _, tt := range tests {
		t.Run(tt.rule, func(t *testing.T) {
			result := ExtractHostnames(tt.rule)
			if len(result) != len(tt.expected) {
				t.Errorf("ExtractHostnames(%q) = %v, expected %v", tt.rule, result, tt.expected)
				return
			}
			for i, v := range result {
				if v != tt.expected[i] {
					t.Errorf("ExtractHostnames(%q)[%d] = %q, expected %q", tt.rule, i, v, tt.expected[i])
				}
			}
		})
	}
}
