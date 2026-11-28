// 9RouterX — all source code port and modification by https://github.com/yanstd888/ — https://t.me/aiclub_id
package db

import (
	"context"
	"sync"
	"time"

	"9routerx/proxy/internal/analyticsrange"
)

// The compression analytics summary aggregates a dozen grouped statements (or
// a usageHistory backfill) over the window per request. The summary is cached
// per window shape with the usage-window-cache properties: a watermark
// signature over both source tables gates every poll, so a quiet ledger
// serves the cached summary and any write to either source rebuilds it.
// A Go-side delta fold was deliberately not attempted: the summary's buckets,
// truncations and top-N lists come from a dozen SQL statements whose exact
// semantics a second Go fold would have to mirror line for line.

const (
	// compressionCacheTTL bounds how long an untouched entry is served.
	compressionCacheTTL = 5 * time.Minute
	// compressionCacheMaxEntries bounds memory; a miss is the old cost.
	compressionCacheMaxEntries = 16
)

// compressionSummaryEntry is one cached summary.
type compressionSummaryEntry struct {
	summary   *CompressionAnalyticsSummary
	sig       string
	fetchedAt time.Time
}

// compressionSummaryCache caches one summary per window shape.
type compressionSummaryCache struct {
	mu      sync.Mutex
	entries map[string]*compressionSummaryEntry
}

var compressionSummaries compressionSummaryCache

// resetCompressionSummaries drops every cached summary. Tests use it to
// observe a cold cache; production never calls it.
func resetCompressionSummaries() {
	compressionSummaries.mu.Lock()
	compressionSummaries.entries = nil
	compressionSummaries.mu.Unlock()
}

// compressionSignature composes the watermark fingerprint the cache compares:
// the newest row in each source the summary can aggregate from. Either moving
// means the summary's inputs changed.
func (r *Repo) compressionSignature() (string, error) {
	caMax, err := r.getCompressionAnalyticsWatermark()
	if err != nil {
		return "", err
	}
	uhMax, err := r.GetUsageHistoryWatermark()
	if err != nil {
		return "", err
	}
	return caMax + "|" + uhMax, nil
}

// getCompressionAnalyticsWatermark returns the newest compressionAnalytics
// timestamp ("" when the table is empty).
func (r *Repo) getCompressionAnalyticsWatermark() (string, error) {
	var ts string
	err := r.db.QueryRow(`SELECT COALESCE(MAX(timestamp), '') FROM compressionAnalytics`).Scan(&ts) //nolint:noctx
	if err != nil {
		return "", err
	}
	return ts, nil
}

// GetCompressionAnalyticsSummaryCached serves the summary from the window
// cache. The uncached GetCompressionAnalyticsSummary stays for the tests that
// pin its statements.
func (r *Repo) GetCompressionAnalyticsSummaryCached(ctx context.Context, win analyticsrange.Window, rawPeriod string) (*CompressionAnalyticsSummary, error) {
	cutoff := ""
	if !win.Unbounded() {
		cutoff = win.Cutoff().Format(time.RFC3339)
	}

	compressionSummaries.mu.Lock()
	defer compressionSummaries.mu.Unlock()

	if compressionSummaries.entries == nil || len(compressionSummaries.entries) >= compressionCacheMaxEntries {
		compressionSummaries.entries = make(map[string]*compressionSummaryEntry, compressionCacheMaxEntries)
	}

	entry, ok := compressionSummaries.entries[cutoff]
	if !ok || time.Since(entry.fetchedAt) > compressionCacheTTL {
		summary, err := r.GetCompressionAnalyticsSummary(ctx, win)
		if err != nil {
			return nil, err
		}
		sig, serr := r.compressionSignature()
		if serr != nil {
			sig = ""
		}
		entry = &compressionSummaryEntry{summary: summary, sig: sig, fetchedAt: time.Now()}
		compressionSummaries.entries[cutoff] = entry
		return summary, nil
	}

	sig, err := r.compressionSignature()
	if err != nil || sig != entry.sig {
		summary, err := r.GetCompressionAnalyticsSummary(ctx, win)
		if err != nil {
			return nil, err
		}
		sig, serr := r.compressionSignature()
		if serr != nil {
			sig = ""
		}
		entry = &compressionSummaryEntry{summary: summary, sig: sig, fetchedAt: time.Now()}
		compressionSummaries.entries[cutoff] = entry
		return summary, nil
	}
	entry.fetchedAt = time.Now()
	return entry.summary, nil
}
