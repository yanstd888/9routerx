// 9RouterX — this file is part of a port maintained by https://github.com/yanstd888/ — https://t.me/aiclub_id
package dashboard

import (
	"net/http"
	"sync"
	"time"

	"9routerx/proxy/internal/db"
	"9routerx/proxy/internal/handlerutil"
)

// burnRateCacheTTL bounds how long the cached 24h aggregate is served. The
// dashboard stops polling when the tab is hidden, so without this a quiet
// spell would pin a stale burn rate forever.
const burnRateCacheTTL = 5 * time.Minute

// burnRateCache caches the sliding 24h token and cost totals the burn rate
// reports. /api/burn-rate is polled by the dashboard, and the read was a full
// 24-hour SUM scan of usageHistory per call. The cache mirrors the usage
// window cache's mechanics for two scalars: built once, advanced by delta
// from the usageHistory watermark, the rows that age out of the moved cutoff
// subtracted, and a watermark that moves backwards (a restore or truncation)
// rebuilt. The sums are the prompt/completion columns, exactly what the SQL
// aggregated — not the JSON-resolved counts the stats fold uses.
type burnRateCache struct {
	mu        sync.Mutex
	tokens    int64
	cost      float64
	watermark string
	cutoff    string
	fetchedAt time.Time
}

var burnRates burnRateCache

// resetBurnRateCache drops the cached totals. Tests use it to observe a cold
// cache; production never calls it. The fields are cleared in place — the
// mutex must survive the reset.
func resetBurnRateCache() {
	burnRates.mu.Lock()
	burnRates.tokens = 0
	burnRates.cost = 0
	burnRates.watermark = ""
	burnRates.cutoff = ""
	burnRates.fetchedAt = time.Time{}
	burnRates.mu.Unlock()
}

// totals returns the cached sliding 24h aggregate, building it on a miss and
// advancing it from a delta otherwise.
func (c *burnRateCache) totals(repo *db.Repo, now time.Time) (int64, float64, error) {
	cutoff := now.UTC().Add(-24 * time.Hour).Format(time.RFC3339)

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.fetchedAt.IsZero() || now.Sub(c.fetchedAt) > burnRateCacheTTL {
		return c.rebuild(repo, cutoff)
	}

	watermark, err := repo.GetUsageHistoryWatermark()
	if err != nil {
		return 0, 0, err
	}

	switch {
	case watermark < c.watermark:
		// The newest row moved backwards: the ledger was swapped, restored or
		// truncated, so the cached totals may describe rows that no longer exist.
		return c.rebuild(repo, cutoff)
	case watermark == c.watermark && cutoff == c.cutoff:
		// Nothing was written and nothing aged out.
		return c.tokens, c.cost, nil
	}

	if cutoff != c.cutoff {
		// The rows that aged out of the moved cutoff are read back and
		// subtracted, newest-first, the reverse of the order they were folded
		// in — the same convention the usage window cache applies. The range
		// read is (oldCutoff, newCutoff], so a row stamped exactly at either
		// boundary second is off by one until the TTL rebuild; the pre-cache
		// read had no such seam, but it also rescanned everything per call.
		rows, err := repo.GetUsageHistoryBetween(c.cutoff, cutoff)
		if err != nil {
			return 0, 0, err
		}
		for i := len(rows) - 1; i >= 0; i-- {
			c.tokens -= int64(rows[i].PromptTokens) + int64(rows[i].CompletionTokens)
			c.cost -= rows[i].Cost
		}
		c.cutoff = cutoff
	}

	if watermark != c.watermark {
		rows, err := repo.GetUsageHistoryBetween(c.watermark, watermark)
		if err != nil {
			return 0, 0, err
		}
		for _, row := range rows {
			// A delta row below the moved cutoff is outside the window and will
			// never re-enter it: skip it rather than count and subtract later.
			if row.Timestamp < cutoff {
				continue
			}
			c.tokens += int64(row.PromptTokens) + int64(row.CompletionTokens)
			c.cost += row.Cost
		}
		c.watermark = watermark
	}

	return c.tokens, c.cost, nil
}

// rebuild folds the whole 24h window from scratch and installs it as the
// cached totals. GetUsageHistorySince returns newest-first; the fold is
// defined oldest-first so the float cost sum lands in the same order the
// uncached SQL produced.
func (c *burnRateCache) rebuild(repo *db.Repo, cutoff string) (int64, float64, error) {
	watermark, err := repo.GetUsageHistoryWatermark()
	if err != nil {
		return 0, 0, err
	}
	rows, err := repo.GetUsageHistorySince(cutoff)
	if err != nil {
		return 0, 0, err
	}

	var tokens int64
	var cost float64
	for i := len(rows) - 1; i >= 0; i-- {
		tokens += int64(rows[i].PromptTokens) + int64(rows[i].CompletionTokens)
		cost += rows[i].Cost
	}

	c.tokens = tokens
	c.cost = cost
	c.watermark = watermark
	c.cutoff = cutoff
	c.fetchedAt = time.Now()
	if watermark != "" && watermark <= cutoff {
		// Nothing in this window has been written yet; the cutoff is the
		// honest watermark, so the first delta does not rescan the window.
		c.watermark = cutoff
	}
	return tokens, cost, nil
}

// HandleGetBurnRate serves GET /api/burn-rate: the 24-hour sliding token and
// cost burn rate with a projection of how many hours remain before each
// active budget is exhausted. Read-only, computed from the cached window
// aggregate — no new table, no scheduler.
func (h *DashboardHandler) HandleGetBurnRate(w http.ResponseWriter, r *http.Request) {
	tokens, cost, err := burnRates.totals(h.Repo, time.Now())
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Budget projections for keys with an armed budget.
	var budgets []map[string]any
	rows, err := h.Repo.RawDB().Query(
		`SELECT id, COALESCE(dailyCostLimit, 0) FROM apiKeys WHERE dailyCostLimit > 0`)
	if err == nil {
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var id string
			var limit float64
			if err := rows.Scan(&id, &limit); err != nil {
				continue
			}
			if cost > 0 && limit > 0 {
				hoursLeft := (limit - cost) / (cost / 24)
				if hoursLeft < 0 {
					hoursLeft = 0
				}
				budgets = append(budgets, map[string]any{
					"keyId": id, "spentUsd": cost, "limitUsd": limit,
					"hoursLeft": hoursLeft,
				})
			}
		}
	}
	if budgets == nil {
		budgets = []map[string]any{}
	}
	handlerutil.WriteJSON(w, http.StatusOK, map[string]any{
		"windowHours":    24,
		"tokens":         tokens,
		"costUsd":        cost,
		"tokensPerHour":  float64(tokens) / 24,
		"costPerHourUsd": cost / 24,
		"budgets":        budgets,
	})
}
