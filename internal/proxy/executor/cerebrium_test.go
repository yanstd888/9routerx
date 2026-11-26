package executor

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"9routerx/proxy/internal/providers"
)

func TestForwardCerebriumRequiresDeploymentURL(t *testing.T) {
	rec := httptest.NewRecorder()
	req := &Request{Body: []byte(`{"model":"meta-llama/Meta-Llama-3.1-8B-Instruct","messages":[]}`), Config: &providers.ProviderConfig{}}
	err := ForwardCerebrium(rec, req)
	if err == nil {
		t.Fatal("missing deployment URL was accepted")
	}
	if !strings.Contains(err.Error(), "requires a Base URL") {
		t.Fatalf("error = %v", err)
	}
}

func TestForwardCerebriumUsesDeploymentURL(t *testing.T) {
	var path, auth string
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path, auth = r.URL.Path, r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl-1","object":"chat.completion","created":1,"model":"meta-llama/Meta-Llama-3.1-8B-Instruct","choices":[{"index":0,"message":{"role":"assistant","content":"cerebrium-ok"},"finish_reason":"stop"}]}`))
	}))
	defer server.Close()

	req := &Request{
		Ctx:      context.Background(),
		APIKey:   "cerebrium-test-key",
		Client:   server.Client(),
		Body:     []byte(`{"model":"meta-llama/Meta-Llama-3.1-8B-Instruct","messages":[{"role":"user","content":"hi"}]}`),
		ConnData: map[string]any{"baseUrl": server.URL + "/v4/project/app/run/"},
		Config: &providers.ProviderConfig{
			BaseURL:    "https://api.cerebrium.ai/v4",
			AuthHeader: "Authorization",
			AuthScheme: "bearer",
			Format:     providers.FormatOpenAI,
		},
	}
	rec := httptest.NewRecorder()
	if err := ForwardCerebrium(rec, req); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if path != "/v4/project/app/run" {
		t.Fatalf("path = %q", path)
	}
	if auth != "Bearer cerebrium-test-key" {
		t.Fatalf("authorization = %q", auth)
	}
	if got := body["model"]; got != "meta-llama/Meta-Llama-3.1-8B-Instruct" {
		t.Fatalf("model = %v", got)
	}
	if !strings.Contains(rec.Body.String(), "cerebrium-ok") {
		t.Fatalf("body = %s", rec.Body.String())
	}
}
