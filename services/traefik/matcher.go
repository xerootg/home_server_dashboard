// Package traefik provides Traefik router hostname lookup.
package traefik

import (
	"log"
	"regexp"
	"strings"
	"sync"
)

// MatcherType represents the type of hostname matcher in Traefik rules.
type MatcherType int

const (
	// MatcherTypeHost represents a Host() matcher with exact hostname.
	MatcherTypeHost MatcherType = iota
	// MatcherTypeHostRegexp represents a HostRegexp() matcher with regex pattern.
	MatcherTypeHostRegexp
)

// MatcherInfo contains information about a parsed hostname matcher.
type MatcherInfo struct {
	// Type indicates whether this is a Host or HostRegexp matcher.
	Type MatcherType
	// Hostname is the extracted hostname (exact for Host, simplified for HostRegexp).
	Hostname string
	// OriginalPattern is the original pattern from the rule (same as Hostname for Host).
	OriginalPattern string
	// IsExact indicates if the hostname is an exact match (true for Host, may be false for HostRegexp).
	IsExact bool
}

// RouterMatcherState tracks the state of a router's matcher for change detection.
type RouterMatcherState struct {
	Rule        string
	Hostnames   []string
	LastError   error
	HadError    bool
}

// MatcherLookupService manages hostname extraction from Traefik rules with
// state tracking and intelligent logging for matcher changes and error recovery.
type MatcherLookupService struct {
	mu           sync.RWMutex
	routerStates map[string]*RouterMatcherState // keyed by router name
	hostName     string                          // host name for logging context
}

// NewMatcherLookupService creates a new matcher lookup service for a given host.
func NewMatcherLookupService(hostName string) *MatcherLookupService {
	return &MatcherLookupService{
		routerStates: make(map[string]*RouterMatcherState),
		hostName:     hostName,
	}
}

// hostPattern matches Host(`hostname`) in Traefik rules.
// Supports both single and double quotes, and backticks.
var matcherHostPattern = regexp.MustCompile(`Host\s*\(\s*[\x60"']([^)\x60"']+)[\x60"']\s*\)`)

// hostRegexpPattern matches HostRegexp(`pattern`) in Traefik rules.
// Supports both single and double quotes, and backticks.
var hostRegexpPattern = regexp.MustCompile(`HostRegexp\s*\(\s*[\x60"']([^)\x60"']+)[\x60"']\s*\)`)

// simpleHostRegexpPattern matches HostRegexp patterns that are essentially exact hostnames
// with optional anchors. Examples: `^example\.com$`, `example\.com`, `{name:example\.com}`
var simpleHostRegexpPattern = regexp.MustCompile(`^(?:\^)?(?:\{[^:]+:)?([a-zA-Z0-9][\w.-]*\.[a-zA-Z]{2,})(?:\})?(?:\$)?$`)

// variableHostRegexpPattern extracts the domain portion from patterns like `{subdomain:[a-z]+}.example.com`
// This captures the static domain suffix that follows a variable pattern.
var variableHostRegexpPattern = regexp.MustCompile(`\}\.([\w.-]+\.[a-zA-Z]{2,})(?:\$)?$`)

// ExtractMatchers extracts all hostname matchers from a Traefik rule string.
// It returns detailed matcher information including type and exactness.
// If exact Host() matchers are found, HostRegexp() matchers are excluded
// since the exact hostnames should be preferred.
func ExtractMatchers(rule string) []MatcherInfo {
	var hostMatchers []MatcherInfo
	var regexpMatchers []MatcherInfo

	// Extract Host() matchers - these are always exact
	hostMatches := matcherHostPattern.FindAllStringSubmatch(rule, -1)
	for _, match := range hostMatches {
		if len(match) > 1 {
			hostMatchers = append(hostMatchers, MatcherInfo{
				Type:            MatcherTypeHost,
				Hostname:        match[1],
				OriginalPattern: match[1],
				IsExact:         true,
			})
		}
	}

	// If we have exact Host() matchers, prefer those and skip HostRegexp
	if len(hostMatchers) > 0 {
		return hostMatchers
	}

	// No Host() matchers found, try to extract from HostRegexp() patterns
	regexpMatches := hostRegexpPattern.FindAllStringSubmatch(rule, -1)
	for _, match := range regexpMatches {
		if len(match) > 1 {
			pattern := match[1]
			info := parseHostRegexpPattern(pattern)
			if info != nil {
			regexpMatchers = append(regexpMatchers, *info)
			}
		}
	}

	return regexpMatchers
}

// parseHostRegexpPattern attempts to extract a usable hostname from a HostRegexp pattern.
// Returns nil if no usable hostname can be extracted.
func parseHostRegexpPattern(pattern string) *MatcherInfo {
	// First, unescape common regex escapes to see the actual hostname
	unescaped := strings.ReplaceAll(pattern, `\.`, ".")
	unescaped = strings.ReplaceAll(unescaped, `\-`, "-")

	// Check if it's a simple hostname pattern (just anchored literal)
	if simpleMatch := simpleHostRegexpPattern.FindStringSubmatch(unescaped); len(simpleMatch) > 1 {
		return &MatcherInfo{
			Type:            MatcherTypeHostRegexp,
			Hostname:        simpleMatch[1],
			OriginalPattern: pattern,
			IsExact:         true,
		}
	}

	// Check if it has a variable prefix but static domain suffix
	// e.g., `{subdomain:[a-z]+}.example.com` -> extract `example.com`
	if varMatch := variableHostRegexpPattern.FindStringSubmatch(unescaped); len(varMatch) > 1 {
		return &MatcherInfo{
			Type:            MatcherTypeHostRegexp,
			Hostname:        varMatch[1],
			OriginalPattern: pattern,
			IsExact:         false, // Not exact because of wildcard subdomain
		}
	}

	// Try to extract any domain-like pattern from the regex
	// This handles cases like `.*\.example\.com` or `(foo|bar)\.example\.com`
	domainPattern := regexp.MustCompile(`([\w-]+(?:\.[\w-]+)*\.[a-zA-Z]{2,})(?:\$)?$`)
	if domainMatch := domainPattern.FindStringSubmatch(unescaped); len(domainMatch) > 1 {
		return &MatcherInfo{
			Type:            MatcherTypeHostRegexp,
			Hostname:        domainMatch[1],
			OriginalPattern: pattern,
			IsExact:         false,
		}
	}

	// Cannot extract a usable hostname
	return nil
}

