package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSessionStore(t *testing.T) {
	store := NewSessionStore()

	// Test Set and Get
	session := &Session{
		User: &User{
			ID:      "user123",
			Email:   "test@example.com",
			IsAdmin: true,
		},
		ExpiresAt: time.Now().Add(1 * time.Hour),
	}

	store.Set("session1", session)

	// Test retrieval
	retrieved, ok := store.Get("session1")
	if !ok {
		t.Fatal("Expected to retrieve session")
	}
	if retrieved.User.ID != "user123" {
		t.Errorf("Expected user ID 'user123', got '%s'", retrieved.User.ID)
	}

	// Test non-existent session
	_, ok = store.Get("nonexistent")
	if ok {
		t.Error("Expected false for non-existent session")
	}

	// Test Delete
	store.Delete("session1")
	_, ok = store.Get("session1")
	if ok {
		t.Error("Expected session to be deleted")
	}
}

func TestSessionStore_ExpiredSession(t *testing.T) {
	store := NewSessionStore()

	// Create expired session
	session := &Session{
		User: &User{
			ID: "user123",
		},
		ExpiresAt: time.Now().Add(-1 * time.Hour), // Already expired
	}

	store.Set("expired", session)

	// Should not be retrievable
	_, ok := store.Get("expired")
	if ok {
		t.Error("Expected expired session to not be retrievable")
	}
}

func TestStateStore(t *testing.T) {
	store := NewStateStore()

	// Set a state
	store.Set("state123")

	// Validate should return true and remove it
	if !store.Validate("state123") {
		t.Error("Expected state to be valid")
	}

	// Second validation should fail (state was consumed)
	if store.Validate("state123") {
		t.Error("Expected state to be consumed after validation")
	}

	// Non-existent state
	if store.Validate("nonexistent") {
		t.Error("Expected non-existent state to be invalid")
	}
}

