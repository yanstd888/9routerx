// 9RouterX — source port and modifications by https://github.com/yanstd888/ — https://t.me/aiclub_id
package handlers

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"9routerx/proxy/internal/db"
	"9routerx/proxy/internal/handlerutil"
)

// chartBucket is one point of the usage chart. ByModel carries the per-model
// family token split the stacked area chart layers on top of the bucket total;
// the server folds every family past the top ones into "other" so the legend
// stays readable (parity with techysy/10router's getChartData).
type chartBucket struct {
	Label   string           `json:"label"`
	Tokens  int64            `json:"tokens"`
	Cost    float64          `json:"cost"`
	ByModel map[string]int64 `json:"byModel"`
}

// codenameFamilies maps opaque or proprietary codenames to the product family
// they belong to (e.g. Qoder's qfmodel → qwen, dfmodel → deepseek). Ported from
// 10router's CODENAME_FAMILIES so the chart legend shows one bar per product.
var codenameFamilies = []struct {
	name   string
	family string
}{
	{"qmodel", "qwen"},
	{"qfmodel", "qwen"},
	{"qwq", "qwen"},
	{"dmodel", "deepseek"},
	{"dfmodel", "deepseek"},
	{"kmodel", "kimi"},
	{"gmodel", "glm"},
	{"gfm", "glm"},
	{"mmodel", "minimax"},
	{"ultimate", "claude"},
	{"performance", "claude"},
}

// brandFamilies folds different product-line prefixes of one brand into a
// single family (stepaudio → step), applied after the first-segment strip.
var brandFamilies = map[string]string{"stepaudio": "step"}

// modelFamilyName reduces a model id to its chart family. The rules mirror
// 10router: strip a provider prefix, fold known codenames, drop a trailing
// version run from the first hyphen segment (gpt-5.6-sol → gpt), and classify
// custom-channel uuid prefixes as "other".
func modelFamilyName(model string) string {
	raw := strings.TrimSpace(model)
	if raw == "" {
		return "unknown"
	}
	if i := strings.LastIndex(raw, "/"); i >= 0 {
		raw = raw[i+1:]
	}
	lower := strings.ToLower(raw)
	if hasHexPrefix(lower, 8) {
		return "other"
	}
	for _, cf := range codenameFamilies {
		if lower == cf.name || strings.HasPrefix(lower, cf.name) {
			return cf.family
		}
	}
	first := strings.SplitN(lower, "-", 2)[0]
	if first == "" {
		return "other"
	}
	stripped := trimTrailingVersion(first)
	if stripped == "" {
		stripped = first
	}
	if fam, ok := brandFamilies[stripped]; ok {
		return fam
	}
	return stripped
}

