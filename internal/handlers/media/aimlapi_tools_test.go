// 9RouterX — source port and modifications by https://github.com/yanstd888/ — https://t.me/aiclub_id
package media

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"9routerx/proxy/internal/db"
	"9routerx/proxy/internal/handlers/chat"
	"9routerx/proxy/internal/providers"
	json "encoding/json/v2"
)

// aimlapiSeenRequest is what the fake transport saw for one outgoing request.
type aimlapiSeenRequest struct {
	URL    string
	Host   string
	Auth   string
	Header http.Header
	Body   map[string]any
}

// aimlapiFakeTransport answers every request in-process, so tests can check
// which host the key went to without leaving the machine.
type aimlapiFakeTransport struct {
	mu     sync.Mutex
	seen   []aimlapiSeenRequest
	answer func(r *http.Request) *http.Response
}

func (f *aimlapiFakeTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	var body map[string]any
	if r.Body != nil {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
	}
	f.mu.Lock()
	f.seen = append(f.seen, aimlapiSeenRequest{URL: r.URL.String(), Host: r.URL.Host, Auth: r.Header.Get("Authorization"), Header: r.Header.Clone(), Body: body})
	f.mu.Unlock()
	resp := f.answer(r)
	resp.Request = r
	return resp, nil
}

func aimlapiJSONResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Status:     http.StatusText(status),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func newAimlapiToolsHandler(t *testing.T, rt http.RoundTripper) *MediaHandler {
	t.Helper()
	database, cleanup := setupMultimodalTestDB(t)
	t.Cleanup(cleanup)
	repo := db.NewRepo(database)
	connData, _ := json.Marshal(map[string]any{"apiKey": "test-aimlapi-key"})
	if _, err := database.Exec(`INSERT INTO providerConnections (id, provider, authType, name, priority, isActive, data, createdAt, updatedAt) VALUES
		('conn-aiml-1', 'aimlapi', 'apikey', 'AI/ML API', 1, 1, ?, '2026-07-18T00:00:00Z', '2026-07-18T00:00:00Z')`, string(connData)); err != nil {
		t.Fatalf("seed connection: %v", err)
	}
	chatH := chat.NewChatHandler(repo, nil)
	chatH.Client = &http.Client{Transport: rt}
	return NewMediaHandler(repo, nil, chatH)
}

func runAimlapiTool(t *testing.T, h *MediaHandler, endpoint, body string) (*httptest.ResponseRecorder, error) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, endpoint, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer gateway-client-key")
	rec := httptest.NewRecorder()
	err := h.handleAimlapiTool(rec, req, []byte(body), endpoint, &chat.ModelInfo{Provider: "aimlapi", Model: "aimlapi"})
	return rec, err
}

func TestAimlapiProviderConfig_ToolboxLanes(t *testing.T) {
	cfg, ok := providers.KnownProviders["aimlapi"]
	if !ok {
		t.Fatal("aimlapi missing from KnownProviders")
	}
	if cfg.BaseURL != "https://api.aimlapi.com/v1/chat/completions" || cfg.AuthHeader != "Authorization" || cfg.AuthScheme != "bearer" {
		t.Errorf("chat lane = %q %s/%s", cfg.BaseURL, cfg.AuthHeader, cfg.AuthScheme)
	}
	for name, u := range map[string]string{"SearchURL": cfg.SearchURL, "FetchURL": cfg.FetchURL} {
		if !aimlapiToolsURLAllowed(u) {
			t.Errorf("%s = %q, want https://tools.aimlapi.com/...", name, u)
		}
	}
}

func TestAimlapiToolsURLAllowed_ExactHostOnly(t *testing.T) {
	allowed := []string{"https://tools.aimlapi.com/v1/tools/run", "https://TOOLS.aimlapi.com/v1/tools/run"}
	denied := []string{
		"http://tools.aimlapi.com/v1/tools/run",
		"https://tools.aimlapi.com.evil.example/v1/tools/run",
		"https://evil.example/tools.aimlapi.com",
		"https://tools.aimlapi.com:8443/v1/tools/run",
		"https://user:pw@tools.aimlapi.com/v1/tools/run",
		"https://api.aimlapi.com/v1/tools/run",
		"https://xtools.aimlapi.com/v1/tools/run",
		"",
	}
	for _, u := range allowed {
		if !aimlapiToolsURLAllowed(u) {
			t.Errorf("%q should be allowed", u)
		}
	}
	for _, u := range denied {
		if aimlapiToolsURLAllowed(u) {
			t.Errorf("%q should be refused", u)
		}
	}
}

