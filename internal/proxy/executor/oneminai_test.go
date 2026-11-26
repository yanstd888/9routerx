package executor

import (
	"context"
	json "encoding/json/v2"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"9routerx/proxy/internal/providers"
	"9routerx/proxy/internal/proxy"
)

func TestOneminaiPromptAndPayload(t *testing.T) {
	prompt := oneminaiPrompt([]byte(`{"messages":[{"role":"system","content":"be concise"},{"role":"user","content":"hi"},{"role":"assistant","content":"there"},{"role":"user","content":"again"}]}`))
	if !strings.Contains(prompt, "System: be concise") || !strings.Contains(prompt, "Assistant: there") || !strings.HasSuffix(prompt, "User: again") {
		t.Fatalf("prompt = %q", prompt)
	}
	if got := oneminaiError(`{"error":{"message":"bad key"}}`, 401); got != "bad key" {
		t.Fatalf("error = %q", got)
	}
}

func TestForwardOneminai(t *testing.T) {
	var path, apiKey string
	var payload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path, apiKey = r.URL.Path, r.Header.Get("API-KEY")
		body, readErr := io.ReadAll(r.Body)
		if readErr != nil || json.Unmarshal(body, &payload) != nil {
			t.Errorf("decode payload: %v", readErr)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"aiRecord":{"aiRecordDetail":{"resultObject":["one-", "min-ok"]}}}`))
	}))
	defer server.Close()

	req := &Request{
		Ctx: proxy.WithAllowLoopback(context.Background()), Client: server.Client(),
		APIKey: "oneminai-key",
		Body:   []byte(`{"model":"oneminai/gpt-4o-mini","messages":[{"role":"user","content":"hi"}],"tools":[{"type":"function"}]}`),
		Config: &providers.ProviderConfig{BaseURL: server.URL + "/api/chat-with-ai", AuthHeader: "API-KEY", AuthScheme: "raw", Format: providers.FormatOpenAI},
	}
	rec := httptest.NewRecorder()
	if err := ForwardOneminai(rec, req); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if path != "/api/chat-with-ai" || apiKey != "oneminai-key" {
		t.Fatalf("path=%q key=%q", path, apiKey)
	}
	if payload["type"] != "UNIFY_CHAT_WITH_AI" || payload["model"] != "gpt-4o-mini" {
		t.Fatalf("payload = %+v", payload)
	}
	if _, ok := payload["tools"]; ok {
		t.Error("unsupported tools param leaked")
	}
	if !strings.Contains(rec.Body.String(), "one-min-ok") {
		t.Fatalf("body = %s", rec.Body.String())
	}
}
