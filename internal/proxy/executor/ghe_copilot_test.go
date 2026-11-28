package executor

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"9routerx/proxy/internal/providers"
	"9routerx/proxy/internal/translator"
)

type gheFixtureTransport func(*http.Request) (*http.Response, error)

func (f gheFixtureTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

const gheChatFixture = "data: {\"id\":\"fixture-chat\",\"object\":\"chat.completion.chunk\",\"model\":\"gpt-4o-mini\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"fixture-ok\"},\"finish_reason\":null}]}\n\ndata: {\"id\":\"fixture-chat\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":2,\"completion_tokens\":3,\"total_tokens\":5}}\n\ndata: [DONE]\n\n"
const gheResponsesFixture = "event: response.created\ndata: {\"type\":\"response.created\",\"response\":{\"id\":\"resp-fixture\",\"object\":\"response\",\"model\":\"gpt-5-mini\",\"status\":\"in_progress\",\"output\":[]}}\n\nevent: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"fixture-ok\",\"output_index\":0,\"content_index\":0}\n\nevent: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp-fixture\",\"object\":\"response\",\"status\":\"completed\",\"model\":\"gpt-5-mini\",\"output\":[{\"id\":\"item-fixture\",\"type\":\"message\",\"role\":\"assistant\",\"content\":[{\"type\":\"output_text\",\"text\":\"fixture-ok\"}]}],\"usage\":{\"input_tokens\":2,\"output_tokens\":3,\"total_tokens\":5}}}\n\n"
const gheMessagesFixture = "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg-fixture\",\"type\":\"message\",\"role\":\"assistant\",\"model\":\"claude-sonnet-4.6\",\"content\":[],\"usage\":{\"input_tokens\":2,\"cache_read_input_tokens\":4}}}\n\nevent: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\nevent: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"fixture-ok\"}}\n\nevent: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\nevent: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":3}}\n\nevent: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"

