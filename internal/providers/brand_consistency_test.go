// 9RouterX — this file is part of a port maintained by https://github.com/yanstd888/ — https://t.me/aiclub_id
package providers

import (
	"strings"
	"testing"
)

// Brand consistency guard for the web-cookie providers (defbeae regression
// guard). These providers are onboarded through a pasted cookie and the
// brand the operator believes they are signing in to is the brand whose
// cookie they paste — a display name naming the wrong vendor (muse-web
// advertising "Baidu Muse" while the site is Baidu Yiyan) reads as a
// different product and hides what the credential actually is.
//
// The expectations pin one brand per web-cookie provider. A NEW web-cookie
// provider without a pin fails the first test, so the next wrong-brand
// regression is caught at the table instead of in production.

// webCookieBrandExpectations pins the brand every web-cookie provider's
// display name must carry (and the brand it must not carry, when a wrong
// one has already shipped once).
var webCookieBrandExpectations = map[string]struct {
	mustContain    string
	mustNotContain string
}{
	"muse-web":       {mustContain: "Yiyan", mustNotContain: "Muse"},
	"perplexity-web": {mustContain: "Perplexity"},
	"grok-web":       {mustContain: "Grok"},
	"gemini-web":     {mustContain: "Gemini"},
	"deepseek-web":   {mustContain: "DeepSeek"},
	"claude-web":     {mustContain: "Claude"},
	"copilot-web":    {mustContain: "Copilot"},
	"yuanbao-web":    {mustContain: "Yuanbao"},
	"zai-web":        {mustContain: "Z.ai"},
	"notion-web":     {mustContain: "Notion"},
	"arena-ai":       {mustContain: "Arena"},
	"uc-web":         {mustContain: "UC"},
}

// TestWebCookieProviders_KeepTheirBrand walks every web-cookie entry in the
// registry and checks its display name against the pinned brand.
func TestWebCookieProviders_KeepTheirBrand(t *testing.T) {
	for id, entry := range ProviderRegistry {
		if entry.Category != CategoryWebCookie {
			continue
		}
		pin, pinned := webCookieBrandExpectations[id]
		if !pinned {
			t.Errorf("web-cookie provider %q has no brand pin in webCookieBrandExpectations — add one so a wrong display name cannot ship", id)
			continue
		}
		if !strings.Contains(entry.Name, pin.mustContain) {
			t.Errorf("%s display name = %q, want it to carry %q", id, entry.Name, pin.mustContain)
		}
		if pin.mustNotContain != "" && strings.Contains(entry.Name, pin.mustNotContain) {
			t.Errorf("%s display name = %q, must not carry the wrong brand %q", id, entry.Name, pin.mustNotContain)
		}
	}
}

// TestWebCookieBrandPins_ReferToRealProviders keeps the pin table honest:
// an entry removed from the registry leaves a stale pin behind, and a pin
// naming a provider that is not web-cookie any more would never run again.
func TestWebCookieBrandPins_ReferToRealProviders(t *testing.T) {
	for id := range webCookieBrandExpectations {
		entry, ok := ProviderRegistry[id]
		if !ok {
			t.Errorf("brand pin for %q names a provider that no longer exists", id)
			continue
		}
		if entry.Category != CategoryWebCookie {
			t.Errorf("brand pin for %q targets a provider that is no longer web-cookie (category %v)", id, entry.Category)
		}
	}
}
