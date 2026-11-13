// 9RouterX — this file is part of a port maintained by https://github.com/yanstd888/ — https://t.me/aiclub_id
package chat

import (
	"testing"
	"time"

	"9routerx/proxy/internal/db"
	"9routerx/proxy/internal/models"
)

// stickyConn builds a connection with LastUsedAt relative to now and a known
// consecutive-use count, so a test can place a candidate inside or outside the
// TTL window.
func stickyConn(id, provider string, lastUsedAgo time.Duration, consecutive int) *models.ProviderConnection {
	stamp := time.Now().UTC().Add(-lastUsedAgo).Format(time.RFC3339)
	count := consecutive
	return &models.ProviderConnection{
		ID:                  id,
		Provider:            provider,
		IsActive:            1,
		LastUsedAt:          &stamp,
		ConsecutiveUseCount: &count,
	}
}

// runSticky drives selectByRecency with the given mode/TTL and returns the
// winner connection plus its post-call consecutive count.
func runSticky(conns []*models.ProviderConnection, stickyLimit int, ttlSeconds int) (*models.ProviderConnection, int) {
	h := &ChatHandler{Repo: nil}
	out := h.selectByRecency(conns, stickyLimit, ttlSeconds)
	count := 0
	if out[0].ConsecutiveUseCount != nil {
		count = *out[0].ConsecutiveUseCount
	}
	return out[0], count
}

// TestStickyCountModeDefaultUnchanged pins the regression-safe property: when
// StickyMode is empty (the default), the count-only sticky behaves exactly as
// before — the same account is served until the count limit is reached, even
// if its last-used stamp is old enough to be outside any TTL window.
func TestStickyCountModeDefaultUnchanged(t *testing.T) {
	// 10 minutes old: far outside a hypothetical 5-minute TTL window, but
	// count mode must not care.
	conns := []*models.ProviderConnection{
		stickyConn("a", "deepseek", 5*time.Minute, 2),
		stickyConn("b", "deepseek", 10*time.Minute, 0),
	}
	winner, count := runSticky(conns, 3, 0)
	if winner.ID != "a" {
		t.Errorf("count mode winner = %s, want a (sticky regardless of age)", winner.ID)
	}
	if count != 3 {
		t.Errorf("count mode consecutive = %d, want 3", count)
	}
}

// TestStickyTTLWithinWindowKeepsAccount proves that TTL mode keeps serving the
// same account while its last-used stamp is younger than the TTL.
func TestStickyTTLWithinWindowKeepsAccount(t *testing.T) {
	conns := []*models.ProviderConnection{
		stickyConn("a", "deepseek", 3*time.Minute, 1),
		stickyConn("b", "deepseek", 5*time.Minute, 0),
	}
	winner, count := runSticky(conns, 3, 300) // 5-minute TTL
	if winner.ID != "a" {
		t.Errorf("TTL mode winner = %s, want a (3min < 5min TTL)", winner.ID)
	}
	if count != 2 {
		t.Errorf("TTL mode consecutive = %d, want 2", count)
	}
}

// TestStickyTTLExpiredFallsBackToLRU proves that an expired TTL stamp drops
// the account from the sticky window and falls through to the LRU path, which
// picks the least-recently-used candidate.
func TestStickyTTLExpiredFallsBackToLRU(t *testing.T) {
	conns := []*models.ProviderConnection{
		stickyConn("a", "deepseek", 6*time.Minute, 2),
		stickyConn("b", "deepseek", 10*time.Minute, 0),
	}
	winner, count := runSticky(conns, 3, 300) // 5-minute TTL, stamp is 6min old
	if winner.ID != "b" {
		t.Errorf("expired TTL winner = %s, want b (LRU: b is older)", winner.ID)
	}
	if count != 1 {
		t.Errorf("LRU consecutive = %d, want 1 (rotation happened)", count)
	}
}

// TestStickyTTLZeroPreservesLegacy proves that a TTL of 0 (or an absent TTL)
// preserves the legacy count-only behaviour even when the stamp is old.
func TestStickyTTLZeroPreservesLegacy(t *testing.T) {
	conns := []*models.ProviderConnection{
		stickyConn("a", "deepseek", 6*time.Minute, 2),
		stickyConn("b", "deepseek", 10*time.Minute, 0),
	}
	winner, count := runSticky(conns, 3, 0)
	if winner.ID != "a" {
		t.Errorf("TTL=0 winner = %s, want a (count mode, TTL disabled)", winner.ID)
	}
	if count != 3 {
		t.Errorf("TTL=0 consecutive = %d, want 3", count)
	}
}

// TestStickyTTLExpiredResetsConsecutiveCount proves that a rotation triggered
// by TTL expiry resets the winner's consecutive-use count to 1.
func TestStickyTTLExpiredResetsConsecutiveCount(t *testing.T) {
	conns := []*models.ProviderConnection{
		stickyConn("a", "deepseek", 6*time.Minute, 4),
		stickyConn("b", "deepseek", 10*time.Minute, 0),
	}
	_, count := runSticky(conns, 5, 300) // TTL expired for a
	if count != 1 {
		t.Errorf("post-rotation consecutive = %d, want 1 (TTL expiry resets count)", count)
	}
}

// TestStickyTTLParseRotationStampIntegration verifies that the TTL window
// reads the stamp written by TouchConnectionRotation (the rotation-stamp
// format, not just RFC3339), via the exported ParseRotationStamp helper.
func TestStickyTTLParseRotationStampIntegration(t *testing.T) {
	// Write a stamp in the rotation format (not RFC3339).
	highest := ""
	stamp := db.NextRotationStamp(highest)
	parsed, ok := db.ParseRotationStamp(stamp)
	if !ok {
		t.Fatalf("rotation stamp %q did not parse", stamp)
	}
	if time.Since(parsed) > time.Minute {
		t.Errorf("fresh stamp parsed to %v, want within a minute", parsed)
	}
}
