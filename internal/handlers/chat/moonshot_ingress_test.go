package chat

import (
	json "encoding/json/v2"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"9routerx/proxy/internal/translator"
)

func TestMoonshotStrictEffortOnTranslatedClients(t *testing.T) {
	for _, format := range []string{"claude", "responses"} {
		for _, effort := range []string{`"none"`, `"mystery"`, `null`, `123`, `{}`} {
			t.Run(format+"/"+effort, func(t *testing.T) {
				capture := &upstreamCapture{}
				h := moonshotHandlerFixture(t, false, capture)
				body := `{"model":"moonshot/kimi-k3.1-pro","thinking":{"type":"enabled","effort":` + effort + `},"messages":[{"role":"user","content":"hello"}]}`
				path := "/v1/messages"
				if format == "responses" {
					path = "/v1/responses"
					body = `{"model":"moonshot/kimi-k3.1-pro","reasoning":{"effort":` + effort + `},"input":"hello"}`
				}
				recorder := httptest.NewRecorder()
				request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
				if format == "claude" {
					h.HandleMessages(recorder, request)
				} else {
					h.HandleResponses(recorder, request)
				}
				if recorder.Code != http.StatusOK {
					t.Fatalf("status=%d %s", recorder.Code, recorder.Body.String())
				}
				var sent map[string]any
				if err := json.Unmarshal([]byte(capture.got()), &sent); err != nil {
					t.Fatal(err)
				}
				var expected any
				if err := json.Unmarshal([]byte(effort), &expected); err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(sent["reasoning_effort"], expected) {
					t.Fatalf("translated effort lost: %s", capture.got())
				}
			})
		}
	}
}

func TestMoonshotIngressToolUnionAndAuthenticReasoning(t *testing.T) {
	capture := &upstreamCapture{}
	h := moonshotHandlerFixture(t, false, capture)
	body := `{"model":"moonshot/kimi-k3.1-pro","messages":[{"role":"assistant","content":null,"tool_calls":[{"id":"call1","type":"function","function":{"name":"search","arguments":"{}"}}]},{"role":"tool","tool_call_id":"call1","content":"found"},{"role":"user","content":"continue"}],"tools":[{"type":"function","function":{"name":"search","parameters":{"anyOf":[{"type":"object","properties":{"a":{"anyOf":[{"type":"string"},{"type":"null"}]},"shared":{"type":"string"}},"required":["a","shared"]},{"type":"object","properties":{"b":{"type":"string"},"shared":{"type":"number"}},"required":["b","shared"]}]}}}]}`
	recorder := httptest.NewRecorder()
	h.HandleChatCompletions(recorder, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d %s", recorder.Code, recorder.Body.String())
	}
	var sent map[string]any
	if err := json.Unmarshal([]byte(capture.got()), &sent); err != nil {
		t.Fatal(err)
	}
	messages := sent["messages"].([]any)
	if _, fabricated := messages[0].(map[string]any)["reasoning_content"]; fabricated {
		t.Fatal("generic injector fabricated reasoning")
	}
	if messages[1].(map[string]any)["name"] != "search" {
		t.Fatalf("tool name missing: %s", capture.got())
	}
	parameters := sent["tools"].([]any)[0].(map[string]any)["function"].(map[string]any)["parameters"].(map[string]any)
	if _, remaining := parameters["anyOf"]; remaining {
		t.Fatal("root anyOf not flattened")
	}
	if !reflect.DeepEqual(parameters["required"], []any{"shared"}) {
		t.Fatalf("union intersection lost: %+v", parameters)
	}
	property := parameters["properties"].(map[string]any)["a"].(map[string]any)
	if _, present := property["anyOf"]; !present {
		t.Fatalf("generic schema sanitizer destroyed nested union: %+v", parameters)
	}
}

func TestMoonshotPreservationDoesNotChangeOtherProviders(t *testing.T) {
	body := []byte(`{"messages":[{"role":"user","content":[{"type":"text","text":"clip"},{"type":"video_url","video_url":{"url":"https://example.invalid/clip.mp4"}}]},{"role":"assistant","content":"","partial":true}]}`)
	filtered, err := translator.FilterToOpenAIFormatJSON(body, translator.FilterOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(filtered), "video_url") || strings.Contains(string(filtered), "partial") {
		t.Fatalf("generic filter behavior changed: %s", filtered)
	}
	out, _, _ := wireHandler().prepareInboundThinking(t.Context(), "moonshot", "kimi-k3.1-pro", []byte(`{"reasoning_effort":"none"}`), false)
	if string(out) != `{"reasoning_effort":"none"}` {
		t.Fatalf("raw Moonshot intent changed: %s", out)
	}
}
