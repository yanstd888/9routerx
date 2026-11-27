// 9RouterX — source port and modifications by https://github.com/yanstd888/ — https://t.me/aiclub_id
package media

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"9routerx/proxy/internal/db"
	"9routerx/proxy/internal/handlers/chat"
	"9routerx/proxy/internal/providers"
	json "encoding/json/v2"
)

// newKeenableHandler builds a media handler whose outgoing calls all land on rt.
// With key == "" no connection is stored, which is the keyless case.
func newKeenableHandler(t *testing.T, rt http.RoundTripper, key string) *MediaHandler {
	t.Helper()
	database, cleanup := setupMultimodalTestDB(t)
	t.Cleanup(cleanup)
	repo := db.NewRepo(database)
	if key != "" {
		connData, _ := json.Marshal(map[string]any{"apiKey": key})
		if _, err := database.Exec(`INSERT INTO providerConnections (id, provider, authType, name, priority, isActive, data, createdAt, updatedAt) VALUES
			('conn-keenable-1', 'keenable', 'apikey', 'Keenable', 1, 1, ?, '2026-11-27T00:00:00Z', '2026-11-27T00:00:00Z')`, string(connData)); err != nil {
			t.Fatalf("seed connection: %v", err)
		}
	}
	chatH := chat.NewChatHandler(repo, nil)
	chatH.Client = &http.Client{Transport: rt}
	h := NewMediaHandler(repo, nil, chatH)
	h.Client = &http.Client{Transport: rt}
	return h
}

func runKeenable(t *testing.T, h *MediaHandler, endpoint, body string) (*httptest.ResponseRecorder, error) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, endpoint, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer gateway-client-key")
	rec := httptest.NewRecorder()
	err := h.handleKeenable(rec, req, []byte(body), endpoint, &chat.ModelInfo{Provider: "keenable", Model: "keenable"})
	return rec, err
}

func TestKeenableProviderConfig_Lanes(t *testing.T) {
	cfg, ok := providers.KnownProviders["keenable"]
	if !ok {
		t.Fatal("keenable missing from KnownProviders")
	}
	if cfg.SearchURL != "https://api.keenable.ai/v1/search" || cfg.FetchURL != "https://api.keenable.ai/v1/fetch" {
		t.Errorf("lanes = %q, %q", cfg.SearchURL, cfg.FetchURL)
	}
	if cfg.AuthHeader != "X-API-Key" || cfg.AuthScheme != "raw" {
		t.Errorf("auth = %s/%s, want X-API-Key/raw", cfg.AuthHeader, cfg.AuthScheme)
	}
	for _, u := range []string{cfg.SearchURL, cfg.FetchURL, keenablePublicURL(cfg.SearchURL), keenablePublicURL(cfg.FetchURL)} {
		if !keenableURLAllowed(u) {
			t.Errorf("%q should be allowed", u)
		}
	}
	if got := keenablePublicURL(cfg.SearchURL); got != "https://api.keenable.ai/v1/search/public" {
		t.Errorf("public search URL = %q", got)
	}
}

func TestKeenableURLAllowed_ExactHostOnly(t *testing.T) {
	denied := []string{
		"http://api.keenable.ai/v1/search",
		"https://api.keenable.ai.evil.example/v1/search",
		"https://evil.example/api.keenable.ai",
		"https://api.keenable.ai:8443/v1/search",
		"https://user:pw@api.keenable.ai/v1/search",
		"https://keenable.ai/v1/search",
		"",
	}
	for _, u := range denied {
		if keenableURLAllowed(u) {
			t.Errorf("%q should be refused", u)
		}
	}
}

