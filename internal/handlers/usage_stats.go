// 9RouterX — ported & modified by https://github.com/yanstd888/ — https://t.me/aiclub_id
package handlers

import (
	"9routerx/proxy/internal/db"
	"9routerx/proxy/internal/handlerutil"
	"9routerx/proxy/internal/translator"
	"9routerx/proxy/internal/usagetracker"
	"database/sql"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// usagePeriod is the window /api/usage/stats aggregates over.
type usagePeriod struct {
	// daily reports whether the window is aggregated from usageDaily rows. The
	// two sub-day windows instead scan raw usageHistory, because a daily rollup
	// cannot express "since 14:00 yesterday".
	daily bool
	// days is the usageDaily row limit for a daily window.
	days int
	// since is the usageHistory cutoff for a raw-history window.
	since time.Time
	// shape names the window for the aggregate cache. It is decided here because
	// this is where the difference is actually known: `today` starts at midnight
	// and every poll in that day shares one start, while `24h` and `<n>h` are
	// measured back from now and move on every request. Deriving it later from
	// the instant alone cannot tell those apart.
	shape windowShape
}

// allUsageDays bounds an unbounded period. usageDaily holds one row per calendar
// day, so this covers every window this build can have written while keeping a
// query parameter from asking for an unbounded scan.
const allUsageDays = 3650

// resolveUsagePeriod maps the ?period parameter onto a window. The named
// shortcuts cover the dashboard presets; the <n>d and <n>h forms let a client ask
// for a window that is not one of them.
//
// A zero day count means "unbounded", which only the explicit `all` shortcut
// produces: a user-supplied 0 would otherwise silently read as the floor.
func resolveUsagePeriod(raw string, now time.Time) usagePeriod {
	if raw == "" {
		raw = "today"
	}

	switch raw {
	case "today":
		day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		return usagePeriod{since: day, shape: anchoredShape(day)}
	case "24h":
		return usagePeriod{since: now.Add(-24 * time.Hour), shape: slidingShape(24 * time.Hour)}
	case "all":
		return usagePeriod{daily: true, days: allUsageDays}
	}

	if n, ok := usagePeriodCount(raw, "d"); ok {
		return usagePeriod{daily: true, days: n}
	}
	if n, ok := usagePeriodCount(raw, "h"); ok {
		span := time.Duration(n) * time.Hour
		return usagePeriod{since: now.Add(-span), shape: slidingShape(span)}
	}

	// Unrecognized input falls back to the shared 24h window rather than erroring:
	// the dashboard keeps working against an older gateway that does not know a
	// period, which is the common case when the SPA is newer than the server. The
	// fallback matches analyticsrange.Resolve so one typo'd period cannot mean a
	// 7-day rollup on the Overview and a 24h window on Cache Analytics.
	return usagePeriod{since: now.Add(-24 * time.Hour), shape: slidingShape(24 * time.Hour)}
}

// localMidnight truncates an instant to the start of its local day.
func localMidnight(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

// dailyRawWindow maps a daily calendar span onto the raw-history window the
// aggregate cache serves it from: local midnight of the earliest day in range.
// A <n>d window anchors at today-(n-1) midnight — the same calendar days the
// usageDaily rollup used to sum — and an unbounded window anchors at the
// oldest recorded row's midnight, so "all" folds recorded data rather than ten
// empty years. A daily window keyed this way is built once and advanced by
// delta, and when the anchor moves at local midnight the rows that aged out are
// subtracted (see usageWindowCache).
//
// An oldest-row read failure falls back to the calendar anchor rather than
// failing the request: the calendar window is a superset scan that returns the
// same data, only slower.
func dailyRawWindow(repo *db.Repo, window usagePeriod, now time.Time) usagePeriod {
	if window.days >= allUsageDays {
		if oldest, err := repo.GetUsageHistoryOldestTimestamp(); err == nil && oldest != "" {
			if ts, perr := time.Parse(time.RFC3339Nano, oldest); perr == nil {
				start := localMidnight(ts)
				return usagePeriod{since: start, shape: dailyShape(start)}
			}
		}
		start := localMidnight(now)
		return usagePeriod{since: start, shape: dailyShape(start)}
	}
	start := localMidnight(now).AddDate(0, 0, -(window.days - 1))
	return usagePeriod{since: start, shape: dailyShape(start)}
}

// usagePeriodCount parses the "<n><unit>" shorthand, accepting only a positive
// count with no sign or decimal part.
func usagePeriodCount(raw, unit string) (int, bool) {
	digits, ok := strings.CutSuffix(raw, unit)
	if !ok || digits == "" {
		return 0, false
	}
	n, err := strconv.Atoi(digits)
	if err != nil || n <= 0 {
		return 0, false
	}
	return n, true
}

type ProviderUsageItem struct {
	Requests         int     `json:"requests"`
	Errors           int     `json:"errors,omitempty"`
	PromptTokens     int64   `json:"promptTokens"`
	CompletionTokens int64   `json:"completionTokens"`
	CachedTokens     int64   `json:"cachedTokens"`
	Cost             float64 `json:"cost"`
	// LastUsed is the newest request timestamp in the bucket. A daily window
	// cannot fill it (a usageDaily row is keyed by date and carries no time),
	// so the dashboard renders it blank there rather than inventing midnight.
	LastUsed string `json:"lastUsed,omitempty"`
}

type ModelUsageItem struct {
	Requests         int     `json:"requests"`
	PromptTokens     int64   `json:"promptTokens"`
	CompletionTokens int64   `json:"completionTokens"`
	CachedTokens     int64   `json:"cachedTokens"`
	Cost             float64 `json:"cost"`
	RawModel         string  `json:"rawModel"`
	Provider         string  `json:"provider"`
	LastUsed         string  `json:"lastUsed"`
}

type AccountUsageItem struct {
	Requests         int     `json:"requests"`
	PromptTokens     int64   `json:"promptTokens"`
	CompletionTokens int64   `json:"completionTokens"`
	CachedTokens     int64   `json:"cachedTokens"`
	Cost             float64 `json:"cost"`
	RawModel         string  `json:"rawModel"`
	Provider         string  `json:"provider"`
	ConnectionID     string  `json:"connectionId"`
	AccountName      string  `json:"accountName"`
	LastUsed         string  `json:"lastUsed"`
}

type ApiKeyUsageItem struct {
	Requests         int     `json:"requests"`
	PromptTokens     int64   `json:"promptTokens"`
	CompletionTokens int64   `json:"completionTokens"`
	CachedTokens     int64   `json:"cachedTokens"`
	Cost             float64 `json:"cost"`
	RawModel         string  `json:"rawModel"`
	Provider         string  `json:"provider"`
	ApiKeyMasked     string  `json:"apiKeyMasked,omitempty"`
	KeyName          string  `json:"keyName"`
	ApiKeyKey        string  `json:"apiKeyKey"`
	LastUsed         string  `json:"lastUsed"`
}

type EndpointUsageItem struct {
	Requests         int     `json:"requests"`
	PromptTokens     int64   `json:"promptTokens"`
	CompletionTokens int64   `json:"completionTokens"`
	CachedTokens     int64   `json:"cachedTokens"`
	Cost             float64 `json:"cost"`
	Endpoint         string  `json:"endpoint"`
	RawModel         string  `json:"rawModel"`
	Provider         string  `json:"provider"`
	LastUsed         string  `json:"lastUsed"`
}

type UsageStatsResponse struct {
	TotalRequests         int                          `json:"totalRequests"`
	TotalErrors           int                          `json:"totalErrors"`
	SuccessRate           *float64                     `json:"successRate"`
	TotalPromptTokens     int64                        `json:"totalPromptTokens"`
	TotalCompletionTokens int64                        `json:"totalCompletionTokens"`
	TotalCachedTokens     int64                        `json:"totalCachedTokens"`
	TotalCost             float64                      `json:"totalCost"`
	ByProvider            map[string]ProviderUsageItem `json:"byProvider"`
	ByModel               map[string]ModelUsageItem    `json:"byModel"`
	ByAccount             map[string]AccountUsageItem  `json:"byAccount"`
	ByApiKey              map[string]ApiKeyUsageItem   `json:"byApiKey"`
	ByEndpoint            map[string]EndpointUsageItem `json:"byEndpoint"`
	ActiveRequests        []usagetracker.ActiveRequest `json:"activeRequests"`
	RecentRequests        []usagetracker.RecentRequest `json:"recentRequests"`
	ErrorProvider         string                       `json:"errorProvider"`
	Pending               usagetracker.PendingState    `json:"pending"`
}

// HandleUsageStats aggregates usage over the window named by ?period. Besides the
// dashboard presets ("today", "24h", "7d", "30d", "60d", "all") it accepts any
// <n>d or <n>h window, so the selector is not limited to a fixed set.
// maskAPIKeyForStats masks an API key for usage stats display and grouping.
// Keys <= 12 characters mask to firstChar + "***" to prevent leaking the raw
// key in apiKeyMasked, keyName, and apiKeyKey (upstream usageRepo.js parity).
func maskAPIKeyForStats(key string) string {
	trimmed := strings.TrimSpace(key)
	if trimmed == "" || trimmed == "local-no-key" || trimmed == "***" {
		return ""
	}
	if handlerutil.RuneCount(trimmed) <= 12 {
		return handlerutil.RunePrefix(trimmed, 1) + "***"
	}
	return handlerutil.MaskAPIKey(trimmed)
}

// usageStatsViews names the breakdowns a caller wants built. The Overview's
// breakdown table renders exactly one at a time (model, provider, account, API
// key or endpoint), but the handler used to build all five on every read: on a
// ledger with 411 models the payload measured 415 KB, and the dashboard polls
// it every five seconds — 29 MB a minute of mostly-unrendered JSON.
//
// A caller that omits ?views= still gets every breakdown, so nothing that
// exists today changes shape. The dashboard asks for the one it draws plus
// byProvider, which the totals are summed from.
type usageStatsViews struct {
	provider bool
	model    bool
	account  bool
	apiKey   bool
	endpoint bool
}

// allUsageStatsViews is the default: no ?views= parameter means "everything".
var allUsageStatsViews = usageStatsViews{provider: true, model: true, account: true, apiKey: true, endpoint: true}

// parseUsageStatsViews reads the comma-separated ?views= list. An empty value
// keeps the default. Unknown names are ignored rather than rejected: the
// endpoint is a display feed, and a typo should cost a missing table, not a
// failed read. byProvider is always built because the totals are summed from
// it, so asking for it is redundant but harmless.
func parseUsageStatsViews(raw string) usageStatsViews {
	if strings.TrimSpace(raw) == "" {
		return allUsageStatsViews
	}
	var v usageStatsViews
	v.provider = true
	for _, name := range strings.Split(raw, ",") {
		switch strings.TrimSpace(name) {
		case "none":
			// Totals only. The dashboard's five-second poll uses this: the
			// KPI row and the live pulse read totals, activeRequests and
			// recentRequests, and none of the four per-model maps — which are
			// 98% of a 415 KB payload at 411 models. The breakdown table is
			// refreshed on its own, slower schedule.
			return usageStatsViews{provider: true}
		case "model":
			v.model = true
		case "account":
			v.account = true
		case "apiKey":
			v.apiKey = true
		case "endpoint":
			v.endpoint = true
		}
	}
	return v
}

func HandleUsageStats(repo *db.Repo) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		now := time.Now()
		window := resolveUsagePeriod(r.URL.Query().Get("period"), now)
		views := parseUsageStatsViews(r.URL.Query().Get("views"))

		tracker := usagetracker.GetTracker()
		activeState := tracker.GetActiveState(repo)

		resp := UsageStatsResponse{
			ByProvider:     make(map[string]ProviderUsageItem),
			ByModel:        make(map[string]ModelUsageItem),
			ByAccount:      make(map[string]AccountUsageItem),
			ByApiKey:       make(map[string]ApiKeyUsageItem),
			ByEndpoint:     make(map[string]EndpointUsageItem),
			ActiveRequests: activeState.ActiveRequests,
			Pending:        activeState.Pending,
			ErrorProvider:  activeState.ErrorProvider,
		}

		// Load connection mapping for friendly account names
		connMap := make(map[string]string)
		if conns, err := repo.GetProviderConnections("", true); err == nil {
			for _, c := range conns {
				name := c.ID
				if c.Name != nil && *c.Name != "" {
					name = *c.Name
				} else if c.Email != nil && *c.Email != "" {
					name = *c.Email
				}
				connMap[c.ID] = name
			}
		}

		// Load provider nodes for custom reverse-proxy display names
		nodeNameMap := make(map[string]string)
		if nodes, err := repo.GetProviderNodes(); err == nil {
			for _, n := range nodes {
				if n.ID != "" && n.Name != nil && *n.Name != "" {
					nodeNameMap[n.ID] = *n.Name
				}
			}
		}

		// Friendly names for the API-key breakdown. The stored value is not
		// uniform across the shared database: rows this build writes keep only
		// the masked key, while rows the Next.js dashboard wrote keep the full
		// one. Index both forms so a stored row resolves either way.
		keyNames := make(map[string]string)
		if keys, err := repo.GetApiKeys(); err == nil {
			for _, k := range keys {
				if k.Name == nil || *k.Name == "" {
					continue
				}
				// LedgerIdentity resolves the spelling the ledger actually
				// carries: the plaintext for a legacy row, the stored mask for a
				// hash-only one. Index it and its masked form, so a stored row
				// resolves either way without ever leaking the secret.
				if secret := db.LedgerIdentity(k); secret != "" {
					keyNames[secret] = *k.Name
					if m := maskAPIKeyForStats(secret); m != "" {
						keyNames[m] = *k.Name
					}
				}
			}
		}

		if window.daily {
			// Daily calendar windows are aggregated from raw history through the
			// same window cache the sub-day windows use, keyed by the calendar
			// span (local midnight of the earliest day). Re-reading and
			// JSON-unmarshalling every usageDaily row per request is what made a
			// 30d/60d/all poll cost a full re-fold every five seconds; the cached
			// path folds once and then advances by delta. The rollup is no longer
			// a data source for this read — a change in it rebuilds the entry
			// (see dailyShape) — so rollup numbers can never stack on top of the
			// history fold. LastUsed comes free: the raw rows carry timestamps.
			dailyWin := dailyRawWindow(repo, window, now)
			agg, rdErrors, err := rawWindows.getRawWindow(repo, dailyWin)
			if err != nil {
				handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
				return
			}
			applyRawAggregate(&resp, agg, nodeNameMap, connMap, keyNames, views)
			// Mirror the sub-day branch: errors that only exist in requestDetails
			// (a failure that never wrote a usage row) are counted on top of the
			// ledger rows. The count rides the same cache entry, advanced by
			// delta instead of rescanned per poll.
			resp.TotalErrors = rdErrors
		} else {
			// Sub-day windows read raw history: a daily rollup cannot express
			// "since 14:00 yesterday".
			//
			// The fold is served from usageWindowCache, which builds a window's
			// aggregate once and then advances it from a delta. The first read
			// costs what the handler always cost; every poll after it reads only
			// the rows written since the previous poll.
			agg, rdErrors, err := rawWindows.getRawWindow(repo, window)
			if err != nil {
				handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
				return
			}
			applyRawAggregate(&resp, agg, nodeNameMap, connMap, keyNames, views)
			// Errors that only exist in requestDetails (a failure that never
			// wrote a ledger row) ride the same cache entry, advanced by delta
			// instead of a COUNT over the table on every poll.
			resp.TotalErrors = rdErrors
		}

		// Calculate total aggregates from byProvider
		successfulRequests := 0
		for _, p := range resp.ByProvider {
			successfulRequests += p.Requests
			resp.TotalPromptTokens += p.PromptTokens
			resp.TotalCompletionTokens += p.CompletionTokens
			resp.TotalCachedTokens += p.CachedTokens
			resp.TotalCost += p.Cost
		}
		resp.TotalRequests = successfulRequests + resp.TotalErrors
		if resp.TotalRequests > 0 {
			rate := math.Round(float64(successfulRequests)/float64(resp.TotalRequests)*1000) / 10
			resp.SuccessRate = &rate
		}

		// Build recent requests list (20 deduped from usageHistory)
		recentHistory, err := cachedRecentHistory(repo)
		if err != nil {
			handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		var dedupedRecent []usagetracker.RecentRequest
		seen := make(map[string]bool)
		for _, rh := range recentHistory {
			promptTok, complTok := translator.ResolveTokensCounts([]byte(rh.Tokens), rh.PromptTokens, rh.CompletionTokens)
			status := "ok"
			if rh.Status != "success" && rh.Status != "ok" && rh.Status != "" {
				status = rh.Status
			}
			if promptTok == 0 && complTok == 0 && status == "ok" {
				continue
			}
			min := ""
			if len(rh.Timestamp) >= 16 {
				min = rh.Timestamp[:16]
			}
			k := rh.Model + "|" + rh.Provider + "|" + strconv.FormatInt(promptTok, 10) + "|" + strconv.FormatInt(complTok, 10) + "|" + status + "|" + min
			if seen[k] {
				continue
			}
			seen[k] = true

			cachedTok := int(translator.CachedTokensFromJSON([]byte(rh.Tokens)))

			dedupedRecent = append(dedupedRecent, usagetracker.RecentRequest{
				Timestamp:        rh.Timestamp,
				Model:            rh.Model,
				Provider:         rh.Provider,
				PromptTokens:     int(promptTok),
				CompletionTokens: int(complTok),
				CachedTokens:     cachedTok,
				Status:           status,
			})
			if len(dedupedRecent) >= 20 {
				break
			}
		}
		resp.RecentRequests = dedupedRecent

		sanitizeAPIKeyBuckets(&resp)

		handlerutil.WriteJSON(w, http.StatusOK, resp)
	}
}

// sanitizeAPIKeyBuckets rebuilds the byApiKey map with non-secret object keys.
//
// The buckets are keyed by the stored key value so keys sharing a display mask
// stay separate rows (upstream v0.5.91), and the shared database holds rows the
// legacy Next.js dashboard wrote with the full key in that value. JSON object
// keys are serialized verbatim, so a raw key in a bucket key would leave the
// process on every /api/usage/stats read — including to a caller holding only
// an engine client API key. The response is re-keyed to the masked value the
// item already carries; rows that would collide on mask+model+provider get a
// numeric suffix, so the row separation the raw keying bought is preserved.
func sanitizeAPIKeyBuckets(resp *UsageStatsResponse) {
	if len(resp.ByApiKey) == 0 {
		return
	}
	rebuilt := make(map[string]ApiKeyUsageItem, len(resp.ByApiKey))
	for _, item := range resp.ByApiKey {
		label := item.ApiKeyKey
		if label == "" {
			label = item.ApiKeyMasked
		}
		if label == "" {
			label = "local-no-key"
		}
		key := label + "|" + item.RawModel + "|" + item.Provider
		for i := 2; ; i++ {
			if _, dup := rebuilt[key]; !dup {
				break
			}
			key = fmt.Sprintf("%s|%s|%s#%d", label, item.RawModel, item.Provider, i)
		}
		rebuilt[key] = item
	}
	resp.ByApiKey = rebuilt
}

// detailListItem is one collapsed Details-tab row.
type detailListItem struct {
	ID           string           `json:"id"`
	Timestamp    string           `json:"timestamp"`
	Provider     string           `json:"provider"`
	Model        string           `json:"model"`
	ConnectionID string           `json:"connectionId,omitempty"`
	Status       string           `json:"status"`
	Latency      map[string]int64 `json:"latency"`
	Tokens       map[string]any   `json:"tokens"`
	Account      string           `json:"account,omitempty"`
	Cost         float64          `json:"cost,omitempty"`
	Combo        string           `json:"combo,omitempty"`
	Protocol     string           `json:"protocol,omitempty"`
	LoopWarning  string           `json:"loopWarning,omitempty"`
}

// HandleRequestDetails returns paged request detail objects for the Details tab.
//
// Only the values the table renders travel in the list. The request messages and
// response body stay in the database until /api/usage/request-detail/{id} is
// called for the row a user opens, which turns a 400 KB page into a 4 KB one.
func HandleRequestDetails(repo *db.Repo) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit := 50
		offset := 0
		if lStr := r.URL.Query().Get("limit"); lStr != "" {
			if parsed, err := strconv.Atoi(lStr); err == nil && parsed > 0 && parsed <= 100 {
				limit = parsed
			}
		}
		if oStr := r.URL.Query().Get("offset"); oStr != "" {
			if parsed, err := strconv.Atoi(oStr); err == nil && parsed >= 0 {
				offset = parsed
			}
		}

		rows, total, err := repo.GetRequestDetailsPaged(r.Context(), limit, offset, detailsSince(r.URL.Query().Get("period"), time.Now()))
		if err != nil {
			handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}

		details := make([]detailListItem, 0, len(rows))
		for _, row := range rows {
			details = append(details, newDetailListItem(row))
		}

		handlerutil.WriteJSON(w, http.StatusOK, map[string]any{
			"details": details,
			"total":   total,
			"limit":   limit,
			"offset":  offset,
		})
	}
}

