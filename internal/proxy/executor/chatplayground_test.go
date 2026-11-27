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

func cplTestJWT(t *testing.T) string {
	t.Helper()
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"sid":"sess-1","exp":` + fmt.Sprintf("%d", time.Now().Add(time.Hour).Unix()) + `}`))
	return header + "." + payload + ".signature"
}

func TestChatplaygroundModelAndPayload(t *testing.T) {
	model, ok := chatplaygroundModel("cpl/openai/gpt-6.1-sol", nil)
	if !ok || model.Endpoint != "azure" || model.ModelName != "gpt-6.1-sol" {
		t.Fatalf("model = %+v ok=%t", model, ok)
	}
	payload, err := chatplaygroundPayload([]byte(`{"messages":[{"role":"user","content":"hi"}],"chatId":"c1"}`), model)
	if err != nil {
		t.Fatalf("payload: %v", err)
	}
	var parsed map[string]any
	if json.Unmarshal(payload, &parsed) != nil || parsed["model"] != "openai/gpt-6.1-sol" || parsed["botId"] != "gpt-6.1-sol" {
		t.Fatalf("payload = %s", payload)
	}
	if !strings.Contains(chatplaygroundStripID("hello CHAT_ID:abc", true), "hello") {
		t.Fatalf("strip Chat_ID failed")
	}
}

func TestForwardChatplayground(t *testing.T) {
	var path, auth string
	var payload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path, auth = r.URL.Path, r.Header.Get("authorization")
		body := make([]byte, 1<<15)
		n, _ := r.Body.Read(body)
		if err := json.Unmarshal(body[:n], &payload); err != nil {
			t.Errorf("decode payload: %v", err)
		}
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("cpl-ok CHAT_ID:abc"))
	}))
	defer server.Close()

	req := &Request{
		Ctx:     proxy.WithAllowLoopback(context.Background()),
		Client:  server.Client(),
		APIKey:  "Bearer " + cplTestJWT(t),
		Body:    []byte(`{"model":"chatplayground/gpt-6.1-sol","messages":[{"role":"user","content":"hi"}]}`),
		Config: &providers.ProviderConfig{BaseURL: server.URL + "/api/chat", AuthHeader: "Authorization", AuthScheme: "bearer"},
	}
	rec := httptest.NewRecorder()
	if err := ForwardChatplayground(rec, req); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if path != "/api/chat/azure" || !strings.HasPrefix(auth, "Bearer eyJ") {
		t.Fatalf("path=%q auth=%q", path, auth)
	}
	if payload["model"] != "openai/gpt-6.1-sol" || payload["botId"] != "gpt-6.1-sol" {
		t.Fatalf("payload = %+v", payload)
	}
	if !strings.Contains(rec.Body.String(), "cpl-ok") || strings.Contains(rec.Body.String(), "CHAT_ID") {
		t.Fatalf("body = %s", rec.Body.String())
	}
}
