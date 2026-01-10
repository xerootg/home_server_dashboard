// Package auth provides OIDC authentication for the dashboard.
package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/msteinert/pam/v2"
	"golang.org/x/oauth2"

	"home_server_dashboard/config"
)

// ContextKey is a type for context keys used by the auth package.
type ContextKey string

const (
	// UserContextKey is the context key for the authenticated user.
	UserContextKey ContextKey = "auth_user"

	// SessionCookieName is the name of the session cookie.
	SessionCookieName = "hsd_session"

	// StateCookieName is the name of the OIDC state cookie.
	StateCookieName = "hsd_oidc_state"

	// OriginalURLCookieName stores the URL the user was trying to access.
	OriginalURLCookieName = "hsd_original_url"

	// DefaultSessionDuration is the default session lifetime.
	DefaultSessionDuration = 24 * time.Hour

	// StateExpiry is how long OIDC state tokens are valid.
	StateExpiry = 10 * time.Minute
)

// User represents an authenticated user.
type User struct {
	ID       string   `json:"id"`
	Email    string   `json:"email"`
	Name     string   `json:"name"`
	Groups   []string `json:"groups"`
	IsAdmin  bool     `json:"is_admin"`
	Expiry   time.Time `json:"-"`
}

// Session represents a user session.
type Session struct {
	User      *User
	ExpiresAt time.Time
}

// SessionStore manages user sessions in memory.
type SessionStore struct {
	sessions map[string]*Session
	mu       sync.RWMutex
}

// NewSessionStore creates a new session store.
func NewSessionStore() *SessionStore {
	store := &SessionStore{
		sessions: make(map[string]*Session),
	}
	// Start cleanup goroutine
	go store.cleanupExpired()
	return store
}

// Set stores a session.
func (s *SessionStore) Set(id string, session *Session) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[id] = session
}

// Get retrieves a session by ID.
func (s *SessionStore) Get(id string) (*Session, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	session, ok := s.sessions[id]
	if !ok {
		return nil, false
	}
	if time.Now().After(session.ExpiresAt) {
		return nil, false
	}
	return session, true
}

// Delete removes a session.
func (s *SessionStore) Delete(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, id)
}

// cleanupExpired periodically removes expired sessions.
func (s *SessionStore) cleanupExpired() {
	ticker := time.NewTicker(5 * time.Minute)
	for range ticker.C {
		s.mu.Lock()
		now := time.Now()
		for id, session := range s.sessions {
			if now.After(session.ExpiresAt) {
				delete(s.sessions, id)
			}
		}
		s.mu.Unlock()
	}
}

// StateStore manages OIDC state tokens.
type StateStore struct {
	states map[string]time.Time
	mu     sync.RWMutex
}

// NewStateStore creates a new state store.
func NewStateStore() *StateStore {
	store := &StateStore{
		states: make(map[string]time.Time),
	}
	go store.cleanupExpired()
	return store
}

// Set stores a state token.
func (s *StateStore) Set(state string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.states[state] = time.Now().Add(StateExpiry)
}

// Validate checks if a state token is valid and removes it.
func (s *StateStore) Validate(state string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	expiry, ok := s.states[state]
	if !ok {
		return false
	}
	delete(s.states, state)
	return time.Now().Before(expiry)
}

// cleanupExpired periodically removes expired states.
func (s *StateStore) cleanupExpired() {
	ticker := time.NewTicker(1 * time.Minute)
	for range ticker.C {
		s.mu.Lock()
		now := time.Now()
		for state, expiry := range s.states {
			if now.After(expiry) {
				delete(s.states, state)
			}
		}
		s.mu.Unlock()
	}
}

// Provider handles OIDC authentication.
type Provider struct {
	config         *config.OIDCConfig
	localConfig    *config.LocalConfig
	oauth2Config   *oauth2.Config
	verifier       *oidc.IDTokenVerifier
	sessions       *SessionStore
	states         *StateStore
	adminClaim     string
	serviceURLHost string // hostname from service_url for Host header comparison
	localAdmins    map[string]bool // parsed local admin usernames
}

