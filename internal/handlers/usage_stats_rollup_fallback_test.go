// 9RouterX — this file is part of a port maintained by https://github.com/yanstd888/ — https://t.me/aiclub_id
package handlers

import (
	json "encoding/json/v2"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"9routerx/proxy/internal/db"
)

// seedFallbackHistory inserts two successful deepseek rows inside any daily
// window: 1500/100 prompt+completion tokens total, $0.06 total.
func seedFallbackHistory(t *testing.T, repo *db.Repo) {
	t.Helper()
	now := time.Now()
	rows := []struct {
		ago    time.Duration
		prompt int
		compl  int
		cost   string
	}{
		{1 * time.Hour, 1000, 50, "0.04"},
		{2 * time.Hour, 500, 50, "0.02"},
	}
	for i, r := range rows {
		if _, err := repo.RawDB().Exec(
			`INSERT INTO usageHistory (timestamp, provider, model, connectionId, apiKey, endpoint, promptTokens, completionTokens, cost, status, tokens)
			 VALUES (?, 'deepseek', 'deepseek-chat', 'conn-1', 'sk-test', '/v1/chat/completions', ?, ?, ?, 'success', '[]')`,
			now.Add(-r.ago).UTC().Format(time.RFC3339), r.prompt, r.compl, r.cost,
		); err != nil {
			t.Fatalf("seed history row %d: %v", i, err)
		}
	}
}

func statsResponse(t *testing.T, repo *db.Repo, period string) UsageStatsResponse {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/usage/stats?period="+period, nil)
	HandleUsageStats(repo)(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	var resp UsageStatsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return resp
}

// TestUsageStats_RollupEmptyFallsBackToHistory pins the daily-window read: a
// day+ window is aggregated from raw history through the window cache (the
// usageDaily rollup is no longer a data source for this endpoint), so a rollup
// that carries nothing still reports the rows the ledger holds instead of
// zeros next to a working chart.
func TestUsageStats_RollupEmptyFallsBackToHistory(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()
	repo := db.NewRepo(database)
	resetUsageWindowCache()
	t.Cleanup(resetUsageWindowCache)

	seedFallbackHistory(t, repo)

	for _, period := range []string{"7d", "30d", "60d", "90d", "all"} {
		t.Run("period="+period, func(t *testing.T) {
			resp := statsResponse(t, repo, period)
			if resp.TotalRequests != 2 {
				t.Errorf("TotalRequests = %d, want 2", resp.TotalRequests)
			}
			if resp.TotalPromptTokens != 1500 {
				t.Errorf("TotalPromptTokens = %d, want 1500", resp.TotalPromptTokens)
			}
			if resp.TotalCompletionTokens != 100 {
				t.Errorf("TotalCompletionTokens = %d, want 100", resp.TotalCompletionTokens)
			}
			if resp.TotalCost < 0.059 || resp.TotalCost > 0.061 {
				t.Errorf("TotalCost = %f, want ~0.06", resp.TotalCost)
			}
			p := resp.ByProvider["deepseek"]
			if p.Requests != 2 || p.PromptTokens != 1500 || p.CompletionTokens != 100 {
				t.Errorf("byProvider deepseek = %+v, want the folded rows", p)
			}
			if resp.SuccessRate == nil || *resp.SuccessRate != 100 {
				t.Errorf("SuccessRate = %v, want 100", resp.SuccessRate)
			}
		})
	}
}

// TestUsageStats_EmptyWindowStaysEmpty proves a daily window stays silent when
// there is genuinely nothing to report: no rollup AND no history rows keeps
// the zero response the panel renders as an empty state.
func TestUsageStats_EmptyWindowStaysEmpty(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()
	repo := db.NewRepo(database)
	resetUsageWindowCache()
	t.Cleanup(resetUsageWindowCache)

	resp := statsResponse(t, repo, "7d")
	if resp.TotalRequests != 0 || resp.TotalPromptTokens != 0 || resp.TotalCost != 0 {
		t.Errorf("empty window = %d req / %d tok / %f cost, want zeros", resp.TotalRequests, resp.TotalPromptTokens, resp.TotalCost)
	}
	if len(resp.ByProvider) != 0 {
		t.Errorf("ByProvider = %+v, want empty", resp.ByProvider)
	}
}
