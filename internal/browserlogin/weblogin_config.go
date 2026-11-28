// 9RouterX — this file is part of a port maintained by https://github.com/yanstd888/ — https://t.me/aiclub_id
package browserlogin

// Generic per-provider web-login configuration (weblogin_config.go): one
// struct per provider describing the page to open, the cookie domain the
// session lives on, and the token sources the flow captures (cookies via
// CDP Network.getAllCookies, localStorage via Runtime.evaluate). The gemini
// and deepseek flows keep their dedicated implementations — their configs
// here are reference entries so the catalog can name every provider the
// gateway knows how to onboard.

import "sort"

// TokenSourceKind distinguishes where a credential lives on the page.
type TokenSourceKind string

const (
	TokenSourceCookie       TokenSourceKind = "cookie"
	TokenSourceLocalStorage TokenSourceKind = "localStorage"
)

// TokenSource is one credential the flow captures from the signed-in page.
type TokenSource struct {
	Kind TokenSourceKind
	Name string // cookie name or localStorage key
	// Domain narrows a cookie match to the cookie's own domain (a suffix
	// match on the normalized host; never a substring match).
	Domain string
}

// WebLoginConfig describes one provider's web sign-in.
type WebLoginConfig struct {
	ProviderID string
	LoginURL   string
	// CookieDomain is the domain the captured session belongs to.
	CookieDomain string
	// TokenSources lists what must exist before the flow reports success.
	TokenSources []TokenSource
	// PersistentProfile keeps the throwaway browser's profile on disk under
	// config.ResolveDataDir()/browser-profiles/<provider>/ so a re-login is
	// fast and edge cookies (cf_clearance) stay warm. Gemini deliberately
	// keeps a throwaway profile: Google rotates __Secure-1PSIDTS while the
	// browser is open, and a lingering profile cancels the router's session.
	PersistentProfile bool
}