func TestGHECopilotRequestWireResponseMatrix(t *testing.T) {
	for _, model := range []string{"gpt-4o-mini", "gpt-5-mini", "claude-sonnet-4.6"} {
		for _, source := range []string{"chat", "responses", "messages"} {
			for _, stream := range []bool{false, true} {
				t.Run(model+"/"+source+"/"+map[bool]string{true: "stream", false: "json"}[stream], func(t *testing.T) {
					ctx := translator.WithUsageCapture(t.Context())
					ctx = translator.WithRequestedModel(ctx, model)
					body := `{"model":"ghe-copilot/` + model + `","messages":[{"role":"user","content":"hello"}],"max_tokens":32}`
					if source == "responses" {
						ctx = translator.WithClientFormat(ctx, translator.ClientFormatResponses)
						body = `{"model":"` + model + `","input":[{"role":"user","content":[{"type":"input_text","text":"hello"}]}],"max_output_tokens":32}`
						if model != "gpt-5-mini" {
							ctx = translator.WithResponsesBridge(ctx)
						}
					}
					if source == "messages" && model == "claude-sonnet-4.6" {
						body = `{"model":"` + model + `","system":"fixture-system","messages":[{"role":"user","content":[{"type":"text","text":"hello"}]}],"max_tokens":32}`
					}
					fixture, path := gheChatFixture, "/chat/completions"
					if model == "gpt-5-mini" {
						fixture, path = gheResponsesFixture, "/responses"
					}
					if model == "claude-sonnet-4.6" {
						fixture, path = gheMessagesFixture, "/v1/messages"
					}
					calls := 0
					client := &http.Client{Transport: gheFixtureTransport(func(r *http.Request) (*http.Response, error) {
						calls++
						if r.URL.Host != "api.enterprise.example" || r.URL.Path != path {
							t.Errorf("wire URL=%s want API %s", r.URL, path)
						}
						if r.Header.Get("Authorization") != "Bearer derived" || r.Header.Get("Accept") != "text/event-stream" {
							t.Errorf("headers=%v", r.Header)
						}
						var wire map[string]any
						raw, _ := io.ReadAll(r.Body)
						if err := json.Unmarshal(raw, &wire); err != nil {
							t.Fatal(err)
						}
						if wire["model"] != model || wire["stream"] != true {
							t.Errorf("model/stream=%s", raw)
						}
						if path == "/responses" {
							if wire["input"] == nil || wire["messages"] != nil {
								t.Errorf("Responses wrong body %s", raw)
							}
						} else if wire["input"] != nil || wire["messages"] == nil {
							t.Errorf("Chat/Messages wrong body %s", raw)
						}
						if path == "/v1/messages" && (wire["max_tokens"] == nil || r.Header.Get("Anthropic-Version") != "2023-06-01") {
							t.Errorf("Messages headers/body %v %s", r.Header, raw)
						}
						return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(fixture))}, nil
					})}
					cfg := providers.KnownProviders["ghe-copilot"]
					req := &Request{Ctx: ctx, Client: client, Config: &cfg, Provider: "ghe-copilot", APIKey: "derived", Body: []byte(body), IsStream: stream, ClaudeClient: source == "messages", TranslateResp: source == "messages" && model != "claude-sonnet-4.6", StartTime: time.Now(), ConnData: map[string]any{"gheUrl": "https://enterprise.example", "copilotApiUrl": "https://api.enterprise.example", "copilotProxyUrl": "https://nes.enterprise.example"}}
					rec := httptest.NewRecorder()
					if err := ForwardGHECopilot(rec, req); err != nil {
						t.Fatalf("forward: %v", err)
					}
					if calls != 1 || rec.Code != 200 || !strings.Contains(rec.Body.String(), "fixture-ok") {
						t.Fatalf("calls=%d status=%d body=%s", calls, rec.Code, rec.Body)
					}
					if stream {
						if !strings.Contains(rec.Header().Get("Content-Type"), "text/event-stream") {
							t.Fatalf("not SSE %v", rec.Header())
						}
					} else {
						if !jsontext.Value(rec.Body.Bytes()).IsValid() || rec.Header().Get("Content-Type") != "application/json" {
							t.Fatalf("not JSON %s %v", rec.Body, rec.Header())
						}
					}
					if source == "responses" && !strings.Contains(rec.Body.String(), "response") {
						t.Errorf("not Responses %s", rec.Body)
					}
					if source == "messages" && !strings.Contains(rec.Body.String(), "message") {
						t.Errorf("not Messages %s", rec.Body)
					}
				})
			}
		}
	}
}

func TestGHEBodyNativeVisionAndTools(t *testing.T) {
	body := []byte(`{"model":"claude-sonnet-4.6","system":[{"type":"text","text":"system","cache_control":{"type":"ephemeral"}}],"messages":[{"role":"assistant","content":[{"type":"thinking","thinking":"thought","signature":"signed"},{"type":"tool_use","id":"call-fixture","name":"lookup","input":{"q":"x"}}]},{"role":"user","content":[{"type":"tool_result","tool_use_id":"call-fixture","content":"ok"},{"type":"image","source":{"type":"base64","media_type":"image/png","data":"AAAA"}}]}],"tools":[{"name":"lookup","input_schema":{"type":"object","properties":{"q":{"type":"string"}}}}],"max_tokens":32}`)
	cfg := providers.KnownProviders["ghe-copilot"]
	req := &Request{Ctx: t.Context(), Config: &cfg, Body: body, ClaudeClient: true}
	got, err := gheBody(req, "claude-sonnet-4.6", providers.FormatClaude)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"thinking", "signed", "tool_use", "tool_result", "cache_control", "input_schema"} {
		if !strings.Contains(string(got), field) {
			t.Errorf("native field lost %s in %s", field, got)
		}
	}
	if gheHeaders(req, got, providers.FormatClaude)["copilot-vision-request"] != "true" {
		t.Fatal("missing vision header")
	}
	if strings.Contains(string(got), `"function"`) {
		t.Fatal("native tools converted twice")
	}
}