// detailsSince turns ?period into the timestamp floor for the request-details
// table. An empty result means unbounded: an absent parameter and `all` read the
// whole table, while `today`, `24h`, `<n>h` and `<n>d` each start where
// resolveUsagePeriod says — the same resolver /api/usage/stats uses, so the two
// sections cannot disagree about what "last 7 days" means.
func detailsSince(raw string, now time.Time) string {
	if raw == "" || raw == "all" {
		return ""
	}
	window := resolveUsagePeriod(raw, now)
	if !window.since.IsZero() {
		return window.since.UTC().Format(time.RFC3339)
	}
	if window.days > 0 {
		return now.AddDate(0, 0, -window.days).UTC().Format(time.RFC3339)
	}
	return ""
}

// newDetailListItem maps a collapsed row onto the wire shape, running the same
// cached_tokens normalization the full-payload path applied, so a row reads the
// same before and after the collapse.
func newDetailListItem(row db.RequestDetailListRow) detailListItem {
	tokens := map[string]any{}
	if row.TokensJSON != "" {
		if err := json.Unmarshal([]byte(row.TokensJSON), &tokens); err != nil {
			tokens = map[string]any{}
		}
	}
	if rawTokens, err := json.Marshal(tokens); err == nil {
		tokens["cached_tokens"] = float64(translator.CachedTokensFromJSON(rawTokens))
	}
	account := row.Account
	if account == "" {
		if row.ConnectionID != "" {
			account = row.ConnectionID
		} else {
			account = "Default"
		}
	}
	protocol := row.Protocol
	if protocol == "" {
		protocol = "OpenAI-Chat"
	}
	return detailListItem{
		ID:           row.ID,
		Timestamp:    row.Timestamp,
		Provider:     row.Provider,
		Model:        row.Model,
		ConnectionID: row.ConnectionID,
		Status:       row.Status,
		Latency:      map[string]int64{"ttft": row.LatencyTTFT, "total": row.LatencyTotal},
		Tokens:       tokens,
		Account:      account,
		Cost:         row.Cost,
		Combo:        row.Combo,
		Protocol:     protocol,
		LoopWarning:  row.LoopWarning,
	}
}

