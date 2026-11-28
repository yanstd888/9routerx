package executor

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"9routerx/proxy/internal/providers"
)

type nlpcloudCapture struct {
	path    string
	headers http.Header
	body    map[string]any
}

func nlpcloudFixture(t *testing.T, reply string) (*[]nlpcloudCapture, *Request) {
	t.Helper()
	captures := &[]nlpcloudCapture{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode upstream body: %v", err)
		}
		*captures = append(*captures, nlpcloudCapture{path: r.URL.Path, headers: r.Header.Clone(), body: body})
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(reply))
	}))
	t.Cleanup(server.Close)
	return captures, &Request{
		APIKey: "nlpc-test-key",
		Client: server.Client(),
		Config: &providers.ProviderConfig{
			BaseURL:    server.URL,
			AuthHeader: "Authorization",
			AuthScheme: "raw",
			Format:     providers.FormatOpenAI,
		},
		ConnData: map[string]any{"baseUrl": server.URL + "/v1/gpu/chatdolphin/chatbot"},
	}
}

func TestForwardNLPCloudNonStream(t *testing.T) {
	captures, req := nlpcloudFixture(t, `{"response":"nlp-ok"}`)
	req.Body = []byte(`{"model":"nlpcloud/chatdolphin","messages":[{"role":"system","content":"be concise"},{"role":"user","content":"first"},{"role":"assistant","content":"reply"},{"role":"user","content":"now"}],"temperature":0.4}`)
	rec := httptest.NewRecorder()
	if err := ForwardNLPCloud(rec, req); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(rec.Body.String(), "nlp-ok") {
		t.Fatalf("body = %s", rec.Body.String())
	}
	if len(*captures) != 1 {
		t.Fatalf("upstream calls = %d", len(*captures))
	}
	call := (*captures)[0]
	if call.path != "/v1/gpu/chatdolphin/chatbot" {
		t.Fatalf("path = %q", call.path)
	}
	if got := call.headers.Get("Authorization"); got != "Token nlpc-test-key" {
		t.Fatalf("authorization = %q", got)
	}
	if got := call.body["input"]; got != "now" {
		t.Errorf("input = %v", got)
	}
	if got := call.body["context"]; got != "be concise" {
		t.Errorf("context = %v", got)
	}
	history, ok := call.body["history"].([]any)
	if !ok || len(history) != 1 {
		t.Fatalf("history = %v", call.body["history"])
	}
	if _, ok := call.body["messages"]; ok {
		t.Error("OpenAI messages leaked upstream")
	}
}

func TestForwardNLPCloudStream(t *testing.T) {
	captures, req := nlpcloudFixture(t, "data: {\"response\":\"Hel\"}\n\ndata: {\"response\":\"lo\"}\n\n")
	req.IsStream = true
	req.Body = []byte(`{"model":"chatdolphin","messages":[{"role":"user","content":"hi"}]}`)
	rec := httptest.NewRecorder()
	if err := ForwardNLPCloud(rec, req); err != nil {
		t.Fatalf("execute: %v", err)
	}
	body := rec.Body.String()
	for _, want := range []string{`"role":"assistant"`, `"content":"Hel"`, `"content":"lo"`, "data: [DONE]"} {
		if !strings.Contains(body, want) {
			t.Fatalf("stream missing %q: %s", want, body)
		}
	}
	if got := (*captures)[0].body["stream"]; got != true {
		t.Errorf("upstream stream flag = %v", got)
	}
}

func TestNLPCloudBaseURLNormalization(t *testing.T) {
	base, err := nlpcloudBaseURL(&Request{ConnData: map[string]any{"baseUrl": "https://example.test/v1/gpu/chatdolphin/chatbot"}})
	if err != nil || base != "https://example.test/v1/gpu" {
		t.Fatalf("base = %q, err = %v", base, err)
	}
}
