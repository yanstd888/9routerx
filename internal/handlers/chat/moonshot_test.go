package chat

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"9routerx/proxy/internal/db"
	"9routerx/proxy/internal/proxy"
	"9routerx/proxy/internal/translator"
)

const moonshotUsageFixture = `"usage":{"prompt_tokens":20,"completion_tokens":5,"total_tokens":25,"cached_tokens":7}`
const moonshotJSONFixture = `{"id":"chatcmpl-moonshot","object":"chat.completion","created":1,"model":"kimi-k3.1-pro","choices":[{"index":0,"message":{"role":"assistant","content":"moonshot-ok","reasoning_content":"authentic answer reasoning"},"finish_reason":"stop"}],` + moonshotUsageFixture + `}`
const moonshotSSEFixture = "data: {\"id\":\"chatcmpl-moonshot\",\"model\":\"kimi-k3.1-pro\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"reasoning_content\":\"authentic answer reasoning\",\"content\":\"moonshot-ok\"},\"finish_reason\":null}]}\n\ndata: {\"id\":\"chatcmpl-moonshot\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: {\"id\":\"chatcmpl-moonshot\",\"choices\":[]," + moonshotUsageFixture + "}\n\ndata: [DONE]\n\n"

func moonshotHandlerFixture(t *testing.T, stream bool, capture *upstreamCapture) *ChatHandler {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			return
		}
		capture.record(raw)
		if r.Method != http.MethodPost || r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer moonshot-fixture-key" {
			t.Errorf("wrong Moonshot wire: %s %s %s", r.Method, r.URL.Path, r.Header.Get("Authorization"))
		}
		if stream {
			w.Header().Set("Content-Type", "text/event-stream")
			if _, err := io.WriteString(w, moonshotSSEFixture); err != nil {
				t.Error(err)
			}
		} else {
			w.Header().Set("Content-Type", "application/json")
			if _, err := io.WriteString(w, moonshotJSONFixture); err != nil {
				t.Error(err)
			}
		}
	}))
	t.Cleanup(server.Close)
	database, cleanup := setupChatTestDB(t)
	t.Cleanup(cleanup)
	seedConnDB(t, database, "moonshot", "moonshot-fixture", "moonshot-fixture-key", server.URL)
	return NewChatHandler(db.NewRepo(database))
}

func TestMoonshotClientFormatsAndUsage(t *testing.T) {
	for _, format := range []string{"chat", "claude", "responses"} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%t", format, stream), func(t *testing.T) {
				capture := &upstreamCapture{}
				h := moonshotHandlerFixture(t, stream, capture)
				path := "/v1/chat/completions"
				body := fmt.Sprintf(`{"model":"moonshot/kimi-k3.1-pro","stream":%t,"messages":[{"role":"user","content":"hello"}]}`, stream)
				switch format {
				case "claude":
					path = "/v1/messages"
					body = fmt.Sprintf(`{"model":"moonshot/kimi-k3.1-pro","stream":%t,"max_tokens":20,"messages":[{"role":"user","content":"hello"}]}`, stream)
				case "responses":
					path = "/v1/responses"
					body = fmt.Sprintf(`{"model":"moonshot/kimi-k3.1-pro","stream":%t,"input":"hello"}`, stream)
				}
				recorder := httptest.NewRecorder()
				request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
				switch format {
				case "chat":
					h.HandleChatCompletions(recorder, request)
				case "claude":
					h.HandleMessages(recorder, request)
				case "responses":
					h.HandleResponses(recorder, request)
				}
				if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "moonshot-ok") {
					t.Fatalf("client response = %d %s", recorder.Code, recorder.Body.String())
				}
				var sent map[string]any
				if err := json.Unmarshal([]byte(capture.got()), &sent); err != nil {
					t.Fatal(err)
				}
				if sent["model"] != "kimi-k3.1-pro" || sent["messages"] == nil || sent["input"] != nil || sent["reasoning_effort"] != "max" || sent["stream"] != stream {
					t.Fatalf("prepared input converted twice or lost: %s", capture.got())
				}
				if stream && sent["stream_options"].(map[string]any)["include_usage"] != true {
					t.Fatal("usage not requested")
				}
				out := recorder.Body.String()
				if format == "responses" {
					if !strings.Contains(out, `"cached_tokens":7`) || !strings.Contains(out, `"input_tokens_details"`) {
						t.Fatalf("Responses cache usage lost: %s", out)
					}
					if stream && !strings.Contains(out, "response.completed") {
						t.Fatal("Responses terminal missing")
					}
				} else if format == "claude" {
					if !strings.Contains(out, `"cache_read_input_tokens":7`) {
						t.Fatalf("Claude cache usage lost: %s", out)
					}
					if stream && !strings.Contains(out, "message_stop") {
						t.Fatal("Claude terminal missing")
					}
				} else if !strings.Contains(out, `"cached_tokens":7`) {
					t.Fatalf("flat cache usage lost: %s", out)
				}
			})
		}
	}
}

