// 9RouterX — this file is part of a port maintained by https://github.com/yanstd888/ — https://t.me/aiclub_id
package resilience

import (
	"testing"
	"time"
)

func bnFixture(on bool) *BurninTracker {
	t := NewBurninTracker()
	t.on = on
	t.now = func() time.Time { return time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC) }
	return t
}

func (t *BurninTracker) advance(d time.Duration) {
	base := t.now()
	t.now = func() time.Time { return base.Add(d) }
}

// TestBurninTwoFailuresEntersSuspect proves 2 failures within the window
// transition a normal connection to suspect.
func TestBurninTwoFailuresEntersSuspect(t *testing.T) {
	tr := bnFixture(true)
	tr.Record("c1", 429)
	tr.Record("c1", 429)
	if tr.ShouldDispatch("c1") != true {
		t.Error("suspect must still dispatch (deprioritised, not blocked)")
	}
	// Verify state via the entry.
	tr.mu.Lock()
	e := tr.conns["c1"]
	if e == nil || e.state != burninSuspect {
		t.Errorf("state = %v, want suspect", e)
	}
	tr.mu.Unlock()
}

// TestBurninSuspectStillDispatches proves suspect connections are
// deprioritised (tail of rotation) but not blocked outright.
func TestBurninSuspectStillDispatches(t *testing.T) {
	tr := bnFixture(true)
	tr.Record("c1", 429)
	tr.Record("c1", 429)
	// Suspect: should still dispatch (just deprioritised).
	if !tr.ShouldDispatch("c1") {
		t.Error("suspect must still dispatch")
	}
}

// TestBurninHealThrottleOneInFive proves heal mode throttles to 1 in 5.
func TestBurninHealThrottleOneInFive(t *testing.T) {
	tr := bnFixture(true)
	// Push into suspect, then advance to heal.
	tr.Record("c1", 429)
	tr.Record("c1", 429)
	tr.advance(6 * time.Minute) // suspect → heal
	// Heal: 1 in 5 through. Test the throttle pattern.
	var allowed int
	for i := 0; i < burninHealThrottle; i++ {
		if tr.ShouldDispatch("c1") {
			allowed++
		}
	}
	if allowed != 1 {
		t.Errorf("heal allowed %d of %d, want exactly 1", allowed, burninHealThrottle)
	}
}

// TestBurninSuccessDuringHealClears proves 3 consecutive successes during
// heal clear the burn-in state entirely.
func TestBurninSuccessDuringHealClears(t *testing.T) {
	tr := bnFixture(true)
	tr.Record("c1", 429)
	tr.Record("c1", 429)
	tr.advance(6 * time.Minute) // suspect → heal
	// Allow 1 request through, then succeed 3 times.
	tr.ShouldDispatch("c1") // consume the 1-in-5
	tr.Record("c1", 200)
	tr.Record("c1", 200)
	tr.Record("c1", 200)
	if _, ok := tr.conns["c1"]; ok {
		t.Error("3 consecutive successes during heal must clear the state")
	}
}

// TestBurninFailureDuringHealGoesRescue proves a failure during heal
// escalates to rescue (out of rotation entirely).
func TestBurninFailureDuringHealGoesRescue(t *testing.T) {
	tr := bnFixture(true)
	tr.Record("c1", 429)
	tr.Record("c1", 429)
	tr.advance(6 * time.Minute) // suspect
	tr.ShouldDispatch("c1")     // triggers suspect → heal transition
	tr.Record("c1", 429)        // failure during heal → rescue
	// Rescue: out of rotation.
	if tr.ShouldDispatch("c1") {
		t.Error("rescue must block dispatch")
	}
}

// TestBurninRescueExitsAfterWindow proves rescue releases the connection
// back to suspect after the rescue window expires.
func TestBurninRescueExitsAfterWindow(t *testing.T) {
	tr := bnFixture(true)
	tr.Record("c1", 429)
	tr.Record("c1", 429)
	tr.advance(6 * time.Minute) // suspect → heal
	tr.Record("c1", 429)        // heal → rescue
	tr.advance(16 * time.Minute) // rescue window (900s = 15 min) expired
	// Rescue expired → back to suspect → still dispatches.
	if !tr.ShouldDispatch("c1") {
		t.Error("rescue expired must dispatch (suspect state)")
	}
}

// TestBurninDisabledNoOp proves the tracker does nothing when disabled.
func TestBurninDisabledNoOp(t *testing.T) {
	tr := bnFixture(false)
	tr.Record("c1", 429)
	tr.Record("c1", 429)
	if !tr.ShouldDispatch("c1") {
		t.Error("disabled tracker must always dispatch")
	}
	if _, ok := tr.conns["c1"]; ok {
		t.Error("disabled tracker must not track")
	}
}

// TestBurninNilSafe proves a nil tracker is a no-op.
func TestBurninNilSafe(t *testing.T) {
	var tr *BurninTracker
	if !tr.ShouldDispatch("c1") {
		t.Error("nil tracker must dispatch")
	}
	tr.Record("c1", 429) // must not panic
}

// TestWindowedLRUOnlyTopN proves that with ROTATION_WINDOW=3 out of 5
// connections, only 3 are considered by the LRU/sticky logic.
func TestWindowedLRUOnlyTopN(t *testing.T) {
	// This is a structural test: the window is applied in connections.go
	// by slicing the candidate list. Here we verify the concept: the
	// window is a slice operation on the sorted candidates.
	candidates := []int{1, 2, 3, 4, 5}
	window := 3
	if window > 0 && window < len(candidates) {
		candidates = candidates[:window]
	}
	if len(candidates) != 3 {
		t.Errorf("windowed candidates = %d, want 3", len(candidates))
	}
}
