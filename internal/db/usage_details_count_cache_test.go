// 9RouterX — all source code port and modification by https://github.com/yanstd888/ — https://t.me/aiclub_id
package db

import (
	"context"
	"testing"
	"time"
)

// seedRequestDetail writes one requestDetails row with a readable payload at a
// chosen instant.
func seedRequestDetail(t *testing.T, repo *Repo, id string, at time.Time, status string) {
	t.Helper()
	if _, err := repo.RawDB().Exec(
		`INSERT OR IGNORE INTO requestDetails (id, timestamp, provider, model, connectionId, status, data)
		 VALUES (?, ?, 'openai', 'gpt-5.5', 'conn-1', ?, '{"ok":1}')`,
		id, at.UTC().Format(time.RFC3339), status); err != nil {
		t.Fatalf("seed requestDetail %s: %v", id, err)
	}
}

// TestRequestDetailsPagedCountFollowsWatermark pins R10: the page count is
// cached behind the requestDetails watermark, so page navigations within one
// watermark keep reporting the same filtered total, and a new row moves the
// watermark — and with it the count — on the next read.
func TestRequestDetailsPagedCountFollowsWatermark(t *testing.T) {
	repo, cleanup := setupAnalyticsTestDB(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now().UTC()
	seedRequestDetail(t, repo, "rd-1", now.Add(-3*time.Hour), "success")
	seedRequestDetail(t, repo, "rd-2", now.Add(-2*time.Hour), "error")
	seedRequestDetail(t, repo, "rd-3", now.Add(-time.Hour), "success")

	rows, total, err := repo.GetRequestDetailsPaged(ctx, 2, 0, "")
	if err != nil {
		t.Fatalf("first page: %v", err)
	}
	if len(rows) != 2 || total != 3 {
		t.Fatalf("first page = %d rows, total %d; want 2 rows, 3 total", len(rows), total)
	}

	// A page navigation inside the same watermark reports the same count.
	_, total2, err := repo.GetRequestDetailsPaged(ctx, 2, 2, "")
	if err != nil {
		t.Fatalf("second page: %v", err)
	}
	if total2 != 3 {
		t.Fatalf("second page total = %d, want 3", total2)
	}

	// A new row moves the watermark; the count follows on the next read.
	seedRequestDetail(t, repo, "rd-4", now.Add(-30*time.Second), "success")
	_, total3, err := repo.GetRequestDetailsPaged(ctx, 2, 0, "")
	if err != nil {
		t.Fatalf("read after insert: %v", err)
	}
	if total3 != 4 {
		t.Fatalf("total after insert = %d, want 4 (the watermark must recount)", total3)
	}

	// The since-filter counts only its own window: an 'all' cache entry and a
	// windowed one are distinct keys.
	_, windowedTotal, err := repo.GetRequestDetailsPaged(ctx, 2, 0, now.Add(-2*time.Hour).UTC().Format(time.RFC3339))
	if err != nil {
		t.Fatalf("windowed read: %v", err)
	}
	if windowedTotal != 3 {
		t.Fatalf("windowed total = %d, want 3 (rows rd-2, rd-3, rd-4)", windowedTotal)
	}
}