func TestAimlapiSearch_MapsRequestAndResponse(t *testing.T) {
	rt := &aimlapiFakeTransport{answer: func(r *http.Request) *http.Response {
		return aimlapiJSONResponse(http.StatusOK, `{"status":"completed","is_error":false,
			"output":{"results":[{"url":"https://go.dev/doc/devel/release","title":"Release History","snippet":"Go release notes"}]},
			"cost":{"actual_usd":0.0091}}`)
	}}
	h := newAimlapiToolsHandler(t, rt)
	rec, err := runAimlapiTool(t, h, "/v1/search", `{"query":"golang release","max_results":50}`)
	if err != nil {
		t.Fatalf("search error: %v", err)
	}
	if len(rt.seen) != 1 {
		t.Fatalf("upstream calls = %d, want 1", len(rt.seen))
	}
	got := rt.seen[0]
	if got.URL != "https://tools.aimlapi.com/v1/tools/run" {
		t.Errorf("upstream URL = %q", got.URL)
	}
	if got.Auth != "Bearer test-aimlapi-key" {
		t.Errorf("Authorization = %q, want the connection key", got.Auth)
	}
	if got.Body["tool"] != "web/search" {
		t.Errorf("tool = %v", got.Body["tool"])
	}
	input, _ := got.Body["input"].(map[string]any)
	if input["query"] != "golang release" || input["max_results"] != float64(20) {
		t.Errorf("input = %v, want query and max_results capped at 20", input)
	}
	if len(input) != 2 {
		t.Errorf("input carries extra fields the Toolbox schema refuses: %v", input)
	}

	var res map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if res["provider"] != "aimlapi" {
		t.Errorf("provider = %v", res["provider"])
	}
	results, _ := res["results"].([]any)
	if len(results) != 1 {
		t.Fatalf("results = %v", res["results"])
	}
	first := results[0].(map[string]any)
	if first["url"] != "https://go.dev/doc/devel/release" || first["title"] != "Release History" || first["snippet"] != "Go release notes" {
		t.Errorf("result = %v", first)
	}
	if usage := res["usage"].(map[string]any); usage["search_cost_usd"] != 0.0091 {
		t.Errorf("search_cost_usd = %v", usage["search_cost_usd"])
	}
}

func TestAimlapiFetch_MapsRequestAndResponse(t *testing.T) {
	rt := &aimlapiFakeTransport{answer: func(r *http.Request) *http.Response {
		return aimlapiJSONResponse(http.StatusOK, `{"status":"completed","output":{"url":"https://example.com/","title":"Example Domain","content":"# Example\n\nbody"}}`)
	}}
	h := newAimlapiToolsHandler(t, rt)
	rec, err := runAimlapiTool(t, h, "/v1/web/fetch", `{"url":" https://example.com "}`)
	if err != nil {
		t.Fatalf("fetch error: %v", err)
	}
	if len(rt.seen) != 1 || rt.seen[0].Body["tool"] != "litescrape/web-fetch" {
		t.Fatalf("upstream = %+v", rt.seen)
	}
	if input, _ := rt.seen[0].Body["input"].(map[string]any); input["url"] != "https://example.com" || len(input) != 1 {
		t.Errorf("input = %v", input)
	}
	var res map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &res)
	if res["content"] != "# Example\n\nbody" || res["title"] != "Example Domain" || res["provider"] != "aimlapi" {
		t.Errorf("fetch response = %v", res)
	}
}

