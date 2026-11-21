package executor

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"google.golang.org/protobuf/encoding/protowire"
)

func TestDevinDesktopNativeRequestFixture(t *testing.T) {
	body := []byte(`{"model":"claude-opus-5-low","conversation_id":"cascade-fixture","messages":[{"role":"system","content":"first"},{"role":"developer","content":[{"type":"text","text":"second"}]},{"role":"user","content":[{"type":"text","text":"a"},{"type":"image_url","image_url":{"url":"ignored"}},{"type":"text","text":"b"}]},{"role":"assistant","content":"","tool_calls":[{"id":"call-prev","type":"function","function":{"name":"lookup","arguments":"{\"x\":1}"}}]},{"role":"tool","content":"found","tool_call_id":"call-prev"}],"tools":[{"type":"function","function":{"name":"lookup","description":"Find it","parameters":{"type":"object","properties":{"x":{"type":"number"}}},"strict":true}},{"type":"web_search"}],"parallel_tool_calls":false,"tool_choice":{"type":"function","function":{"name":"lookup"}}}`)
	var session string
	var authBytes, chatBytes []byte
	calls := 0
	client := &http.Client{Transport: devinDesktopTestTransport(func(req *http.Request) (*http.Response, error) {
		calls++
		if req.Method != http.MethodPost || req.Header.Get("Authorization") != "" {
			t.Errorf("unexpected method/auth header")
		}
		if req.Header.Get("Connect-Protocol-Version") != "1" {
			t.Errorf("missing Connect version")
		}
		payload, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatal(err)
		}
		if calls == 1 {
			authBytes = payload
			if req.Header.Get("Content-Type") != "application/proto" || req.URL.Path != devinDesktopAuthPath {
				t.Errorf("auth path/content type")
			}
			fields := devinDesktopTestFields(t, payload)
			if len(fields) != 1 {
				t.Fatalf("auth must be unframed Metadata wrapper")
			}
			metadata := devinDesktopTestFields(t, devinDesktopTestField(t, fields, 1).bytes)
			session = string(devinDesktopTestField(t, metadata, 10).bytes)
			if _, err := uuid.Parse(session); err != nil {
				t.Fatalf("session UUID: %v", err)
			}
			if len(metadata[21]) != 0 {
				t.Errorf("auth should not carry JWT")
			}
			// Enterprise response URL must not affect the chat destination.
			resp, _ := devinDesktopTestReply(append(devinDesktopTestString(1, "fixture-jwt"), devinDesktopTestString(2, "http://169.254.169.254")...))
			return resp, nil
		}
		chatBytes = payload
		if calls != 2 || req.URL.Host != "server.codeium.com" || req.URL.Path != devinDesktopChatPath {
			t.Errorf("chat destination = %s", req.URL)
		}
		if req.Header.Get("Content-Type") != "application/connect+proto" || req.Header.Get("Accept") != "application/connect+proto" || req.Header.Get("Connect-Accept-Encoding") != "gzip" || req.Header.Get("User-Agent") != "windsurf/3.6.27" {
			t.Errorf("chat headers = %v", req.Header)
		}
		resp, _ := devinDesktopTestReply(devinDesktopTestChat(t))
		return resp, nil
	})}
	req := devinDesktopTestRequest(client, body)
	if err := ForwardDevinDesktop(httptest.NewRecorder(), req); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("requests = %d", calls)
	}
	if len(chatBytes) < 5 || chatBytes[0] != 0 || binary.BigEndian.Uint32(chatBytes[1:5]) != uint32(len(chatBytes)-5) {
		t.Fatalf("incorrect Connect frame")
	}
	fields := devinDesktopTestFields(t, chatBytes[5:])
	metadata := devinDesktopTestFields(t, devinDesktopTestField(t, fields, 1).bytes)
	for num, want := range map[int]string{1: "windsurf", 2: "1.48.2", 3: "fixture-key", 4: "en-US", 7: "3.6.27", 10: session, 12: "windsurf", 21: "fixture-jwt"} {
		if got := string(devinDesktopTestField(t, metadata, protowire.Number(num)).bytes); got != want {
			t.Errorf("metadata %d=%q, want %q", num, got, want)
		}
	}
	if devinDesktopTestField(t, fields, 7).value != 5 || devinDesktopTestField(t, fields, 11).value != 1 {
		t.Error("cascade type or parallel flag")
	}
	if string(devinDesktopTestField(t, fields, 2).bytes) != "first\n\nsecond" {
		t.Error("system extraction")
	}
	for _, num := range []int{14, 21} {
		if string(devinDesktopTestField(t, fields, protowire.Number(num)).bytes) != "claude-opus-5-low" {
			t.Error("literal model lost")
		}
	}
	if string(devinDesktopTestField(t, fields, 16).bytes) != "cascade-fixture" {
		t.Error("cascade id")
	}
	if len(fields[3]) != 3 {
		t.Fatalf("history length = %d", len(fields[3]))
	}
	for i, wantSource := range []uint64{1, 2, 4} {
		p := devinDesktopTestFields(t, fields[3][i].bytes)
		if devinDesktopTestField(t, p, 2).value != wantSource {
			t.Errorf("prompt source %d", i)
		}
		id := string(devinDesktopTestField(t, p, 1).bytes)
		if i == 1 && !strings.HasPrefix(id, "bot-") {
			t.Error("assistant id")
		}
		if _, err := uuid.Parse(strings.TrimPrefix(id, "bot-")); err != nil {
			t.Error(err)
		}
	}
	user := devinDesktopTestFields(t, fields[3][0].bytes)
	if string(devinDesktopTestField(t, user, 3).bytes) != "ab" {
		t.Error("multipart concatenation")
	}
	assistant := devinDesktopTestFields(t, fields[3][1].bytes)
	call := devinDesktopTestFields(t, devinDesktopTestField(t, assistant, 6).bytes)
	if string(devinDesktopTestField(t, call, 1).bytes) != "call-prev" || string(devinDesktopTestField(t, call, 2).bytes) != "lookup" || string(devinDesktopTestField(t, call, 3).bytes) != `{"x":1}` {
		t.Error("tool history")
	}
	result := devinDesktopTestFields(t, fields[3][2].bytes)
	if string(devinDesktopTestField(t, result, 7).bytes) != "call-prev" {
		t.Error("tool result association")
	}
	tool := devinDesktopTestFields(t, devinDesktopTestField(t, fields, 10).bytes)
	if string(devinDesktopTestField(t, tool, 1).bytes) != "lookup" || devinDesktopTestField(t, tool, 12).value != 1 || !bytes.Contains(devinDesktopTestField(t, tool, 3).bytes, []byte(`"properties"`)) {
		t.Error("tool definition")
	}
	choice := devinDesktopTestFields(t, devinDesktopTestField(t, fields, 12).bytes)
	if string(devinDesktopTestField(t, choice, 2).bytes) != "lookup" {
		t.Error("named tool choice")
	}
	if bytes.Equal(authBytes, chatBytes) {
		t.Error("auth/chat schema must differ")
	}
}

func TestDevinDesktopToolChoicesAndEmptyPrompt(t *testing.T) {
	for _, option := range []string{"auto", "none", "required"} {
		t.Run(option, func(t *testing.T) {
			req := devinDesktopTestRequest(nil, []byte(`{"model":"gpt-5-6-sol-max","messages":[],"tool_choice":"`+option+`"}`))
			input, _, err := devinDesktopInput(req, context.Background())
			if err != nil {
				t.Fatal(err)
			}
			fields := devinDesktopTestFields(t, input.encode(devinDesktopMetadata{apiKey: "k", sessionID: "s"}))
			if len(fields[3]) != 1 || len(fields[11]) != 0 {
				t.Error("empty prompt / parallel default")
			}
			choice := devinDesktopTestFields(t, devinDesktopTestField(t, fields, 12).bytes)
			if string(devinDesktopTestField(t, choice, 1).bytes) != option {
				t.Error("option choice")
			}
		})
	}
}
