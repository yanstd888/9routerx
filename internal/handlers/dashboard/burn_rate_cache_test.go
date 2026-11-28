// 9RouterX — ported & modified by https://github.com/yanstd888/ — https://t.me/aiclub_id
package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"9routerx/proxy/internal/db"
)

// seedBurnRowAt writes one usageHistory row at a chosen instant. The burn
// rate sums the prompt and completion columns exactly as its SQL did.
func seedBurnRowAt(t *testing.T, repo *db.Repo, at time.Time, prompt, completion int, cost float64) {
	t.Helper()
	_, err := repo.RawDB().Exec(
		`INSERT INTO usageHistory (timestamp, provider, model, connectionId, apiKey, endpoint, promptTokens, completionTokens, cost, status, tokens)
		 VALUES (?, 'openai', 'gpt-5.5', 'conn-1', 'sk-1', '/v1/chat/completions', ?, ?, ?, 'success', '{}')`,
		at.UTC().Format(time.RFC3339), prompt, completion, cost)
	if err != nil {
		t.Fatalf("seed burn row: %v", err)
	}
}

// burnRateJSON runs GET /api/burn-rate and decodes the response.
func burnRateJSON(t *testing.T, repo *db.Repo) map[string]any {
	t.Helper()
	router := setupTestRouter(repo)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/burn-rate", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body)
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode burn rate: %v (raw %s)", err, rec.Body)
	}
	return out
}

func burnNumber(t map[string]any, key string) float64 {
	n, _ := t[key].(float64)
	return n
}

// TestBurnRateServedFromCachedWindow pins R9: the burn rate serves its 24h
// totals from a cached aggregate that advances by delta, so a poll folds only
// the rows written since the last one and a row outside the window stays out.
func TestBurnRateServedFromCachedWindow(t *testing.T) {
	repo, cleanup := setupTestDB(t)
	defer cleanup()
	resetBurnRateCache()
	t.Cleanup(resetBurnRateCache)

	now := time.Now().UTC()
	seedBurnRowAt(t, repo, now.Add(-2*time.Hour), 200, 100, 0.40)
	seedBurnRowAt(t, repo, now.Add(-time.Hour), 100, 50, 0.20)

	first := burnRateJSON(t, repo)
	if got := burnNumber(first, "tokens"); got != 450 {
		t.Errorf("tokens = %v, want 450", got)
	}
	if got := burnNumber(first, "costUsd"); got < 0.59 || got > 0.61 {
		t.Errorf("costUsd = %v, want ~0.60", got)
	}

	// A row written after the first read is folded by the second.
	seedBurnRowAt(t, repo, now.Add(-time.Minute), 30, 20, 0.05)
	second := burnRateJSON(t, repo)
	if got := burnNumber(second, "tokens"); got != 500 {
		t.Errorf("tokens after a new row = %v, want 500 (the delta must fold in)", got)
	}

	// A row older than the 24h window never counts, before or after the cache
	// is warm.
	seedBurnRowAt(t, repo, now.Add(-30*time.Hour), 999999, 999999, 999)
	third := burnRateJSON(t, repo)
	if got := burnNumber(third, "tokens"); got != 500 {
		t.Errorf("tokens with an out-of-window row = %v, want 500", got)
	}
}