// With no connection the search goes to the keyless path, names the app, and
// sends no key; the answer is mapped onto the gateway's search shape.
func TestKeenableSearch_KeylessMapsRequestAndResponse(t *testing.T) {
	long := strings.Repeat("word ", 300)
	rt := &aimlapiFakeTransport{answer: func(r *http.Request) *http.Response {
		return aimlapiJSONResponse(http.StatusOK, `{"results":[
			{"title":"Release History","url":"https://go.dev/doc/devel/release","description":"","snippet":"`+long+`"},
			{"title":"Second","url":"https://example.com/b","description":"fallback text","snippet":""}]}`)
	}}
	h := newKeenableHandler(t, rt, "")
	rec, err := runKeenable(t, h, "/v1/search", `{"query":"  golang   release ","max_results":80,"search_type":"web"}`)
	if err != nil {
		t.Fatalf("search error: %v", err)
	}
	if len(rt.seen) != 1 {
		t.Fatalf("upstream calls = %d, want 1", len(rt.seen))
	}
	got := rt.seen[0]
	if got.URL != "https://api.keenable.ai/v1/search/public" {
		t.Errorf("upstream URL = %q, want the keyless path", got.URL)
	}
	if v := got.Header.Get("X-Keenable-Title"); v != "9RouterX" {
		t.Errorf("X-Keenable-Title = %q", v)
	}
	if v := got.Header.Get("X-API-Key"); v != "" {
		t.Errorf("keyless call carries X-API-Key %q", v)
	}
	if got.Body["query"] != "golang release" || got.Body["max_results"] != float64(50) || len(got.Body) != 2 {
		t.Errorf("body = %v, want the normalised query and max_results capped at 50", got.Body)
	}

	var res map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if res["provider"] != "keenable" || res["query"] != "golang release" {
		t.Errorf("response = %v", res)
	}
	results, _ := res["results"].([]any)
	if len(results) != 2 {
		t.Fatalf("results = %v", res["results"])
	}
	first := results[0].(map[string]any)
	if first["url"] != "https://go.dev/doc/devel/release" || first["title"] != "Release History" || first["display_url"] != "go.dev/doc/devel/release" {
		t.Errorf("first = %v", first)
	}
	snippet, _ := first["snippet"].(string)
	if n := len([]rune(snippet)); n != keenableSnippetRunes+1 || !strings.HasSuffix(snippet, "…") {
		t.Errorf("snippet is %d runes, want a %d-rune lead", n, keenableSnippetRunes)
	}
	if content := first["content"].(map[string]any); content["text"] != long {
		t.Error("content.text should carry the full page text")
	}
	if second := results[1].(map[string]any); second["snippet"] != "fallback text" || second["position"] != float64(2) {
		t.Errorf("second = %v", second)
	}
}

// A stored connection's key moves the call to the keyed path.
func TestKeenableSearch_KeyedUsesConnectionKey(t *testing.T) {
	rt := &aimlapiFakeTransport{answer: func(r *http.Request) *http.Response {
		return aimlapiJSONResponse(http.StatusOK, `{"results":[]}`)
	}}
	h := newKeenableHandler(t, rt, "test-keenable-key")
	if _, err := runKeenable(t, h, "/v1/search", `{"query":"x"}`); err != nil {
		t.Fatalf("search error: %v", err)
	}
	if len(rt.seen) != 1 {
		t.Fatalf("upstream calls = %d, want 1", len(rt.seen))
	}
	got := rt.seen[0]
	if got.URL != "https://api.keenable.ai/v1/search" {
		t.Errorf("upstream URL = %q, want the keyed path", got.URL)
	}
	if v := got.Header.Get("X-API-Key"); v != "test-keenable-key" {
		t.Errorf("X-API-Key = %q", v)
	}
	if got.Body["max_results"] != float64(defaultSearchMaxResults) {
		t.Errorf("max_results = %v, want the default", got.Body["max_results"])
	}
}

func TestKeenableFetch_MapsRequestAndResponse(t *testing.T) {
	rt := &aimlapiFakeTransport{answer: func(r *http.Request) *http.Response {
		return aimlapiJSONResponse(http.StatusOK, `{"results":[{"url":"https://example.com/","title":"Example Domain","content":"# Example\n\nbody"}]}`)
	}}
	h := newKeenableHandler(t, rt, "")
	rec, err := runKeenable(t, h, "/v1/web/fetch", `{"url":" https://example.com ","format":"markdown","max_characters":2000}`)
	if err != nil {
		t.Fatalf("fetch error: %v", err)
	}
	if len(rt.seen) != 1 || rt.seen[0].URL != "https://api.keenable.ai/v1/fetch/public" {
		t.Fatalf("upstream = %+v", rt.seen)
	}
	urls, _ := rt.seen[0].Body["urls"].([]any)
	if len(urls) != 1 || urls[0] != "https://example.com" {
		t.Errorf("urls = %v", rt.seen[0].Body["urls"])
	}
	if c, _ := rt.seen[0].Body["content"].(map[string]any); c["max_chars"] != float64(2000) {
		t.Errorf("content = %v, want max_chars 2000", rt.seen[0].Body["content"])
	}
	var res map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &res)
	if res["content"] != "# Example\n\nbody" || res["title"] != "Example Domain" || res["provider"] != "keenable" || res["format"] != "markdown" {
		t.Errorf("fetch response = %v", res)
	}
}

