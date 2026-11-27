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

func TestConolCookieModelEffort(t *testing.T) {
	if got := conolCookie("Cookie: token-only"); got != "__Secure-better-auth.session_token=token-only" {
		t.Fatalf("cookie = %q", got)
	}
	model, effort := conolModel("cnl/claude-sonnet-5-xhigh", nil)
	if model != "claude-sonnet-5" || effort != "high" {
		t.Fatalf("model=%q effort=%q", model, effort)
	}
}

func TestForwardConolWeb(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.Method+" "+r.URL.Path)
		if got := r.Header.Get("cookie"); got != "session=abc" {
			t.Errorf("cookie = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/sessions":
			_, _ = w.Write([]byte(`{"sessionId":"ses-1"}`))
		case strings.HasSuffix(r.URL.Path, "/model"):
			_, _ = w.Write([]byte(`{"ok":true}`))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/messages"):
			_, _ = w.Write([]byte(`{"ok":true}`))
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/messages"):
			_, _ = w.Write([]byte("data: {\"type\":\"stream_event\",\"delta\":\"conol\"}\n" +
				"data: {\"type\":\"stages\",\"stages\":[{\"logs\":[{\"content\":{\"text\":\"conol-ok\"}}]}]}\n" +
				"data: {\"type\":\"done\"}\n"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	req := &Request{
		Ctx:     proxy.WithAllowLoopback(context.Background()),
		Client:  server.Client(),
		APIKey:  "session=abc",
		Body:    []byte(`{"model":"conol-web/claude-sonnet-5","messages":[{"role":"user","content":"hi"}]}`),
		Config: &providers.ProviderConfig{BaseURL: server.URL + "/api/sessions", AuthHeader: "Cookie", AuthScheme: "raw"},
	}
	rec := httptest.NewRecorder()
	if err := ForwardConolWeb(rec, req); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(rec.Body.String(), "conol-ok") {
		t.Fatalf("body = %s", rec.Body.String())
	}
	var parsed map[string]any
	if json.Unmarshal([]byte(rec.Body.String()), &parsed) != nil {
		t.Fatalf("invalid completion: %s", rec.Body.String())
	}
	if len(paths) < 6 {
		t.Fatalf("paths = %v", paths)
	}
}
