// 9RouterX — all source code port and modification by https://github.com/yanstd888/ — https://t.me/aiclub_id
package db

import (
	"sort"
	"sync"
	"time"

	"9routerx/proxy/internal/analyticsrange"
)

// The cache analytics read usageHistory with per-row JSON extraction on every
// request — the prompt-cache cards and the hourly trend re-scanned the whole
// window's rows per poll. This cache mirrors the usage window cache's
// mechanics for their aggregates: built once per window shape, advanced by
// delta from the usageHistory watermark, the rows that age out of a moved
// sliding cutoff subtracted (every counter is an integer, so the subtraction
// is exact), and a watermark that moves backwards rebuilt.
//
// The cached state is the UNTRUNCATED fold — every provider, model and pair —
// because a delta row can introduce a group the top-N cut had dropped; the
// truncation and the savings pricing are derived per response, exactly as the
// uncached path derives them from its own full fold.

const (
	// promptCacheTTL bounds how long an untouched entry is served, matching the
	// usage window cache's bound.
	promptCacheTTL = 5 * time.Minute
	// promptCacheMaxEntries bounds memory; a miss is merely the old cost.
	promptCacheMaxEntries = 16
)

// promptCacheTrendBucket accumulates one hour of the trend. The hour key is
// the strftime bucket the trend query emits ("2006-01-02T15:00:00Z", UTC).
type promptCacheTrendBucket struct {
	requests            int64
	cachedRequests      int64
	inputTokens         int64
	cachedTokens        int64
	cacheCreationTokens int64
}

// promptCacheEntry is one cached window's analytics state.
type promptCacheEntry struct {
	totals     cacheBucket
	byProvider map[string]*cacheBucket
	byModel    map[string]*cacheBucket
	byPair     map[[2]string]*cacheBucket
	trend      map[string]*promptCacheTrendBucket

	watermark string
	// cutoff is the window's lower bound as the WHERE clause sees it (RFC3339),
	// or "" for the unbounded window.
	cutoff    string
	fetchedAt time.Time
}

// promptCacheWindowCache caches one analytics state per window shape.
type promptCacheWindowCache struct {
	mu      sync.Mutex
	entries map[string]*promptCacheEntry
}

var promptCacheWindows promptCacheWindowCache

// resetPromptCacheWindows drops every cached analytics window. Tests use it to
// observe a cold cache; production never calls it.
func resetPromptCacheWindows() {
	promptCacheWindows.mu.Lock()
	promptCacheWindows.entries = nil
	promptCacheWindows.mu.Unlock()
}

// subtractBucket removes one row's contribution from a bucket: the exact
// reverse of cacheBucket.add (integer counters, no drift).
func subtractBucket(b *cacheBucket, row CacheScanRow) {
	b.requests--
	b.inputTokens -= row.InputTokens
	b.cachedTokens -= row.CachedTokens
	b.creationTokens -= row.CreationTokens
	if row.CachedTokens > 0 || row.CreationTokens > 0 {
		b.cachedRequests--
	}
}

// bucketOf returns the bucket for key, creating it on first sight.
func bucketOf[K comparable](buckets map[K]*cacheBucket, key K) *cacheBucket {
	b, ok := buckets[key]
	if !ok {
		b = &cacheBucket{}
		buckets[key] = b
	}
	return b
}

// subtractRow removes one aged-out row's contribution from every grouping.
func (e *promptCacheEntry) subtractRow(row CacheScanRow) {
	subtractBucket(&e.totals, row)
	subtractBucket(bucketOf(e.byProvider, row.Provider), row)
	subtractBucket(bucketOf(e.byModel, row.Model), row)
	subtractBucket(bucketOf(e.byPair, [2]string{row.Provider, row.Model}), row)
}

// foldRow folds one resolved row into every grouping.
func (e *promptCacheEntry) foldRow(row CacheScanRow) {
	e.totals.requests++
	e.totals.inputTokens += row.InputTokens
	e.totals.cachedTokens += row.CachedTokens
	e.totals.creationTokens += row.CreationTokens
	if row.CachedTokens > 0 || row.CreationTokens > 0 {
		e.totals.cachedRequests++
	}
	accumulate(e.byProvider, row.Provider, row)
	accumulate(e.byModel, row.Model, row)
	accumulate(e.byPair, [2]string{row.Provider, row.Model}, row)
}

