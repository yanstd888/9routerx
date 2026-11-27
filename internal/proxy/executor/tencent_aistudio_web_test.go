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

func TestForwardTencentAistudioWeb(t *testing.T) {
	var path, cookie, origin, referer string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path, cookie, origin, referer = r.URL.Path, r.Header.Get("Cookie"), r.Header.Get("Origin"), r.Header.Get("Referer")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl-1","object":"chat.completion","created":1,"model":"HunyuanDefault","choices":[{"index":0,"message":{"role":"assistant","content":"tencent-ok"},"finish_reason":"stop"}]}`))
	}))
	defer server.Close()

	req := &Request{
		Ctx:      proxy.WithAllowLoopback(context.Background()),
		APIKey:   "Cookie: session=abc",
		Client:   server.Client(),
		Body:     []byte(`{"model":"hy3-g","messages":[{"role":"user","content":"hi"}]}`),
		ConnData: map[string]any{"baseUrl": server.URL},
		Config:   &providers.ProviderConfig{BaseURL: server.URL, AuthHeader: "Cookie", AuthScheme: "raw"},
	}
	rec := httptest.NewRecorder()
	if err := ForwardTencentAistudioWeb(rec, req); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if path != "/api/chat/HunyuanDefault" || cookie != "session=abc" || origin != server.URL || referer != server.URL+"/" {
		t.Fatalf("path=%q cookie=%q origin=%q referer=%q", path, cookie, origin, referer)
	}
	if !strings.Contains(rec.Body.String(), "tencent-ok") {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestTencentAistudioModelMap(t *testing.T) {
	for input, want := range map[string]string{
		"hy3-g": "HunyuanDefault", "hunyuan-default": "HunyuanDefault", "hunyuan-3d": "Hunyuan3D",
	} {
		if got := tencentAistudioModels[input]; got != want {
			t.Errorf("%s = %q, want %q", input, got, want)
		}
	}
}
