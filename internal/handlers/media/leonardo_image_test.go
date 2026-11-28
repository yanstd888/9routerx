// 9RouterX — ported & modified by https://github.com/yanstd888/ — https://t.me/aiclub_id
package media

import (
	"9routerx/proxy/internal/db"
	"9routerx/proxy/internal/providers"
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

const (
	leonardoPhoenixUUID = "de7d3faf-762f-48e0-b3b7-9d0ac3a3fcf3"
	leonardoSdxlUUID    = "16e7060a-803e-4df3-97ee-edcfa5dc9cc8"
	leonardoRawUUID     = "12345678-1234-4234-8234-123456789abc"
)

var leonardoImageBytes = []byte("leonardo-fake-png-bytes")

func leonardoTestHandler(t *testing.T, base string) *MediaHandler {
	t.Helper()
	orig, ok := providers.KnownProviders["leonardo"]
	if !ok {
		t.Fatal("leonardo config missing")
	}
	cfg := orig
	cfg.BaseURL = base
	cfg.ImageURL = base + "/generations"
	providers.KnownProviders["leonardo"] = cfg
	t.Cleanup(func() { providers.KnownProviders["leonardo"] = orig })
	database, cleanup := setupMultimodalTestDB(t)
	t.Cleanup(cleanup)
	data, _ := json.Marshal(map[string]any{"apiKey": "leo-secret-key"})
	_, err := database.Exec(`INSERT INTO providerConnections (id, provider, authType, name, priority, isActive, data, createdAt, updatedAt) VALUES
		('conn-leo', 'leonardo', 'apikey', 'Leonardo Test', 1, 1, ?, '2026-07-18T00:00:00Z', '2026-07-18T00:00:00Z')`, string(data))
	if err != nil {
		t.Fatalf("seed connection: %v", err)
	}
	return newTestMediaHandler(db.NewRepo(database))
}
func leonardoFastPoll(t *testing.T, interval, deadline time.Duration) {
	t.Helper()
	oldInterval, oldDeadline := leonardoPollInterval, leonardoPollDeadline
	leonardoPollInterval, leonardoPollDeadline = interval, deadline
	t.Cleanup(func() { leonardoPollInterval, leonardoPollDeadline = oldInterval, oldDeadline })
}
func leonardoPost(t *testing.T, h *MediaHandler, payload string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/v1/images/generations", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.HandleImages(rec, req)
	return rec
}
func leonardoCompleteAnswer(imageURL string) []byte {
	answer, _ := json.Marshal(map[string]any{"generations_by_pk": map[string]any{
		"status": "COMPLETE", "generated_images": []map[string]string{{"url": imageURL}},
	}})
	return answer
}

func TestLeonardoModelAndSize(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"", leonardoPhoenixUUID}, {"phoenix", leonardoPhoenixUUID}, {"sdxl", leonardoSdxlUUID},
		{"leonardo/sdxl", leonardoSdxlUUID}, {"leo/phoenix", leonardoPhoenixUUID}, {leonardoRawUUID, leonardoRawUUID},
	} {
		if _, got := leonardoModel(nil, tc.in); got != tc.want {
			t.Errorf("model %q = %q, want %q", tc.in, got, tc.want)
		}
	}
	cases := []struct {
		req    leonardoImageRequest
		width  int
		height int
	}{
		{leonardoImageRequest{}, 1024, 1024},
		{leonardoImageRequest{Size: "1024x576"}, 1024, 576},
		{leonardoImageRequest{Size: "1024X576", Width: 576, Height: 1024}, 576, 1024},
		{leonardoImageRequest{Size: "1024x576", Width: 800}, 800, 576},
	}
	for i, tc := range cases {
		w, h := leonardoImageSize(tc.req)
		if w != tc.width || h != tc.height {
			t.Errorf("size case %d = %dx%d, want %dx%d", i, w, h, tc.width, tc.height)
		}
	}
}

