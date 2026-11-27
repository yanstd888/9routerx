// 9RouterX — ported & modified by https://github.com/yanstd888/ — https://t.me/aiclub_id
package media

import (
	"9routerx/proxy/internal/db"
	"9routerx/proxy/internal/providers"
	"context"
	"database/sql"
	"encoding/base64"
	json "encoding/json/v2"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

var ideogramImageBytes = []byte("ideogram-fake-png-bytes")

func seedIdeogramConn(t *testing.T, database *sql.DB) {
	t.Helper()
	data, _ := json.Marshal(map[string]any{"apiKey": "ideo-secret-key"})
	if _, err := database.Exec(`INSERT INTO providerConnections (id, provider, authType, name, priority, isActive, data, createdAt, updatedAt) VALUES
		('conn-ideo', 'ideogram', 'apikey', 'Ideogram Test', 1, 1, ?, '2026-07-18T00:00:00Z', '2026-07-18T00:00:00Z')`, string(data)); err != nil {
		t.Fatalf("seed connection: %v", err)
	}
}
func ideogramTestHandler(t *testing.T, generateBase string) *MediaHandler {
	t.Helper()
	orig, ok := providers.KnownProviders["ideogram"]
	if !ok {
		t.Fatal("ideogram config missing")
	}
	cfg := orig
	cfg.BaseURL = generateBase
	cfg.ImageURL = generateBase + "/generate"
	providers.KnownProviders["ideogram"] = cfg
	t.Cleanup(func() { providers.KnownProviders["ideogram"] = orig })
	database, cleanup := setupMultimodalTestDB(t)
	t.Cleanup(cleanup)
	seedIdeogramConn(t, database)
	return newTestMediaHandler(db.NewRepo(database))
}
func ideogramPost(t *testing.T, h *MediaHandler, model string) *httptest.ResponseRecorder {
	t.Helper()
	body := `{"model":"` + model + `","prompt":"a red fox","size":"1024x1792","n":4,"seed":7}`
	req := httptest.NewRequest(http.MethodPost, "/v1/images/generations", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.HandleImages(rec, req)
	return rec
}
func TestIdeogramBlankPrompt(t *testing.T) {
	h := ideogramTestHandler(t, "http://127.0.0.1:1")
	req := httptest.NewRequest(http.MethodPost, "/v1/images/generations", strings.NewReader(`{"model":"ideogram/V_3","prompt":"   "}`))
	rec := httptest.NewRecorder()
	h.HandleImages(rec, req)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "missing required field: prompt") {
		t.Fatalf("status=%d body=%s, want 400 JSON", rec.Code, rec.Body.String())
	}
}
func TestIdeogramImageSuccess(t *testing.T) {
	var imageHits, generateHits atomic.Int32
	image := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		imageHits.Add(1)
		if got := r.Header.Get("Api-Key"); got != "" {
			t.Errorf("Api-Key leaked to image host: %q", got)
		}
		if got := r.Header.Get("Authorization"); got != "" {
			t.Errorf("Authorization leaked to image host: %q", got)
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(ideogramImageBytes)
	}))
	defer image.Close()
	generate := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		generateHits.Add(1)
		// ImageURL already carries /generate; the handler must not append another path.
		if r.URL.RequestURI() != "/generate" {
			t.Errorf("upstream path = %q, want /generate", r.URL.RequestURI())
		}
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type = %q", got)
		}
		if got := r.Header.Get("Api-Key"); got != "ideo-secret-key" {
			t.Errorf("Api-Key = %q", got)
		}
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		var sent map[string]any
		if err := json.Unmarshal(raw, &sent); err != nil {
			t.Errorf("body not JSON: %v", err)
		}
		if len(sent) != 3 || sent["prompt"] != "a red fox" ||
			sent["aspect_ratio"] != "ASPECT_16_9" || sent["model"] != "V_3" {
			t.Errorf("body = %s, want only prompt/aspect_ratio/model with default V_3", raw)
		}
		answer, _ := json.Marshal(map[string]any{
			"data": []map[string]string{{"url": image.URL + "/result.png"}},
		})
		_, _ = w.Write(answer)
	}))
	defer generate.Close()
	h := ideogramTestHandler(t, generate.URL)
	rec := ideogramPost(t, h, "ideogram") // bare alias: no model id -> V_3
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	if generateHits.Load() != 1 || imageHits.Load() != 1 {
		t.Fatalf("generate hits=%d image hits=%d, want 1/1", generateHits.Load(), imageHits.Load())
	}
	var out struct {
		Created int64 `json:"created"`
		Data    []struct {
			B64JSON string `json:"b64_json"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("parse response: %v", err)
	}
	if len(out.Data) != 1 {
		t.Fatalf("response = %s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), `"success"`) {
		t.Errorf("response kept the nested wrapper: %s", rec.Body.String())
	}
	if want := base64.StdEncoding.EncodeToString(ideogramImageBytes); out.Data[0].B64JSON != want {
		t.Errorf("b64_json = %q, want %q", out.Data[0].B64JSON, want)
	}
	if delta := time.Now().Unix() - out.Created; delta < 0 || delta > 5 {
		t.Errorf("created delta = %ds", delta)
	}
	if strings.Contains(rec.Body.String(), `"usage"`) {
		t.Errorf("response carries fabricated usage: %s", rec.Body.String())
	}
}
func TestIdeogramImageAliasModel(t *testing.T) {
	image := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(ideogramImageBytes)
	}))
	defer image.Close()
	var gotModel string
	generate := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var sent map[string]any
		_ = json.Unmarshal(raw, &sent)
		gotModel, _ = sent["model"].(string)
		answer, _ := json.Marshal(map[string]any{"data": []map[string]string{{"url": image.URL + "/x.png"}}})
		_, _ = w.Write(answer)
	}))
	defer generate.Close()
	h := ideogramTestHandler(t, generate.URL)
	rec := ideogramPost(t, h, "ideo/V_2A")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	if gotModel != "V_2A" {
		t.Errorf("model = %q, want V_2A", gotModel)
	}
}
func TestIdeogramImageUpstreamError(t *testing.T) {
	generate := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte("ideogram quota exhausted; retry later"))
	}))
	defer generate.Close()
	h := ideogramTestHandler(t, generate.URL)
	rec := ideogramPost(t, h, "ideogram/V_3")
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", rec.Code)
	}
	if got := rec.Body.String(); got != "ideogram quota exhausted; retry later" {
		t.Errorf("body = %q, want raw upstream text", got)
	}
}
func TestIdeogramImageNoImages(t *testing.T) {
	generate := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer generate.Close()
	h := ideogramTestHandler(t, generate.URL)
	rec := ideogramPost(t, h, "ideogram/V_3")
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "no images returned from Ideogram") {
		t.Errorf("body = %s", rec.Body.String())
	}
}
func TestIdeogramImageDownloadFailure(t *testing.T) {
	imageURL := "http://127.0.0.1:1/secret-result.png"
	generate := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		answer, _ := json.Marshal(map[string]any{"data": []map[string]string{{"url": imageURL}}})
		_, _ = w.Write(answer)
	}))
	defer generate.Close()
	h := ideogramTestHandler(t, generate.URL)
	rec := ideogramPost(t, h, "ideogram/V_3")
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", rec.Code)
	}
	if strings.Contains(rec.Body.String(), imageURL) || strings.Contains(rec.Body.String(), "secret-result") {
		t.Errorf("download error leaked the upstream URL: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "failed to download image") {
		t.Errorf("body = %s", rec.Body.String())
	}
}
func TestIdeogramImageDownloadLimit(t *testing.T) {
	image := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		chunk := bytes25MiBChunk()
		for sent := 0; sent <= (25<<20)+len(chunk); sent += len(chunk) {
			if _, err := w.Write(chunk); err != nil {
				return
			}
		}
	}))
	defer image.Close()
	generate := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		answer, _ := json.Marshal(map[string]any{"data": []map[string]string{{"url": image.URL + "/big.png"}}})
		_, _ = w.Write(answer)
	}))
	defer generate.Close()
	h := ideogramTestHandler(t, generate.URL)
	rec := ideogramPost(t, h, "ideogram/V_3")
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "exceeds 25MiB") {
		t.Errorf("body = %s", rec.Body.String())
	}
}
func bytes25MiBChunk() []byte { return make([]byte, 1<<20) }
func TestIdeogramImageNoRedirectWithCredentials(t *testing.T) {
	var redirected atomic.Int32
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		redirected.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer destination.Close()
	generate := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", destination.URL+"/steal")
		w.WriteHeader(http.StatusFound)
		_, _ = w.Write([]byte("redirect offered"))
	}))
	defer generate.Close()
	h := ideogramTestHandler(t, generate.URL)
	rec := ideogramPost(t, h, "ideogram/V_3")
	if rec.Code != http.StatusFound || rec.Body.String() != "redirect offered" {
		t.Fatalf("status=%d body=%q, want raw 302 response", rec.Code, rec.Body.String())
	}
	if redirected.Load() != 0 {
		t.Errorf("client followed a redirect with credentials (%d hits)", redirected.Load())
	}
}
func TestIdeogramImageResultRedirectReGuarded(t *testing.T) {
	blocked := "https://169.254.169.254/latest/meta-data/"
	image := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", blocked)
		w.WriteHeader(http.StatusFound)
	}))
	defer image.Close()
	generate := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		answer, _ := json.Marshal(map[string]any{"data": []map[string]string{{"url": image.URL + "/redirect"}}})
		_, _ = w.Write(answer)
	}))
	defer generate.Close()
	h := ideogramTestHandler(t, generate.URL)
	rec := ideogramPost(t, h, "ideogram/V_3")
	if rec.Code != http.StatusBadGateway || !strings.Contains(rec.Body.String(), "blocked by SSRF guard") {
		t.Fatalf("status=%d body=%s, want re-guarded 502", rec.Code, rec.Body.String())
	}
}
func TestIdeogramGuardURL(t *testing.T) {
	cases := []struct {
		name, url, allowLocal string
		blocked               bool
	}{
		{"public https", "https://api.ideogram.ai/generate", "false", false}, {"public http", "http://example.com/generate", "false", true},
		{"loopback", "http://127.0.0.1:8080/x", "false", true},
		{"private", "https://10.0.0.5/x", "false", true},
		{"metadata", "https://169.254.169.254/latest/meta-data/", "false", true},
		{"local opt-in", "http://127.0.0.1:8080/x", "true", false},
		{"metadata with opt-in", "https://169.254.169.254/latest/meta-data/", "true", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("ALLOW_PRIVATE_NETWORKING", tc.allowLocal)
			_, err := ideogramGuardURL(context.Background(), tc.url)
			if (err != nil) != tc.blocked {
				t.Errorf("guard(%q, local=%s) = %v, blocked=%v", tc.url, tc.allowLocal, err, tc.blocked)
			}
		})
	}
}
