// 9RouterX — this file is part of a port maintained by https://github.com/yanstd888/ — https://t.me/aiclub_id
package dashboard

import (
	"net/http"
	"sort"
	"time"

	"9routerx/proxy/internal/analyticsrange"
	"9routerx/proxy/internal/db"
	"9routerx/proxy/internal/handlerutil"
	"9routerx/proxy/internal/pricing"
)

// HandleGetSavingsSummary aggregates the savings the gateway measures —
// prompt-cache reuse and RTK/GCF compression — into one USD figure for the
// Savings Ledger panel. The window is the `period` parameter the analytics
// endpoints share, resolved through analyticsrange so this panel and the
// analytics views cannot disagree about what "7d" means.
//
// The compression rows carry tokens-saved but no USD (the strip happens
// before pricing applies), so the compression USD is computed per model from
// the pricing table at the model's input rate — conservative: output tokens
// saved bill higher, so this understates rather than overstates. Models the
// pricing table does not know contribute tokens but zero USD, and the
// response says how much of the token total was unpriced.
//
// Token saver has no separate server-side figure (the RTK strip lands in the
// compression rows), and the response says so rather than inventing one.
func (h *DashboardHandler) HandleGetSavingsSummary(w http.ResponseWriter, r *http.Request) {
	raw := r.URL.Query().Get("period")
	if raw == "" {
		raw = "7d"
	}
	win := analyticsrange.Resolve(raw, time.Now())

	cache, err := h.Repo.GetPromptCacheMetricsCached(win, raw)
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	comp, err := h.Repo.GetCompressionAnalyticsSummaryCached(r.Context(), win, raw)
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Compression USD from the pricing table, per model.
	compUSD := 0.0
	var pricedTokens, unpricedTokens int64
	for modelKey, stats := range comp.ByModel {
		if stats.TokensSaved <= 0 {
			continue
		}
		provider, model := splitCompressionKey(modelKey)
		if p, known := pricing.GetPricingForModel(provider, model); known && p.InputPer1M > 0 {
			compUSD += float64(stats.TokensSaved) / 1e6 * p.InputPer1M
			pricedTokens += stats.TokensSaved
		} else {
			unpricedTokens += stats.TokensSaved
		}
	}

	totalTokens := cache.TokensSaved + comp.TotalTokensSaved
	totalUSD := cache.EstimatedCostSaved + compUSD

	handlerutil.WriteJSON(w, http.StatusOK, map[string]any{
		"window":           win.Label(raw),
		"totalTokensSaved": totalTokens,
		"totalUsd":         totalUSD,
		"sources": []map[string]any{
			{
				"source":      "prompt_cache",
				"tokensSaved": cache.TokensSaved,
				"usd":         cache.EstimatedCostSaved,
				"top5":        topSavedProviders(cache.ByProvider, cache.EstimatedCostSaved),
			},
			{
				"source":      "compression",
				"tokensSaved": comp.TotalTokensSaved,
				"usd":         compUSD,
				"top5":        compressionTop5(comp),
			},
			{
				"source":      "token_saver",
				"tokensSaved": int64(0),
				"usd":         0.0,
				"note":        "RTK strip lands in the compression rows; no separate server-side figure exists",
			},
		},
		"pricedTokens":   pricedTokens,
		"unpricedTokens": unpricedTokens,
		"generatedAt":    time.Now().UTC().Format(time.RFC3339),
	})
}

// topSavedProviders attributes the cache's USD to providers by their cached
// token share (the per-provider rows carry cached tokens, not USD).
func topSavedProviders(by map[string]db.PromptCacheProviderStats, totalUSD float64) []map[string]any {
	type row struct {
		name         string
		cachedTokens int64
	}
	rows := make([]row, 0, len(by))
	var allCached int64
	for name, stats := range by {
		rows = append(rows, row{name, stats.CachedTokens})
		allCached += stats.CachedTokens
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].cachedTokens > rows[j].cachedTokens })
	if len(rows) > 5 {
		rows = rows[:5]
	}
	out := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		usd := 0.0
		if allCached > 0 && totalUSD > 0 {
			usd = totalUSD * float64(r.cachedTokens) / float64(allCached)
		}
		out = append(out, map[string]any{"provider": r.name, "cachedTokens": r.cachedTokens, "usd": usd})
	}
	return out
}

// compressionTop5 lists the top five models by tokens saved with their USD
// at the input rate.
func compressionTop5(comp *db.CompressionAnalyticsSummary) []map[string]any {
	type row struct {
		model       string
		tokensSaved int64
		usd         float64
	}
	rows := make([]row, 0, len(comp.ByModel))
	for modelKey, stats := range comp.ByModel {
		if stats.TokensSaved <= 0 {
			continue
		}
		usd := 0.0
		provider, model := splitCompressionKey(modelKey)
		if p, known := pricing.GetPricingForModel(provider, model); known && p.InputPer1M > 0 {
			usd = float64(stats.TokensSaved) / 1e6 * p.InputPer1M
		}
		rows = append(rows, row{modelKey, stats.TokensSaved, usd})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].tokensSaved > rows[j].tokensSaved })
	if len(rows) > 5 {
		rows = rows[:5]
	}
	out := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		out = append(out, map[string]any{"model": r.model, "tokensSaved": r.tokensSaved, "usd": r.usd})
	}
	return out
}

// splitCompressionKey splits the ByModel key "provider|model" (the shape the
// compression rows are keyed by).
func splitCompressionKey(key string) (string, string) {
	for i := 0; i < len(key); i++ {
		if key[i] == '|' {
			return key[:i], key[i+1:]
		}
	}
	return "", key
}
