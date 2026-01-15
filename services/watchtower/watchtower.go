// Package watchtower provides a client for monitoring Watchtower's metrics endpoint.
// It queries the /v1/metrics endpoint to detect when container updates are in progress,
// allowing the dashboard to suppress false-positive notifications during updates.
// This is a read-only integration - the dashboard does not trigger updates via the API.
package watchtower

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"sync"
	"time"

	"home_server_dashboard/config"
)

// Metrics represents the parsed Watchtower metrics.
type Metrics struct {
	// ContainersScanned is the number of containers scanned during the last run.
	ContainersScanned int
	// ContainersUpdated is the number of containers updated during the last run.
	ContainersUpdated int
	// ContainersFailed is the number of containers where update failed.
	ContainersFailed int
	// ContainersRestarted is the number of containers restarted due to linked dependencies.
	ContainersRestarted int
	// ScansTotal is the total number of scans since Watchtower started.
	ScansTotal int
	// Timestamp is when the metrics were fetched.
	Timestamp time.Time
}

// Client provides access to the Watchtower metrics endpoint.
type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
	
	// Metrics caching
	mu             sync.RWMutex
	cachedMetrics  *Metrics
	lastScanCount  int
	lastUpdateTime time.Time
}

// NewClient creates a new Watchtower API client from host configuration.
// Returns nil if Watchtower is not configured or token is missing.
func NewClient(hostCfg *config.HostConfig) *Client {
	if hostCfg == nil || !hostCfg.HasWatchtower() {
		return nil
	}

	token := hostCfg.GetWatchtowerToken()
	if token == "" {
		return nil
	}

	// Determine the base URL based on whether this is local or remote
	address := hostCfg.Address
	if hostCfg.IsLocal() {
		address = "localhost"
	}

	baseURL := fmt.Sprintf("http://%s:%d", address, hostCfg.GetWatchtowerPort())

	return &Client{
		baseURL: baseURL,
		token:   token,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// GetMetrics fetches the current metrics from Watchtower.
func (c *Client) GetMetrics(ctx context.Context) (*Metrics, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/v1/metrics", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch metrics: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("metrics request failed with status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	metrics, err := parsePrometheusMetrics(string(body))
	if err != nil {
		return nil, err
	}

	// Cache the metrics
	c.mu.Lock()
	c.cachedMetrics = metrics
	c.mu.Unlock()

	return metrics, nil
}

// parsePrometheusMetrics parses Prometheus-format metrics text.
func parsePrometheusMetrics(text string) (*Metrics, error) {
	metrics := &Metrics{
		Timestamp: time.Now(),
	}

	// Parse each relevant metric using regex
	patterns := map[string]*int{
		`watchtower_containers_scanned\s+(\d+)`:   &metrics.ContainersScanned,
		`watchtower_containers_updated\s+(\d+)`:   &metrics.ContainersUpdated,
		`watchtower_containers_failed\s+(\d+)`:    &metrics.ContainersFailed,
		`watchtower_containers_restarted\s+(\d+)`: &metrics.ContainersRestarted,
		`watchtower_scans_total\s+(\d+)`:          &metrics.ScansTotal,
	}

	for pattern, target := range patterns {
		re := regexp.MustCompile(pattern)
		match := re.FindStringSubmatch(text)
		if len(match) >= 2 {
			val, _ := strconv.Atoi(match[1])
			*target = val
		}
	}

	return metrics, nil
}

// IsUpdateInProgress checks if Watchtower is currently updating containers.
// It detects this by checking if metrics indicate an active scan or recent update activity.
func (c *Client) IsUpdateInProgress(ctx context.Context) (bool, error) {
	metrics, err := c.GetMetrics(ctx)
	if err != nil {
		return false, err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Store cached metrics for later comparison
	c.cachedMetrics = metrics

	// Check if scan count increased (new scan started)
	if c.lastScanCount > 0 && metrics.ScansTotal > c.lastScanCount {
		c.lastUpdateTime = time.Now()
		c.lastScanCount = metrics.ScansTotal
		return true, nil
	}

	// First time seeing metrics
	if c.lastScanCount == 0 {
		c.lastScanCount = metrics.ScansTotal
	}

	// Check if containers are being scanned (active scan)
	if metrics.ContainersScanned > 0 {
		c.lastUpdateTime = time.Now()
		return true, nil
	}

	// Consider update "in progress" for 30 seconds after the last scan
	// This helps catch containers that stop right after a scan starts
	if !c.lastUpdateTime.IsZero() && time.Since(c.lastUpdateTime) < 30*time.Second {
		return true, nil
	}

	return false, nil
}

// WasRecentlyUpdated checks if there was update activity within the given duration.
func (c *Client) WasRecentlyUpdated(ctx context.Context, within time.Duration) (bool, error) {
	metrics, err := c.GetMetrics(ctx)
	if err != nil {
		return false, err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if scan count increased since last check
	if c.lastScanCount > 0 && metrics.ScansTotal > c.lastScanCount {
		c.lastUpdateTime = time.Now()
		c.lastScanCount = metrics.ScansTotal
		return true, nil
	}

	// First time seeing metrics
	if c.lastScanCount == 0 {
		c.lastScanCount = metrics.ScansTotal
		return false, nil
	}

	// Check if the last update was within the specified duration
	if !c.lastUpdateTime.IsZero() && time.Since(c.lastUpdateTime) <= within {
		return true, nil
	}

	return false, nil
}

// GetLastMetrics returns the last cached metrics without making a network request.
func (c *Client) GetLastMetrics() *Metrics {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.cachedMetrics
}

// BaseURL returns the base URL the client is using.
func (c *Client) BaseURL() string {
	return c.baseURL
}
