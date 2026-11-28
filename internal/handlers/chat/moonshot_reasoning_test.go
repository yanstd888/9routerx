package chat

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMoonshotClaudeAuthenticReasoning(t *testing.T) {
	capture := &upstreamCapture{}
	h := moonshotHandlerFixture(t, false, capture)
	body := `{"model":"moonshot/kimi-k3.1-pro","messages":[{"role":"assistant","content":[{"type":"thinking","thinking":"authentic Claude thinking"},{"type":"text","text":"prior answer"}]},{"role":"user","content":"continue"}]}`
	recorder := httptest.NewRecorder()
	h.HandleMessages(recorder, httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body)))
	if recorder.Code != http.StatusOK || !strings.Contains(capture.got(), `"reasoning_content":"authentic Claude thinking"`) {
		t.Fatalf("Claude reasoning lost: %d %s", recorder.Code, capture.got())
	}
}

func TestMoonshotResponsesAuthenticReasoning(t *testing.T) {
	capture := &upstreamCapture{}
	h := moonshotHandlerFixture(t, false, capture)
	body := `{"model":"moonshot/kimi-k3.1-pro","input":[{"role":"user","content":"hello"},{"type":"reasoning","content":[{"type":"reasoning_text","text":"authentic Responses thinking"}]},{"type":"function_call","call_id":"call1","name":"search","arguments":"{}"},{"type":"function_call_output","call_id":"call1","output":"found"},{"role":"user","content":"continue"}],"tools":[{"type":"function","name":"search","parameters":{"type":"object","properties":{}}}]}`
	recorder := httptest.NewRecorder()
	h.HandleResponses(recorder, httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body)))
	if recorder.Code != http.StatusOK || !strings.Contains(capture.got(), `"reasoning_content":"authentic Responses thinking"`) {
		t.Fatalf("Responses reasoning lost: %d %s", recorder.Code, capture.got())
	}
}