// NewProvider creates a new OIDC provider.
func NewProvider(ctx context.Context, cfg *config.OIDCConfig, localCfg *config.LocalConfig) (*Provider, error) {
	if cfg == nil {
		return nil, fmt.Errorf("OIDC config is nil")
	}

	// Extract hostname from service_url for Host header comparison
	serviceURLHost := extractHostname(cfg.ServiceURL)

	// Parse local admins
	localAdmins := make(map[string]bool)
	if localCfg != nil && localCfg.Admins != "" {
		for _, admin := range strings.Split(localCfg.Admins, ",") {
			admin = strings.TrimSpace(admin)
			if admin != "" {
				localAdmins[admin] = true
			}
		}
	}

	// Discover OIDC provider using the exact config_url provided.
	// We fetch the discovery document manually to respect the user's config_url exactly.
	discoveryDoc, err := fetchDiscoveryDocument(ctx, cfg.ConfigURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch OIDC discovery document at %s: %w", cfg.ConfigURL, err)
	}

	// Create the OIDC provider from the discovery document
	provider, err := oidc.NewProvider(ctx, discoveryDoc.Issuer)
	if err != nil {
		return nil, fmt.Errorf("failed to create OIDC provider for issuer %s: %w", discoveryDoc.Issuer, err)
	}

	// Build redirect URL
	redirectURL := strings.TrimSuffix(cfg.ServiceURL, "/") + cfg.Callback

	// Create OAuth2 config
	oauth2Config := &oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		RedirectURL:  redirectURL,
		Endpoint:     provider.Endpoint(),
		Scopes:       []string{oidc.ScopeOpenID, "profile", "email", "groups"},
	}

	// Create ID token verifier
	verifier := provider.Verifier(&oidc.Config{
		ClientID: cfg.ClientID,
	})

	// Determine admin claim
	adminClaim := cfg.AdminClaim
	if adminClaim == "" {
		adminClaim = "groups"
	}

	return &Provider{
		config:         cfg,
		localConfig:    localCfg,
		oauth2Config:   oauth2Config,
		verifier:       verifier,
		sessions:       NewSessionStore(),
		states:         NewStateStore(),
		adminClaim:     adminClaim,
		serviceURLHost: serviceURLHost,
		localAdmins:    localAdmins,
	}, nil
}

// discoveryDocument represents the OIDC discovery document.
type discoveryDocument struct {
	Issuer                string `json:"issuer"`
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	UserinfoEndpoint      string `json:"userinfo_endpoint"`
	JwksURI               string `json:"jwks_uri"`
}

// fetchDiscoveryDocument fetches and parses the OIDC discovery document from the given URL.
func fetchDiscoveryDocument(ctx context.Context, configURL string) (*discoveryDocument, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", configURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("%s: %s", resp.Status, string(body))
	}

	var doc discoveryDocument
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return nil, fmt.Errorf("failed to decode discovery document: %w", err)
	}

	if doc.Issuer == "" {
		return nil, fmt.Errorf("discovery document missing issuer")
	}

	return &doc, nil
}

// validatePAMAuth validates a username and password using PAM.
func validatePAMAuth(username, password string) error {
	t, err := pam.StartFunc("login", username, func(s pam.Style, msg string) (string, error) {
		switch s {
		case pam.PromptEchoOff:
			// Password prompt
			return password, nil
		case pam.PromptEchoOn:
			// Username prompt (shouldn't happen since we provide it)
			return username, nil
		case pam.ErrorMsg, pam.TextInfo:
			// Informational messages - just acknowledge
			return "", nil
		default:
			return "", fmt.Errorf("unrecognized PAM message style: %v", s)
		}
	})
	if err != nil {
		return fmt.Errorf("failed to start PAM transaction: %w", err)
	}
	defer t.End()

	if err := t.Authenticate(0); err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}

	if err := t.AcctMgmt(0); err != nil {
		return fmt.Errorf("account validation failed: %w", err)
	}

	return nil
}

// extractHostname extracts the hostname from a URL string.
func extractHostname(urlStr string) string {
	// Remove protocol
	host := urlStr
	if idx := strings.Index(host, "://"); idx != -1 {
		host = host[idx+3:]
	}
	// Remove path
	if idx := strings.Index(host, "/"); idx != -1 {
		host = host[:idx]
	}
	// Remove port
	if idx := strings.LastIndex(host, ":"); idx != -1 {
		// Make sure it's not part of an IPv6 address
		if !strings.Contains(host, "[") {
			host = host[:idx]
		}
	}
	return strings.ToLower(host)
}