func TestMoonshotIngressStrictEffort(t *testing.T) {
	for _, effort := range []string{`"none"`, `"off"`, `"mystery"`, `""`, `null`, `123`, `false`, `{}`, `[]`} {
		t.Run(effort, func(t *testing.T) {
			capture := &upstreamCapture{}
			h := moonshotHandlerFixture(t, false, capture)
			body := `{"model":"moonshot/kimi-k3.1-pro","reasoning_effort":` + effort + `,"messages":[{"role":"user","content":"hello"}]}`
			recorder := httptest.NewRecorder()
			h.HandleChatCompletions(recorder, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body)))
			if recorder.Code != http.StatusOK {
				t.Fatalf("status=%d %s", recorder.Code, recorder.Body.String())
			}
			var sent map[string]jsontext.Value
			if err := json.Unmarshal([]byte(capture.got()), &sent); err != nil {
				t.Fatal(err)
			}
			if string(sent["reasoning_effort"]) != effort {
				t.Fatalf("effort clamped: %s", capture.got())
			}
		})
	}
}

func TestMoonshotIngressReasoningVideoPartial(t *testing.T) {
	capture := &upstreamCapture{}
	h := moonshotHandlerFixture(t, false, capture)
	body := `{"model":"moonshot/kimi-k3.1-pro","messages":[{"role":"assistant","content":"prior","reasoning_content":"authentic prior"},{"role":"user","content":[{"type":"text","text":"clip"},{"type":"video_url","video_url":{"url":"https://example.invalid/clip.mp4"}}]},{"role":"assistant","content":"","partial":true,"name":"prefix"}]}`
	recorder := httptest.NewRecorder()
	h.HandleChatCompletions(recorder, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d %s", recorder.Code, recorder.Body.String())
	}
	for _, want := range []string{`"reasoning_content":"authentic prior"`, `"video_url"`, `"partial":true`, `"name":"prefix"`} {
		if !strings.Contains(capture.got(), want) {
			t.Errorf("ingress lost %s: %s", want, capture.got())
		}
	}
	if strings.Contains(capture.got(), `"reasoning_content":" "`) {
		t.Fatal("fabricated reasoning")
	}
}

func TestMoonshotErrorStatusSurvives(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "4")
		w.WriteHeader(http.StatusBadRequest)
		if _, err := io.WriteString(w, `{"error":{"type":"invalid_request_error","message":"invalid reasoning_effort"}}`); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	h := &ChatHandler{}
	err := h.tryForwardWithConnection(forwardRequestParams{Ctx: translator.WithUsageCapture(t.Context()), W: httptest.NewRecorder(), Provider: "moonshot", Model: "kimi-k3.1-pro", ConnData: &ConnectionData{APIKey: "fixture", BaseURL: server.URL}, Body: []byte(`{"model":"kimi-k3.1-pro","reasoning_effort":"none","messages":[{"role":"user","content":"hello"}]}`), Endpoint: "/v1/chat/completions"})
	if err == nil {
		t.Fatal("upstream rejection lost")
	}
	if !strings.Contains(err.Error(), "400") || !strings.Contains(err.Error(), "invalid reasoning_effort") {
		t.Fatalf("error = %v", err)
	}
	var upstreamErr *proxy.UpstreamError
	if !errors.As(err, &upstreamErr) || upstreamErr.StatusCode != http.StatusBadRequest || upstreamErr.Header.Get("Retry-After") != "4" {
		t.Fatalf("typed status/header lost: %v", err)
	}
}
