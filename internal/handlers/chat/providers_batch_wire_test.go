// 9RouterX — port and modifications by https://github.com/yanstd888/ — https://t.me/aiclub_id
package chat

import (
	json "encoding/json/v2"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"9routerx/proxy/internal/db"
	"9routerx/proxy/internal/providers"
)

func TestProviderBatchClaudeWireFormat(t *testing.T) {
	for _, provider := range []string{"zai", "tabitoken", "agentrouter", "wafer"} {
		t.Run(provider, func(t *testing.T) {
			cfg := providers.KnownProviders[provider]
			if cfg.Format != providers.FormatClaude {
				t.Fatalf("format = %q, want Claude", cfg.Format)
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if got := r.Header.Get("Anthropic-Version"); got != "2023-06-01" {
					t.Errorf("Anthropic-Version = %q", got)
				}
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Fatal(err)
				}
				var payload map[string]any
				if err := json.Unmarshal(body, &payload); err != nil {
					t.Fatal(err)
				}
				if payload["system"] == nil || payload["max_tokens"] == nil {
					t.Errorf("OpenAI body was not converted to Claude: %s", body)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"id":"msg-fixture","type":"message","role":"assistant","model":"fixture-model","content":[{"type":"text","text":"fixture-ok"}],"stop_reason":"end_turn","usage":{"input_tokens":2,"output_tokens":2}}`)
			}))
			defer server.Close()
			database, cleanup := setupChatTestDB(t)
			defer cleanup()
			h := NewChatHandler(db.NewRepo(database))
			rec := httptest.NewRecorder()
			if err := h.tryForwardWithConnection(forwardRequestParams{
				Ctx: t.Context(), W: rec, Provider: provider, Model: "fixture-model",
				ConnData: &ConnectionData{APIKey: "fixture-key", BaseURL: server.URL + "/v1/messages"},
				Body:     []byte(`{"model":"fixture-model","messages":[{"role":"system","content":"fixture-system"},{"role":"user","content":"hello"}],"max_tokens":20}`),
				Endpoint: "/v1/chat/completions",
			}); err != nil {
				t.Fatal(err)
			}
			if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "choices") || !strings.Contains(rec.Body.String(), "fixture-ok") {
				t.Fatalf("Claude response was not translated: %d %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestProviderBatchLocalOptionalKey(t *testing.T) {
	for _, key := range []string{"", "fixture-key"} {
		t.Run("key="+key, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				want := ""
				if key != "" {
					want = "Bearer " + key
				}
				if got := r.Header.Get("Authorization"); got != want {
					t.Errorf("Authorization = %q, want %q", got, want)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"fixture-ok"}}]}`)
			}))
			defer server.Close()
			database, cleanup := setupChatTestDB(t)
			defer cleanup()
			h := NewChatHandler(db.NewRepo(database))
			_, virtual, err := h.GetBestConnectionWithContext(t.Context(), "lm-studio", "", nil, "fixture-model")
			if err != nil || virtual == nil {
				t.Fatalf("local optional-key lane has no virtual connection: %v", err)
			}
			rec := httptest.NewRecorder()
			if err := h.tryForwardWithConnection(forwardRequestParams{
				Ctx: t.Context(), W: rec, Provider: "lm-studio", Model: "fixture-model",
				ConnData: &ConnectionData{APIKey: key, BaseURL: server.URL + "/v1/chat/completions"},
				Body:     []byte(`{"model":"fixture-model","messages":[{"role":"user","content":"hello"}]}`), Endpoint: "/v1/chat/completions",
			}); err != nil {
				t.Fatal(err)
			}
			if rec.Code != http.StatusOK {
				t.Fatalf("response = %d: %s", rec.Code, rec.Body.String())
			}
		})
	}
}