// hasHexPrefix reports whether s starts with at least n hex characters — the
// shape of a custom-channel uuid prefix like "85d2a64e-…".
func hasHexPrefix(s string, n int) bool {
	count := 0
	for _, r := range s {
		if count == n {
			return true
		}
		isHex := (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')
		if !isHex {
			return false
		}
		count++
	}
	return count == n
}

// trimTrailingVersion drops a trailing numeric run (with optional dot groups)
// from a family candidate: "gpt5" → "gpt", "qwen3.8" → "qwen". A purely
// numeric segment is left intact instead of collapsing to "".
func trimTrailingVersion(s string) string {
	i := len(s)
	for i > 0 {
		c := s[i-1]
		if (c >= '0' && c <= '9') || c == '.' {
			i--
			continue
		}
		break
	}
	if i == 0 {
		return s
	}
	return s[:i]
}

// finalizeChartBuckets keeps the top families by period total and folds the
// rest into "other" so the chart legend stays bounded (parity with 10router's
// finalizeModelBuckets, max 7 families).
func finalizeChartBuckets(buckets []chartBucket, maxFamilies int) {
	totals := map[string]int64{}
	for _, b := range buckets {
		for fam, t := range b.ByModel {
			totals[fam] += t
		}
	}
	type famTotal struct {
		fam   string
		total int64
	}
	ranked := make([]famTotal, 0, len(totals))
	for fam, t := range totals {
		ranked = append(ranked, famTotal{fam, t})
	}
	sort.Slice(ranked, func(i, j int) bool { return ranked[i].total > ranked[j].total })
	top := map[string]bool{}
	for i, ft := range ranked {
		if i >= maxFamilies {
			break
		}
		top[ft.fam] = true
	}
	for i := range buckets {
		merged := map[string]int64{}
		for fam, t := range buckets[i].ByModel {
			key := fam
			if !top[fam] {
				key = "other"
			}
			merged[key] += t
		}
		buckets[i].ByModel = merged
	}
}

// dayKeyFor is the UTC calendar key of a timestamp. usageDaily rows are keyed
// by UTC day (see the chat usage writer), so the chart buckets by UTC too —
// mixing a local key here would split one day's rollup across two buckets.
func dayKeyFor(t time.Time) string {
	return t.UTC().Format("2006-01-02")
}

// addFamily folds a row's tokens into a bucket's family map.
func addFamily(byModel map[string]int64, model string, tokens int64) {
	if tokens == 0 {
		return
	}
	byModel[modelFamilyName(model)] += tokens
}

// parseChartPeriod maps the ?period parameter onto a bucket kind and count.
// It mirrors the dashboard presets; anything unrecognized falls back to 7d.
func parseChartPeriod(raw string) (kind string, count int) {
	switch raw {
	case "today", "24h":
		return raw, 24
	case "all":
		return "all", 0
	case "30d":
		return "daily", 30
	case "60d":
		return "daily", 60
	}
	// The selector also allows custom "<n>d" / "<n>h" windows (see
	// resolveUsagePeriod); mirror that here so the chart follows the same
	// window the stats cards use. Hourly windows cap at 7 days of buckets.
	if n, ok := usagePeriodCount(raw, "d"); ok {
		return "daily", min(n, allUsageDays)
	}
	if n, ok := usagePeriodCount(raw, "h"); ok {
		return "hourly", min(n, 168)
	}
	return "daily", 7
}

// HandleUsageChart serves GET /api/usage/chart?period=: the time series the
// Overview chart draws. Today and 24h bucket by hour out of usageHistory;
// daily periods read the usageDaily rollup for totals and fill the per-family
// split from usageHistory rows in range. Daily periods are served through the
// chart window cache: the rollup range read, the whole-window history scan and
// the oldest-row probe run once per cache entry, and every poll after that
// folds only the rows written since the last one.
func HandleUsageChart(repo *db.Repo) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		kind, count := parseChartPeriod(r.URL.Query().Get("period"))
		now := time.Now()

		var buckets []chartBucket
		switch kind {
		case "today":
			start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
			buckets = hourlyChartBuckets(repo, start, count)
		case "24h", "hourly":
			buckets = hourlyChartBuckets(repo, now.Add(-time.Duration(count)*time.Hour), count)
		default:
			buckets = chartWindows.getBuckets(repo, now, kind, count)
		}

		finalizeChartBuckets(buckets, 7)
		handlerutil.WriteJSON(w, http.StatusOK, buckets)
	}
}

// hourlyChartBuckets builds count one-hour buckets from raw history rows.
func hourlyChartBuckets(repo *db.Repo, start time.Time, count int) []chartBucket {
	buckets := make([]chartBucket, count)
	for i := range buckets {
		buckets[i] = chartBucket{Label: start.Add(time.Duration(i) * time.Hour).Format("15:04"), ByModel: map[string]int64{}}
	}
	rows, err := repo.GetUsageHistoryChartSince(start.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return buckets
	}
	for _, row := range rows {
		ts, err := time.Parse(time.RFC3339Nano, row.Timestamp)
		if err != nil {
			continue
		}
		idx := int(ts.Sub(start) / time.Hour)
		if idx < 0 || idx >= count {
			continue
		}
		tokens := int64(row.PromptTokens) + int64(row.CompletionTokens)
		buckets[idx].Tokens += tokens
		buckets[idx].Cost += row.Cost
		addFamily(buckets[idx].ByModel, row.Model, tokens)
	}
	return buckets
}

