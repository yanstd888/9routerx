// 9RouterX — all source code port and modification by https://github.com/yanstd888/ — https://t.me/aiclub_id
package db

import (
	"context"
	"testing"
	"time"

	"9routerx/proxy/internal/analyticsrange"
)

// seedCacheAnalyticsRow writes one usageHistory row with the token payload the
// cache analytics resolve. When is in the past, the row lands inside any
// bounded window the tests resolve.
func seedCacheAnalyticsRow(t *testing.T, repo *Repo, when time.Time, provider, model string, prompt, cached, creation int) {
	t.Helper()
	if _, err := repo.RawDB().Exec(
		`INSERT INTO usageHistory (timestamp, provider, model, connectionId, apiKey, endpoint, promptTokens, completionTokens, cost, status, tokens)
		 VALUES (?, ?, ?, 'conn-1', 'sk-1', '/v1/chat/completions', ?, 50, 0.01, 'success', ?)`,
		when.UTC().Format(time.RFC3339), provider, model, prompt,
		`{"prompt_tokens":`+itoaCache(prompt)+`,"completion_tokens":50,"cached_tokens":`+itoaCache(cached)+`,"cache_creation_input_tokens":`+itoaCache(creation)+`}`); err != nil {
		t.Fatalf("seed cache analytics row: %v", err)
	}
}

func itoaCache(n int) string {
	if n == 0 {
		return "0"
	}
	digits := ""
	for n > 0 {
		digits = string(rune('0'+n%10)) + digits
		n /= 10
	}
	return digits
}

// metricsEqual compares the response fields the dashboard renders.
func metricsEqual(t *testing.T, fresh, cached *PromptCacheMetrics) {
	t.Helper()
	if fresh.TotalRequests != cached.TotalRequests ||
		fresh.RequestsWithCacheControl != cached.RequestsWithCacheControl ||
		fresh.TotalInputTokens != cached.TotalInputTokens ||
		fresh.TotalCachedTokens != cached.TotalCachedTokens ||
		fresh.TotalCacheCreationTokens != cached.TotalCacheCreationTokens ||
		fresh.TokensSaved != cached.TokensSaved ||
		fresh.EstimatedCostSaved != cached.EstimatedCostSaved {
		t.Fatalf("cached metrics diverge from the fresh fold:\n  fresh  %+v\n  cached %+v", *fresh, *cached)
	}
	if len(fresh.ByProvider) != len(cached.ByProvider) || len(fresh.ByModel) != len(cached.ByModel) {
		t.Fatalf("breakdown sizes diverge: providers %d/%d models %d/%d",
			len(fresh.ByProvider), len(cached.ByProvider), len(fresh.ByModel), len(cached.ByModel))
	}
	for name, stats := range fresh.ByProvider {
		if cached.ByProvider[name] != stats {
			t.Errorf("byProvider[%q] diverges: %+v vs %+v", name, stats, cached.ByProvider[name])
		}
	}
	for name, stats := range fresh.ByModel {
		if cached.ByModel[name] != stats {
			t.Errorf("byModel[%q] diverges: %+v vs %+v", name, stats, cached.ByModel[name])
		}
	}
}

// TestPromptCacheMetricsCachedParityWithFresh pins R7's correctness half: the
// cached aggregate describes exactly what a full fold of the same window
// describes, across rows that exercise every resolve branch of the token
// expressions.
func TestPromptCacheMetricsCachedParityWithFresh(t *testing.T) {
	repo, cleanup := setupAnalyticsTestDB(t)
	defer cleanup()
	resetPromptCacheWindows()
	t.Cleanup(resetPromptCacheWindows)

	now := time.Now().UTC()
	seedCacheAnalyticsRow(t, repo, now.Add(-2*time.Hour), "anthropic", "claude-3-5-sonnet", 1000, 800, 100)
	seedCacheAnalyticsRow(t, repo, now.Add(-time.Hour), "openai", "gpt-5.5", 500, 0, 0)
	seedCacheAnalyticsRow(t, repo, now.Add(-30*time.Minute), "deepseek", "deepseek-chat", 300, 200, 0)

	win := analyticsrange.Resolve("30d", now)
	fresh, err := repo.GetPromptCacheMetrics(context.Background(), win)
	if err != nil {
		t.Fatalf("fresh metrics: %v", err)
	}
	cached, err := repo.GetPromptCacheMetricsCached(win, "30d")
	if err != nil {
		t.Fatalf("cached metrics: %v", err)
	}
	metricsEqual(t, fresh, cached)

	// The trend folds the same rows: the cached hours must match the SQL
	// grouping's hours.
	trendFresh, err := repo.GetPromptCacheTrend(context.Background(), win)
	if err != nil {
		t.Fatalf("fresh trend: %v", err)
	}
	trendCached, err := repo.GetPromptCacheTrendCached(win)
	if err != nil {
		t.Fatalf("cached trend: %v", err)
	}
	if len(trendFresh) != len(trendCached) {
		t.Fatalf("trend buckets = %d cached vs %d fresh", len(trendCached), len(trendFresh))
	}
	for i, pt := range trendFresh {
		if trendCached[i] != pt {
			t.Errorf("trend[%d] diverges: %+v vs %+v", i, pt, trendCached[i])
		}
	}
}

