package executor

import (
	"context"
	"encoding/base64"
	json "encoding/json/v2"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"9routerx/proxy/internal/providers"
	"9routerx/proxy/internal/proxy"
)

func adaptaTestJWT(t *testing.T, client string) string {
	t.Helper()
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	payload := base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf(`{"exp":%d}`, time.Now().Add(time.Hour).Unix())))
	return header + "." + payload + "." + client
}

func TestAdaptaClientJWTAndMessages(t *testing.T) {
	if got := adaptaClientJWT("__client=eyJabc"); got != "eyJabc" {
		t.Fatalf("client jwt = %q", got)
	}
	messages, err := adaptaMessages([]byte(`{"messages":[{"role":"system","content":"be brief"},{"role":"user","content":"hi"}]}`))
	if err != nil || len(messages) != 1 {
		t.Fatalf("messages = %+v err=%v", messages, err)
	}
	first := messages[0].(map[string]any)
	parts := first["parts"].([]any)
	text := parts[0].(map[string]any)["text"].(string)
	if !strings.HasPrefix(text, "be brief\n\nhi") {
		t.Fatalf("injected text = %q", text)
	}
}

func TestForwardAdaptaWeb(t *testing.T) {
	clientJWT := fmt.Sprintf("adapta-client-%d", time.Now().UnixNano())
	clerk := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Cookie"); got != "__client="+clientJWT {
			t.Errorf("cookie = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/client":
			_, _ = w.Write([]byte(`{"response":{"sessions":[{"id":"sess-1","status":"active"}]}}`))
		case "/v1/client/sessions/sess-1/tokens":
			_, _ = w.Write([]byte(`{"jwt":"` + adaptaTestJWT(t, clientJWT) + `"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer clerk.Close()

	app := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat/stream/v1" || !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer eyJ") {
			t.Errorf("path=%q auth=%q", r.URL.Path, r.Header.Get("Authorization"))
		}
		var payload map[string]any
		body := make([]byte, 1<<15)
		n, _ := r.Body.Read(body)
		if json.Unmarshal(body[:n], &payload) != nil || payload["aiModelId"] != float64(14) {
			t.Errorf("payload = %+v", payload)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"type\":\"text-delta\",\"id\":\"quick-response\",\"delta\":\"hidden\"}\n" +
			"data: {\"type\":\"text-delta\",\"id\":\"msg\",\"delta\":\"adapta\"}\n" +
			"data: {\"type\":\"text-delta\",\"id\":\"msg\",\"delta\":\"-ok\"}\n" +
			"data: {\"type\":\"done\"}\n"))
	}))
	defer app.Close()

	req := &Request{
		Ctx:      proxy.WithAllowLoopback(context.Background()),
		Client:   app.Client(),
		APIKey:   "__client=" + clientJWT,
		Body:     []byte(`{"model":"adapta-web/adapta-one","messages":[{"role":"user","content":"hi"}]}`),
		ConnData: map[string]any{"clerkUrl": clerk.URL},
		Config:   &providers.ProviderConfig{BaseURL: app.URL, AuthHeader: "Authorization", AuthScheme: "bearer"},
	}
	rec := httptest.NewRecorder()
	if err := ForwardAdaptaWeb(rec, req); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(rec.Body.String(), "adapta-ok") || strings.Contains(rec.Body.String(), "hidden") {
		t.Fatalf("body = %s", rec.Body.String())
	}
}
