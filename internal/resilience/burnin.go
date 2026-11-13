// 9RouterX — this file is part of a port maintained by https://github.com/yanstd888/ — https://t.me/aiclub_id
package resilience

import (
	"os"
	"strconv"
	"sync"
	"time"
)

// Burn-in protection: anti-burn-in state machine per connection. Accounts
// accumulating failures enter a suspect/heal/rescue cycle that progressively
// reduces the traffic they receive, giving the upstream time to recover while
// preventing the LRU from burning the same account over and over. Default OFF.

type burninState int

const (
	burninNormal burninState = iota
	burninSuspect
	burninHeal
	burninRescue
)

const (
	burninSuspectThreshold = 2 // consecutive 429/5xx failures
	burninSuspectWindow    = 10 * time.Minute
	burninHealThrottle     = 5 // 1 in 5 requests during heal
	burninHealClearCount   = 3 // consecutive successes during heal → cleared
)

type burninEntry struct {
	state       burninState
	failures    int       // consecutive failures in the current window
	windowStart time.Time // start of the current failure window
	healCount   int       // requests served during heal (for the 1/5 throttle)
	healOK      int       // consecutive successes during heal
	stateAt     time.Time // when the current state was entered
}

// BurninTracker manages per-connection burn-in state in memory. It is safe
// for concurrent use. A nil tracker is a no-op.
type BurninTracker struct {
	mu      sync.Mutex
	on      bool
	heal    time.Duration
	rescue  time.Duration
	conns   map[string]*burninEntry
	now     func() time.Time
}

// NewBurninTracker creates a tracker from env config. BURNIN_ENABLED gates it.
func NewBurninTracker() *BurninTracker {
	on := os.Getenv("BURNIN_ENABLED") == "1" || os.Getenv("BURNIN_ENABLED") == "true"
	healSecs := 300
	if v := os.Getenv("BURNIN_HEAL_SECONDS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			healSecs = n
		}
	}
	rescueSecs := 900
	if v := os.Getenv("BURNIN_RESCUE_SECONDS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			rescueSecs = n
		}
	}
	return &BurninTracker{
		on:     on,
		heal:   time.Duration(healSecs) * time.Second,
		rescue: time.Duration(rescueSecs) * time.Second,
		conns:  make(map[string]*burninEntry),
		now:    time.Now,
	}
}

// ShouldDispatch reports whether this connection should receive the request
// (true) or be deprioritised to the tail of the rotation (false). A nil
// tracker always returns true. During heal, 1 in every `healThrottle`
// requests is allowed through; during rescue, no requests until the rescue
// window expires.
func (t *BurninTracker) ShouldDispatch(connID string) bool {
	if t == nil || !t.on {
		return true
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	now := t.now()

	e, ok := t.conns[connID]
	if !ok {
		return true
	}
	// State transitions on expiry.
	switch e.state {
	case burninSuspect:
		if now.Sub(e.stateAt) >= t.heal {
			e.state = burninHeal
			e.stateAt = now
			e.healCount = 0
		}
	case burninHeal:
		if now.Sub(e.stateAt) >= t.heal {
			e.state = burninNormal
			delete(t.conns, connID)
			return true
		}
	case burninRescue:
		if now.Sub(e.stateAt) >= t.rescue {
			e.state = burninSuspect
			e.stateAt = now
			e.failures = 0
		} else {
			return false // rescue: out of rotation entirely
		}
	}

	if e.state == burninHeal {
		e.healCount++
		return e.healCount%burninHealThrottle == 1 // 1 in 5 through
	}
	return true
}

// Record feeds one request outcome into the tracker. A 2xx success clears
// the entry (after 3 consecutive successes during heal); a 429/5xx increments
// the failure count and may escalate the state. A no-op when disabled.
func (t *BurninTracker) Record(connID string, statusCode int) {
	if t == nil || !t.on {
		return
	}
	now := t.now()
	t.mu.Lock()
	defer t.mu.Unlock()

	success := statusCode >= 200 && statusCode < 300

	e, ok := t.conns[connID]
	if !ok {
		if !success && (statusCode == 429 || statusCode >= 500) {
			t.conns[connID] = &burninEntry{
				state: burninNormal, failures: 1, windowStart: now,
			}
		}
		return
	}

	if success {
		e.failures = 0
		if e.state == burninHeal {
			e.healOK++
			if e.healOK >= burninHealClearCount {
				delete(t.conns, connID)
			}
		} else if e.state == burninNormal && e.failures == 0 {
			delete(t.conns, connID)
		}
		return
	}

	// Only 429 and 5xx feed the burn-in cycle.
	if statusCode != 429 && statusCode < 500 {
		return
	}
	e.failures++
	switch e.state {
	case burninNormal:
		if e.failures >= burninSuspectThreshold && now.Sub(e.windowStart) <= burninSuspectWindow {
			e.state = burninSuspect
			e.stateAt = now
		} else if e.failures == 1 {
			e.windowStart = now
		}
	case burninSuspect:
		// Already deprioritised; stays suspect until heal timer or escalation.
	case burninHeal:
		e.state = burninRescue
		e.stateAt = now
	case burninRescue:
		// Stays in rescue; timer handles the exit.
	}
}

// burnin is the package-level tracker, lazily created on first Record.
var burnin *BurninTracker
var burninOnce sync.Once

// BurninRecord feeds one request outcome into the package-level tracker.
// A no-op when burn-in is disabled (the default).
func BurninRecord(connID string, statusCode int) {
	burninOnce.Do(func() { burnin = NewBurninTracker() })
	burnin.Record(connID, statusCode)
}