// isLocalAccess checks if the request is coming from a local hostname (not the service_url).
func (p *Provider) isLocalAccess(r *http.Request) bool {
	requestHost := strings.ToLower(r.Host)
	// Remove port from request host
	if idx := strings.LastIndex(requestHost, ":"); idx != -1 {
		if !strings.Contains(requestHost, "[") {
			requestHost = requestHost[:idx]
		}
	}
	return requestHost != p.serviceURLHost
}

// generateRandomString generates a cryptographically secure random string.
func generateRandomString(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(bytes)[:length], nil
}

// LoginHandler initiates the OIDC login flow.
func (p *Provider) LoginHandler(w http.ResponseWriter, r *http.Request) {
	// Generate state token
	state, err := generateRandomString(32)
	if err != nil {
		log.Printf("Failed to generate state: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Store state
	p.states.Set(state)

	// Store original URL
	originalURL := r.URL.Query().Get("redirect")
	if originalURL == "" {
		originalURL = "/"
	}
	http.SetCookie(w, &http.Cookie{
		Name:     OriginalURLCookieName,
		Value:    originalURL,
		Path:     "/",
		HttpOnly: true,
		Secure:   strings.HasPrefix(p.config.ServiceURL, "https"),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(StateExpiry.Seconds()),
	})

	// Redirect to OIDC provider
	authURL := p.oauth2Config.AuthCodeURL(state)
	http.Redirect(w, r, authURL, http.StatusTemporaryRedirect)
}

// CallbackHandler handles the OIDC callback.
func (p *Provider) CallbackHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Verify state
	state := r.URL.Query().Get("state")
	if !p.states.Validate(state) {
		log.Printf("Invalid OIDC state")
		http.Error(w, "Invalid state parameter", http.StatusBadRequest)
		return
	}

	// Check for errors from provider
	if errParam := r.URL.Query().Get("error"); errParam != "" {
		errDesc := r.URL.Query().Get("error_description")
		log.Printf("OIDC error: %s - %s", errParam, errDesc)
		http.Error(w, fmt.Sprintf("Authentication error: %s", errDesc), http.StatusUnauthorized)
		return
	}

	// Exchange code for tokens
	code := r.URL.Query().Get("code")
	token, err := p.oauth2Config.Exchange(ctx, code)
	if err != nil {
		log.Printf("Failed to exchange code: %v", err)
		http.Error(w, "Failed to exchange authorization code", http.StatusInternalServerError)
		return
	}

	// Extract ID token
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		log.Printf("No ID token in response")
		http.Error(w, "No ID token in response", http.StatusInternalServerError)
		return
	}

	// Verify ID token
	idToken, err := p.verifier.Verify(ctx, rawIDToken)
	if err != nil {
		log.Printf("Failed to verify ID token: %v", err)
		http.Error(w, "Failed to verify ID token", http.StatusUnauthorized)
		return
	}

	// Extract claims
	var claims map[string]interface{}
	if err := idToken.Claims(&claims); err != nil {
		log.Printf("Failed to extract claims: %v", err)
		http.Error(w, "Failed to extract claims", http.StatusInternalServerError)
		return
	}

	// Build user from claims
	user := p.buildUserFromClaims(claims)

	// Check admin access
	if !user.IsAdmin {
		log.Printf("User %s (%s) denied access - not an admin", user.Email, user.ID)
		http.Error(w, "Access denied: admin privileges required", http.StatusForbidden)
		return
	}

	// Create session
	sessionID, err := generateRandomString(64)
	if err != nil {
		log.Printf("Failed to generate session ID: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	session := &Session{
		User:      user,
		ExpiresAt: time.Now().Add(DefaultSessionDuration),
	}
	p.sessions.Set(sessionID, session)

	log.Printf("User %s (%s) logged in successfully", user.Email, user.ID)

	// Set session cookie
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    sessionID,
		Path:     "/",
		HttpOnly: true,
		Secure:   strings.HasPrefix(p.config.ServiceURL, "https"),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(DefaultSessionDuration.Seconds()),
	})

	// Get original URL
	originalURL := "/"
	if cookie, err := r.Cookie(OriginalURLCookieName); err == nil {
		originalURL = cookie.Value
	}

	// Clear original URL cookie
	http.SetCookie(w, &http.Cookie{
		Name:     OriginalURLCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	})

	// Redirect to original URL
	http.Redirect(w, r, originalURL, http.StatusTemporaryRedirect)
}