// A page Keenable could not read comes back as an entry with an error, not as
// an HTTP error; the gateway must report it rather than an empty page.
func TestKeenableFetch_PageErrorIsReported(t *testing.T) {
	rt := &aimlapiFakeTransport{answer: func(r *http.Request) *http.Response {
		return aimlapiJSONResponse(http.StatusOK, `{"results":[{"url":"https://example.com/missing","error":{"status":404,"message":"page not found"}}]}`)
	}}
	h := newKeenableHandler(t, rt, "")
	_, err := runKeenable(t, h, "/v1/web/fetch", `{"url":"https://example.com/missing"}`)
	if err == nil || !strings.Contains(err.Error(), "page not found") {
		t.Fatalf("err = %v, want the page error", err)
	}
	if _, status := searchErrorParts(err); status != http.StatusNotFound {
		t.Errorf("status = %d, want 404", status)
	}
}

// The caller's own headers (its gateway key among them) must not reach Keenable.
func TestKeenable_DoesNotRelayCallerHeaders(t *testing.T) {
	rt := &aimlapiFakeTransport{answer: func(r *http.Request) *http.Response {
		return aimlapiJSONResponse(http.StatusOK, `{"results":[]}`)
	}}
	h := newKeenableHandler(t, rt, "test-keenable-key")
	if _, err := runKeenable(t, h, "/v1/search", `{"query":"x"}`); err != nil {
		t.Fatalf("search error: %v", err)
	}
	for k, v := range rt.seen[0].Header {
		for _, vv := range v {
			if strings.Contains(vv, "gateway-client-key") {
				t.Errorf("caller key leaked upstream in %s", k)
			}
		}
	}
}

// A redirect must not be followed: the key would be replayed to the host the
// Location names.
func TestKeenable_DoesNotFollowRedirect(t *testing.T) {
	rt := &aimlapiFakeTransport{answer: func(r *http.Request) *http.Response {
		resp := aimlapiJSONResponse(http.StatusFound, `{}`)
		resp.Header.Set("Location", "https://evil.example/v1/search")
		return resp
	}}
	h := newKeenableHandler(t, rt, "test-keenable-key")
	if _, err := runKeenable(t, h, "/v1/search", `{"query":"x"}`); err == nil {
		t.Fatal("expected an error for a redirect")
	}
	if len(rt.seen) != 1 || rt.seen[0].Host != "api.keenable.ai" {
		t.Errorf("upstream = %+v", rt.seen)
	}
}

// If the configured URL ever points elsewhere, nothing is sent.
func TestKeenable_RefusesForeignHost(t *testing.T) {
	orig := providers.KnownProviders["keenable"]
	t.Cleanup(func() { providers.KnownProviders["keenable"] = orig })
	changed := orig
	changed.SearchURL = "https://evil.example/v1/search"
	providers.KnownProviders["keenable"] = changed

	rt := &aimlapiFakeTransport{answer: func(r *http.Request) *http.Response {
		return aimlapiJSONResponse(http.StatusOK, `{"results":[]}`)
	}}
	h := newKeenableHandler(t, rt, "test-keenable-key")
	if _, err := runKeenable(t, h, "/v1/search", `{"query":"x"}`); err == nil {
		t.Fatal("expected refusal for a non-Keenable host")
	}
	if len(rt.seen) != 0 {
		t.Errorf("request sent to %q", rt.seen[0].Host)
	}
}