// webLoginConfigs is the registry every generic flow reads.
var webLoginConfigs = map[string]WebLoginConfig{
	"perplexity-web": {
		ProviderID:   "perplexity-web",
		LoginURL:     "https://www.perplexity.ai/",
		CookieDomain: "perplexity.ai",
		TokenSources: []TokenSource{
			{Kind: TokenSourceCookie, Name: "__Secure-next-auth.session-token", Domain: ".perplexity.ai"},
		},
		PersistentProfile: true,
	},
	"grok-web": {
		ProviderID:   "grok-web",
		LoginURL:     "https://grok.com/",
		CookieDomain: "grok.com",
		TokenSources: []TokenSource{
			// sso is the session; sso-rw, cf_clearance and __cf_bm ride the
			// same Cookie header when present.
			{Kind: TokenSourceCookie, Name: "sso", Domain: ".grok.com"},
		},
		PersistentProfile: true,
	},
	"chatgpt-web": {
		ProviderID:   "chatgpt-web",
		LoginURL:     "https://chatgpt.com/",
		CookieDomain: "chatgpt.com",
		TokenSources: []TokenSource{
			{Kind: TokenSourceCookie, Name: "__Secure-next-auth.session-token", Domain: ".chatgpt.com"},
		},
		PersistentProfile: true,
	},
	"muse-web": {
		// 9RouterX's muse-web is Baidu Yiyan (yiyan.baidu.com, BDUSS) — not
		// muse.ai, and not the upstream's Meta AI muse entry.
		ProviderID:   "muse-web",
		LoginURL:     "https://yiyan.baidu.com/",
		CookieDomain: "baidu.com",
		TokenSources: []TokenSource{
			{Kind: TokenSourceCookie, Name: "BDUSS", Domain: ".baidu.com"},
		},
		PersistentProfile: true,
	},
	// Reference entries: these two keep their dedicated flows.
	"gemini-web": {
		ProviderID:   "gemini-web",
		LoginURL:     "https://gemini.google.com/app",
		CookieDomain: "google.com",
		TokenSources: []TokenSource{
			{Kind: TokenSourceCookie, Name: "__Secure-1PSID", Domain: ".google.com"},
			{Kind: TokenSourceCookie, Name: "__Secure-1PSIDTS", Domain: ".google.com"},
		},
		PersistentProfile: false,
	},
	"deepseek-web": {
		ProviderID:   "deepseek-web",
		LoginURL:     "https://chat.deepseek.com/sign_in",
		CookieDomain: "deepseek.com",
		TokenSources: []TokenSource{
			// userToken lives in localStorage as {"value": "..."} (or a raw
			// string on some builds); the deepseek flow validates the shape.
			{Kind: TokenSourceLocalStorage, Name: "userToken"},
		},
		PersistentProfile: true,
	},
	"copilot-web": {
		// The Copilot session rides the _U cookie (OmniRoute capture metadata:
		// credentialName "p-b"-style single cookie, acceptsFullCookieHeader).
		ProviderID:   "copilot-web",
		LoginURL:     "https://copilot.microsoft.com/",
		CookieDomain: "copilot.microsoft.com",
		TokenSources: []TokenSource{
			{Kind: TokenSourceCookie, Name: "_U", Domain: "copilot.microsoft.com"},
		},
		PersistentProfile: true,
	},
	"yuanbao-web": {
		// OmniRoute capture metadata: the Yuanbao session is the full Cookie
		// header, of which hy_user + hy_token are the load-bearing pair.
		ProviderID:   "yuanbao-web",
		LoginURL:     "https://yuanbao.tencent.com/",
		CookieDomain: "yuanbao.tencent.com",
		TokenSources: []TokenSource{
			{Kind: TokenSourceCookie, Name: "hy_token", Domain: "yuanbao.tencent.com"},
			{Kind: TokenSourceCookie, Name: "hy_user", Domain: "yuanbao.tencent.com"},
		},
		PersistentProfile: true,
	},
	"zai-web": {
		// OmniRoute + xrouter capture metadata: the Z.ai credential is the
		// "token" value in the site's Local Storage, not a cookie.
		ProviderID:   "zai-web",
		LoginURL:     "https://chat.z.ai/",
		CookieDomain: "chat.z.ai",
		TokenSources: []TokenSource{
			{Kind: TokenSourceLocalStorage, Name: "token"},
		},
		PersistentProfile: true,
	},
	"notion-web": {
		// OmniRoute capture metadata: the Notion session rides token_v2
		// (space_id / notion_browser_id ride the same Cookie header).
		ProviderID:   "notion-web",
		LoginURL:     "https://www.notion.so/",
		CookieDomain: "notion.so",
		TokenSources: []TokenSource{
			{Kind: TokenSourceCookie, Name: "token_v2", Domain: ".notion.so"},
		},
		PersistentProfile: true,
	},
	"arena-ai": {
		// xrouter authHint: the arena.ai credential is the user token /
		// evaluation_session cookie.
		ProviderID:   "arena-ai",
		LoginURL:     "https://arena.ai/",
		CookieDomain: "arena.ai",
		TokenSources: []TokenSource{
			{Kind: TokenSourceCookie, Name: "evaluation_session", Domain: "arena.ai"},
		},
		PersistentProfile: true,
	},
	"uc-web": {
		// The UC (uncensored.com) session mints per-request Clerk sessions
		// from __client (see providers.go); the durable sign-in cookie is
		// what the capture needs.
		ProviderID:   "uc-web",
		LoginURL:     "https://uncensored.com/",
		CookieDomain: "uncensored.com",
		TokenSources: []TokenSource{
			{Kind: TokenSourceCookie, Name: "__client", Domain: "uncensored.com"},
		},
		PersistentProfile: true,
	},
}

// ConfigFor returns the provider's web-login config and whether one exists.
func ConfigFor(providerID string) (WebLoginConfig, bool) {
	cfg, ok := webLoginConfigs[providerID]
	return cfg, ok
}

// HasWebLoginConfig reports whether the provider can run a generic flow.
func HasWebLoginConfig(providerID string) bool {
	_, ok := webLoginConfigs[providerID]
	return ok
}

// WebLoginConfigIDs lists the configured providers, sorted, for the catalog.
func WebLoginConfigIDs() []string {
	ids := make([]string, 0, len(webLoginConfigs))
	for id := range webLoginConfigs {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}
