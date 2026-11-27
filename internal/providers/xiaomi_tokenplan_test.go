package providers

import "testing"

// Xiaomi Token Plan is served from three regional origins, but the registry
// names only the Singapore one. A connection with region=cn used to be routed
// to token-plan-sgp, which answers 401 "Invalid API Key" for a perfectly valid
// key — the dashboard probe used the CN host and chat did not.
//
// These cases pin the resolution so the two lanes cannot drift apart again.

func TestXiaomiTokenplanBaseURL(t *testing.T) {
	tests := []struct {
		name   string
		region string
		want   string
	}{
		{name: "cn", region: "cn", want: "https://token-plan-cn.xiaomimimo.com/v1"},
		{name: "sgp", region: "sgp", want: "https://token-plan-sgp.xiaomimimo.com/v1"},
		{name: "ams", region: "ams", want: "https://token-plan-ams.xiaomimimo.com/v1"},
		{name: "uppercase is normalised", region: "CN", want: "https://token-plan-cn.xiaomimimo.com/v1"},
		{name: "surrounding space is trimmed", region: " cn ", want: "https://token-plan-cn.xiaomimimo.com/v1"},
		{name: "unknown region falls back to the default", region: "mars", want: "https://token-plan-sgp.xiaomimimo.com/v1"},
		{name: "empty region falls back to the default", region: "", want: "https://token-plan-sgp.xiaomimimo.com/v1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := XiaomiTokenplanBaseURL(tt.region); got != tt.want {
				t.Errorf("XiaomiTokenplanBaseURL(%q) = %q, want %q", tt.region, got, tt.want)
			}
		})
	}
}

// The chat route must be the base root plus the completions path, never a
// second /v1 — the duplicated-version 404 is exactly the failure this helper
// exists to remove.
func TestXiaomiTokenplanChatURL(t *testing.T) {
	got := XiaomiTokenplanChatURL("cn")
	want := "https://token-plan-cn.xiaomimimo.com/v1/chat/completions"
	if got != want {
		t.Fatalf("XiaomiTokenplanChatURL(\"cn\") = %q, want %q", got, want)
	}
}

// The registry entry is what an unconfigured connection dials. It must stay in
// step with the default region, or a connection with no region would resolve
// one way through the helper and another through the registry.
func TestXiaomiTokenplanRegistryMatchesDefaultRegion(t *testing.T) {
	cfg, ok := KnownProviders["xiaomi-tokenplan"]
	if !ok {
		t.Fatal("xiaomi-tokenplan is missing from KnownProviders")
	}
	want := XiaomiTokenplanChatURL(XiaomiTokenplanDefaultRegion)
	if cfg.BaseURL != want {
		t.Errorf("registry BaseURL = %q, want %q (the default region's chat URL)", cfg.BaseURL, want)
	}
}

// The dashboard probe and the chat lane must read one table. A second copy is
// how they drifted before: a region the probe accepted and then chat routed
// elsewhere.
func TestXiaomiTokenplanRegionOf(t *testing.T) {
	tests := []struct {
		name string
		psd  map[string]any
		want string
	}{
		{name: "nil map", psd: nil, want: ""},
		{name: "absent region", psd: map[string]any{}, want: ""},
		{name: "cn", psd: map[string]any{"region": "cn"}, want: "cn"},
		{name: "non-string region is ignored", psd: map[string]any{"region": 42}, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := XiaomiTokenplanRegionOf(tt.psd); got != tt.want {
				t.Errorf("XiaomiTokenplanRegionOf(%v) = %q, want %q", tt.psd, got, tt.want)
			}
		})
	}
}
