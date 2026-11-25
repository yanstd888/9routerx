package executor

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"9routerx/proxy/internal/providers"
	"9routerx/proxy/internal/translator"
)

type poeCapture struct {
	path    string
	headers http.Header
	body    map[string]any
}

func poeFixture(t *testing.T, reply string) (*[]poeCapture, *Request) {
	t.Helper()
	captures := &[]poeCapture{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode upstream body: %v", err)
		}
		*captures = append(*captures, poeCapture{path: r.URL.Path, headers: r.Header.Clone(), body: body})
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(reply))
	}))
	t.Cleanup(server.Close)
	return captures, &Request{
		APIKey:   "poe-test-key",
		Client:   server.Client(),
		Provider: "poe",
		Config: &providers.ProviderConfig{
			BaseURL:    server.URL,
			AuthHeader: "Authorization",
			AuthScheme: "bearer",
			Format:     providers.FormatOpenAI,
		},
		ConnData: map[string]any{"baseUrl": server.URL},
	}
}

func TestForwardPoeChatLane(t *testing.T) {
	captures, req := poeFixture(t, `{"id":"chatcmpl-1","object":"chat.completion","created":1,"model":"gpt-5.2","choices":[{"index":0,"message":{"role":"assistant","content":"poe-ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
	req.Body = []byte(`{"model":"poe/gpt-5.2","messages":[{"role":"user","content":"hi"}]}`)
	rec := httptest.NewRecorder()
	if err := ForwardPoe(rec, req); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(rec.Body.String(), "poe-ok") {
		t.Fatalf("body = %s", rec.Body.String())
	}
	if len(*captures) != 1 {
		t.Fatalf("upstream calls = %d", len(*captures))
	}
	call := (*captures)[0]
	if call.path != "/v1/chat/completions" {
		t.Fatalf("path = %q", call.path)
	}
	if got := call.headers.Get("Authorization"); got != "Bearer poe-test-key" {
		t.Fatalf("authorization = %q", got)
	}
	if got := call.headers.Get("anthropic-version"); got != "" {
		t.Fatalf("chat lane leaked anthropic-version %q", got)
	}
	if got := call.body["model"]; got != "gpt-5.2" {
		t.Fatalf("model = %v", got)
	}
}

func TestForwardPoeMessagesLane(t *testing.T) {
	captures, req := poeFixture(t, `{"id":"msg_1","type":"message","role":"assistant","model":"claude-opus-4.8","content":[{"type":"text","text":"claude-ok"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`)
	req.Body = []byte(`{"model":"claude-opus-4.8","messages":[{"role":"system","content":"be concise"},{"role":"user","content":"hi"}]}`)
	rec := httptest.NewRecorder()
	if err := ForwardPoe(rec, req); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(rec.Body.String(), "claude-ok") {
		t.Fatalf("translated body = %s", rec.Body.String())
	}
	call := (*captures)[0]
	if call.path != "/v1/messages" {
		t.Fatalf("path = %q", call.path)
	}
	if got := call.headers.Get("anthropic-version"); got != "2023-06-01" {
		t.Fatalf("anthropic-version = %q", got)
	}
	if _, ok := call.body["system"]; !ok {
		t.Fatalf("system prompt missing: %v", call.body)
	}
}

func TestPoeLaneAndBaseURL(t *testing.T) {
	ctx := context.Background()
	if got := poeLane(ctx, "gpt-5.2"); got != "chat" {
		t.Errorf("gpt lane = %q", got)
	}
	if got := poeLane(ctx, "claude-opus-4.8"); got != "messages" {
		t.Errorf("claude lane = %q", got)
	}
	responsesCtx := translator.WithClientFormat(ctx, translator.ClientFormatResponses)
	if got := poeLane(responsesCtx, "gpt-5.2"); got != "responses" {
		t.Errorf("Responses lane = %q", got)
	}

	base, err := poeBaseURL(&Request{Config: &providers.ProviderConfig{BaseURL: "https://api.poe.com/v1/chat/completions"}})
	if err != nil || base != "https://api.poe.com/v1" {
		t.Fatalf("base = %q, err = %v", base, err)
	}
	base, err = poeBaseURL(&Request{ConnData: map[string]any{"baseUrl": "https://example.test/"}})
	if err != nil || base != "https://example.test/v1" {
		t.Fatalf("override base = %q, err = %v", base, err)
	}
}
