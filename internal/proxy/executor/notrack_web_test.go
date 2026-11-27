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

func TestNotrackCookieAndModel(t *testing.T) {
	cookie := notrackCookie("Cookie: uid=1; si_usr_id=2; si_ses_id=3; nt_session=ntk-x; other=drop")
	if cookie != "uid=1; si_usr_id=2; si_ses_id=3; nt_session=ntk-x" {
		t.Fatalf("cookie = %q", cookie)
	}
	if got := notrackCookie("raw-cookie"); got != "raw-cookie" {
		t.Fatalf("raw cookie = %q", got)
	}
	if got := notrackModel("ntw/notrack-c"); got != "notrack-c" {
		t.Fatalf("model = %q", got)
	}
}

func TestForwardNotrackWeb(t *testing.T) {
	var path, cookie, userAgent string
	var payload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path, cookie, userAgent = r.URL.Path, r.Header.Get("cookie"), r.Header.Get("user-agent")
		body := make([]byte, 1<<15)
		n, _ := r.Body.Read(body)
		if err := json.Unmarshal(body[:n], &payload); err != nil {
			t.Errorf("decode payload: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"type\":\"chat_meta\",\"chat_id\":\"chat-1\"}\n" +
			"data: {\"type\":\"delta\",\"chunk\":\"not\"}\n" +
			"data: {\"type\":\"delta\",\"chunk\":\"rack-ok\"}\n" +
			"data: {\"type\":\"message\",\"content\":\"ignored\"}\n"))
	}))
	defer server.Close()

	req := &Request{
		Ctx:      proxy.WithAllowLoopback(context.Background()),
		APIKey:   "uid=1; si_usr_id=2; si_ses_id=3",
		Client:   server.Client(),
		IsStream: true,
		Body:     []byte(`{"model":"ntw/notrack-c","messages":[{"role":"user","content":"hi"}]}`),
		ConnData: map[string]any{"baseUrl": server.URL},
		Config: &providers.ProviderConfig{BaseURL: server.URL, AuthHeader: "Cookie", AuthScheme: "raw"},
	}
	rec := httptest.NewRecorder()
	if err := ForwardNotrackWeb(rec, req); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if path != "/api/dispatch" || !strings.Contains(cookie, "uid=1") || !strings.Contains(userAgent, "Edg/150") {
		t.Fatalf("path=%q cookie=%q ua=%q", path, cookie, userAgent)
	}
	if payload["user_input"] != "hi" || payload["model"] != "C" || payload["max_turns"] != float64(6) {
		t.Fatalf("payload = %+v", payload)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"content":"not"`) || !strings.Contains(body, `"content":"rack-ok"`) || strings.Contains(body, "ignored") {
		t.Fatalf("body = %s", body)
	}
}
