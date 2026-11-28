// 9RouterX — ported & modified by https://github.com/yanstd888/ — https://t.me/aiclub_id
package handlers

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	json "encoding/json/v2"

	"9routerx/proxy/internal/db"
)

// seedErrorDetail writes one failed requestDetails row at a chosen instant.
// requestDetails timestamps are RFC3339 with milliseconds — the format the
// writer uses and the error-count cutoff compares against.
func seedErrorDetail(t *testing.T, repo *db.Repo, id string, at time.Time) {
	t.Helper()
	_, err := repo.RawDB().Exec(
		`INSERT OR IGNORE INTO requestDetails (id, timestamp, provider, model, connectionId, status, data)
		 VALUES (?, ?, 'deepseek', 'deepseek-chat', 'conn-1', 'error', '{}')`,
		id, at.UTC().Format("2006-01-02T15:04:05.000Z"))
	if err != nil {
		t.Fatalf("seed error detail %s: %v", id, err)
	}
}

// statsTotalErrors runs the stats handler and returns the TotalErrors it
// reported for the period.
func statsTotalErrors(t *testing.T, repo *db.Repo, period string) int {
	t.Helper()
	rec := httptest.NewRecorder()
	HandleUsageStats(repo)(rec, httptest.NewRequest(http.MethodGet, "/api/usage/stats?period="+period, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("period=%s: status = %d, body %s", period, rec.Code, rec.Body)
	}
	var resp struct {
		TotalErrors int `json:"totalErrors"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v (raw %s)", err, rec.Body)
	}
	return resp.TotalErrors
}

// TestUsageStats_ErrorCountAdvancesByDelta pins R6: the requestDetails error
// count rides the window cache, so a poll folds only the error rows written
// since the last one and a repeat poll does not rescan the table.
func TestUsageStats_ErrorCountAdvancesByDelta(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()
	repo := db.NewRepo(database)
	resetUsageWindowCache()
	t.Cleanup(resetUsageWindowCache)

	now := time.Now().UTC()
	seedErrorDetail(t, repo, "err-1", now.Add(-2*time.Hour))
	seedErrorDetail(t, repo, "err-2", now.Add(-time.Hour))

	if got := statsTotalErrors(t, repo, "24h"); got != 2 {
		t.Fatalf("TotalErrors = %d, want 2", got)
	}

	// A row written after the first poll is counted by the second without a
	// rescan: the entry advances by delta from the requestDetails watermark.
	seedErrorDetail(t, repo, "err-3", now.Add(-30*time.Second))
	if got := statsTotalErrors(t, repo, "24h"); got != 3 {
		t.Fatalf("TotalErrors after a new error row = %d, want 3 (the delta must fold in)", got)
	}

	// A third poll with nothing new reports the same number: the cached count
	// is stable, not recomputed by coincidence.
	if got := statsTotalErrors(t, repo, "24h"); got != 3 {
		t.Fatalf("TotalErrors on a quiet poll = %d, want 3", got)
	}
}

// TestUsageStats_DailyWindowErrorCount pins the same fold on the daily branch:
// a day+ window counts requestDetails errors since the calendar anchor, and
// rows older than the anchor stay out of the window.
func TestUsageStats_DailyWindowErrorCount(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()
	repo := db.NewRepo(database)
	resetUsageWindowCache()
	t.Cleanup(resetUsageWindowCache)

	now := time.Now().UTC()
	seedErrorDetail(t, repo, "err-in", now.Add(-3*24*time.Hour))
	seedErrorDetail(t, repo, "err-old", now.Add(-40*24*time.Hour))

	// 30d: the anchor is midnight 29 days back, so the 40-day-old error is out.
	if got := statsTotalErrors(t, repo, "30d"); got != 1 {
		t.Fatalf("30d TotalErrors = %d, want 1 (the 40-day-old row is outside the window)", got)
	}

	// all: the anchor is the oldest recorded row's day — with no history rows
	// the anchor is today's midnight, so requestDetails errors since today are
	// counted and the 3-day-old one is out.
	if got := statsTotalErrors(t, repo, "all"); got != 0 {
		t.Fatalf("all TotalErrors = %d, want 0 on an empty ledger (anchor is today)", got)
	}

	// Seeding one history row moves the all-window anchor to that row's day,
	// which brings the 3-day-old error inside.
	if _, err := repo.RawDB().Exec(
		`INSERT INTO usageHistory (timestamp, provider, model, apiKey, endpoint, promptTokens, completionTokens, cost, status, tokens)
		 VALUES (?, 'deepseek', 'deepseek-chat', 'sk', '/v1/chat/completions', 1, 1, 0, 'success', '{}')`,
		now.Add(-3*24*time.Hour).Format(time.RFC3339)); err != nil {
		t.Fatalf("seed history: %v", err)
	}
	resetUsageWindowCache()
	if got := statsTotalErrors(t, repo, "all"); got != 1 {
		t.Fatalf("all TotalErrors with an anchored ledger = %d, want 1", got)
	}
}

// TestGetRequestDetailsErrorCountAgedOut pins the aging-out arithmetic the
// sliding windows subtract with: exactly the rows in [oldCutoff, newCutoff)
// leave the window when the cutoff moves forward.
func TestGetRequestDetailsErrorCountAgedOut(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()
	repo := db.NewRepo(database)

	base := time.Date(2026, 11, 28, 12, 0, 0, 0, time.UTC)
	format := func(t time.Time) string { return t.UTC().Format("2006-01-02T15:04:05.000Z") }
	for i, at := range []time.Time{
		base.Add(-3 * time.Hour), // outside [t-2h, t-1h)
		base.Add(-2 * time.Hour), // at the old cutoff: in the aged span
		base.Add(-90 * time.Minute),
		base.Add(-1 * time.Hour), // at the new cutoff: still in the window
		base.Add(-30 * time.Minute),
	} {
		seedErrorDetail(t, repo, fmt.Sprintf("err-%d", i), at)
	}

	aged, err := repo.GetRequestDetailsErrorCountAgedOut(format(base.Add(-2*time.Hour)), format(base.Add(-time.Hour)))
	if err != nil {
		t.Fatalf("aged-out count: %v", err)
	}
	if aged != 2 {
		t.Errorf("aged-out = %d, want 2 (the row at the old cutoff ages out, the one at the new cutoff does not)", aged)
	}
}
