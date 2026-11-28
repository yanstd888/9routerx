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

func TestForwardSnowflakeRequiresAccountURL(t *testing.T) {
	rec := httptest.NewRecorder()
	req := &Request{Body: []byte(`{"model":"llama3.3-70b","messages":[]}`), APIKey: "jwt-token", Config: &providers.ProviderConfig{}}
	err := ForwardSnowflake(rec, req)
	if err == nil {
		t.Fatal("missing account URL was accepted")
	}
	if !strings.Contains(err.Error(), "requires a Base URL") {
		t.Fatalf("error = %v", err)
	}
}

func TestForwardSnowflakeRefusesPlaceholder(t *testing.T) {
	rec := httptest.NewRecorder()
	req := &Request{
		Body:     []byte(`{"model":"llama3.3-70b","messages":[]}`),
		APIKey:   "jwt-token",
		ConnData: map[string]any{"baseUrl": "https://{account}.snowflakecomputing.com"},
		Config:   &providers.ProviderConfig{},
	}
	err := ForwardSnowflake(rec, req)
	if err == nil {
		t.Fatal("pinned {account} placeholder was accepted")
	}
	if !strings.Contains(err.Error(), "requires a Base URL") {
		t.Fatalf("error = %v", err)
	}
}

func TestForwardSnowflakeRequiresCredential(t *testing.T) {
	rec := httptest.NewRecorder()
	req := &Request{
		Body:     []byte(`{"model":"llama3.3-70b","messages":[]}`),
		ConnData: map[string]any{"baseUrl": "https://acme.snowflakecomputing.com"},
		Config:   &providers.ProviderConfig{},
	}
	err := ForwardSnowflake(rec, req)
	if err == nil {
		t.Fatal("missing credential was accepted")
	}
	if !strings.Contains(err.Error(), "API key required") {
		t.Fatalf("error = %v", err)
	}
}

func TestForwardSnowflakeRoutesCortexInference(t *testing.T) {
	var path, auth, tokenType string
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		auth = r.Header.Get("Authorization")
		tokenType = r.Header.Get("X-Snowflake-Authorization-Token-Type")
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl-1","object":"chat.completion","created":1,"model":"llama3.3-70b","choices":[{"index":0,"message":{"role":"assistant","content":"snowflake-ok"},"finish_reason":"stop"}]}`))
	}))
	defer server.Close()

	req := &Request{
		Ctx:    context.Background(),
		APIKey: "pat/test-token",
		Client: server.Client(),
		Body:   []byte(`{"model":"llama3.3-70b","messages":[{"role":"user","content":"hi"}]}`),
		ConnData: map[string]any{
			"baseUrl": server.URL + "/api/v2/",
		},
		Config: &providers.ProviderConfig{
			BaseURL:    "https://{account}.snowflakecomputing.com/api/v2",
			AuthHeader: "Authorization",
			AuthScheme: "bearer",
			Format:     providers.FormatOpenAI,
		},
	}
	rec := httptest.NewRecorder()
	if err := ForwardSnowflake(rec, req); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if path != "/api/v2/cortex/inference:complete" {
		t.Fatalf("path = %q", path)
	}
	if auth != "Bearer test-token" {
		t.Fatalf("authorization = %q", auth)
	}
	if tokenType != "PROGRAMMATIC_ACCESS_TOKEN" {
		t.Fatalf("token type = %q", tokenType)
	}
	if got := body["model"]; got != "llama3.3-70b" {
		t.Fatalf("model = %v", got)
	}
	if !strings.Contains(rec.Body.String(), "snowflake-ok") {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestForwardSnowflakeKeypairJWTClassified(t *testing.T) {
	var auth, tokenType string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		tokenType = r.Header.Get("X-Snowflake-Authorization-Token-Type")
		_, _ = w.Write([]byte(`{"id":"1","object":"chat.completion","created":1,"model":"llama3.3-70b","choices":[{"index":0,"message":{"role":"assistant","content":"snowflake-jwt-ok"},"finish_reason":"stop"}]}`))
	}))
	defer server.Close()

	req := &Request{
		Ctx:      context.Background(),
		APIKey:   "jwt-token",
		Client:   server.Client(),
		Body:     []byte(`{"model":"llama3.3-70b","messages":[{"role":"user","content":"hi"}]}`),
		ConnData: map[string]any{"baseUrl": server.URL},
		Config: &providers.ProviderConfig{
			AuthHeader: "Authorization",
			AuthScheme: "bearer",
			Format:     providers.FormatOpenAI,
		},
	}
	rec := httptest.NewRecorder()
	if err := ForwardSnowflake(rec, req); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if auth != "Bearer jwt-token" {
		t.Fatalf("authorization = %q", auth)
	}
	if tokenType != "KEYPAIR_JWT" {
		t.Fatalf("token type = %q", tokenType)
	}
}

func TestNormalizeSnowflakeChatURL(t *testing.T) {
	cases := map[string]string{
		"https://acme.snowflakecomputing.com":                                  "https://acme.snowflakecomputing.com/api/v2/cortex/inference:complete",
		"https://acme.snowflakecomputing.com/api/v2":                           "https://acme.snowflakecomputing.com/api/v2/cortex/inference:complete",
		"https://acme.snowflakecomputing.com/api/v2/cortex/inference:complete": "https://acme.snowflakecomputing.com/api/v2/cortex/inference:complete",
		"": "",
	}
	for in, want := range cases {
		if got := normalizeSnowflakeChatURL(in); got != want {
			t.Errorf("normalizeSnowflakeChatURL(%q) = %q, want %q", in, got, want)
		}
	}
}
