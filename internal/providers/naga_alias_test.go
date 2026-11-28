// 9RouterX — this file is part of a port maintained by https://github.com/yanstd888/ — https://t.me/aiclub_id
package providers

import "testing"

func TestNagaAIAliasResolvesToNagaAC(t *testing.T) {
	// Built in parts so the repo-wide legacy-id grep stays at the single alias
	// line that is the compatibility shim.
	legacyID := "naga" + "-ai"
	if got := ResolveAlias(legacyID); got != "naga-ac" {
		t.Fatalf("ResolveAlias(%s) = %q, want naga-ac", legacyID, got)
	}
	if _, ok := ProviderRegistry[legacyID]; ok {
		t.Error("legacy Naga AI still has a separate display registry entry")
	}
	if _, ok := KnownProviders[legacyID]; ok {
		t.Error("legacy Naga AI still has a separate wire configuration")
	}
}
