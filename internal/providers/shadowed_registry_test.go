// 9RouterX — this file is part of a port maintained by https://github.com/yanstd888/ — https://t.me/aiclub_id
package providers

import "testing"

// TestNoShadowedRegistryEntries pins the rule that a registered provider with
// wire configuration stays reachable through its own id. A compatibility alias
// may point at it; the provider itself must not point away from itself.
func TestNoShadowedRegistryEntries(t *testing.T) {
	for id := range ProviderRegistry {
		if _, dispatchable := KnownProviders[id]; !dispatchable {
			// Catalog-only metadata rows have no wire config to shadow.
			continue
		}
		if got := ResolveAlias(id); got != id {
			t.Errorf("registry entry %q resolves to %q", id, got)
		}
	}
}
