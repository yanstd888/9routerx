// 9RouterX — this file is part of a port maintained by https://github.com/yanstd888/ — https://t.me/aiclub_id
package providers

import "testing"

func TestProviderModelsNoDuplicateIDs(t *testing.T) {
	for provider, models := range ProviderModels {
		seen := make(map[string]bool, len(models))
		for _, id := range models {
			if seen[id] {
				t.Errorf("provider %q lists model %q more than once", provider, id)
			}
			seen[id] = true
		}
	}
}
