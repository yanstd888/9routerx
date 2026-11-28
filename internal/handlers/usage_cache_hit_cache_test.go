// 9RouterX — ported & modified by https://github.com/yanstd888/ — https://t.me/aiclub_id
package handlers

import (
	"fmt"
	"testing"
	"time"

	"9routerx/proxy/internal/db"
)

// seedCacheHitRow writes one usageHistory row with the token payload the
// cache-hit totals resolve: hit = the row's cached tokens, total = the
// denominator max(prompt, cached + creation).
func seedCacheHitRow(t *testing.T, repo *db.Repo, at time.Time, prompt, cached, creation int) {
	t.Helper()
	if _, err := repo.RawDB().Exec(
		`INSERT INTO usageHistory (timestamp, provider, model, connectionId, apiKey, endpoint, promptTokens, completionTokens, cost, status, tokens)
		 VALUES (?, 'anthropic', 'claude-3-5-sonnet', 'conn-1', 'sk-1', '/v1/messages', ?, 50, 0.01, 'success', ?)`,
		at.UTC().Format(time.RFC3339), prompt,
		fmt.Sprintf(`{"prompt_tokens":%d,"completion_tokens":50,"cached_tokens":%d,"cache_creation_input_tokens":%d}`, prompt, cached, creation)); err != nil {
		t.Fatalf("seed cache hit row: %v", err)
	}
}

// TestCacheHitTotalsCachedAdvancesByDelta pins R11: the cache-hit totals ride
// a watermark-gated cache, so the JSON-extracting CTE runs once per ledger
// change instead of once per dashboard read, and a new row is counted by the
// next read.
func TestCacheHitTotalsCachedAdvancesByDelta(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()
	repo := db.NewRepo(database)
	resetCacheHitTotalsCache()
	t.Cleanup(resetCacheHitTotalsCache)

	now := time.Now().UTC()
	weekAgo := now.AddDate(0, 0, -7).Format(time.RFC3339)
	seedCacheHitRow(t, repo, now.Add(-2*time.Hour), 1000, 800, 100)

	hit, total, err := cachedUsageCacheHitTotals(repo, weekAgo)
	if err != nil {
		t.Fatalf("first read: %v", err)
	}
	if hit != 800 {
		t.Errorf("hit = %d, want 800", hit)
	}
	if total != 1000 {
		t.Errorf("total = %d, want 1000 (the denominator is the prompt)", total)
	}

	// A quiet read serves the same numbers from the cache.
	hit2, total2, err := cachedUsageCacheHitTotals(repo, weekAgo)
	if err != nil {
		t.Fatalf("quiet read: %v", err)
	}
	if hit2 != hit || total2 != total {
		t.Errorf("quiet read moved the totals: %d/%d -> %d/%d", hit, total, hit2, total2)
	}

	// A new row folds in by the next read.
	seedCacheHitRow(t, repo, now.Add(-time.Minute), 500, 300, 0)
	hit3, total3, err := cachedUsageCacheHitTotals(repo, weekAgo)
	if err != nil {
		t.Fatalf("read after a new row: %v", err)
	}
	if hit3 != 800+300 {
		t.Errorf("hit = %d, want 1100 (the delta must fold in)", hit3)
	}
	if total3 != 1000+500 {
		t.Errorf("total = %d, want 1500", total3)
	}

	// Parity with the uncached read on the same data.
	freshHit, freshTotal, err := repo.GetUsageCacheHitTotalsSince(weekAgo)
	if err != nil {
		t.Fatalf("fresh read: %v", err)
	}
	if freshHit != hit3 || freshTotal != total3 {
		t.Errorf("cached %d/%d diverges from fresh %d/%d", hit3, total3, freshHit, freshTotal)
	}
}