// foldIntoTrend folds one resolved row into its UTC hour bucket, creating the
// bucket on first sight. The cached/creation predicates are the same ones the
// trend query's CASE expressions encode.
func (e *promptCacheEntry) foldIntoTrend(row CacheScanRow) {
	if row.Timestamp == "" {
		return
	}
	b, ok := e.trend[row.Timestamp]
	if !ok {
		b = &promptCacheTrendBucket{}
		e.trend[row.Timestamp] = b
	}
	b.requests++
	b.inputTokens += row.InputTokens
	b.cachedTokens += row.CachedTokens
	b.cacheCreationTokens += row.CreationTokens
	if row.CachedTokens > 0 || row.CreationTokens > 0 {
		b.cachedRequests++
	}
}

// unfoldFromTrend removes one aged-out row's contribution from its hour
// bucket, dropping the bucket when it empties.
func (e *promptCacheEntry) unfoldFromTrend(row CacheScanRow) {
	b, ok := e.trend[row.Timestamp]
	if !ok {
		return
	}
	b.requests--
	b.inputTokens -= row.InputTokens
	b.cachedTokens -= row.CachedTokens
	b.cacheCreationTokens -= row.CreationTokens
	if row.CachedTokens > 0 || row.CreationTokens > 0 {
		b.cachedRequests--
	}
	if b.requests <= 0 {
		delete(e.trend, row.Timestamp)
	}
}

// metricsAssemble derives the response from the cached fold: the truncation
// and the savings pricing run per response exactly as the uncached path runs
// them over its own full fold.
func (e *promptCacheEntry) metricsAssemble(period string) *PromptCacheMetrics {
	metrics := &PromptCacheMetrics{
		TotalRequests:            e.totals.requests,
		RequestsWithCacheControl: e.totals.cachedRequests,
		TotalInputTokens:         e.totals.inputTokens,
		TotalCachedTokens:        e.totals.cachedTokens,
		TotalCacheCreationTokens: e.totals.creationTokens,
		ByProvider:               make(map[string]PromptCacheProviderStats),
		ByModel:                  make(map[string]PromptCacheModelStats),
		LastUpdated:              time.Now().UTC().Format(time.RFC3339),
		Period:                   period,
	}
	metrics.TokensSaved = metrics.TotalCachedTokens
	metrics.EstimatedCostSaved = estimateCacheSavings(e.byPair, metrics.TotalCachedTokens)
	metrics.ByProvider, metrics.TruncatedProviders = topProviderStats(e.byProvider, maxBreakdownGroups)
	metrics.ByModel, metrics.TruncatedModels = topModelStats(e.byModel, maxBreakdownGroups)
	return metrics
}

// trendAssemble derives the trend points from the cached hour buckets, in the
// ascending order the SQL query produced.
func (e *promptCacheEntry) trendAssemble() []CacheTrendPoint {
	points := make([]CacheTrendPoint, 0, len(e.trend))
	for key, b := range e.trend {
		points = append(points, CacheTrendPoint{
			Timestamp:           key,
			Requests:            b.requests,
			CachedRequests:      b.cachedRequests,
			InputTokens:         b.inputTokens,
			CachedTokens:        b.cachedTokens,
			CacheCreationTokens: b.cacheCreationTokens,
		})
	}
	sort.Slice(points, func(i, j int) bool { return points[i].Timestamp < points[j].Timestamp })
	return points
}

// rebuildPromptCacheEntry folds the whole window from scratch. The watermark
// is read before the rows so a row written during the fold is re-read by the
// next delta rather than skipped.
func rebuildPromptCacheEntry(r *Repo, cutoff string) (*promptCacheEntry, error) {
	watermark, err := r.GetUsageHistoryWatermark()
	if err != nil {
		return nil, err
	}
	var rows []CacheScanRow
	if cutoff == "" {
		rows, err = r.promptCacheRows("timestamp <= ?", watermark)
	} else {
		rows, err = r.promptCacheRows("timestamp <= ? AND timestamp >= ?", watermark, cutoff)
	}
	if err != nil {
		return nil, err
	}

	entry := &promptCacheEntry{
		byProvider: map[string]*cacheBucket{},
		byModel:    map[string]*cacheBucket{},
		byPair:     map[[2]string]*cacheBucket{},
		trend:      map[string]*promptCacheTrendBucket{},
		watermark:  watermark,
		cutoff:     cutoff,
		fetchedAt:  time.Now(),
	}
	if watermark != "" && cutoff != "" && watermark <= cutoff {
		// Nothing in this window has been written yet; the cutoff is the honest
		// watermark, so the first delta does not rescan the window.
		entry.watermark = cutoff
	}
	for _, row := range rows {
		entry.foldRow(row)
		entry.foldIntoTrend(row)
	}
	return entry, nil
}

