// 9RouterX — this file is part of a port maintained by https://github.com/yanstd888/ — https://t.me/aiclub_id
package resilience

import (
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

)

// Auto-prune permanently dead connections.
//
// A 401 (invalid key), 403 (banned/disabled/deactivated body), 402, or
// insufficient_quota body signals a credential that cannot recover on its own —
// unlike a 429 or 5xx, which the existing cooldown/lockout machinery handles.
// After AUTOPRUNE_THRESHOLD consecutive permanent failures within a 1-hour
// window, the connection is marked inactive so the rotation skips it for good.
// A success at any point resets the counter.
//
// Default is OFF (no tracking) with DRY_RUN ON (log would-evict, do not evict),
// so an operator can observe the would-evict lines before arming eviction.

type autopruneConfig struct {
	Enabled  bool
	Threshold int
	DryRun   bool
}

func loadAutoPruneConfig() autopruneConfig {
	cfg := autopruneConfig{
		Enabled:   false,
		Threshold: 3,
		DryRun:    true,
	}
	if v := os.Getenv("AUTOPRUNE_ENABLED"); v == "1" || v == "true" {
		cfg.Enabled = true
	}
	if v := os.Getenv("AUTOPRUNE_THRESHOLD"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.Threshold = n
		}
	}
	if v := os.Getenv("AUTOPRUNE_DRY_RUN"); v == "0" || v == "false" {
		cfg.DryRun = false
	}
	return cfg
}

// autopruneWindow bounds how far back a failure counts toward the threshold.
const autopruneWindow = time.Hour

// autopruneState tracks consecutive permanent failures for one connection.
type autopruneState struct {
	count     int
	firstAt   time.Time // when the current streak started
	lastAt    time.Time
}

// AutoPruneTracker tracks permanent credential failures per connection and
// reports when the threshold is reached. It is safe for concurrent use.
type AutoPruneTracker struct {
	mu    sync.Mutex
	cfg   autopruneConfig
	conns map[string]*autopruneState
}

// NewAutoPruneTracker creates a tracker from env config.
func NewAutoPruneTracker() *AutoPruneTracker {
	return &AutoPruneTracker{
		cfg:   loadAutoPruneConfig(),
		conns: make(map[string]*autopruneState),
	}
}

// ClassifyPermanent reports whether an upstream error is a permanent credential
// failure: HTTP 401; HTTP 403 only when the body says banned/disabled/
// deactivated (403 rate-limit is transient); HTTP 402; or an
// insufficient_quota body.
func ClassifyPermanent(statusCode int, body string) bool {
	switch statusCode {
	case 401:
		return true
	case 402:
		return true
	case 403:
		lower := strings.ToLower(body)
		return strings.Contains(lower, "banned") ||
			strings.Contains(lower, "disabled") ||
			strings.Contains(lower, "deactivated")
	}
	lower := strings.ToLower(body)
	return strings.Contains(lower, "insufficient_quota")
}

// Record feeds one upstream response into the tracker. A 2xx resets the
// connection's counter; a permanent failure increments it. Returns true when
// the threshold is reached and the caller should evict (or log a would-evict
// during dry-run). Transient errors (429, 5xx) are no-ops. A no-op when the
// tracker is disabled.
func (t *AutoPruneTracker) Record(connID string, statusCode int, body []byte) bool {
	if t == nil || !t.cfg.Enabled {
		return false
	}
	now := time.Now().UTC()
	t.mu.Lock()
	defer t.mu.Unlock()

	if statusCode >= 200 && statusCode < 300 {
		delete(t.conns, connID)
		return false
	}
	if !ClassifyPermanent(statusCode, string(body)) {
		return false
	}

	st, ok := t.conns[connID]
	if !ok || now.Sub(st.lastAt) > autopruneWindow {
		st = &autopruneState{firstAt: now}
	}
	st.count++
	st.lastAt = now
	t.conns[connID] = st

	if st.count < t.cfg.Threshold {
		return false
	}
	if t.cfg.DryRun {
		// Dry-run keeps the counter for observation but never signals evict.
		return false
	}
	// Evicted: reset so a re-added connection starts fresh.
	delete(t.conns, connID)
	return true
}

// autoPrune is the package-level tracker, lazily created on first Record.
var autoPrune *AutoPruneTracker
var autoPruneOnce sync.Once

// AutoPruneRecord feeds one upstream response into the package-level tracker
// and returns true when the caller should deactivate the connection (threshold
// reached). Dry-run mode always returns false; the caller logs in that case.
func AutoPruneRecord(connID string, statusCode int, body []byte) bool {
	autoPruneOnce.Do(func() { autoPrune = NewAutoPruneTracker() })
	return autoPrune.Record(connID, statusCode, body)
}