// ExtractHostnamesFromRule extracts all hostnames from a Traefik rule string.
// This is the main entry point for hostname extraction, returning simple hostname strings.
// It logs warnings for patterns that couldn't be fully extracted.
func (s *MatcherLookupService) ExtractHostnamesFromRule(routerName, rule string) []string {
	matchers := ExtractMatchers(rule)
	var hostnames []string
	seen := make(map[string]bool)

	for _, m := range matchers {
		if !seen[m.Hostname] {
			hostnames = append(hostnames, m.Hostname)
			seen[m.Hostname] = true
		}
	}

	return hostnames
}

// ProcessRouter processes a router's rule and returns extracted hostnames.
// It tracks state changes and logs appropriately:
// - Logs when a router's rule changes
// - Logs when an error resolves (was error, now successful)
// - Logs HostRegexp patterns that couldn't be extracted
func (s *MatcherLookupService) ProcessRouter(routerName, rule string, currentError error) []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	state := s.routerStates[routerName]
	if state == nil {
		state = &RouterMatcherState{}
		s.routerStates[routerName] = state
	}

	// Check for error recovery
	if state.HadError && currentError == nil {
		hostnames := s.extractHostnamesInternal(routerName, rule)
		log.Printf("[%s] Router %q recovered from error. Extracted hostnames: %v",
			s.hostName, routerName, hostnames)
		state.HadError = false
		state.LastError = nil
		state.Rule = rule
		state.Hostnames = hostnames
		return hostnames
	}

	// Track current error state
	if currentError != nil {
		if !state.HadError {
			log.Printf("[%s] Router %q encountered error: %v", s.hostName, routerName, currentError)
		}
		state.HadError = true
		state.LastError = currentError
		return nil
	}

	// Check for rule changes
	if state.Rule != "" && state.Rule != rule {
		oldHostnames := state.Hostnames
		newHostnames := s.extractHostnamesInternal(routerName, rule)
		log.Printf("[%s] Router %q rule changed. Old hostnames: %v, New hostnames: %v",
			s.hostName, routerName, oldHostnames, newHostnames)
		state.Rule = rule
		state.Hostnames = newHostnames
		return newHostnames
	}

	// Normal processing
	if state.Rule == "" {
		// First time seeing this router
		hostnames := s.extractHostnamesInternal(routerName, rule)
		state.Rule = rule
		state.Hostnames = hostnames
		return hostnames
	}

	return state.Hostnames
}

// extractHostnamesInternal is the internal hostname extraction without state tracking.
// It logs warnings for HostRegexp patterns that couldn't be extracted.
func (s *MatcherLookupService) extractHostnamesInternal(routerName, rule string) []string {
	matchers := ExtractMatchers(rule)
	var hostnames []string
	seen := make(map[string]bool)
	hasUnextractable := false

	// Check if rule contains HostRegexp that we couldn't parse
	regexpMatches := hostRegexpPattern.FindAllStringSubmatch(rule, -1)
	extractedRegexpCount := 0
	for _, m := range matchers {
		if m.Type == MatcherTypeHostRegexp {
			extractedRegexpCount++
		}
	}

	if len(regexpMatches) > extractedRegexpCount {
		hasUnextractable = true
	}

	for _, m := range matchers {
		if !seen[m.Hostname] {
			hostnames = append(hostnames, m.Hostname)
			seen[m.Hostname] = true

			// Log non-exact HostRegexp extractions
			if m.Type == MatcherTypeHostRegexp && !m.IsExact {
				log.Printf("[%s] Router %q has HostRegexp pattern %q, using extracted domain: %s",
					s.hostName, routerName, m.OriginalPattern, m.Hostname)
			}
		}
	}

	if hasUnextractable {
		log.Printf("[%s] Router %q has HostRegexp patterns that could not be extracted. Rule: %s",
			s.hostName, routerName, rule)
	}

	return hostnames
}

// ClearState removes the state for a router (useful for testing).
func (s *MatcherLookupService) ClearState(routerName string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.routerStates, routerName)
}

// ClearAllState removes all router states (useful for testing).
func (s *MatcherLookupService) ClearAllState() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.routerStates = make(map[string]*RouterMatcherState)
}

// GetState returns a copy of the current state for a router (useful for testing/debugging).
func (s *MatcherLookupService) GetState(routerName string) *RouterMatcherState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if state := s.routerStates[routerName]; state != nil {
		// Return a copy
		hostsCopy := make([]string, len(state.Hostnames))
		copy(hostsCopy, state.Hostnames)
		return &RouterMatcherState{
			Rule:      state.Rule,
			Hostnames: hostsCopy,
			LastError: state.LastError,
			HadError:  state.HadError,
		}
	}
	return nil
}
