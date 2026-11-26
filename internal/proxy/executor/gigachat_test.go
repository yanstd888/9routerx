package executor

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"9routerx/proxy/internal/providers"
)

func TestForwardGigaChatExchangesAndCachesToken(t *testing.T) {
	basic := fmt.Sprintf("client-%d:secret", time.Now().UnixNano())
	tokenCalls := 0
	authServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokenCalls++
		if got := r.Header.Get("Authorization"); got != "Basic "+basic {
			t.Errorf("token auth = %q", got)
		}
		if r.Header.Get("RqUID") == "" {
			t.Error("missing RqUID")
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), "scope=GIGACHAT_API_PERS") {
			t.Errorf("token body = %s", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"access_token":"giga-short-token","expires_at":%d}`, time.Now().Add(time.Hour).UnixMilli())
	}))
	defer authServer.Close()

	chatCalls := 0
	chatServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		chatCalls++
		if got := r.Header.Get("Authorization"); got != "Bearer giga-short-token" {
			t.Errorf("chat auth = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl-1","object":"chat.completion","created":1,"model":"GigaChat-2-Pro","choices":[{"index":0,"message":{"role":"assistant","content":"giga-ok"},"finish_reason":"stop"}]}`))
	}))
	defer chatServer.Close()

	req := &Request{
		Ctx: context.Background(), APIKey: basic, Client: chatServer.Client(),
		Body:     []byte(`{"model":"GigaChat-2-Pro","messages":[{"role":"user","content":"hi"}]}`),
		ConnData: map[string]any{"authUrl": authServer.URL},
		Config: &providers.ProviderConfig{
			BaseURL:    chatServer.URL,
			AuthHeader: "Authorization",
			AuthScheme: "bearer",
			Format:     providers.FormatOpenAI,
		},
	}
	for i := 0; i < 2; i++ {
		rec := httptest.NewRecorder()
		if err := ForwardGigaChat(rec, req); err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
		if !strings.Contains(rec.Body.String(), "giga-ok") {
			t.Fatalf("call %d body = %s", i, rec.Body.String())
		}
	}
	if tokenCalls != 1 {
		t.Errorf("token calls = %d, want 1", tokenCalls)
	}
	if chatCalls != 2 {
		t.Errorf("chat calls = %d, want 2", chatCalls)
	}
}
