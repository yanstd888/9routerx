package executor

import (
	crand "crypto/rand"
	"encoding/base64"

	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"9routerx/proxy/internal/providers"
)

func cinfTestConfig(serverURL string) *providers.ProviderConfig {
	return &providers.ProviderConfig{
		BaseURL:    "https://api.cheaperinference.com/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
		Format:     providers.FormatOpenAI,
	}
}

func TestForwardCheaperInferenceRoutesResponsesModel(t *testing.T) {
	var path string
	var payload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode body: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"type\":\"response.output_text.delta\",\"delta\":\"ci-responses-ok\"}\n\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	req := &Request{
		Ctx:       context.Background(),
		APIKey:    "ci-test-key",
		Client:    server.Client(),
		ModelName: "gpt-5.5",
		Body:      []byte(`{"model":"cinf/gpt-5.5","messages":[{"role":"user","content":"hi"}]}`),
		Config: &providers.ProviderConfig{
			BaseURL:    server.URL + "/v1/chat/completions",
			AuthHeader: "Authorization",
			AuthScheme: "bearer",
			Format:     providers.FormatOpenAI,
		},
	}
	rec := httptest.NewRecorder()
	if err := ForwardCheaperInference(rec, req); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if path != "/v1/responses" {
		t.Fatalf("path = %q, want the responses surface", path)
	}
	if store, ok := payload["store"].(bool); !ok || store {
		t.Fatalf("store = %v, want false on the stateless responses endpoint", payload["store"])
	}
	if !strings.Contains(rec.Body.String(), "ci-responses-ok") {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestForwardCheaperInferenceChatPassthrough(t *testing.T) {
	var path, auth string
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path, auth = r.URL.Path, r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl-1","object":"chat.completion","created":1,"model":"claude-opus-5","choices":[{"index":0,"message":{"role":"assistant","content":"ci-chat-ok"},"finish_reason":"stop"}]}`))
	}))
	defer server.Close()

	req := &Request{
		Ctx:       context.Background(),
		APIKey:    "ci-test-key",
		Client:    server.Client(),
		ModelName: "claude-opus-5",
		Body:      []byte(`{"model":"cinf/claude-opus-5","messages":[{"role":"user","content":"hi"}]}`),
		Config: &providers.ProviderConfig{
			BaseURL:    server.URL + "/v1/chat/completions",
			AuthHeader: "Authorization",
			AuthScheme: "bearer",
			Format:     providers.FormatOpenAI,
		},
	}
	rec := httptest.NewRecorder()
	if err := ForwardCheaperInference(rec, req); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if path != "/v1/chat/completions" {
		t.Fatalf("path = %q", path)
	}
	if auth != "Bearer ci-test-key" {
		t.Fatalf("authorization = %q", auth)
	}
	if got := body["model"]; got != "cinf/claude-opus-5" {
		t.Fatalf("model = %v", got)
	}
	if !strings.Contains(rec.Body.String(), "ci-chat-ok") {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestForwardCheaperInferenceRefusesUnverifiedMedia(t *testing.T) {
	cases := []struct {
		model string
		part  string
		want  string
	}{
		{"claude-opus-5", `{"type":"input_audio","input_audio":{"data":"aGVsbG8=","format":"wav"}}`, "audio"},
		{"gemini-3.7-flash", `{"type":"file","file":{"filename":"doc.pdf","file_data":"data:application/pdf;base64,aGVsbG8="}}`, "pdf"},
		{"gemini-3.7-flash", `{"type":"video_url","video_url":"data:video/mp4;base64,aGVsbG8="}`, "video"},
	}
	for _, tc := range cases {
		body := `{"model":"cinf/` + tc.model + `","messages":[{"role":"user","content":[{"type":"text","text":"see attachment"},` + tc.part + `]}]}`
		req := &Request{ModelName: tc.model, Body: []byte(body), Config: cinfTestConfig("")}
		rec := httptest.NewRecorder()
		err := ForwardCheaperInference(rec, req)
		if err == nil {
			t.Fatalf("%s/%s was accepted", tc.model, tc.want)
		}
		if !strings.Contains(err.Error(), "no verified "+tc.want+" input transport") {
			t.Fatalf("error = %v", err)
		}
	}
}

func TestForwardCheaperInferenceAllowsVerifiedAudio(t *testing.T) {
	var seenAudio bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		seenAudio = strings.Contains(string(raw), "input_audio") || strings.Contains(string(raw), "\"file\"")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"1","object":"chat.completion","created":1,"model":"gemini-3.7-flash","choices":[{"index":0,"message":{"role":"assistant","content":"audio-ok"},"finish_reason":"stop"}]}`))
	}))
	defer server.Close()

	req := &Request{
		Ctx:       context.Background(),
		APIKey:    "k",
		Client:    server.Client(),
		ModelName: "gemini-3.7-flash",
		Body:      []byte(`{"model":"gemini-3.7-flash","messages":[{"role":"user","content":[{"type":"input_audio","input_audio":{"data":"aGVsbG8=","format":"wav"}}]}]}`),
		Config: &providers.ProviderConfig{
			BaseURL:    server.URL + "/v1/chat/completions",
			AuthHeader: "Authorization",
			AuthScheme: "bearer",
			Format:     providers.FormatOpenAI,
		},
	}
	rec := httptest.NewRecorder()
	if err := ForwardCheaperInference(rec, req); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !seenAudio {
		t.Fatal("audio carrier was dropped from the forwarded body")
	}
}

func TestForwardCheaperInferenceGzipsLargeChatBody(t *testing.T) {
	var encoding string
	var decoded map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		encoding = r.Header.Get("Content-Encoding")
		reader := r.Body
		if encoding == "gzip" {
			gz, err := gzip.NewReader(r.Body)
			if err != nil {
				t.Errorf("open gzip: %v", err)
				return
			}
			reader = gz
		}
		if err := json.NewDecoder(reader).Decode(&decoded); err != nil {
			t.Errorf("decode body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"1","object":"chat.completion","created":1,"model":"glm-5.2","choices":[{"index":0,"message":{"role":"assistant","content":"gz-ok"},"finish_reason":"stop"}]}`))
	}))
	defer server.Close()

	// A >32KiB prompt trips the gzip tier but stays under the 4MiB wire tier.
	prompt := strings.Repeat("compress me ", 6000)
	req := &Request{
		Ctx:       context.Background(),
		APIKey:    "k",
		Client:    server.Client(),
		ModelName: "glm-5.2",
		Body:      []byte(`{"model":"glm-5.2","messages":[{"role":"user","content":"` + prompt + `"}]}`),
		Config: &providers.ProviderConfig{
			BaseURL:    server.URL + "/v1/chat/completions",
			AuthHeader: "Authorization",
			AuthScheme: "bearer",
			Format:     providers.FormatOpenAI,
		},
	}
	rec := httptest.NewRecorder()
	if err := ForwardCheaperInference(rec, req); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if encoding != "gzip" {
		t.Fatalf("content-encoding = %q", encoding)
	}
	if got := decoded["model"]; got != "glm-5.2" {
		t.Fatalf("decoded model = %v", got)
	}
}

