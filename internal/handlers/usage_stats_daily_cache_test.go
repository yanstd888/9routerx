// 9RouterX — ported & modified by https://github.com/yanstd888/ — https://t.me/aiclub_id
package handlers

import (
	"fmt"
	"testing"
	"time"

	"9routerx/proxy/internal/db"
)

// seedDailyHistoryAt writes one usageHistory row at a chosen instant with an
// explicit API key, so a daily-window response can be checked for the fields
// the raw fold fills.
func seedDailyHistoryAt(t *testing.T, repo *db.Repo, at time.Time, key string, prompt, completion int, cost float64) {
	t.Helper()
	_, err := repo.RawDB().Exec(
		`INSERT INTO usageHistory (timestamp, provider, model, connectionId, apiKey, endpoint, promptTokens, completionTokens, cost, status, tokens)
		 VALUES (?, 'deepseek', 'deepseek-chat', 'conn-1', ?, '/v1/chat/completions', ?, ?, ?, 'success', ?)`,
		at.UTC().Format(time.RFC3339), key, prompt, completion, cost,
		fmt.Sprintf(`{"prompt_tokens":%d,"completion_tokens":%d}`, prompt, completion))
	if err != nil {
		t.Fatalf("seed daily history row: %v", err)
	}
}

// dailyCacheEntry returns the live cache entry the given window resolves to,
// for white-box assertions on rebuild-vs-advance.
func dailyCacheEntry(t *testing.T, repo *db.Repo, period string, now time.Time) *rawWindowEntry {
	t.Helper()
	window := resolveUsagePeriod(period, now)
	daily := dailyRawWindow(repo, window, now)
	rawWindows.mu.Lock()
	defer rawWindows.mu.Unlock()
	entry, ok := rawWindows.entries[daily.shape.key()]
	if !ok {
		t.Fatalf("no cache entry for daily window %s", daily.shape.key())
	}
	return entry
}

// TestDailyRawWindowAnchorsTheCalendarSpan pins the window the daily cache is
// keyed by: local midnight of the earliest day in range. A <n>d window anchors
// at today-(n-1) midnight; `all` anchors at the oldest recorded row's midnight,
// so an unbounded read scans recorded data rather than ten empty years.
func TestDailyRawWindowAnchorsTheCalendarSpan(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()
	repo := db.NewRepo(database)

	now := time.Date(2026, 3, 10, 15, 30, 0, 0, time.UTC)

	daily := dailyRawWindow(repo, resolveUsagePeriod("30d", now), now)
	want := time.Date(2026, 2, 9, 0, 0, 0, 0, time.UTC)
	if !daily.since.Equal(want) {
		t.Errorf("30d anchor = %s, want %s", daily.since, want)
	}

	// An "all" window with no recorded rows anchors at today's midnight: the
	// window is empty either way, and the anchor must not be the zero time.
	empty := dailyRawWindow(repo, resolveUsagePeriod("all", now), now)
	if want := time.Date(2026, 3, 10, 0, 0, 0, 0, time.UTC); !empty.since.Equal(want) {
		t.Errorf("empty all anchor = %s, want %s", empty.since, want)
	}

	seedDailyHistoryAt(t, repo, time.Date(2020, 1, 15, 10, 33, 0, 0, time.UTC), "sk-test", 10, 5, 0.01)

	anchored := dailyRawWindow(repo, resolveUsagePeriod("all", now), now)
	if want := time.Date(2020, 1, 15, 0, 0, 0, 0, time.UTC); !anchored.since.Equal(want) {
		t.Errorf("all anchor = %s, want the oldest row's midnight %s", anchored.since, want)
	}
}