func TestLeonardoImageRequestContract(t *testing.T) {
	leonardoFastPoll(t, time.Millisecond, 2*time.Second)
	var imageHits, postHits, pollHits atomic.Int32
	image := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		imageHits.Add(1)
		if r.Header.Get("Authorization") != "" {
			t.Errorf("Authorization leaked to image host: %q", r.Header.Get("Authorization"))
		}
		_, _ = w.Write(leonardoImageBytes)
	}))
	defer image.Close()
	var gotBody map[string]any
	generate := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/generations":
			postHits.Add(1)
			if got := r.Header.Get("Content-Type"); got != "application/json" {
				t.Errorf("Content-Type = %q", got)
			}
			if got := r.Header.Get("Authorization"); got != "Bearer leo-secret-key" {
				t.Errorf("Authorization = %q", got)
			}
			raw, _ := io.ReadAll(r.Body)
			if err := json.Unmarshal(raw, &gotBody); err != nil {
				t.Errorf("body not JSON: %v", err)
			}
			_, _ = w.Write([]byte(`{"sdGenerationJob":{"generationId":"gen-1"}}`))
		case strings.HasPrefix(r.URL.Path, "/generations/"):
			pollHits.Add(1)
			if got := r.Header.Get("Authorization"); got != "Bearer leo-secret-key" {
				t.Errorf("poll Authorization = %q", got)
			}
			if got := r.Header.Get("Content-Type"); got != "" {
				t.Errorf("poll Content-Type = %q", got)
			}
			_, _ = w.Write(leonardoCompleteAnswer(image.URL + "/result.png"))
		default:
			t.Errorf("unexpected upstream request %s %s", r.Method, r.URL.RequestURI())
		}
	}))
	defer generate.Close()
	h := leonardoTestHandler(t, generate.URL)
	rec := leonardoPost(t, h, `{"model":"leonardo/sdxl","prompt":"a red fox","size":"1024x576","n":4,"seed":7}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	if len(gotBody) != 5 || gotBody["modelId"] != leonardoSdxlUUID || gotBody["prompt"] != "a red fox" ||
		gotBody["width"] != float64(1024) || gotBody["height"] != float64(576) || gotBody["num_images"] != float64(1) {
		t.Errorf("body = %v, want exactly the five pinned fields", gotBody)
	}
	if postHits.Load() != 1 || pollHits.Load() != 1 || imageHits.Load() != 1 {
		t.Fatalf("hits post/poll/image = %d/%d/%d", postHits.Load(), pollHits.Load(), imageHits.Load())
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
	want := base64.StdEncoding.EncodeToString(leonardoImageBytes)
	if len(out.Data) != 1 || out.Data[0].B64JSON != want {
		t.Errorf("flat image response = %s", rec.Body.String())
	}
	if delta := time.Now().Unix() - out.Created; delta < 0 || delta > 5 {
		t.Errorf("created delta = %ds", delta)
	}
	if strings.Contains(rec.Body.String(), `"usage"`) || strings.Contains(rec.Body.String(), `"success"`) {
		t.Errorf("response has fabricated usage or wrapper: %s", rec.Body.String())
	}
}

func TestLeonardoMissingGenerationID(t *testing.T) {
	generate := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"sdGenerationJob":{}}`))
	}))
	defer generate.Close()
	rec := leonardoPost(t, leonardoTestHandler(t, generate.URL), `{"model":"leonardo/phoenix","prompt":"fox"}`)
	if rec.Code != http.StatusBadGateway || !strings.Contains(rec.Body.String(), "no generation ID returned") {
		t.Fatalf("status=%d body=%s, want 502 missing id", rec.Code, rec.Body.String())
	}
}

func TestLeonardoUpstreamErrors(t *testing.T) {
	for _, tc := range []struct {
		status int
		body   string
	}{{http.StatusUnauthorized, "invalid api key"}, {http.StatusTooManyRequests, "quota exhausted"}} {
		generate := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(tc.status)
			_, _ = w.Write([]byte(tc.body))
		}))
		h := leonardoTestHandler(t, generate.URL)
		rec := leonardoPost(t, h, `{"model":"leonardo/phoenix","prompt":"fox"}`)
		generate.Close()
		if rec.Code != tc.status || rec.Body.String() != tc.body {
			t.Errorf("status=%d body=%q, want %d/%q", rec.Code, rec.Body.String(), tc.status, tc.body)
		}
	}
}