// HandleRequestDetail serves the full stored payload for one request: the
// request messages and the response body the list deliberately omits.
func HandleRequestDetail(repo *db.Repo) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// chi keys a request id by the raw path segment, so a model id that
		// contains a "/" arrives still percent-encoded ("<millis>-<model>" is
		// the requestDetails primary key, and slashed custom model ids are
		// valid since #584). Unescape before the repository read, or the
		// lookup misses a row that exists and the payload modal breaks for
		// every such request.
		id, err := url.PathUnescape(r.PathValue("id"))
		if err != nil {
			handlerutil.WriteJSONError(w, http.StatusBadRequest, "invalid request id")
			return
		}
		raw, err := repo.GetRequestDetailByID(id)
		if err != nil {
			// A detail that was never recorded, or has since been pruned by
			// retention, is a 404 rather than a server fault.
			if errors.Is(err, sql.ErrNoRows) {
				handlerutil.WriteJSONError(w, http.StatusNotFound, "request detail not found")
				return
			}
			handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}

		var item map[string]any
		if err := json.Unmarshal([]byte(raw), &item); err != nil {
			handlerutil.WriteJSONError(w, http.StatusInternalServerError, "stored request detail is unreadable")
			return
		}
		if tokens, ok := item["tokens"].(map[string]any); ok {
			if rawTokens, marshalErr := json.Marshal(tokens); marshalErr == nil {
				tokens["cached_tokens"] = float64(translator.CachedTokensFromJSON(rawTokens))
			}
		}

		handlerutil.WriteJSON(w, http.StatusOK, map[string]any{"detail": item})
	}
}
