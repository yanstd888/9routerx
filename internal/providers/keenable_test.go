// 9RouterX — source port and modifications by https://github.com/yanstd888/ — https://t.me/aiclub_id
package providers

import (
	"slices"
	"testing"
)

func TestKeenableProvider_Registered(t *testing.T) {
	cfg, ok := KnownProviders["keenable"]
	if !ok {
		t.Fatal("keenable missing from KnownProviders")
	}
	if cfg.SearchURL != "https://api.keenable.ai/v1/search" || cfg.SearchMethod != "POST" {
		t.Errorf("search lane = %s %q", cfg.SearchMethod, cfg.SearchURL)
	}
	if cfg.FetchURL != "https://api.keenable.ai/v1/fetch" || cfg.FetchMethod != "POST" {
		t.Errorf("fetch lane = %s %q", cfg.FetchMethod, cfg.FetchURL)
	}
	if cfg.AuthHeader != "X-API-Key" || cfg.AuthScheme != "raw" {
		t.Errorf("auth = %s/%s, want X-API-Key/raw", cfg.AuthHeader, cfg.AuthScheme)
	}

	meta, ok := ProviderRegistry["keenable"]
	if !ok {
		t.Fatal("keenable missing from ProviderRegistry")
	}
	if meta.Name != "Keenable" || meta.Category != CategoryAPIKey || meta.AuthType != "apikey" {
		t.Errorf("meta = %+v", meta)
	}
	if !slices.Equal(meta.ServiceKinds, []string{"webSearch", "webFetch"}) {
		t.Errorf("ServiceKinds = %v, want webSearch and webFetch only", meta.ServiceKinds)
	}
	// The key is optional, so the Connections card must stay: an explicit
	// noAuth flag would hide the place to add one.
	if meta.NoAuth {
		t.Error("keenable must not be noAuth; the dashboard would hide the optional key")
	}
	if meta.Notice == nil || meta.Notice.APIKeyURL != "https://keenable.ai/console" {
		t.Errorf("Notice = %+v", meta.Notice)
	}
}