// advance moves the cached state forward: the rows that aged out of a moved
// sliding cutoff are subtracted, the rows written since the last watermark
// fold in, and a backwards watermark rebuilds the window wholesale.
func (e *promptCacheEntry) advance(r *Repo, cutoff string) error {
	watermark, err := r.GetUsageHistoryWatermark()
	if err != nil {
		return err
	}

	if cutoff != e.cutoff && cutoff != "" && e.cutoff != "" {
		aged, err := r.GetPromptCacheRowsAgedOut(e.cutoff, cutoff)
		if err != nil {
			return err
		}
		for _, row := range aged {
			e.subtractRow(row)
			e.unfoldFromTrend(row)
		}
		// Hour buckets that slid out of the window entirely are dropped: their
		// hour is below the cutoff's hour.
		if parsedCutoff, perr := time.Parse(time.RFC3339, cutoff); perr == nil {
			floor := parsedCutoff.UTC().Format("2006-01-02T15") + ":00:00Z"
			for key := range e.trend {
				if key < floor {
					delete(e.trend, key)
				}
			}
		}
		e.cutoff = cutoff
	}

	if watermark != e.watermark {
		rows, err := r.GetPromptCacheRowsBetween(e.watermark, watermark)
		if err != nil {
			return err
		}
		for _, row := range rows {
			e.foldRow(row)
			e.foldIntoTrend(row)
		}
		e.watermark = watermark
	}
	return nil
}

// get returns the cached state for cutoff, building it on a miss and advancing
// it from a delta otherwise.
func (c *promptCacheWindowCache) get(r *Repo, cutoff string) (*promptCacheEntry, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.entries == nil || len(c.entries) >= promptCacheMaxEntries {
		c.entries = make(map[string]*promptCacheEntry, promptCacheMaxEntries)
	}

	entry, ok := c.entries[cutoff]
	if !ok || time.Since(entry.fetchedAt) > promptCacheTTL {
		built, err := rebuildPromptCacheEntry(r, cutoff)
		if err != nil {
			return nil, err
		}
		c.entries[cutoff] = built
		return built, nil
	}

	watermark, err := r.GetUsageHistoryWatermark()
	if err != nil {
		return nil, err
	}
	if watermark < entry.watermark {
		// The newest row moved backwards: the ledger was swapped, restored or
		// truncated, so the cached state may describe rows that no longer exist.
		built, err := rebuildPromptCacheEntry(r, cutoff)
		if err != nil {
			return nil, err
		}
		c.entries[cutoff] = built
		return built, nil
	}

	if err := entry.advance(r, cutoff); err != nil {
		return nil, err
	}
	entry.fetchedAt = time.Now()
	return entry, nil
}

// GetPromptCacheMetricsCached serves the prompt cache metrics from the window
// cache: the aggregate is built once per window shape and advanced by delta.
// The uncached GetPromptCacheMetrics stays for the tests that pin the fold.
func (r *Repo) GetPromptCacheMetricsCached(win analyticsrange.Window, period string) (*PromptCacheMetrics, error) {
	cutoff := ""
	if !win.Unbounded() {
		cutoff = win.Cutoff().Format(time.RFC3339)
	}
	entry, err := promptCacheWindows.get(r, cutoff)
	if err != nil {
		return nil, err
	}
	return entry.metricsAssemble(period), nil
}

// GetPromptCacheTrendCached serves the hourly trend from the same cached
// window the metrics use, so the cards and the chart cannot disagree. The
// unbounded window keeps the answer the uncached trend gives: an hour per
// hour since the ledger began cannot be read off a 170px chart.
func (r *Repo) GetPromptCacheTrendCached(win analyticsrange.Window) ([]CacheTrendPoint, error) {
	if win.Unbounded() {
		return []CacheTrendPoint{}, nil
	}
	cutoff := win.Cutoff().Format(time.RFC3339)
	entry, err := promptCacheWindows.get(r, cutoff)
	if err != nil {
		return nil, err
	}
	return entry.trendAssemble(), nil
}

