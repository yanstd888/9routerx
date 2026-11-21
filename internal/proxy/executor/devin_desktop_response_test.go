package executor

import (
	"bytes"
	json "encoding/json/v2"
	"net/http/httptest"
	"strings"
	"testing"

	"9routerx/proxy/internal/translator"
)

func TestDevinDesktopResponseFixture(t *testing.T) {
	for _, stream := range []bool{false, true} {
		name := "json"
		if stream {
			name = "sse"
		}
		t.Run(name, func(t *testing.T) {
			client, authBody, chatBody := devinDesktopTestClient(t, devinDesktopTestChat(t))
			req := devinDesktopTestRequest(client, []byte(`{"model":"claude-opus-5-low","messages":[{"role":"user","content":"hi"}]}`))
			req.IsStream = stream
			var capture bytes.Buffer
			req.ResponseBuf = &capture
			w := httptest.NewRecorder()
			if err := ForwardDevinDesktop(w, req); err != nil {
				t.Fatal(err)
			}
			if !authBody.closed.Load() || !chatBody.closed.Load() {
				t.Error("upstream body not closed")
			}
			if capture.Len() == 0 {
				t.Error("response not captured")
			}
			usage := translator.GetAndClearUsage(req.Ctx)
			if usage == nil || usage.PromptTokens != 12 || usage.CompletionTokens != 4 || usage.GetCachedTokens() != 5 || usage.CacheCreationInputTokens != 3 {
				t.Fatalf("captured usage = %+v", usage)
			}
			payload := w.Body.Bytes()
			if stream {
				if !strings.HasPrefix(w.Header().Get("Content-Type"), "text/event-stream") {
					t.Error("not SSE")
				}
				if strings.Count(string(payload), "data: [DONE]") != 1 {
					t.Error("missing/duplicate DONE")
				}
				folded, ok := sseToOpenAIJSON(payload)
				if !ok {
					t.Fatal("unable to fold SSE")
				}
				payload = folded
			} else if w.Header().Get("Content-Type") != "application/json" {
				t.Error("not JSON")
			}
			var completion struct {
				Model   string `json:"model"`
				Choices []struct {
					Message struct {
						Content  string                      `json:"content"`
						Thinking string                      `json:"reasoning_content"`
						Calls    []devinDesktopCollectedCall `json:"tool_calls"`
					} `json:"message"`
					Reason string `json:"finish_reason"`
				} `json:"choices"`
				Usage devinDesktopUsage `json:"usage"`
			}
			if err := json.Unmarshal(payload, &completion); err != nil {
				t.Fatal(err)
			}
			if len(completion.Choices) != 1 {
				t.Fatalf("choices %s", payload)
			}
			choice := completion.Choices[0]
			if choice.Message.Content != "hello" || choice.Message.Thinking != "hmm" || choice.Reason != "tool_calls" {
				t.Errorf("completion = %s", payload)
			}
			calls := choice.Message.Calls
			if len(calls) != 1 || calls[0].ID != "c1" || calls[0].Function.Name != "look" || calls[0].Function.Arguments != `{"x":1} ` {
				t.Errorf("calls = %+v", calls)
			}
			if completion.Usage.TotalTokens != 16 || completion.Usage.CacheWriteTokens != 3 {
				t.Errorf("usage %+v", completion.Usage)
			}
		})
	}
}

func TestDevinDesktopClientModes(t *testing.T) {
	for _, mode := range []string{"claude-json", "claude-sse", "responses-json", "responses-sse", "responses-chat-body"} {
		t.Run(mode, func(t *testing.T) {
			client, _, _ := devinDesktopTestClient(t, devinDesktopTestChat(t))
			body := []byte(`{"model":"claude-opus-5-low","system":"Be concise","messages":[{"role":"user","content":"hello"}],"max_tokens":200}`)
			if strings.HasPrefix(mode, "responses") {
				body = []byte(`{"model":"claude-opus-5-low","instructions":"Be concise","input":"hello"}`)
			}
			if mode == "responses-chat-body" {
				body = []byte(`{"model":"claude-opus-5-low","messages":[{"role":"user","content":"hello"}]}`)
			}
			req := devinDesktopTestRequest(client, body)
			req.IsStream = strings.HasSuffix(mode, "sse")
			req.ClaudeClient = strings.HasPrefix(mode, "claude")
			if strings.HasPrefix(mode, "responses") {
				req.Ctx = translator.WithClientFormat(req.Ctx, translator.ClientFormatResponses)
			}
			w := httptest.NewRecorder()
			if err := ForwardDevinDesktop(w, req); err != nil {
				t.Fatal(err)
			}
			text := w.Body.String()
			if req.IsStream {
				want := []string{"event: message_start", "thinking_delta", "input_json_delta", "event: message_stop"}
				if strings.HasPrefix(mode, "responses") {
					want = []string{"event: response.created", "response.reasoning", "response.function_call_arguments.delta", "event: response.completed"}
				}
				for _, part := range want {
					if !strings.Contains(text, part) {
						t.Errorf("missing %q in %s", part, text)
					}
				}
				if strings.Contains(text, `"object":"chat.completion.chunk"`) {
					t.Error("raw Chat leaked")
				}
				return
			}
			var envelope struct {
				Type   string `json:"type"`
				Object string `json:"object"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil {
				t.Fatal(err)
			}
			if req.ClaudeClient && envelope.Type != "message" {
				t.Fatalf("Claude JSON = %s", text)
			}
			if !req.ClaudeClient && envelope.Object != "response" {
				t.Fatalf("Responses JSON = %s", text)
			}
			for _, part := range []string{"hello", "look", "hmm"} {
				if !strings.Contains(text, part) {
					t.Errorf("lost %q in %s", part, text)
				}
			}
		})
	}
}

func TestDevinDesktopStopReasons(t *testing.T) {
	for _, tc := range []struct {
		name   string
		stop   uint64
		reason string
	}{
		{"stop", 1, "stop"}, {"length", 3, "length"}, {"tools", 10, "tool_calls"}, {"filter", 11, "content_filter"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			payload := append(devinDesktopTestString(3, "answer"), devinDesktopTestNumber(5, tc.stop)...)
			chat := append(devinDesktopTestFrame(payload, 0), devinDesktopTestFrame([]byte(`{}`), 2)...)
			client, _, _ := devinDesktopTestClient(t, chat)
			w := httptest.NewRecorder()
			if err := ForwardDevinDesktop(w, devinDesktopTestRequest(client, []byte(`{"messages":[]}`))); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(w.Body.String(), `"finish_reason":"`+tc.reason+`"`) {
				t.Error(w.Body.String())
			}
		})
	}
}
