// 9RouterX — ported & modified by https://github.com/yanstd888/ — https://t.me/aiclub_id
package middleware

import (
	"context"
	"sync"
	"testing"
	"time"
)

// meteredUsage counts every underlying read, so the cache hit rate a test
// needs is the counter itself.
type meteredUsage struct {
	mu    sync.Mutex
	reads map[string]int
	value int
	err   error
}

func (c *meteredUsage) GetApiKeyTokenUsageByID(_ context.Context, keyID string) (int, error) {
	c.mu.Lock()
	c.reads[keyID]++
	reads := c.reads[keyID]
	value, err := c.value, c.err
	c.mu.Unlock()
	if err != nil {
		return 0, err
	}
	// The spend grows as the limiter admits traffic; the second read sees a
	// higher number, which a caching wrapper must NOT surface within its TTL.
	c.mu.Lock()
	c.value += 100
	c.mu.Unlock()
	return value + reads*100, nil
}

// TestCachedTokenUsageHitsWithinTTL pins R8: a key with an armed lifetime
// quota reads its whole-lifetime SUM per request; the wrapper answers
// repeated checks from the cache for the TTL and reads through after it
// expires.
func TestCachedTokenUsageHitsWithinTTL(t *testing.T) {
	inner := &meteredUsage{reads: map[string]int{}, value: 100}
	cached := NewCachedTokenUsageWithTTL(inner, 100*time.Millisecond)

	used, err := cached.GetApiKeyTokenUsageByID(context.Background(), "key-1")
	if err != nil {
		t.Fatalf("first read: %v", err)
	}
	if used != 200 {
		t.Fatalf("used = %d, want 200 (the first read passes through)", used)
	}

	// Within the TTL the cached number is served and the inner read is not
	// repeated: a second read inside the TTL must return the SAME value, not
	// the grown one.
	used2, err := cached.GetApiKeyTokenUsageByID(context.Background(), "key-1")
	if err != nil {
		t.Fatalf("second read: %v", err)
	}
	if used2 != used {
		t.Errorf("second read = %d, want the cached %d", used2, used)
	}
	inner.mu.Lock()
	if inner.reads["key-1"] != 1 {
		t.Errorf("inner reads = %d, want 1 within the TTL", inner.reads["key-1"])
	}
	inner.mu.Unlock()

	// After the TTL expires the read goes through again.
	time.Sleep(150 * time.Millisecond)
	used3, err := cached.GetApiKeyTokenUsageByID(context.Background(), "key-1")
	if err != nil {
		t.Fatalf("post-TTL read: %v", err)
	}
	if used3 == used {
		t.Error("post-TTL read returned the stale cached value")
	}
	inner.mu.Lock()
	if inner.reads["key-1"] != 2 {
		t.Errorf("inner reads = %d, want 2 after the TTL", inner.reads["key-1"])
	}
	inner.mu.Unlock()

	// Different keys do not share an entry.
	inner2 := &meteredUsage{reads: map[string]int{}, value: 7}
	cached2 := NewCachedTokenUsageWithTTL(inner2, time.Second)
	if got, _ := cached2.GetApiKeyTokenUsageByID(context.Background(), "key-2"); got != 107 {
		t.Errorf("second key = %d, want 107", got)
	}
}

// TestCachedTokenUsageErrorNotCached pins the fail-open path: a read failure
// is returned, not remembered, so a transient storage blip does not pin a
// zero spend for the whole TTL.
func TestCachedTokenUsageErrorNotCached(t *testing.T) {
	inner := &meteredUsage{reads: map[string]int{}, err: context.DeadlineExceeded}
	cached := NewCachedTokenUsageWithTTL(inner, time.Second)

	if _, err := cached.GetApiKeyTokenUsageByID(context.Background(), "key-1"); err == nil {
		t.Fatal("expected the error to pass through")
	}
	if _, err := cached.GetApiKeyTokenUsageByID(context.Background(), "key-1"); err == nil {
		t.Fatal("expected the error to pass through again (uncached)")
	}
	inner.mu.Lock()
	if inner.reads["key-1"] != 2 {
		t.Errorf("inner reads = %d, want 2 (errors must not be cached)", inner.reads["key-1"])
	}
	inner.mu.Unlock()
}
