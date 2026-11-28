package executor

import (
	"context"
	json "encoding/json/v2"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"9routerx/proxy/internal/providers"
	"9routerx/proxy/internal/proxy"
)

func TestSyntxModelAndPrompt(t *testing.T) {
	model, ok := syntxModel("stx/claude-fable-5-1", nil)
	if !ok || model.AIName != "claude" || !model.Thinking {
		t.Fatalf("model = %+v ok=%t", model, ok)
	}
	text, title, thinking := syntxPrompt([]byte(`{"messages":[{"role":"system","content":"be brief"},{"role":"user","content":"hello syntx"}],"reasoning_effort":"none"}`), model.Thinking)
	if !strings.Contains(text, "[System]\nbe brief") || !strings.Contains(text, "[User]\nhello syntx") || title != "hello syntx" || thinking {
		t.Fatalf("text=%q title=%q thinking=%t", text, title, thinking)
	}
}

func TestForwardSyntx(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/chats":
			if got := r.Header.Get("authorization"); got != "Bearer syntx-token" {
				t.Errorf("auth = %q", got)
			}
			_, _ = w.Write([]byte(`{"chat_uuid":"chat-1"}`))
		case "/api/v1/llm/generate":
			if r.URL.Query().Get("ai_name") != "claude" {
				t.Errorf("ai_name = %q", r.URL.Query().Get("ai_name"))
			}
			_, _ = w.Write([]byte(`{"stream_url":"` + "http://" + r.Host + `/stream/1"}`))
		case "/stream/1":
			_, _ = w.Write([]byte("data: {\"type\":\"content\",\"content\":\"syntx\"}\n\ndata: {\"type\":\"content\",\"content\":\"-ok\"}\n\ndata: {\"type\":\"usage_final\",\"tokens_input\":3,\"tokens_output\":2}\n\n"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	req := &Request{
		Ctx:    proxy.WithAllowLoopback(context.Background()),
		Client: server.Client(),
		APIKey: "Bearer syntx-token",
		Body:   []byte(`{"model":"syntx/claude-fable-5-1","messages":[{"role":"user","content":"hi"}]}`),
		Config: &providers.ProviderConfig{BaseURL: server.URL, AuthHeader: "Authorization", AuthScheme: "bearer"},
	}
	rec := httptest.NewRecorder()
	if err := ForwardSyntx(rec, req); err != nil {
		t.Fatalf("execute: %v", err)
	}
	completion := rec.Body.String()
	if !strings.Contains(completion, "syntx-ok") || !strings.Contains(completion, `"prompt_tokens":3`) {
		t.Fatalf("body = %s", completion)
	}
	var parsed struct {
		Model string `json:"model"`
	}
	if json.Unmarshal([]byte(completion), &parsed) != nil || parsed.Model != "claude-fable-5-1" {
		t.Fatalf("model echo missing: %s", completion)
	}
}