func TestCheckAdminClaim(t *testing.T) {
	// Create a minimal provider for testing
	p := &Provider{
		adminClaim: "groups",
	}

	tests := []struct {
		name     string
		claims   map[string]interface{}
		expected bool
	}{
		{
			name: "admin in groups array",
			claims: map[string]interface{}{
				"groups": []interface{}{"users", "admin", "developers"},
			},
			expected: true,
		},
		{
			name: "Admin in groups array (case insensitive)",
			claims: map[string]interface{}{
				"groups": []interface{}{"users", "Admin", "developers"},
			},
			expected: true,
		},
		{
			name: "no admin in groups",
			claims: map[string]interface{}{
				"groups": []interface{}{"users", "developers"},
			},
			expected: false,
		},
		{
			name: "direct admin boolean true",
			claims: map[string]interface{}{
				"admin": true,
			},
			expected: true,
		},
		{
			name: "direct admin boolean false",
			claims: map[string]interface{}{
				"admin": false,
			},
			expected: false,
		},
		{
			name: "groups as string 'admin'",
			claims: map[string]interface{}{
				"groups": "admin",
			},
			expected: true,
		},
		{
			name:     "empty claims",
			claims:   map[string]interface{}{},
			expected: false,
		},
		{
			name: "groups as boolean true",
			claims: map[string]interface{}{
				"groups": true,
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := p.checkAdminClaim(tt.claims)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestBuildUserFromClaims(t *testing.T) {
	p := &Provider{
		adminClaim: "groups",
	}

	claims := map[string]interface{}{
		"sub":                "user-uuid-123",
		"email":              "admin@example.com",
		"name":               "Admin User",
		"preferred_username": "admin",
		"groups":             []interface{}{"admin", "developers"},
	}

	user := p.buildUserFromClaims(claims)

	if user.ID != "user-uuid-123" {
		t.Errorf("Expected ID 'user-uuid-123', got '%s'", user.ID)
	}
	if user.Email != "admin@example.com" {
		t.Errorf("Expected email 'admin@example.com', got '%s'", user.Email)
	}
	if user.Name != "Admin User" {
		t.Errorf("Expected name 'Admin User', got '%s'", user.Name)
	}
	if len(user.Groups) != 2 {
		t.Errorf("Expected 2 groups, got %d", len(user.Groups))
	}
	if !user.IsAdmin {
		t.Error("Expected user to be admin")
	}
}

func TestBuildUserFromClaims_FallbackToPreferredUsername(t *testing.T) {
	p := &Provider{
		adminClaim: "groups",
	}

	claims := map[string]interface{}{
		"sub":                "user-uuid-123",
		"email":              "user@example.com",
		"preferred_username": "fallback_user",
	}

	user := p.buildUserFromClaims(claims)

	if user.Name != "fallback_user" {
		t.Errorf("Expected name 'fallback_user', got '%s'", user.Name)
	}
}

func TestGenerateRandomString(t *testing.T) {
	s1, err := generateRandomString(32)
	if err != nil {
		t.Fatalf("Failed to generate random string: %v", err)
	}

	s2, err := generateRandomString(32)
	if err != nil {
		t.Fatalf("Failed to generate random string: %v", err)
	}

	if len(s1) != 32 {
		t.Errorf("Expected length 32, got %d", len(s1))
	}

	if s1 == s2 {
		t.Error("Expected different random strings")
	}
}

func TestGetUserFromContext(t *testing.T) {
	// Test with user in context
	user := &User{
		ID:    "user123",
		Email: "test@example.com",
	}
	ctx := context.WithValue(context.Background(), UserContextKey, user)

	retrieved := GetUserFromContext(ctx)
	if retrieved == nil {
		t.Fatal("Expected to retrieve user from context")
	}
	if retrieved.ID != "user123" {
		t.Errorf("Expected user ID 'user123', got '%s'", retrieved.ID)
	}

	// Test without user in context
	emptyCtx := context.Background()
	retrieved = GetUserFromContext(emptyCtx)
	if retrieved != nil {
		t.Error("Expected nil user from empty context")
	}
}

func TestNoAuthStatusHandler(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/auth/status", nil)
	w := httptest.NewRecorder()

	NoAuthStatusHandler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Expected Content-Type 'application/json', got '%s'", contentType)
	}

	body := w.Body.String()
	if body == "" {
		t.Error("Expected non-empty response body")
	}
	// Should contain oidc_enabled: false
	if !contains(body, "oidc_enabled") {
		t.Error("Expected response to contain 'oidc_enabled'")
	}
}

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

func TestCustomAdminClaim(t *testing.T) {
	// Test with a custom admin claim name
	p := &Provider{
		adminClaim: "is_admin",
	}

	tests := []struct {
		name     string
		claims   map[string]interface{}
		expected bool
	}{
		{
			name: "custom claim true",
			claims: map[string]interface{}{
				"is_admin": true,
			},
			expected: true,
		},
		{
			name: "custom claim false",
			claims: map[string]interface{}{
				"is_admin": false,
			},
			expected: false,
		},
		{
			name: "custom claim string 'true'",
			claims: map[string]interface{}{
				"is_admin": "true",
			},
			expected: true,
		},
		{
			name: "groups doesn't work with custom claim",
			claims: map[string]interface{}{
				"groups": []interface{}{"admin"},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := p.checkAdminClaim(tt.claims)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestExtractHostname(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		expected string
	}{
		{"https with path", "https://dashboard.example.com/path", "dashboard.example.com"},
		{"https with port", "https://dashboard.example.com:8443/path", "dashboard.example.com"},
		{"http simple", "http://localhost", "localhost"},
		{"http with port", "http://localhost:9001", "localhost"},
		{"no protocol", "dashboard.example.com", "dashboard.example.com"},
		{"ip address", "http://192.168.1.8:9001", "192.168.1.8"},
		{"uppercase", "HTTPS://Dashboard.Example.COM", "dashboard.example.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractHostname(tt.url)
			if result != tt.expected {
				t.Errorf("extractHostname(%q) = %q, want %q", tt.url, result, tt.expected)
			}
		})
	}
}

func TestIsLocalAccess(t *testing.T) {
	p := &Provider{
		serviceURLHost: "dashboard.example.com",
	}

	tests := []struct {
		name     string
		host     string
		expected bool
	}{
		{"matches service_url", "dashboard.example.com", false},
		{"matches with port", "dashboard.example.com:443", false},
		{"localhost is local", "localhost", true},
		{"localhost with port", "localhost:9001", true},
		{"ip address is local", "192.168.1.8", true},
		{"ip with port is local", "192.168.1.8:9001", true},
		{"different domain is local", "other.example.com", true},
		{"case insensitive match", "Dashboard.Example.COM", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Host = tt.host
			result := p.isLocalAccess(req)
			if result != tt.expected {
				t.Errorf("isLocalAccess(Host=%q) = %v, want %v", tt.host, result, tt.expected)
			}
		})
	}
}

func TestLocalAdminsParsing(t *testing.T) {
	tests := []struct {
		name     string
		admins   string
		expected map[string]bool
	}{
		{"single admin", "xero", map[string]bool{"xero": true}},
		{"multiple admins", "xero, admin, root", map[string]bool{"xero": true, "admin": true, "root": true}},
		{"with spaces", "  xero  ,  admin  ", map[string]bool{"xero": true, "admin": true}},
		{"empty string", "", map[string]bool{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate what NewProvider does
			localAdmins := make(map[string]bool)
			if tt.admins != "" {
				for _, admin := range splitAndTrim(tt.admins) {
					if admin != "" {
						localAdmins[admin] = true
					}
				}
			}
			if len(localAdmins) != len(tt.expected) {
				t.Errorf("Got %d admins, expected %d", len(localAdmins), len(tt.expected))
			}
			for admin := range tt.expected {
				if !localAdmins[admin] {
					t.Errorf("Expected admin %q to be present", admin)
				}
			}
		})
	}
}

// Helper function to split and trim
func splitAndTrim(s string) []string {
	var result []string
	for _, part := range splitByComma(s) {
		part = trimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}

func splitByComma(s string) []string {
	var result []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == ',' {
			result = append(result, s[start:i])
			start = i + 1
		}
	}
	result = append(result, s[start:])
	return result
}

func trimSpace(s string) string {
	start, end := 0, len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
}
