package executor

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"9routerx/proxy/internal/providers"
)

type commandCodeCapture struct {
	path    string
	headers http.Header
	body    map[string]any
}

func commandCodeFixture(t *testing.T, reply string) (*httptest.Server, *[]commandCodeCapture, *Request) {
	t.Helper()
	captures := &[]commandCodeCapture{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode upstream body: %v", err)
		}
		*captures = append(*captures, commandCodeCapture{path: r.URL.Path, headers: r.Header.Clone(), body: body})
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(reply))
	}))
	t.Cleanup(server.Close)
	req := &Request{
		APIKey: "command-code-test-key",
		Config: &providers.ProviderConfig{
			BaseURL:    server.URL,
			AuthHeader: "Authorization",
			AuthScheme: "bearer",
			Format:     providers.FormatOpenAI,
		},
		ConnData: map[string]any{"baseUrl": server.URL},
	}
	return server, captures, req
}

func TestForwardCommandCodeChatLane(t *testing.T) {
	_, captures, req := commandCodeFixture(t, `{"id":"chatcmpl-1","object":"chat.completion","created":1,"model":"gpt-5.5","choices":[{"index":0,"message":{"role":"assistant","content":"chat-ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":3,"total_tokens":5}}`)
	req.ModelName = "cmd/gpt-5.5"
	req.Body = []byte(`{"model":"cmd/gpt-5.5","messages":[{"role":"user","content":"hi"}],"max_tokens":300000,"max_completion_tokens":7}`)
	rec := httptest.NewRecorder()
	if err := ForwardCommandCode(rec, req); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(rec.Body.String(), "chat-ok") {
		t.Fatalf("completion body = %s", rec.Body.String())
	}
	if len(*captures) != 1 {
		t.Fatalf("upstream calls = %d", len(*captures))
	}
	call := (*captures)[0]
	if call.path != commandCodeChatPath {
		t.Fatalf("path = %q, want %q", call.path, commandCodeChatPath)
	}
	if got := call.headers.Get("Authorization"); got != "Bearer command-code-test-key" {
		t.Fatalf("authorization = %q", got)
	}
	for _, header := range []string{"x-command-code-version", "x-cli-environment", "x-project-slug", "x-session-id"} {
		if got := call.headers.Get(header); got != "" {
			t.Errorf("CLI header %s leaked: %q", header, got)
		}
	}
	if got := call.body["model"]; got != "gpt-5.5" {
		t.Errorf("model = %v, want gpt-5.5", got)
	}
	if got := call.body["max_tokens"]; got != float64(commandCodeMaxTokens) {
		t.Errorf("max_tokens = %v, want %d", got, commandCodeMaxTokens)
	}
	if _, ok := call.body["max_completion_tokens"]; ok {
		t.Error("max_completion_tokens was not normalized away")
	}
}

func TestForwardCommandCodeMessagesLane(t *testing.T) {
	_, captures, req := commandCodeFixture(t, `{"id":"msg_1","type":"message","role":"assistant","model":"claude-opus-4-7","content":[{"type":"text","text":"claude-ok"}],"stop_reason":"end_turn","usage":{"input_tokens":2,"output_tokens":3}}`)
	req.ModelName = "claude-opus-4-7"
	req.Body = []byte(`{"model":"claude-opus-4-7","messages":[{"role":"system","content":"be concise"},{"role":"user","content":"hi"}],"tools":[{"type":"function","function":{"name":"lookup","description":"look up","parameters":{"type":"object"}}}],"max_tokens":32}`)
	rec := httptest.NewRecorder()
	if err := ForwardCommandCode(rec, req); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(rec.Body.String(), "claude-ok") {
		t.Fatalf("translated body = %s", rec.Body.String())
	}
	if len(*captures) != 1 {
		t.Fatalf("upstream calls = %d", len(*captures))
	}
	call := (*captures)[0]
	if call.path != commandCodeMessagesPath {
		t.Fatalf("path = %q, want %q", call.path, commandCodeMessagesPath)
	}
	if got := call.headers.Get("anthropic-version"); got != "2023-06-01" {
		t.Fatalf("anthropic-version = %q", got)
	}
	if _, ok := call.body["system"]; !ok {
		t.Error("system prompt was not lifted to the Claude top level")
	}
	tools, _ := call.body["tools"].([]any)
	if len(tools) != 1 {
		t.Fatalf("tools = %v", call.body["tools"])
	}
	tool, _ := tools[0].(map[string]any)
	if _, ok := tool["input_schema"]; !ok {
		t.Fatalf("tool = %v, want input_schema", tool)
	}
}

func TestCommandCodeModelAndTokenHelpers(t *testing.T) {
	if got := commandCodeWireModel("cmd/mimo-v2.5"); got != "xiaomi/mimo-v2.5" {
		t.Fatalf("wire model = %q", got)
	}
	if got := commandCodeWireModel("command-code/deepseek/deepseek-v4-pro"); got != "deepseek/deepseek-v4-pro" {
		t.Fatalf("wire model = %q", got)
	}
	if !commandCodeClaudeModel("claude-sonnet-5-5") || commandCodeClaudeModel("gpt-5.5") {
		t.Fatal("Claude target routing mismatch")
	}
	cases := []struct {
		body map[string]any
		want int
	}{
		{map[string]any{}, commandCodeDefaultTokens},
		{map[string]any{"max_tokens": float64(-1)}, commandCodeDefaultTokens},
		{map[string]any{"max_tokens": float64(42)}, 42},
		{map[string]any{"max_output_tokens": float64(300000)}, commandCodeMaxTokens},
	}
	for _, tc := range cases {
		if got := commandCodeTokenCap(tc.body); got != tc.want {
			t.Errorf("token cap %v = %d, want %d", tc.body, got, tc.want)
		}
	}
}
