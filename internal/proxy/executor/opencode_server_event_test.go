package executor

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"9routerx/proxy/internal/translator"
)

func opencodeServerEventFrame(event, data string) string {
	return "event: " + event + "\ndata: " + data + "\n\n"
}

func opencodeServerStreamFixture(t *testing.T) (*opencodeServerFixture, string) {
	f := newOpenCodeServerFixture(t)
	frames := []string{
		opencodeServerEventFrame("message.updated", `{"type":"message.updated","properties":{"sessionID":"ses_fixture","info":{"id":"msg_0","role":"user","sessionID":"ses_fixture"}}}`),
		opencodeServerEventFrame("message.updated", `{"type":"message.updated","properties":{"sessionID":"ses_fixture","info":{"id":"msg_1","role":"assistant","sessionID":"ses_fixture","tokens":{"input":9,"output":4,"cache":{"read":2,"write":1}}}}}`),
		opencodeServerEventFrame("message.part.delta", `{"type":"message.part.delta","properties":{"sessionID":"ses_fixture","messageID":"msg_1","partID":"prt_1","field":"text","delta":"Hel"}}`),
		opencodeServerEventFrame("message.part.delta", `{"type":"message.part.delta","properties":{"sessionID":"ses_fixture","messageID":"msg_1","partID":"prt_1","field":"text","delta":"lo"}}`),
		opencodeServerEventFrame("message.part.updated", `{"type":"message.part.updated","properties":{"sessionID":"ses_fixture","messageID":"msg_1","partID":"prt_1","part":{"type":"text","text":"Hello"}}}`),
		opencodeServerEventFrame("message.part.delta", `{"type":"message.part.delta","properties":{"sessionID":"ses_fixture","messageID":"msg_1","partID":"prt_2","field":"tool","delta":"[tool_call]{\"id\":\"call_1\",\"name\":\"lookup\",\"arguments\":{\"q\":\"x\"}}[/tool_call]"}}`),
		opencodeServerEventFrame("session.idle", `{"type":"session.idle","properties":{"sessionID":"ses_fixture","info":{"tokens":{"input":9,"output":4,"cache":{"read":2,"write":1}}}}}`),
	}
	f.setEvents(frames...)
	return f, strings.Join(frames, "")
}

func TestOpenCodeServerEventStream(t *testing.T) {
	f, _ := opencodeServerStreamFixture(t)
	rec := httptest.NewRecorder()
	req := opencodeServerRequest(f, "big-pickle", true, nil)
	if err := ForwardOpenCodeServer(rec, req); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if got := rec.Header().Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("content type = %q", got)
	}
	body := rec.Body.String()
	for _, want := range []string{`"role":"assistant"`, `"content":"Hel"`, `"content":"lo"`, `"name":"lookup"`, `"usage"`, "data: [DONE]"} {
		if !strings.Contains(body, want) {
			t.Fatalf("stream missing %q: %s", want, body)
		}
	}
	if strings.Count(body, `"content":"Hello"`) != 0 {
		t.Fatalf("part.updated duplicated the streamed text: %s", body)
	}
	if strings.Count(body, `"name":"lookup"`) != 1 {
		t.Fatalf("tool buffer emitted more than once: %s", body)
	}
	if len(f.calls("/session/ses_fixture/prompt_async")) != 1 {
		t.Fatalf("prompt_async calls = %d, want 1", len(f.calls("/session/ses_fixture/prompt_async")))
	}
	for _, path := range []string{"/event", "/session/ses_fixture/prompt_async", "/session"} {
		for _, call := range f.calls(path) {
			if call.header.Get("Authorization") != "" || call.header.Get("x-api-key") != "" {
				t.Fatalf("%s leaked a credential", path)
			}
		}
	}
}

func TestOpenCodeServerResponsesFrames(t *testing.T) {
	f, _ := opencodeServerStreamFixture(t)
	req := opencodeServerRequest(f, "big-pickle", true, nil)
	req.Ctx = translator.WithClientFormat(context.Background(), translator.ClientFormatResponses)
	rec := httptest.NewRecorder()
	if err := ForwardOpenCodeServer(rec, req); err != nil {
		t.Fatalf("execute: %v", err)
	}
	body := rec.Body.String()
	if strings.Contains(body, `"object":"chat.completion.chunk"`) {
		t.Fatalf("Responses stream leaked a raw OpenAI role chunk: %s", body)
	}
	for _, want := range []string{"response.created", "response.output_text.delta", "response.output_item.done", "response.completed"} {
		if !strings.Contains(body, want) {
			t.Fatalf("Responses stream missing %q: %s", want, body)
		}
	}
}

func TestOpenCodeServerSessionError(t *testing.T) {
	f := newOpenCodeServerFixture(t)
	f.setEvents(
		opencodeServerEventFrame("message.updated", `{"type":"message.updated","properties":{"sessionID":"ses_fixture","info":{"id":"msg_1","role":"assistant"}}}`),
		opencodeServerEventFrame("session.error", `{"type":"session.error","properties":{"sessionID":"ses_fixture","error":{"message":"local server failed"}}}`),
	)
	rec := httptest.NewRecorder()
	if err := ForwardOpenCodeServer(rec, opencodeServerRequest(f, "big-pickle", true, nil)); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(rec.Body.String(), "local server failed") {
		t.Fatalf("session.error was not reported: %s", rec.Body.String())
	}
}

func TestOpenCodeServerFilePartGuard(t *testing.T) {
	cases := map[string]bool{
		"data:image/png;base64,AAAA": true,
		"https://example.com/a.png":  true,
		"https://127.0.0.1/a.png":    false,
		"https://192.168.1.4/a.png":  false,
		"https://localhost/a.png":    false,
		"https://metadata.local/a":   false,
		"http://example.com/a.png":   false,
		"https://user@example.com/a": false,
	}
	for raw, want := range cases {
		if got := opencodeServerFileAllowed(raw); got != want {
			t.Errorf("file %q allowed = %t, want %t", raw, got, want)
		}
	}
	var files []map[string]any
	opencodeServerAppendFile(&files, "https://127.0.0.1/secret", "")
	opencodeServerAppendFile(&files, "data:image/png;base64,AAAA", "image/png")
	if len(files) != 1 || files[0]["mime"] != "image/png" {
		t.Fatalf("file parts = %+v", files)
	}
}

func TestOpenCodeServerToolEnvelopeRoundTrip(t *testing.T) {
	envelope := opencodeServerEncodeCall("call_9", "lookup", `{"q":"x"}`, false)
	clean, calls := opencodeServerExtractCalls("answer " + envelope)
	if clean != "answer" || len(calls) != 1 {
		t.Fatalf("clean = %q, calls = %+v", clean, calls)
	}
	if calls[0].ID != "call_9" || calls[0].Function.Name != "lookup" || calls[0].Function.Arguments != `{"q":"x"}` {
		t.Fatalf("call = %+v", calls[0])
	}
	frame := opencodeServerEventFrame("message.part.delta", `{"type":"message.part.delta","properties":{"sessionID":"other","messageID":"msg_1","partID":"prt","field":"text","delta":"x"}}`)
	event := opencodeServerParseEvent("message.part.delta", strings.TrimSpace(strings.TrimPrefix(strings.Split(frame, "\n")[1], "data: ")))
	if event.SessionID != "other" || event.Delta != "x" {
		t.Fatalf("event = %+v", event)
	}
}