// buildUserFromClaims extracts user information from ID token claims.
func (p *Provider) buildUserFromClaims(claims map[string]interface{}) *User {
	user := &User{}

	// Extract standard claims
	if sub, ok := claims["sub"].(string); ok {
		user.ID = sub
	}
	if email, ok := claims["email"].(string); ok {
		user.Email = email
	}
	if name, ok := claims["name"].(string); ok {
		user.Name = name
	} else if preferredUsername, ok := claims["preferred_username"].(string); ok {
		user.Name = preferredUsername
	}

	// Extract groups
	if groups, ok := claims["groups"].([]interface{}); ok {
		for _, g := range groups {
			if group, ok := g.(string); ok {
				user.Groups = append(user.Groups, group)
			}
		}
	}

	// Check for admin status
	user.IsAdmin = p.checkAdminClaim(claims)

	return user
}

// checkAdminClaim determines if the user has admin privileges.
func (p *Provider) checkAdminClaim(claims map[string]interface{}) bool {
	// First, check if there's a direct "admin" boolean claim
	if admin, ok := claims["admin"].(bool); ok && admin {
		return true
	}

	// Check the configured admin claim
	claimValue, ok := claims[p.adminClaim]
	if !ok {
		return false
	}

	// Handle different claim types
	switch v := claimValue.(type) {
	case bool:
		return v
	case string:
		// Check if the string value indicates admin
		lower := strings.ToLower(v)
		return lower == "admin" || lower == "true" || lower == "1"
	case []interface{}:
		// Check if "admin" is in the array (typical for groups claim)
		for _, item := range v {
			if str, ok := item.(string); ok {
				if strings.EqualFold(str, "admin") {
					return true
				}
			}
		}
	}

	return false
}

// LogoutHandler handles user logout.
func (p *Provider) LogoutHandler(w http.ResponseWriter, r *http.Request) {
	// Get session cookie
	if cookie, err := r.Cookie(SessionCookieName); err == nil {
		// Delete session
		p.sessions.Delete(cookie.Value)
	}

	// Clear session cookie
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	})

	log.Printf("User logged out")

	// Redirect to login page
	http.Redirect(w, r, "/login", http.StatusTemporaryRedirect)
}

// StatusHandler returns the current authentication status as JSON.
func (p *Provider) StatusHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	type AuthStatus struct {
		Authenticated bool   `json:"authenticated"`
		User          *User  `json:"user,omitempty"`
		OIDCEnabled   bool   `json:"oidc_enabled"`
		LocalAccess   bool   `json:"local_access"`
	}

	isLocal := p.isLocalAccess(r)
	status := AuthStatus{
		OIDCEnabled: !isLocal, // OIDC is used for external access
		LocalAccess: isLocal,
	}

	// Check for valid session
	if cookie, err := r.Cookie(SessionCookieName); err == nil {
		if session, ok := p.sessions.Get(cookie.Value); ok {
			status.Authenticated = true
			status.User = session.User
		}
	}

	json.NewEncoder(w).Encode(status)
}

// Middleware returns HTTP middleware that requires authentication.
func (p *Provider) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if this is local access (different hostname than service_url)
		if p.isLocalAccess(r) {
			// For local access, use local authentication
			if p.handleLocalAuth(w, r, next) {
				return
			}
			// If local auth fails but allows through (no local config), continue to check session
		}

		// Check for valid session (OIDC or local)
		cookie, err := r.Cookie(SessionCookieName)
		if err != nil {
			p.handleUnauthorized(w, r)
			return
		}

		session, ok := p.sessions.Get(cookie.Value)
		if !ok {
			p.handleUnauthorized(w, r)
			return
		}

		// Add user to context
		ctx := context.WithValue(r.Context(), UserContextKey, session.User)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// handleLocalAuth handles authentication for local (non-service_url) access.
