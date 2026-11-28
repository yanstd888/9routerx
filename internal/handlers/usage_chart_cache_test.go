// 9RouterX — ported & modified by https://github.com/yanstd888/ — https://t.me/aiclub_id
package handlers

import (
	json "encoding/json/v2"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"9routerx/proxy/internal/db"
)

// chartBodies runs the chart handler and decodes the bucket list it wrote.
func chartBodies(t *testing.T, repo *db.Repo, period string) []chartBucket {
	t.Helper()
	rec := httptest.NewRecorder()
	HandleUsageChart(repo)(rec, httptest.NewRequest(http.MethodGet, "/api/usage/chart?period="+period, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("period=%s: status = %d, body %s", period, rec.Code, rec.Body)
	}
	var out []chartBucket
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode chart body: %v (raw %s)", err, rec.Body)
	}
	return out
}

// chartScanCounter swaps the chart cache's history read for a counting
// wrapper, restoring it on cleanup. It returns the counter's address.
func chartScanCounter(t *testing.T) *int {
	t.Helper()
	scans := 0
	orig := chartWindows.historyScan
	chartWindows.historyScan = func(repo *db.Repo, cutoff string) ([]db.UsageChartRow, error) {
		scans++
		return orig(repo, cutoff)
	}
	t.Cleanup(func() { chartWindows.historyScan = orig })
	return &scans
}

// TestUsageChartSingleScanPerCacheEntry pins R2: a daily chart period performs
// exactly one full usageHistory range scan per cache entry. The per-request
// path used to re-run the rollup range read, the whole-window history scan and
// — for `all` — an oldest-row probe on every five-second poll.
func TestUsageChartSingleScanPerCacheEntry(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()
	repo := db.NewRepo(database)
	resetChartWindowCache()
	t.Cleanup(resetChartWindowCache)

	scans := chartScanCounter(t)

	now := time.Now().UTC()
	// 40 days of history: enough that the rollup range read and the history
	// scan are real work, and that a 30d window's span is fully covered.
	for i := 1; i <= 40; i++ {
		seedDailyHistoryAt(t, repo, now.AddDate(0, 0, -i), "sk-test", 100, 50, 0.01)
	}

	first := chartBodies(t, repo, "30d")
	if *scans != 1 {
		t.Fatalf("history scans after the first chart read = %d, want exactly 1", *scans)
	}
	if len(first) != 30 {
		t.Fatalf("buckets = %d, want 30 for a 30d window", len(first))
	}
	var total int64
	for _, b := range first {
		total += b.Tokens
	}
	// The window spans today-29 .. today; rows were seeded for today-1 ..
	// today-40, so 29 of them land inside. No rollup rows exist, so the totals
	// come from the history fallback the rollup-empty path defines.
	if total != 29*150 {
		t.Errorf("bucket tokens total = %d, want %d (29 seeded days inside the window)", total, 29*150)
	}

	// A row written after the first read must fold into today's bucket by the
	// second read — without a second full-range scan.
	seedDailyHistoryAt(t, repo, now.Add(-time.Minute), "sk-test", 40, 20, 0.001)
	second := chartBodies(t, repo, "30d")
	if *scans != 1 {
		t.Fatalf("history scans after a repeat poll = %d, want still 1 (the delta must fold from the watermark)", *scans)
	}
	last := second[len(second)-1]
	if last.Tokens != 60 {
		t.Errorf("today bucket tokens = %d, want 60 (the new row folded in)", last.Tokens)
	}
	if last.ByModel["deepseek"] != 60 {
		t.Errorf("today bucket deepseek family = %d, want 60 (the delta row must join the family split)", last.ByModel["deepseek"])
	}

	// A third poll with nothing new stays cached: one scan still.
	third := chartBodies(t, repo, "30d")
	if *scans != 1 {
		t.Fatalf("history scans after a quiet poll = %d, want still 1", *scans)
	}
	if third[len(third)-1].Tokens != last.Tokens {
		t.Errorf("quiet poll moved the today bucket: %d -> %d", last.Tokens, third[len(third)-1].Tokens)
	}
}

// TestUsageChartAllPeriodSingleScan pins the same guarantee for `all`, whose
// span is derived from the oldest recorded row once per cache entry.
func TestUsageChartAllPeriodSingleScan(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()
	repo := db.NewRepo(database)
	resetChartWindowCache()
	t.Cleanup(resetChartWindowCache)

	scans := chartScanCounter(t)

	now := time.Now().UTC()
	seedDailyHistoryAt(t, repo, now.AddDate(0, 0, -100), "sk-test", 100, 50, 0.01)
	seedDailyHistoryAt(t, repo, now.Add(-time.Hour), "sk-test", 10, 5, 0.001)

	if got := chartBodies(t, repo, "all"); len(got) < 101 {
		t.Fatalf("all buckets = %d, want at least 101 (oldest day through today)", len(got))
	}
	if *scans != 1 {
		t.Fatalf("history scans for the all-period first read = %d, want 1", *scans)
	}

	// The repeat poll is served from the cache: still one scan, and today's
	// bucket picked up the seeded hour.
	second := chartBodies(t, repo, "all")
	if *scans != 1 {
		t.Fatalf("history scans after the all-period repeat poll = %d, want 1", *scans)
	}
	last := second[len(second)-1]
	if last.Tokens != 15 {
		t.Errorf("today bucket tokens = %d, want 15 (the seeded row)", last.Tokens)
	}
}

// TestUsageChartRollupOnlyChangeRebuilds pins the cache invalidation: a
// rollup-only change in usageDaily (no new history rows) must rebuild the
// entry, so a corrected rollup reaches the chart on the next poll.
func TestUsageChartRollupOnlyChangeRebuilds(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()
	repo := db.NewRepo(database)
	resetChartWindowCache()
	t.Cleanup(resetChartWindowCache)

	scans := chartScanCounter(t)

	now := time.Now().UTC()
	seedDailyHistoryAt(t, repo, now.AddDate(0, 0, -3), "sk-test", 100, 50, 0.01)

	if got := chartBodies(t, repo, "7d"); len(got) != 7 {
		t.Fatalf("buckets = %d, want 7", len(got))
	}
	if *scans != 1 {
		t.Fatalf("history scans = %d, want 1", *scans)
	}

	// A rollup-only change: the corrected day total must surface on the next
	// poll, which means the entry rebuilt (and rescanned, once).
	yesterday := now.AddDate(0, 0, -3).UTC().Format("2006-01-02")
	if err := repo.UpsertUsageDaily(yesterday, `{"promptTokens":5000,"completionTokens":100,"cost":1.25}`); err != nil {
		t.Fatalf("seed usageDaily: %v", err)
	}

	second := chartBodies(t, repo, "7d")
	if *scans != 2 {
		t.Fatalf("history scans after a rollup-only change = %d, want 2 (the entry rebuilt)", *scans)
	}
	var corrected int64
	for _, b := range second {
		corrected += b.Tokens
	}
	if corrected < 5000 {
		t.Errorf("chart totals = %d after the rollup correction, want at least the corrected day's 5100", corrected)
	}
}
