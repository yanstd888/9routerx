// 9RouterX — ported & modified by https://github.com/yanstd888/ — https://t.me/aiclub_id
package handlers

import (
	"testing"
	"time"

	"9routerx/proxy/internal/analyticsrange"
)

// TestUsageResolversAgreeOnFallback pins R3: the Overview's resolver and the
// analytics resolver must answer an unrecognised period with the same window.
// They used to disagree — resolveUsagePeriod fell back to a 7-day rollup while
// analyticsrange.Resolve fell back to 24h — so the same typo'd period produced
// different windows on different cards of one screen.
func TestUsageResolversAgreeOnFallback(t *testing.T) {
	now := time.Date(2026, 11, 28, 15, 30, 0, 0, time.UTC)
	wantCutoff := now.Add(-24 * time.Hour).UTC().Truncate(time.Second)

	for _, raw := range []string{"last-tuesday", "garbage", "0d", "-7d", "7x", "d"} {
		stats := resolveUsagePeriod(raw, now)
		if stats.daily {
			t.Errorf("resolveUsagePeriod(%q) fell back to a daily rollup, want the shared 24h raw window", raw)
			continue
		}
		if !stats.since.Equal(now.Add(-24 * time.Hour)) {
			t.Errorf("resolveUsagePeriod(%q).since = %s, want %s", raw, stats.since, now.Add(-24*time.Hour))
		}

		win := analyticsrange.Resolve(raw, now)
		if win.Unbounded() {
			t.Errorf("analyticsrange.Resolve(%q) fell back to the unbounded window, want 24h", raw)
			continue
		}
		if !win.Cutoff().Equal(wantCutoff) {
			t.Errorf("analyticsrange.Resolve(%q).cutoff = %s, want %s", raw, win.Cutoff(), wantCutoff)
		}
	}
}

// TestUsageResolversAgreeOnSharedForms pins the shared shapes: the named
// sub-day presets and the <n>h forms resolve to the same instants on both
// resolvers, so a window never means one thing on the Overview and another on
// Cache Analytics.
func TestUsageResolversAgreeOnSharedForms(t *testing.T) {
	now := time.Date(2026, 11, 28, 15, 30, 0, 0, time.UTC)

	for _, raw := range []string{"24h", "6h", "90h"} {
		stats := resolveUsagePeriod(raw, now)
		win := analyticsrange.Resolve(raw, now)
		if stats.since.IsZero() || !win.Cutoff().Equal(stats.since.UTC().Truncate(time.Second)) {
			t.Errorf("%s: stats since %s and analytics cutoff %s disagree", raw, stats.since, win.Cutoff())
		}
	}
}

// TestWindowLabelReportsServedWindow pins R3's label contract: a response must
// name the window it actually served, so an unrecognised period is reported as
// the 24h fallback rather than echoed verbatim as if it had been honoured.
func TestWindowLabelReportsServedWindow(t *testing.T) {
	now := time.Date(2026, 11, 28, 15, 30, 0, 0, time.UTC)

	fallback := analyticsrange.Resolve("garbage", now)
	if got := fallback.Label("garbage"); got != "24h" {
		t.Errorf("fallback Label() = %q, want %q", got, "24h")
	}

	honoured := analyticsrange.Resolve("30d", now)
	if got := honoured.Label("30d"); got != "30d" {
		t.Errorf("honoured Label() = %q, want %q", got, "30d")
	}

	all := analyticsrange.Resolve("all", now)
	if got := all.Label("all"); got != "all" {
		t.Errorf("unbounded Label() = %q, want %q", got, "all")
	}
}
