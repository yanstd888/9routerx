// 9RouterX — derived from the work of https://github.com/yanstd888/ — port and modifications by the maintainers — https://t.me/aiclub_id
package providers

import (
	"testing"
)

func TestMimocodeProviderCatalogAndAliases(t *testing.T) {
	// 1. Alias resolution
	for _, alias := range []string{"mimocode", "mimocode-free", "mmf", "mimo-free"} {
		canonical := ResolveAlias(alias)
		if canonical != "mimo-free" {
			t.Errorf("ResolveAlias(%q) = %q, want 'mimo-free'", alias, canonical)
		}
	}

	// 2. The shadowed registry row is gone; the compatibility alias remains.
	if alias, ok := RegistryAliases["mimocode"]; ok {
		t.Errorf("RegistryAliases['mimocode'] = %q, want no entry", alias)
	}

	// 3. Wire configuration lives only on the canonical provider.
	if _, ok := KnownProviders["mimocode"]; ok {
		t.Error("KnownProviders still holds the shadowed mimocode row")
	}
	cfg, ok := KnownProviders["mimo-free"]
	if !ok {
		t.Fatal("KnownProviders missing 'mimo-free'")
	}
	if cfg.BaseURL == "" || !cfg.NoAuth {
		t.Errorf("unexpected mimo-free config: %+v", cfg)
	}

	// 4. Provider models
	models := GetProviderModels("mimo-free")
	if len(models) == 0 {
		t.Errorf("expected models for mimo-free, got none")
	}
}