// dailyChartBuckets builds one bucket per calendar day ending today. Totals
// come from the usageDaily rollup; the per-family split comes from a range
// scan of usageHistory over the same window (the rollup carries only totals).
// count == 0 means "all recorded days", derived from the earliest daily row.
func dailyChartBuckets(repo *db.Repo, now time.Time, count int) []chartBucket {
	build := buildDailyChartBuckets(repo, now, count, (*db.Repo).GetUsageHistoryChartSince)
	if build == nil {
		return nil
	}
	return build.buckets
}

// chartBuild is what one full chart build produces: the buckets (unfinalized —
// every family is still present, the top-7 fold runs per response), the
// history-derived per-bucket tokens and cost the rollup-empty fallback
// reports, and whether the rollup carried totals at all.
type chartBuild struct {
	buckets      []chartBucket
	histTokens   []int64
	histCost     []float64
	rollupTotals bool
}

// buildDailyChartBuckets is the once-per-cache-entry build. The history read
// goes through historyScan so the acceptance test can count the full-range
// scans a cache entry performs.
func buildDailyChartBuckets(repo *db.Repo, now time.Time, count int, historyScan func(*db.Repo, string) ([]db.UsageChartRow, error)) *chartBuild {
	dayRows, err := repo.GetUsageDailyRange(allUsageDays)
	if err != nil {
		return nil
	}
	if count == 0 {
		earliestKey := ""
		if len(dayRows) > 0 {
			earliestKey = dayRows[len(dayRows)-1].DateKey
		} else if oldest, oerr := repo.GetUsageHistoryOldestTimestamp(); oerr == nil && oldest != "" {
			if ts, perr := time.Parse(time.RFC3339Nano, oldest); perr == nil {
				earliestKey = dayKeyFor(ts)
			}
		}
		if earliestKey == "" {
			return nil
		}
		earliest, err := time.Parse("2006-01-02", earliestKey)
		if err != nil {
			return nil
		}
		today, _ := time.Parse("2006-01-02", dayKeyFor(now))
		count = int(today.Sub(earliest)/(24*time.Hour)) + 1
		if count < 1 {
			count = 1
		}
	}

	index := make(map[string]int, count)
	buckets := make([]chartBucket, count)
	for i := 0; i < count; i++ {
		d := now.AddDate(0, 0, -(count - 1 - i))
		key := dayKeyFor(d)
		index[key] = i
		buckets[i] = chartBucket{Label: d.Format("Jan 2"), ByModel: map[string]int64{}}
	}

	rollupTokens := int64(0)
	for _, row := range dayRows {
		i, ok := index[row.DateKey]
		if !ok {
			continue
		}
		var dayData struct {
			PromptTokens     int64   `json:"promptTokens"`
			CompletionTokens int64   `json:"completionTokens"`
			Cost             float64 `json:"cost"`
		}
		if err := json.Unmarshal([]byte(row.Data), &dayData); err != nil {
			continue
		}
		buckets[i].Tokens = dayData.PromptTokens + dayData.CompletionTokens
		buckets[i].Cost = dayData.Cost
		rollupTokens += buckets[i].Tokens
	}

	rangeStart := now.AddDate(0, 0, -(count - 1))
	rows, err := historyScan(repo, rangeStart.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return &chartBuild{buckets: buckets, rollupTotals: rollupTokens > 0}
	}
	histTokens := make([]int64, len(buckets))
	histCost := make([]float64, len(buckets))
	for _, row := range rows {
		ts, err := time.Parse(time.RFC3339Nano, row.Timestamp)
		if err != nil {
			continue
		}
		i, ok := index[dayKeyFor(ts)]
		if !ok {
			continue
		}
		tokens := int64(row.PromptTokens) + int64(row.CompletionTokens)
		histTokens[i] += tokens
		histCost[i] += row.Cost
		addFamily(buckets[i].ByModel, row.Model, tokens)
	}
	// The rollup is an optimisation, not the source of truth: an empty or
	// lost rollup would leave the totals at zero while the family split
	// (scanned straight from usageHistory) shows data. When the rollup
	// carries nothing, fall back to the history scan for tokens and cost so
	// both views tell the same story.
	rollupTotals := rollupTokens > 0
	if !rollupTotals {
		for i := range buckets {
			buckets[i].Tokens = histTokens[i]
			buckets[i].Cost = histCost[i]
		}
	}
	return &chartBuild{buckets: buckets, histTokens: histTokens, histCost: histCost, rollupTotals: rollupTotals}
}

