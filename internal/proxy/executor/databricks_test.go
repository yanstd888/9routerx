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

func TestForwardDatabricksRequiresWorkspaceURL(t *testing.T) {
	rec := httptest.NewRecorder()
	req := &Request{Body: []byte(`{"model":"databricks-gpt-5","messages":[]}`), Config: &providers.ProviderConfig{}}
	err := ForwardDatabricks(rec, req)
	if err == nil {
		t.Fatal("missing workspace URL was accepted")
	}
	if !strings.Contains(err.Error(), "requires a Base URL") {
		t.Fatalf("error = %v", err)
	}
}

func TestForwardDatabricksRefusesPinnedPlaceholder(t *testing.T) {
	rec := httptest.NewRecorder()
	req := &Request{
		Body:     []byte(`{"model":"databricks-gpt-5","messages":[]}`),
		ConnData: map[string]any{"baseUrl": "https://adb-0000000000000000.0.azuredatabricks.net/serving-endpoints"},
		Config:   &providers.ProviderConfig{},
	}
	err := ForwardDatabricks(rec, req)
	if err == nil {
		t.Fatal("pinned placeholder workspace was accepted")
	}
	if !strings.Contains(err.Error(), "requires a Base URL") {
		t.Fatalf("error = %v", err)
	}
}

func TestForwardDatabricksUsesWorkspaceURL(t *testing.T) {
	var path, auth string
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path, auth = r.URL.Path, r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl-1","object":"chat.completion","created":1,"model":"databricks-gpt-5","choices":[{"index":0,"message":{"role":"assistant","content":"databricks-ok"},"finish_reason":"stop"}]}`))
	}))
	defer server.Close()

	req := &Request{
		Ctx:    context.Background(),
		APIKey: "databricks-test-key",
		Client: server.Client(),
		Body:   []byte(`{"model":"databricks-gpt-5","messages":[{"role":"user","content":"hi"}]}`),
		ConnData: map[string]any{
			"baseUrl": server.URL + "/serving-endpoints/",
		},
		Config: &providers.ProviderConfig{
			BaseURL:    "https://adb-0000000000000000.0.azuredatabricks.net/serving-endpoints",
			AuthHeader: "Authorization",
			AuthScheme: "bearer",
			Format:     providers.FormatOpenAI,
		},
	}
	rec := httptest.NewRecorder()
	if err := ForwardDatabricks(rec, req); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if path != "/serving-endpoints/chat/completions" {
		t.Fatalf("path = %q", path)
	}
	if auth != "Bearer databricks-test-key" {
		t.Fatalf("authorization = %q", auth)
	}
	if got := body["model"]; got != "databricks-gpt-5" {
		t.Fatalf("model = %v", got)
	}
	if !strings.Contains(rec.Body.String(), "databricks-ok") {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestNormalizeDatabricksChatURL(t *testing.T) {
	cases := map[string]string{
		"https://adb-1.7.azuredatabricks.net/serving-endpoints":                "https://adb-1.7.azuredatabricks.net/serving-endpoints/chat/completions",
		"https://adb-1.7.azuredatabricks.net/serving-endpoints/chat/completions": "https://adb-1.7.azuredatabricks.net/serving-endpoints/chat/completions",
		"": "",
	}
	for in, want := range cases {
		if got := normalizeDatabricksChatURL(in); got != want {
			t.Errorf("normalizeDatabricksChatURL(%q) = %q, want %q", in, got, want)
		}
	}
}
