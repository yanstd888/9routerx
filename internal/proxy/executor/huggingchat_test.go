package executor

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"9routerx/proxy/internal/providers"
	"9routerx/proxy/internal/proxy"
)

func TestHuggingChatPromptAndCookie(t *testing.T) {
	if got := huggingchatCookie("Cookie: hf-chat=abc"); got != "hf-chat=abc" {
		t.Fatalf("cookie = %q", got)
	}
	if got := huggingchatCookie("bare-token"); got != "hf-chat=bare-token" {
		t.Fatalf("bare cookie = %q", got)
	}
	prompt, system := huggingchatPrompt([]byte(`{"messages":[{"role":"system","content":"be concise"},{"role":"user","content":"hi"}]}`))
	if prompt != "hi" || system != "be concise" {
		t.Fatalf("prompt=%q system=%q", prompt, system)
	}
}

func TestForwardHuggingChat(t *testing.T) {
	var form string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Cookie"); !strings.Contains(got, "hf-chat=session") {
			t.Errorf("cookie = %q", got)
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/chat/conversation":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"conversationId":"conv-1"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/chat/api/v2/conversations/conv-1":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"rootMessageId":"root-1"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/chat/conversation/conv-1":
			body := make([]byte, 1<<15)
			n, _ := r.Body.Read(body)
			form = string(body[:n])
			w.Header().Set("Content-Type", "application/x-ndjson")
			_, _ = w.Write([]byte("{\"type\":\"stream\",\"token\":\"hu\\u0000\"}\n{\"type\":\"stream\",\"token\":\"gging\"}\n{\"type\":\"finalAnswer\",\"text\":\"hugging\"}\n{\"type\":\"status\",\"status\":\"finished\"}\n"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	req := &Request{
		Ctx: proxy.WithAllowLoopback(context.Background()), Client: server.Client(),
		APIKey:   "hf-chat=session",
		Body:     []byte(`{"model":"openai/gpt-oss-120b","messages":[{"role":"user","content":"hi"}]}`),
		Config:   &providers.ProviderConfig{BaseURL: server.URL + "/chat", AuthHeader: "Cookie", AuthScheme: "raw"},
		ConnData: map[string]any{"baseUrl": server.URL + "/chat"},
	}
	rec := httptest.NewRecorder()
	if err := ForwardHuggingChat(rec, req); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(form, `"id":"root-1"`) || !strings.Contains(form, `"inputs":"hi"`) {
		t.Fatalf("form = %s", form)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"content":"hugging"`) || strings.Count(body, `"content":"hugging"`) != 1 {
		t.Fatalf("body = %s", body)
	}
}
