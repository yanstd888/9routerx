// 9RouterX — this file is part of a port maintained by https://github.com/yanstd888/ — https://t.me/aiclub_id
package providers_test

import (
	"slices"
	"testing"

	"9routerx/proxy/internal/providers"
)

// TestM5DispatchLane pins the disposition of each provider audited in Section 2.
// The broader catalog still contains other special lanes (web/cookie sessions,
// placeholder URLs) that are outside this task, so the check is scoped to the
// five ids named by the audit rather than pretending the whole catalog is clean.
func TestM5DispatchLane(t *testing.T) {
	t.Run("cursor stays out of the chat catalog", func(t *testing.T) {
		meta, ok := providers.ProviderRegistry["cursor"]
		if !ok {
			t.Fatal("cursor registry entry disappeared")
		}
		if !meta.Hidden {
			t.Error("cursor must stay hidden until its wire protocol is implemented")
		}
	})

	for _, id := range []string{"vertex", "vertex-partner"} {
		t.Run(id+" removed", func(t *testing.T) {
			if _, ok := providers.ProviderRegistry[id]; ok {
				t.Errorf("%s still advertised in the registry", id)
			}
			if _, ok := providers.KnownProviders[id]; ok {
				t.Errorf("%s still has a wire configuration", id)
			}
		})
	}

	t.Run("huggingface stays image and stt only", func(t *testing.T) {
		meta, ok := providers.ProviderRegistry["huggingface"]
		if !ok {
			t.Fatal("huggingface registry entry disappeared")
		}
		if slices.Contains(meta.ServiceKinds, "llm") {
			t.Error("huggingface must not advertise a chat lane it does not dispatch")
		}
	})

	t.Run("gemini-cli uses the native lane", func(t *testing.T) {
		cfg, ok := providers.KnownProviders["gemini-cli"]
		if !ok {
			t.Fatal("gemini-cli wire configuration disappeared")
		}
		if cfg.Format != "gemini-native" {
			t.Errorf("gemini-cli Format = %q, want gemini-native", cfg.Format)
		}
	})
}