// The caller's own headers (its gateway key among them) must not reach the
// Toolbox; only the connection key does.
func TestAimlapiTools_DoesNotRelayCallerHeaders(t *testing.T) {
	rt := &aimlapiFakeTransport{answer: func(r *http.Request) *http.Response {
		return aimlapiJSONResponse(http.StatusOK, `{"status":"completed","output":{"results":[]}}`)
	}}
	h := newAimlapiToolsHandler(t, rt)
	if _, err := runAimlapiTool(t, h, "/v1/search", `{"query":"x"}`); err != nil {
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

// A redirect from the Toolbox must not be followed: the bearer key would be
// replayed to the host the Location names.
func TestAimlapiTools_DoesNotFollowRedirect(t *testing.T) {
	rt := &aimlapiFakeTransport{answer: func(r *http.Request) *http.Response {
		resp := aimlapiJSONResponse(http.StatusFound, `{}`)
		resp.Header.Set("Location", "https://evil.example/v1/tools/run")
		return resp
	}}
	h := newAimlapiToolsHandler(t, rt)
	_, err := runAimlapiTool(t, h, "/v1/search", `{"query":"x"}`)
	if err == nil {
		t.Fatal("expected an error for a redirect")
	}
	for _, s := range rt.seen {
		if s.Host != "tools.aimlapi.com" {
			t.Errorf("request sent to %q (auth %q)", s.Host, s.Auth)
		}
	}
	if len(rt.seen) != 1 {
		t.Errorf("upstream calls = %d, want 1", len(rt.seen))
	}
}

// If the configured Toolbox URL ever points elsewhere, the key is not sent.
func TestAimlapiTools_RefusesForeignHost(t *testing.T) {
	orig := providers.KnownProviders["aimlapi"]
	t.Cleanup(func() { providers.KnownProviders["aimlapi"] = orig })
	changed := orig
	changed.SearchURL = "https://evil.example/v1/tools/run"
	providers.KnownProviders["aimlapi"] = changed

	rt := &aimlapiFakeTransport{answer: func(r *http.Request) *http.Response {
		return aimlapiJSONResponse(http.StatusOK, `{"status":"completed","output":{"results":[]}}`)
	}}
	h := newAimlapiToolsHandler(t, rt)
	if _, err := runAimlapiTool(t, h, "/v1/search", `{"query":"x"}`); err == nil {
		t.Fatal("expected refusal for a non-Toolbox host")
	}
	if len(rt.seen) != 0 {
		t.Errorf("key sent to %q", rt.seen[0].Host)
	}
}

func TestAimlapiTools_UpstreamErrorMessage(t *testing.T) {
	rt := &aimlapiFakeTransport{answer: func(r *http.Request) *http.Response {
		return aimlapiJSONResponse(http.StatusBadRequest, `{"error":{"message":"The input does not match the tool's input schema: /query is required.","code":"invalid_input"}}`)
	}}
	h := newAimlapiToolsHandler(t, rt)
	_, err := runAimlapiTool(t, h, "/v1/search", `{"query":"x"}`)
	if err == nil || !strings.Contains(err.Error(), "/query is required") {
		t.Fatalf("err = %v, want the Toolbox message", err)
	}
}

func TestAimlapiTools_RoutedFromSearchHandler(t *testing.T) {
	if !aimlapiToolEndpoint("/v1/search") || !aimlapiToolEndpoint("/v1/web/fetch") || aimlapiToolEndpoint("/v1/images/generations") {
		t.Error("aimlapiToolEndpoint lanes wrong")
	}
	if !isAimlapiProvider("aimlapi") || !isAimlapiProvider("aiml") || isAimlapiProvider("openrouter") {
		t.Error("isAimlapiProvider wrong")
	}
}

// HandleSearch with model "aimlapi" must reach the Toolbox adapter instead of
// forwarding the raw gateway body to SearchURL.
func TestHandleSearch_AimlapiRoutesThroughToolbox(t *testing.T) {
	rt := &aimlapiFakeTransport{answer: func(r *http.Request) *http.Response {
		return aimlapiJSONResponse(http.StatusOK, `{"status":"completed","output":{"results":[{"url":"https://a.example/","title":"A","snippet":"s"}]}}`)
	}}
	h := newAimlapiToolsHandler(t, rt)
	h.Client = &http.Client{Transport: rt}
	body := `{"model":"aimlapi","query":"hello","max_results":3}`
	req := httptest.NewRequest(http.MethodPost, "/v1/search", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.HandleSearch(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if len(rt.seen) != 1 || rt.seen[0].Body["tool"] != "web/search" {
		t.Fatalf("upstream = %+v", rt.seen)
	}
	var res map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &res)
	if res["provider"] != "aimlapi" {
		t.Errorf("response = %v", res)
	}
}