func TestForwardCheaperInferenceChunkedUploadReservation(t *testing.T) {
	var dispatchUploadHeader string
	var uploadCalls int
	var dispatchBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/payload-uploads") && r.Method == http.MethodPost:
			uploadCalls++
			var manifest map[string]any
			if err := json.NewDecoder(r.Body).Decode(&manifest); err != nil {
				t.Errorf("decode manifest: %v", err)
			}
			if manifest["encoding"] != "gzip" || manifest["endpoint"] != "responses" {
				t.Errorf("manifest = %v", manifest)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"0f4fd1f2-1111-4222-8333-444455556666","part_bytes":1048576}`))
		case strings.Contains(r.URL.Path, "/payload-uploads/") && r.Method == http.MethodPut:
			uploadCalls++
			io.Copy(io.Discard, r.Body)
			w.WriteHeader(http.StatusOK)
		case strings.HasSuffix(r.URL.Path, "/responses"):
			dispatchUploadHeader = r.Header.Get("X-CI-Payload-Upload")
			raw, _ := io.ReadAll(r.Body)
			dispatchBody = raw
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: {\"type\":\"response.output_text.delta\",\"delta\":\"chunk-ok\"}\n\n"))
			_, _ = w.Write([]byte("data: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}}\n\n"))
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	// A >4MiB decoded body forces the chunked tier; the responses surface is
	// reused so the whole reservation→dispatch flow runs end to end. The
	// filler is pseudo-random so the gzip tier cannot shrink the wire body
	// under the 4MiB threshold.
	filler := make([]byte, 9_000_000)
	if _, err := crand.Read(filler); err != nil {
		t.Fatalf("filler: %v", err)
	}
	bigText := base64.StdEncoding.EncodeToString(filler)
	req := &Request{
		Ctx:       context.Background(),
		APIKey:    "k",
		Client:    server.Client(),
		ModelName: "gpt-5.6-sol",
		Body:      []byte(`{"model":"gpt-5.6-sol","messages":[{"role":"user","content":"` + bigText + `"}]}`),
		Config: &providers.ProviderConfig{
			BaseURL:    server.URL + "/v1/chat/completions",
			AuthHeader: "Authorization",
			AuthScheme: "bearer",
			Format:     providers.FormatOpenAI,
		},
	}
	rec := httptest.NewRecorder()
	if err := ForwardCheaperInference(rec, req); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if uploadCalls < 2 {
		t.Fatalf("upload calls = %d, want reservation + parts", uploadCalls)
	}
	if dispatchUploadHeader == "" {
		t.Fatal("inference dispatch carried no x-ci-payload-upload header")
	}
	// The chunked tier dispatches the "{}" reservation reference; the reserved
	// body (already store:false from buildResponsesBody) lives server-side.
	if string(dispatchBody) != "{}" {
		t.Fatalf("dispatch body = %s, want the reservation reference", dispatchBody)
	}
	if !strings.Contains(rec.Body.String(), "chunk-ok") {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestCinfResponsesURL(t *testing.T) {
	if got := cinfResponsesURL("https://api.cheaperinference.com/v1/chat/completions"); got != "https://api.cheaperinference.com/v1/responses" {
		t.Fatalf("responses URL = %q", got)
	}
	if got := cinfResponsesURL(""); got != cinfResponsesBase {
		t.Fatalf("fallback responses URL = %q", got)
	}
}

func TestCinfNormalizeMixedMedia(t *testing.T) {
	// Single kind: untouched.
	single := []byte(`{"messages":[{"role":"user","content":[{"type":"input_audio","input_audio":{"data":"aGk=","format":"wav"}}]}]}`)
	if got := cinfNormalizeMixedMedia(single); string(got) != string(single) {
		t.Fatalf("single-kind body was rewritten: %s", got)
	}
	// Two kinds: audio becomes the MIME-typed file carrier.
	mixed := []byte(`{"messages":[{"role":"user","content":[{"type":"input_audio","input_audio":{"data":"aGk=","format":"mp3"}},{"type":"file","file":{"filename":"doc.pdf","file_data":"data:application/pdf;base64,aGk="}}]}]}`)
	got := cinfNormalizeMixedMedia(mixed)
	var probe struct {
		Messages []struct {
			Content []map[string]any `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(got, &probe); err != nil {
		t.Fatalf("decode normalized: %v", err)
	}
	first := probe.Messages[0].Content[0]
	if first["type"] != "file" {
		t.Fatalf("carrier = %v, want file", first["type"])
	}
	file := first["file"].(map[string]any)
	if file["filename"] != "audio-1.mp3" || file["file_data"] != "data:audio/mpeg;base64,aGk=" {
		t.Fatalf("file carrier = %v", file)
	}
}

func TestCinfReservationIDShape(t *testing.T) {
	if !cinfValidReservationID("0f4fd1f2-1111-4222-8333-444455556666") {
		t.Fatal("valid uuid rejected")
	}
	if cinfValidReservationID("https://evil.example.com") {
		t.Fatal("URL-shaped reservation id accepted — must never follow a returned upload URL")
	}
	if cinfValidReservationID("") {
		t.Fatal("empty reservation id accepted")
	}
}

func TestCinfPrepareRefusesOversize(t *testing.T) {
	req := &Request{Ctx: context.Background(), APIKey: "k", Client: http.DefaultClient}
	body := bytes.Repeat([]byte("x"), cinfMaxJSONBytes+1)
	_, _, _, err := cinfPrepare(req, cinfTestConfig(""), body)
	if err == nil {
		t.Fatal("oversize payload was accepted")
	}
	if !strings.Contains(err.Error(), "20 MiB") {
		t.Fatalf("error = %v", err)
	}
}