// Returns true if the request was handled (either authenticated or denied).
// Returns false if local auth is not configured and should fall through to session check.
func (p *Provider) handleLocalAuth(w http.ResponseWriter, r *http.Request, next http.Handler) bool {
	// Check for existing local session first
	if cookie, err := r.Cookie(SessionCookieName); err == nil {
		if session, ok := p.sessions.Get(cookie.Value); ok {
			// Valid session exists, add user to context and proceed
			ctx := context.WithValue(r.Context(), UserContextKey, session.User)
			next.ServeHTTP(w, r.WithContext(ctx))
			return true
		}
	}

	// If no local admins configured, deny local access
	if len(p.localAdmins) == 0 {
		log.Printf("Local access attempted from %s but no local admins configured", r.Host)
		http.Error(w, "Local access not configured", http.StatusForbidden)
		return true
	}

	// Check for Basic Auth
	username, password, hasAuth := r.BasicAuth()
	if !hasAuth {
		// Request Basic Auth
		w.Header().Set("WWW-Authenticate", `Basic realm="Home Server Dashboard (Local)"`)
		w.WriteHeader(http.StatusUnauthorized)
		return true
	}

	// Check if username is in local admins list
	if !p.localAdmins[username] {
		log.Printf("Local auth failed: user %s not in local admins list", username)
		w.Header().Set("WWW-Authenticate", `Basic realm="Home Server Dashboard (Local)"`)
		http.Error(w, "Invalid credentials", http.StatusUnauthorized)
		return true
	}

	// Validate password using PAM
	if err := validatePAMAuth(username, password); err != nil {
		log.Printf("Local auth failed: PAM authentication failed for user %s: %v", username, err)
		w.Header().Set("WWW-Authenticate", `Basic realm="Home Server Dashboard (Local)"`)
		http.Error(w, "Invalid credentials", http.StatusUnauthorized)
		return true
	}

	// Create a session for the local user
	sessionID, err := generateRandomString(64)
	if err != nil {
		log.Printf("Failed to generate session ID for local user: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return true
	}

	user := &User{
		ID:      "local:" + username,
		Name:    username,
		Email:   username + "@localhost",
		Groups:  []string{"local", "admin"},
		IsAdmin: true,
	}

	session := &Session{
		User:      user,
		ExpiresAt: time.Now().Add(DefaultSessionDuration),
	}
	p.sessions.Set(sessionID, session)

	log.Printf("Local user %s authenticated via local access from %s", username, r.Host)

	// Set session cookie
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    sessionID,
		Path:     "/",
		HttpOnly: true,
		Secure:   false, // Local access typically not over HTTPS
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(DefaultSessionDuration.Seconds()),
	})

	// Add user to context and proceed
	ctx := context.WithValue(r.Context(), UserContextKey, user)
	next.ServeHTTP(w, r.WithContext(ctx))
	return true
}

// handleUnauthorized handles unauthenticated requests.
func (p *Provider) handleUnauthorized(w http.ResponseWriter, r *http.Request) {
	// Check if this is local access
	if p.isLocalAccess(r) {
		// For local access, request Basic Auth
		if len(p.localAdmins) > 0 {
			w.Header().Set("WWW-Authenticate", `Basic realm="Home Server Dashboard (Local)"`)
		}
		w.WriteHeader(http.StatusUnauthorized)
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{
				"error": "local authentication required",
			})
		}
		return
	}

	// For API requests, return 401
	if strings.HasPrefix(r.URL.Path, "/api/") {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "authentication required",
		})
		return
	}

	// For other requests, redirect to login
	redirectURL := r.URL.RequestURI()
	http.Redirect(w, r, "/login?redirect="+redirectURL, http.StatusTemporaryRedirect)
}

// GetUserFromContext retrieves the authenticated user from the request context.
func GetUserFromContext(ctx context.Context) *User {
	if user, ok := ctx.Value(UserContextKey).(*User); ok {
		return user
	}
	return nil
}

// NoAuthStatusHandler returns auth status when OIDC is not configured.
func NoAuthStatusHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"authenticated": true,
		"oidc_enabled":  false,
	})
}
