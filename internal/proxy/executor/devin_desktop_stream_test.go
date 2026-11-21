package executor

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"9routerx/proxy/internal/proxy"
	"9routerx/proxy/internal/translator"
)

func TestDevinDesktopMalformedStreamNeverSucceeds(t *testing.T) {
	for _, mode := range []string{"json", "openai", "claude", "responses"} {
		t.Run(mode, func(t *testing.T) {
			chat := append(devinDesktopTestFrame(devinDesktopTestString(3, "partial"), 0), devinDesktopTestFrame([]byte(`{"error":{"message":"fixture-key"}}`), 2)...)
			client, _, body := devinDesktopTestClient(t, chat)
			req := devinDesktopTestRequest(client, []byte(`{"messages":[{"role":"user","content":"hi"}]}`))
			req.IsStream = mode != "json"
			req.ClaudeClient = mode == "claude"
			if mode == "responses" {
				req.Ctx = translator.WithClientFormat(req.Ctx, translator.ClientFormatResponses)
			}
			w := httptest.NewRecorder()
			err := ForwardDevinDesktop(w, req)
			var upstream *proxy.UpstreamError
			if !errors.As(err, &upstream) || upstream.StatusCode != 502 || !body.closed.Load() {
				t.Fatalf("err=%v closed=%v", err, body.closed.Load())
			}
			text := w.Body.String()
			if mode == "json" {
				if text != "" {
					t.Error("JSON partial success written")
				}
				return
			}
			if !strings.Contains(text, "stream failed") || strings.Contains(text, "fixture-key") {
				t.Errorf("error frame = %s", text)
			}
			for _, success := range []string{`"finish_reason":"stop"`, "event: response.completed", "event: message_stop"} {
				if strings.Contains(text, success) {
					t.Errorf("false success %q", success)
				}
			}
			if mode == "responses" && !strings.Contains(text, "event: response.failed") {
				t.Error("missing Responses failure terminal")
			}
		})
	}
}

type devinDesktopTestFailWriter struct{ http.ResponseWriter }

func (w devinDesktopTestFailWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestDevinDesktopClientWriteFailureClosesUpstream(t *testing.T) {
	client, _, body := devinDesktopTestClient(t, devinDesktopTestChat(t))
	req := devinDesktopTestRequest(client, []byte(`{"messages":[]}`))
	req.IsStream = true
	err := ForwardDevinDesktop(devinDesktopTestFailWriter{httptest.NewRecorder()}, req)
	if err == nil || !body.closed.Load() {
		t.Fatalf("err=%v closed=%v", err, body.closed.Load())
	}
}

func TestDevinDesktopMultipleToolIndexes(t *testing.T) {
	call1 := append(devinDesktopTestString(1, "a"), devinDesktopTestString(2, "one")...)
	call1 = append(call1, devinDesktopTestString(3, `{}`)...)
	call2 := append(devinDesktopTestString(1, "b"), devinDesktopTestString(2, "two")...)
	call2 = append(call2, devinDesktopTestString(3, `{"b":`)...)
	fragment := devinDesktopTestString(3, `1}`)
	payload := append(devinDesktopTestString(6, string(call1)), devinDesktopTestString(6, string(call2))...)
	payload = append(payload, devinDesktopTestString(6, string(fragment))...)
	chat := append(devinDesktopTestFrame(payload, 0), devinDesktopTestFrame([]byte(`{}`), 2)...)
	client, _, _ := devinDesktopTestClient(t, chat)
	w := httptest.NewRecorder()
	if err := ForwardDevinDesktop(w, devinDesktopTestRequest(client, []byte(`{"messages":[]}`))); err != nil {
		t.Fatal(err)
	}
	text := w.Body.String()
	for _, part := range []string{`"id":"a"`, `"id":"b"`, `"arguments":"{\"b\":1}"`} {
		if !strings.Contains(text, part) {
			t.Errorf("missing %s in %s", part, text)
		}
	}
}