// chartEntry is one cached daily chart. It holds the UNFINALIZED buckets —
// every model family is still present — because a delta row can introduce a
// family that belongs in the top seven; the finalize fold runs per response on
// a copy.
type chartEntry struct {
	buckets      []chartBucket
	histTokens   []int64
	histCost     []float64
	rollupTotals bool
	todayIdx     int
	watermark    string
	rollupSig    string
	fetchedAt    time.Time
}

// chartWindowCache caches one built chart per (period, UTC day). The history
// read is a field rather than a direct call so the acceptance test can count
// how many full-range scans one cache entry performs — the guarantee R2 is
// about.
type chartWindowCache struct {
	mu          sync.Mutex
	entries     map[string]*chartEntry
	historyScan func(*db.Repo, string) ([]db.UsageChartRow, error)
}

var chartWindows = &chartWindowCache{historyScan: (*db.Repo).GetUsageHistoryChartSince}

// resetChartWindowCache drops every cached chart. Tests use it to observe a
// cold cache; production never calls it.
func resetChartWindowCache() {
	chartWindows.mu.Lock()
	chartWindows.entries = nil
	chartWindows.mu.Unlock()
}

// getBuckets returns the finalized bucket list for a daily chart period,
// building it on a miss and advancing it from a delta otherwise. The caller
// receives a copy: the cached buckets are mutated in place by the next poll.
func (c *chartWindowCache) getBuckets(repo *db.Repo, now time.Time, kind string, count int) []chartBucket {
	key := kind + ":" + dayKeyFor(now)

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.entries == nil || len(c.entries) >= usageCacheMaxEntries {
		c.entries = make(map[string]*chartEntry, usageCacheMaxEntries)
	}

	entry, ok := c.entries[key]
	if !ok || time.Since(entry.fetchedAt) > usageCacheTTL {
		return c.rebuild(repo, key, now, kind, count)
	}

	// A rollup-only change (a corrected day, a backfill) must reach the chart,
	// which means rebuilding: the totals for the covered days are rollup data.
	if n, newest, err := repo.GetUsageDailySignature(); err != nil || entry.rollupSig != dailyRollupSignature(n, newest) {
		return c.rebuild(repo, key, now, kind, count)
	}

	watermark, err := repo.GetUsageHistoryWatermark()
	if err != nil {
		return copyChartBuckets(entry.buckets)
	}
	if watermark < entry.watermark {
		// The ledger moved backwards: the cached buckets may describe rows that
		// no longer exist.
		return c.rebuild(repo, key, now, kind, count)
	}

	if watermark != entry.watermark {
		rows, err := repo.GetUsageHistoryBetween(entry.watermark, watermark)
		if err != nil {
			return copyChartBuckets(entry.buckets)
		}
		for _, row := range rows {
			ts, perr := time.Parse(time.RFC3339Nano, row.Timestamp)
			if perr != nil || ts.After(now) {
				continue
			}
			day := dayKeyFor(ts)
			// A delta row can only land in a bucket the window already spans:
			// the watermark is inside the window, so anything after it is too.
			// Rows below the window's start are pre-watermark by definition and
			// cannot appear here.
			if idx := chartIndexFor(entry, day, now); idx >= 0 {
				tokens := int64(row.PromptTokens) + int64(row.CompletionTokens)
				entry.histTokens[idx] += tokens
				entry.histCost[idx] += row.Cost
				addFamily(entry.buckets[idx].ByModel, row.Model, tokens)
				if !entry.rollupTotals {
					// The rollup never carried totals for this chart, so the
					// history fallback owns them and moves with the delta.
					entry.buckets[idx].Tokens += tokens
					entry.buckets[idx].Cost += row.Cost
				}
			}
		}
		entry.watermark = watermark
	}

	if entry.rollupTotals {
		// Today's totals live in the rollup's today row, which the request
		// writer updates inline; re-read that one row so the last bucket tracks
		// the day as it grows. A missing row is a day with no rollup yet.
		todayKey := dayKeyFor(now)
		if rowJSON, err := repo.GetUsageDaily(todayKey); err == nil && rowJSON != "" {
			var dayData struct {
				PromptTokens     int64   `json:"promptTokens"`
				CompletionTokens int64   `json:"completionTokens"`
				Cost             float64 `json:"cost"`
			}
			if json.Unmarshal([]byte(rowJSON), &dayData) == nil {
				entry.buckets[entry.todayIdx].Tokens = dayData.PromptTokens + dayData.CompletionTokens
				entry.buckets[entry.todayIdx].Cost = dayData.Cost
			}
		}
	}

	return copyChartBuckets(entry.buckets)
}

