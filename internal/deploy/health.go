package deploy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// HealthChecker polls a URL for a /api/health JSON response and returns
// once the "version" field matches the expected value (or ctx is done).
type HealthChecker struct {
	client   *http.Client
	interval time.Duration
	maxWait  time.Duration
}

// NewHealthChecker builds a checker that polls every `interval` and gives
// up after `maxWait` (whichever happens first vs the caller's ctx).
func NewHealthChecker(interval, maxWait time.Duration) *HealthChecker {
	return &HealthChecker{
		client:   &http.Client{Timeout: 3 * time.Second},
		interval: interval,
		maxWait:  maxWait,
	}
}

// Wait blocks until /api/health reports the expected version. Transient
// connection failures and non-200 responses are silently retried — only
// the deadline (ctx or maxWait) surfaces as an error.
func (h *HealthChecker) Wait(ctx context.Context, url, expectedVersion string) error {
	deadline := time.Now().Add(h.maxWait)
	ctx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()

	ticker := time.NewTicker(h.interval)
	defer ticker.Stop()

	// Probe once immediately so a fast-converging deploy doesn't pay the
	// initial tick latency.
	if h.matches(ctx, url, expectedVersion) {
		return nil
	}

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("health check timed out: %w", ctx.Err())
		case <-ticker.C:
			if h.matches(ctx, url, expectedVersion) {
				return nil
			}
		}
	}
}

// matches performs a single probe. Returns true only on 200 + parseable
// JSON + version match. Any failure mode returns false (caller retries).
func (h *HealthChecker) matches(ctx context.Context, url, expectedVersion string) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false
	}
	resp, err := h.client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body) // drain to allow conn reuse
		return false
	}
	var body struct {
		Version string `json:"version"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return false
	}
	return body.Version == expectedVersion
}