func TestLeonardoPollOutcomes(t *testing.T) {
	for _, tc := range []struct {
		name, answer string
		status       int
		message      string
	}{
		{"failed", `{"status":"FAILED"}`, http.StatusBadGateway, "Leonardo image generation failed"},
		{"timeout", `{"status":"PENDING"}`, http.StatusGatewayTimeout, "Leonardo image generation timed out"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			leonardoFastPoll(t, 5*time.Millisecond, 40*time.Millisecond)
			generate := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					_, _ = w.Write([]byte(`{"sdGenerationJob":{"generationId":"gen-1"}}`))
					return
				}
				_, _ = w.Write([]byte(tc.answer))
			}))
			defer generate.Close()
			rec := leonardoPost(t, leonardoTestHandler(t, generate.URL), `{"model":"leonardo/phoenix","prompt":"fox"}`)
			if rec.Code != tc.status || !strings.Contains(rec.Body.String(), tc.message) {
				t.Fatalf("status=%d body=%s, want %d/%s", rec.Code, rec.Body.String(), tc.status, tc.message)
			}
		})
	}
}

func TestLeonardoPollStaysSameOriginAndNoDownloadCredentials(t *testing.T) {
	leonardoFastPoll(t, time.Millisecond, 2*time.Second)
	var attackerHits, downloadHits atomic.Int32
	attacker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { attackerHits.Add(1) }))
	defer attacker.Close()
	image := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		downloadHits.Add(1)
		if r.Header.Get("Authorization") != "" || r.Header.Get("Api-Key") != "" {
			t.Errorf("credentials reached image host: %v", r.Header)
		}
		_, _ = w.Write(leonardoImageBytes)
	}))
	defer image.Close()
	generate := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			answer, _ := json.Marshal(map[string]any{"sdGenerationJob": map[string]string{"generationId": attacker.URL + "/steal"}})
			_, _ = w.Write(answer)
			return
		}
		if !strings.HasPrefix(r.URL.RequestURI(), "/generations/") {
			t.Errorf("poll escaped generate origin: %s", r.URL.RequestURI())
		}
		_, _ = w.Write(leonardoCompleteAnswer(image.URL + "/result.png"))
	}))
	defer generate.Close()
	rec := leonardoPost(t, leonardoTestHandler(t, generate.URL), `{"model":"leonardo/phoenix","prompt":"fox"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if attackerHits.Load() != 0 || downloadHits.Load() != 1 {
		t.Errorf("attacker hits=%d download hits=%d", attackerHits.Load(), downloadHits.Load())
	}
}

func TestLeonardoDownloadGuardAndCap(t *testing.T) {
	leonardoFastPoll(t, time.Millisecond, 2*time.Second)
	for _, tc := range []struct {
		name, body string
		want       string
	}{
		{"redirect guard", "", "blocked by SSRF guard"},
		{"byte cap", strings.Repeat("x", (25<<20)+1), "exceeds 25MiB"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			image := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tc.body == "" {
					w.Header().Set("Location", "https://169.254.169.254/latest/meta-data/")
					w.WriteHeader(http.StatusFound)
					return
				}
				_, _ = w.Write([]byte(tc.body))
			}))
			defer image.Close()
			generate := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					_, _ = w.Write([]byte(`{"sdGenerationJob":{"generationId":"gen-1"}}`))
					return
				}
				_, _ = w.Write(leonardoCompleteAnswer(image.URL + "/result.png"))
			}))
			defer generate.Close()
			rec := leonardoPost(t, leonardoTestHandler(t, generate.URL), `{"model":"leonardo/phoenix","prompt":"fox"}`)
			if rec.Code != http.StatusBadGateway || !strings.Contains(rec.Body.String(), tc.want) {
				t.Fatalf("status=%d body=%s, want 502 with %q", rec.Code, rec.Body.String(), tc.want)
			}
			if strings.Contains(rec.Body.String(), "169.254.169.254") {
				t.Errorf("download error leaked the result URL: %s", rec.Body.String())
			}
		})
	}
}