// TestPromptCacheMetricsCachedAdvancesByDelta pins the other half: a row
// written after the cached build folds in by the next read, with no full
// rescan, and a quiet read reports the same numbers again.
func TestPromptCacheMetricsCachedAdvancesByDelta(t *testing.T) {
	repo, cleanup := setupAnalyticsTestDB(t)
	defer cleanup()
	resetPromptCacheWindows()
	t.Cleanup(resetPromptCacheWindows)

	now := time.Now().UTC()
	seedCacheAnalyticsRow(t, repo, now.Add(-2*time.Hour), "anthropic", "claude-3-5-sonnet", 1000, 800, 100)

	win := analyticsrange.Resolve("30d", now)
	first, err := repo.GetPromptCacheMetricsCached(win, "30d")
	if err != nil {
		t.Fatalf("first cached read: %v", err)
	}
	if first.TotalRequests != 1 {
		t.Fatalf("TotalRequests = %d, want 1", first.TotalRequests)
	}

	seedCacheAnalyticsRow(t, repo, now.Add(-time.Minute), "openai", "gpt-5.5", 400, 150, 0)
	second, err := repo.GetPromptCacheMetricsCached(win, "30d")
	if err != nil {
		t.Fatalf("second cached read: %v", err)
	}
	if second.TotalRequests != 2 {
		t.Fatalf("TotalRequests = %d, want 2 (the delta must fold in)", second.TotalRequests)
	}
	if second.TotalCachedTokens != 800+150 {
		t.Errorf("TotalCachedTokens = %d, want 950", second.TotalCachedTokens)
	}
	if _, ok := second.ByProvider["openai"]; !ok {
		t.Error("the delta row's provider did not appear in the breakdown")
	}

	third, err := repo.GetPromptCacheMetricsCached(win, "30d")
	if err != nil {
		t.Fatalf("third cached read: %v", err)
	}
	if third.TotalRequests != second.TotalRequests || third.TotalCachedTokens != second.TotalCachedTokens {
		t.Errorf("a quiet read moved the totals: %d/%d -> %d/%d",
			second.TotalRequests, second.TotalCachedTokens, third.TotalRequests, third.TotalCachedTokens)
	}
}

// TestCompressionAnalyticsSummaryCachedParityAndAdvance pins the compression
// side: the cached summary equals the fresh one, and a row written after the
// cached build reaches the next read through the watermark signature.
func TestCompressionAnalyticsSummaryCachedParityAndAdvance(t *testing.T) {
	repo, cleanup := setupAnalyticsTestDB(t)
	defer cleanup()
	resetCompressionSummaries()
	t.Cleanup(resetCompressionSummaries)

	now := time.Now().UTC()
	if err := repo.InsertCompressionAnalytics(context.Background(), CompressionAnalyticsRecord{
		Timestamp: now.Add(-2 * time.Hour).UTC().Format(time.RFC3339),
		Provider:  "anthropic", Model: "claude-3-5-sonnet", Mode: "rtk",
		OriginalTokens: 1000, CompressedTokens: 600, TokensSaved: 400,
	}); err != nil {
		t.Fatalf("seed compression row: %v", err)
	}

	win := analyticsrange.Resolve("30d", now)
	fresh, err := repo.GetCompressionAnalyticsSummary(context.Background(), win)
	if err != nil {
		t.Fatalf("fresh summary: %v", err)
	}
	cached, err := repo.GetCompressionAnalyticsSummaryCached(context.Background(), win, "30d")
	if err != nil {
		t.Fatalf("cached summary: %v", err)
	}
	if fresh.TotalRequests != cached.TotalRequests || fresh.TotalTokensSaved != cached.TotalTokensSaved {
		t.Fatalf("cached summary diverges: requests %d/%d saved %d/%d",
			fresh.TotalRequests, cached.TotalRequests, fresh.TotalTokensSaved, cached.TotalTokensSaved)
	}
	if len(fresh.ByMode) != len(cached.ByMode) || len(fresh.ByProvider) != len(cached.ByProvider) {
		t.Fatalf("breakdown sizes diverge: modes %d/%d providers %d/%d",
			len(fresh.ByMode), len(cached.ByMode), len(fresh.ByProvider), len(cached.ByProvider))
	}

	if err := repo.InsertCompressionAnalytics(context.Background(), CompressionAnalyticsRecord{
		Timestamp: now.Add(-time.Minute).UTC().Format(time.RFC3339),
		Provider:  "anthropic", Model: "claude-3-5-sonnet", Mode: "gcf",
		OriginalTokens: 500, CompressedTokens: 400, TokensSaved: 100,
	}); err != nil {
		t.Fatalf("seed second compression row: %v", err)
	}

	second, err := repo.GetCompressionAnalyticsSummaryCached(context.Background(), win, "30d")
	if err != nil {
		t.Fatalf("second cached read: %v", err)
	}
	if second.TotalRequests != fresh.TotalRequests+1 || second.TotalTokensSaved != fresh.TotalTokensSaved+100 {
		t.Errorf("second read = %d requests / %d saved, want %d/%d (the watermark must fold the new row in)",
			second.TotalRequests, second.TotalTokensSaved, fresh.TotalRequests+1, fresh.TotalTokensSaved+100)
	}
}