// TestUsageStatsDailyWindowServedFromRawHistory pins R1 and R4: a daily window
// is aggregated from raw history through the window cache, so LastUsed is
// filled, a new row is folded by the next poll, and a rollup-only change in
// usageDaily rebuilds the entry instead of stacking rollup numbers on top.
func TestUsageStatsDailyWindowServedFromRawHistory(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()
	repo := db.NewRepo(database)
	resetUsageWindowCache()
	t.Cleanup(resetUsageWindowCache)

	now := time.Now().UTC()
	base := now.AddDate(0, 0, -10)
	seedDailyHistoryAt(t, repo, base, "sk-alpha", 1000, 100, 0.04)
	seedDailyHistoryAt(t, repo, base.Add(48*time.Hour), "sk-alpha", 500, 50, 0.02)
	seedDailyHistoryAt(t, repo, base.Add(72*time.Hour), "sk-beta", 250, 25, 0.01)

	resp := statsResponse(t, repo, "30d")
	if resp.TotalRequests != 3 {
		t.Errorf("TotalRequests = %d, want 3", resp.TotalRequests)
	}
	if resp.TotalPromptTokens != 1750 || resp.TotalCompletionTokens != 175 {
		t.Errorf("totals = %d/%d, want 1750/175", resp.TotalPromptTokens, resp.TotalCompletionTokens)
	}
	if resp.TotalCost < 0.069 || resp.TotalCost > 0.071 {
		t.Errorf("TotalCost = %f, want ~0.07", resp.TotalCost)
	}

	// R4: the raw fold carries each row's timestamp, so the daily branch fills
	// LastUsed the same way the sub-day branch does.
	newest := base.Add(72 * time.Hour).UTC().Format(time.RFC3339)
	p := resp.ByProvider["deepseek"]
	if p.LastUsed != newest {
		t.Errorf("byProvider LastUsed = %q, want %q", p.LastUsed, newest)
	}
	if len(resp.ByApiKey) == 0 {
		t.Fatal("byApiKey is empty, want the seeded key buckets")
	}
	for key, item := range resp.ByApiKey {
		if item.LastUsed == "" {
			t.Errorf("byApiKey bucket %q has an empty LastUsed on a daily window", key)
		}
	}

	// A row written after the first poll is folded by the second.
	seedDailyHistoryAt(t, repo, now.Add(-time.Minute), "sk-alpha", 100, 10, 0.005)
	resp2 := statsResponse(t, repo, "30d")
	if resp2.TotalRequests != 4 {
		t.Errorf("second poll TotalRequests = %d, want 4 (the new row must fold in)", resp2.TotalRequests)
	}

	// A rollup-only change in usageDaily — no new history rows — must rebuild
	// the entry rather than leave it describing a rollup it never read.
	entry := dailyCacheEntry(t, repo, "30d", time.Now())
	before := entry.agg
	yesterday := now.AddDate(0, 0, -1).Format("2006-01-02")
	rollup := `{"promptTokens":999999,"completionTokens":999999,"cost":99,` +
		`"byProvider":{"deepseek":{"requests":777,"promptTokens":999999,"completionTokens":999999,"cost":99}}}`
	if err := repo.UpsertUsageDaily(yesterday, rollup); err != nil {
		t.Fatalf("seed usageDaily: %v", err)
	}
	resp3 := statsResponse(t, repo, "30d")
	if after := dailyCacheEntry(t, repo, "30d", time.Now()).agg; after == before {
		t.Error("a rollup-only usageDaily change did not rebuild the daily cache entry")
	}
	// The rebuild reads history, so the rollup's inflated numbers never surface.
	if resp3.TotalRequests != 4 || resp3.TotalPromptTokens != 1850 {
		t.Errorf("after rollup-only change: %d req / %d tok, want the raw 4 / 1850 (no rollup stacking)",
			resp3.TotalRequests, resp3.TotalPromptTokens)
	}
}

// TestUsageStats_RollupPresentWins keeps the original guarantee under the new
// semantics: rollup numbers do not stack on top of the history fold. The daily
// branch now reads raw history through the cache, so the rollup is a
// maintenance surface (its change rebuilds the entry) rather than a second
// source the response would double-count.
func TestUsageStats_RollupPresentWins(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()
	repo := db.NewRepo(database)
	resetUsageWindowCache()
	t.Cleanup(resetUsageWindowCache)

	yesterday := time.Now().UTC().AddDate(0, 0, -1).Format("2006-01-02")
	rollup := `{"promptTokens":1000,"completionTokens":500,"cost":1.5,` +
		`"byProvider":{"deepseek":{"requests":10,"promptTokens":1000,"completionTokens":500,"cost":1.5}}}`
	if err := repo.UpsertUsageDaily(yesterday, rollup); err != nil {
		t.Fatalf("seed usageDaily: %v", err)
	}
	seedFallbackHistory(t, repo)

	resp := statsResponse(t, repo, "7d")
	if resp.TotalRequests != 2 {
		t.Errorf("TotalRequests = %d, want the history's 2 (rollup must not stack on top)", resp.TotalRequests)
	}
	if resp.TotalPromptTokens != 1500 || resp.TotalCompletionTokens != 100 {
		t.Errorf("totals = %d/%d, want the history's 1500/100", resp.TotalPromptTokens, resp.TotalCompletionTokens)
	}
	if resp.TotalCost < 0.059 || resp.TotalCost > 0.061 {
		t.Errorf("TotalCost = %f, want the history's ~0.06", resp.TotalCost)
	}
}
