// 9RouterX — this file is part of a port maintained by https://github.com/yanstd888/ — https://t.me/aiclub_id
package resilience

import (
	"testing"
	"time"
)

// prFixture builds a tracker with explicit config, bypassing env.
func prFixture(enabled bool, threshold int, dryRun bool) *AutoPruneTracker {
	return &AutoPruneTracker{
		cfg:   autopruneConfig{Enabled: enabled, Threshold: threshold, DryRun: dryRun},
		conns: make(map[string]*autopruneState),
	}
}

// TestAutoPrunePermanentCountsIncrement proves each permanent failure class
// (401, 403-banned, 402, insufficient_quota) increments the counter.
func TestAutoPrunePermanentCountsIncrement(t *testing.T) {
	cases := []struct {
		code int
		body string
	}{
		{401, `{"error":"invalid key"}`},
		{403, `{"error":"key banned"}`},
		{402, `{"error":"payment required"}`},
		{400, `{"error":{"message":"insufficient_quota"}}`},
	}
	for _, tc := range cases {
		tr := prFixture(true, 3, false)
		tr.Record("c1", tc.code, []byte(tc.body))
		tr.Record("c1", tc.code, []byte(tc.body))
		if tr.conns["c1"] == nil || tr.conns["c1"].count != 2 {
			t.Errorf("HTTP %d body %q: count wrong", tc.code, tc.body)
		}
	}
}

// TestAutoPruneTransientNotCounted proves 429 and 5xx do not increment.
func TestAutoPruneTransientNotCounted(t *testing.T) {
	tr := prFixture(true, 2, false)
	tr.Record("c1", 429, []byte("rate limit"))
	tr.Record("c1", 500, []byte("internal error"))
	tr.Record("c1", 503, []byte("unavailable"))
	if _, ok := tr.conns["c1"]; ok {
		t.Error("transient errors must not create a counter entry")
	}
}

// TestAutoPrune403RateLimitNotCounted proves 403 without banned body is transient.
func TestAutoPrune403RateLimitNotCounted(t *testing.T) {
	tr := prFixture(true, 1, false)
	if tr.Record("c1", 403, []byte(`rate limit exceeded`)) {
		t.Error("403 rate-limit must not trigger evict")
	}
	if _, ok := tr.conns["c1"]; ok {
		t.Error("403 rate-limit must not count")
	}
}

// TestAutoPruneThresholdEvicts proves Record returns true at the threshold.
func TestAutoPruneThresholdEvicts(t *testing.T) {
	tr := prFixture(true, 3, false)
	body := []byte(`{"error":"invalid api key"}`)
	if tr.Record("c1", 401, body) {
		t.Error("1st failure should not evict")
	}
	if tr.Record("c1", 401, body) {
		t.Error("2nd failure should not evict")
	}
	if !tr.Record("c1", 401, body) {
		t.Error("3rd failure should evict")
	}
	if _, ok := tr.conns["c1"]; ok {
		t.Error("counter not reset after evict signal")
	}
}

// TestAutoPruneDryRunNoEvict proves dry-run mode never returns true.
func TestAutoPruneDryRunNoEvict(t *testing.T) {
	tr := prFixture(true, 2, true)
	body := []byte(`{"error":"invalid api key"}`)
	if tr.Record("c1", 401, body) {
		t.Error("dry-run must not signal evict")
	}
	if tr.Record("c1", 401, body) {
		t.Error("dry-run must not signal evict at threshold")
	}
	if tr.conns["c1"] == nil || tr.conns["c1"].count != 2 {
		t.Error("dry-run must keep the counter for observation")
	}
}

// TestAutoPruneSuccessResets proves a 2xx response resets the counter.
func TestAutoPruneSuccessResets(t *testing.T) {
	tr := prFixture(true, 3, false)
	body := []byte(`{"error":"invalid api key"}`)
	tr.Record("c1", 401, body)
	tr.Record("c1", 401, body)
	tr.Record("c1", 200, []byte(`{"ok":true}`))
	if _, ok := tr.conns["c1"]; ok {
		t.Error("success must reset the counter")
	}
	tr.Record("c1", 401, body)
	tr.Record("c1", 401, body)
	if !tr.Record("c1", 401, body) {
		t.Error("3rd post-reset failure should evict (threshold=3)")
	}
}

// TestAutoPruneDisabledNoOp proves the tracker does nothing when disabled.
func TestAutoPruneDisabledNoOp(t *testing.T) {
	tr := prFixture(false, 1, false)
	if tr.Record("c1", 401, []byte(`{"error":"invalid"}`)) {
		t.Error("disabled tracker must not signal evict")
	}
	if _, ok := tr.conns["c1"]; ok {
		t.Error("disabled tracker must not track")
	}
}

// TestClassifyPermanent verifies the classification function directly.
func TestClassifyPermanent(t *testing.T) {
	cases := []struct {
		code int
		body string
		want bool
	}{
		{401, "", true},
		{401, `any body`, true},
		{402, "", true},
		{403, `key banned`, true},
		{403, `account disabled`, true},
		{403, `user deactivated`, true},
		{403, `rate limit exceeded`, false},
		{403, ``, false},
		{429, `rate limit`, false},
		{500, `internal error`, false},
		{400, `insufficient_quota`, true},
	}
	for _, tc := range cases {
		if got := ClassifyPermanent(tc.code, tc.body); got != tc.want {
			t.Errorf("ClassifyPermanent(%d, %q) = %v, want %v", tc.code, tc.body, got, tc.want)
		}
	}
}

// TestAutoPruneWindowExpiry proves the 1-hour window resets the streak.
func TestAutoPruneWindowExpiry(t *testing.T) {
	tr := prFixture(true, 3, false)
	body := []byte(`{"error":"invalid api key"}`)
	tr.Record("c1", 401, body)
	tr.Record("c1", 401, body)
	if st, ok := tr.conns["c1"]; ok {
		st.lastAt = st.lastAt.Add(-2 * time.Hour)
	}
	if tr.Record("c1", 401, body) {
		t.Error("should not evict after window expiry (streak restarted)")
	}
	if tr.conns["c1"] == nil || tr.conns["c1"].count != 1 {
		t.Error("count after window expiry wrong")
	}
}
