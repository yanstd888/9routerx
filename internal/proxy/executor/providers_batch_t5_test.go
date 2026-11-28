// 9RouterX — port and modifications by https://github.com/yanstd888/ — https://t.me/aiclub_id
package executor

import (
	json "encoding/json/v2"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"9routerx/proxy/internal/providers"
	"9routerx/proxy/internal/proxy"
)

func TestPendingT5ChatProvidersWireContract(t *testing.T) {
	for _, tc := range []struct {
		provider string
		path     string
		header   string
		value    string
		model    string
	}{
		{"uc-direct", "/api/v1/chat/completions", "X-API-Key", "fixture-key", "glm-5.2"},
		{"synthetic", "/openai/v1/chat/completions", "Authorization", "Bearer fixture-key", "hf:openai/gpt-oss-120b"},
		{"river", "/v1/chat/completions", "Authorization", "Bearer fixture-key", "Qwen/Qwen3.5-9B"},
		{"alibaba-cn", "/compatible-mode/v1/chat/completions", "Authorization", "Bearer fixture-key", "qwen3.8-max"},
		{"agnes-cn", "/v1/chat/completions", "Authorization", "Bearer fixture-key", "agnes-2.5-flash"},
		{"volcengine-coding-plan", "/api/coding/v3/chat/completions", "Authorization", "Bearer fixture-key", "fixture-model"},
		{"volcengine-agent-plan", "/api/plan/v3/chat/completions", "Authorization", "Bearer fixture-key", "fixture-model"},
	} {
		t.Run(tc.provider, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != tc.path {
					t.Errorf("request = %s %s, want POST %s", r.Method, r.URL.Path, tc.path)
				}
				if got := r.Header.Get(tc.header); got != tc.value {
					t.Errorf("%s = %q, want %q", tc.header, got, tc.value)
				}
				var body struct {
					Model string `json:"model"`
				}
				if err := json.Unmarshal(mustReadT5Body(t, r.Body), &body); err != nil {
					t.Fatal(err)
				}
				if body.Model != tc.model {
					t.Errorf("model = %q, want %q", body.Model, tc.model)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"fixture-ok"}}]}`))
			}))
			defer server.Close()
			cfg := providers.KnownProviders[tc.provider]
			cfg.BaseURL = server.URL + tc.path
			RegisterAll()
			forward := Get(tc.provider)
			if forward == nil {
				t.Fatalf("%s has no registered executor", tc.provider)
			}
			rec := httptest.NewRecorder()
			body, err := json.Marshal(map[string]any{"model": tc.model, "messages": []any{map[string]string{"role": "user", "content": "hello"}}})
			if err != nil {
				t.Fatal(err)
			}
			if err := forward(rec, &Request{Ctx: proxy.WithAllowLoopback(t.Context()), Client: server.Client(), Config: &cfg, Provider: tc.provider, APIKey: "fixture-key", Body: body}); err != nil {
				t.Fatal(err)
			}
			if rec.Code != http.StatusOK {
				t.Fatalf("response = %d: %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func mustReadT5Body(t *testing.T, reader io.Reader) []byte {
	t.Helper()
	body, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	return body
}