// A keyless rate limit keeps its status and message, so the caller can tell a
// 429 from a broken provider.
func TestKeenable_KeylessUpstreamErrorKeepsStatus(t *testing.T) {
	rt := &aimlapiFakeTransport{answer: func(r *http.Request) *http.Response {
		return aimlapiJSONResponse(http.StatusTooManyRequests, `{"detail":"Rate limit exceeded. Retry in 12 seconds."}`)
	}}
	h := newKeenableHandler(t, rt, "")
	_, err := runKeenable(t, h, "/v1/search", `{"query":"x"}`)
	if err == nil || !strings.Contains(err.Error(), "Rate limit exceeded") {
		t.Fatalf("err = %v, want the upstream message", err)
	}
	if _, status := searchErrorParts(err); status != http.StatusTooManyRequests {
		t.Errorf("status = %d, want 429", status)
	}
	if len(rt.seen) != 1 {
		t.Errorf("upstream calls = %d, want 1 (no rotation without a connection)", len(rt.seen))
	}
}

func TestKeenable_MissingQueryIsRefused(t *testing.T) {
	rt := &aimlapiFakeTransport{answer: func(r *http.Request) *http.Response {
		return aimlapiJSONResponse(http.StatusOK, `{"results":[]}`)
	}}
	h := newKeenableHandler(t, rt, "")
	if _, err := runKeenable(t, h, "/v1/search", `{"query":"   "}`); err == nil {
		t.Fatal("expected an error for an empty query")
	}
	if len(rt.seen) != 0 {
		t.Errorf("upstream calls = %d, want 0", len(rt.seen))
	}
}

func TestKeenable_RoutedFromSearchHandler(t *testing.T) {
	if !keenableEndpoint("/v1/search") || !keenableEndpoint("/v1/web/fetch") || keenableEndpoint("/v1/images/generations") {
		t.Error("keenableEndpoint lanes wrong")
	}
	if !isKeenableProvider("keenable") || isKeenableProvider("tavily") {
		t.Error("isKeenableProvider wrong")
	}
}

// HandleSearch with model "keenable/search" (the id /v1/models/web lists) and
// no connection must reach the keyless path instead of answering "no
// connection".
func TestHandleSearch_KeenableWorksWithoutConnection(t *testing.T) {
	rt := &aimlapiFakeTransport{answer: func(r *http.Request) *http.Response {
		return aimlapiJSONResponse(http.StatusOK, `{"results":[{"url":"https://a.example/","title":"A","snippet":"s"}]}`)
	}}
	h := newKeenableHandler(t, rt, "")
	body := `{"model":"keenable/search","query":"hello","max_results":3}`
	req := httptest.NewRequest(http.MethodPost, "/v1/search", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.HandleSearch(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if len(rt.seen) != 1 || rt.seen[0].URL != "https://api.keenable.ai/v1/search/public" || rt.seen[0].Body["max_results"] != float64(3) {
		t.Fatalf("upstream = %+v", rt.seen)
	}
	var res map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &res)
	if res["provider"] != "keenable" {
		t.Errorf("response = %v", res)
	}
}

// With a key stored, the bare provider id routes to the keyed path.
func TestHandleSearch_KeenableBareModelUsesStoredKey(t *testing.T) {
	rt := &aimlapiFakeTransport{answer: func(r *http.Request) *http.Response {
		return aimlapiJSONResponse(http.StatusOK, `{"results":[]}`)
	}}
	h := newKeenableHandler(t, rt, "test-keenable-key")
	body := `{"model":"keenable","query":"hello"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/search", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.HandleSearch(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if len(rt.seen) != 1 || rt.seen[0].URL != "https://api.keenable.ai/v1/search" || rt.seen[0].Header.Get("X-API-Key") != "test-keenable-key" {
		t.Fatalf("upstream = %+v", rt.seen)
	}
}

// HandleWebFetch with model "keenable/fetch" reaches the fetch lane the same way.
func TestHandleWebFetch_KeenableWorksWithoutConnection(t *testing.T) {
	rt := &aimlapiFakeTransport{answer: func(r *http.Request) *http.Response {
		return aimlapiJSONResponse(http.StatusOK, `{"results":[{"url":"https://example.com/","title":"Example Domain","content":"body"}]}`)
	}}
	h := newKeenableHandler(t, rt, "")
	body := `{"model":"keenable/fetch","url":"https://example.com"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/web/fetch", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.HandleWebFetch(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if len(rt.seen) != 1 || rt.seen[0].URL != "https://api.keenable.ai/v1/fetch/public" {
		t.Fatalf("upstream = %+v", rt.seen)
	}
}