// chartIndexFor maps a delta row's day onto its bucket, or -1 when the day is
// outside the cached window (a rebuilt entry would carry it).
func chartIndexFor(entry *chartEntry, day string, now time.Time) int {
	count := len(entry.buckets)
	if count == 0 {
		return -1
	}
	if day == dayKeyFor(now) {
		return entry.todayIdx
	}
	// The window ends today; walk the day offset backwards from it.
	dayTime, err := time.Parse("2006-01-02", day)
	if err != nil {
		return -1
	}
	today, _ := time.Parse("2006-01-02", dayKeyFor(now))
	offset := int(today.Sub(dayTime) / (24 * time.Hour))
	idx := count - 1 - offset
	if idx < 0 || idx >= count {
		return -1
	}
	// Confirm the cached bucket's own label matches: a bucket built for a
	// different day is not this row's bucket.
	if entry.buckets[idx].Label != dayTime.Format("Jan 2") {
		return -1
	}
	return idx
}

// rebuild builds the chart from scratch and installs it as the cache entry.
// The caller holds c.mu.
func (c *chartWindowCache) rebuild(repo *db.Repo, key string, now time.Time, kind string, count int) []chartBucket {
	build := buildDailyChartBuckets(repo, now, count, c.historyScan)
	if build == nil {
		delete(c.entries, key)
		return nil
	}
	watermark, err := repo.GetUsageHistoryWatermark()
	if err != nil {
		watermark = ""
	}
	sig := ""
	if n, newest, serr := repo.GetUsageDailySignature(); serr == nil {
		sig = dailyRollupSignature(n, newest)
	}
	entry := &chartEntry{
		buckets:      build.buckets,
		histTokens:   build.histTokens,
		histCost:     build.histCost,
		rollupTotals: build.rollupTotals,
		todayIdx:     len(build.buckets) - 1,
		watermark:    watermark,
		rollupSig:    sig,
		fetchedAt:    time.Now(),
	}
	c.entries[key] = entry
	return copyChartBuckets(entry.buckets)
}

// copyChartBuckets deep-copies a bucket list: the totals are plain fields, the
// per-family maps must not alias the cache.
func copyChartBuckets(buckets []chartBucket) []chartBucket {
	out := make([]chartBucket, len(buckets))
	for i, b := range buckets {
		out[i] = chartBucket{Label: b.Label, Tokens: b.Tokens, Cost: b.Cost, ByModel: make(map[string]int64, len(b.ByModel))}
		for fam, t := range b.ByModel {
			out[i].ByModel[fam] = t
		}
	}
	return out
}
