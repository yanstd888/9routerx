package executor

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/binary"
	"encoding/hex"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"

	"9routerx/proxy/internal/providers"
	"9routerx/proxy/internal/translator"
	"google.golang.org/protobuf/encoding/protowire"
)

type devinDesktopTestTransport func(*http.Request) (*http.Response, error)

func (f devinDesktopTestTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type devinDesktopTestBody struct {
	io.Reader
	closed atomic.Bool
}

func (b *devinDesktopTestBody) Close() error { b.closed.Store(true); return nil }

func devinDesktopTestReply(payload []byte) (*http.Response, *devinDesktopTestBody) {
	body := &devinDesktopTestBody{Reader: bytes.NewReader(payload)}
	return &http.Response{StatusCode: 200, Header: make(http.Header), Body: body, ContentLength: int64(len(payload))}, body
}

func devinDesktopTestBytes(t *testing.T, hexText string) []byte {
	t.Helper()
	payload, err := hex.DecodeString(hexText)
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

func devinDesktopTestFrame(payload []byte, flag byte) []byte {
	frame := make([]byte, len(payload)+5)
	frame[0] = flag
	binary.BigEndian.PutUint32(frame[1:5], uint32(len(payload)))
	copy(frame[5:], payload)
	return frame
}

func devinDesktopTestString(field protowire.Number, text string) []byte {
	return protowire.AppendBytes(protowire.AppendTag(nil, field, protowire.BytesType), []byte(text))
}

func devinDesktopTestNumber(field protowire.Number, value uint64) []byte {
	return protowire.AppendVarint(protowire.AppendTag(nil, field, protowire.VarintType), value)
}

func devinDesktopTestFields(t *testing.T, payload []byte) map[protowire.Number][]devinDesktopField {
	t.Helper()
	fields := make(map[protowire.Number][]devinDesktopField)
	for len(payload) > 0 {
		num, wire, n := protowire.ConsumeTag(payload)
		if n < 0 {
			t.Fatalf("tag: %v", protowire.ParseError(n))
		}
		payload = payload[n:]
		field := devinDesktopField{number: num, wire: wire}
		switch wire {
		case protowire.BytesType:
			field.bytes, n = protowire.ConsumeBytes(payload)
		case protowire.VarintType:
			field.value, n = protowire.ConsumeVarint(payload)
		default:
			t.Fatalf("unexpected fixture wire %d", wire)
		}
		if n < 0 {
			t.Fatalf("value: %v", protowire.ParseError(n))
		}
		fields[num] = append(fields[num], field)
		payload = payload[n:]
	}
	return fields
}

func devinDesktopTestField(t *testing.T, fields map[protowire.Number][]devinDesktopField, num protowire.Number) devinDesktopField {
	t.Helper()
	if len(fields[num]) != 1 {
		t.Fatalf("field %d count = %d, want 1", num, len(fields[num]))
	}
	return fields[num][0]
}

func devinDesktopTestGzip(t *testing.T, payload []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	writer := gzip.NewWriter(&buf)
	if _, err := writer.Write(payload); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func devinDesktopTestChat(t *testing.T) []byte {
	t.Helper()
	// GetChatMessageResponse: 9 thinking, 3 text, 6 id-less arguments,
	// 6 identified call, 6 continuation, 7 usage(2/3/4/5), 5 stop=10.
	first := devinDesktopTestBytes(t, "4a03686d6d1a0568656c6c6f32051a037b2278")
	second := devinDesktopTestBytes(t, "320e0a02633112046c6f6f6b1a02223a32051a03317d203a08100c180420032805280a")
	return append(append(devinDesktopTestFrame(first, 0), devinDesktopTestFrame(second, 0)...), devinDesktopTestFrame([]byte(`{"metadata":{}}`), 2)...)
}

func devinDesktopTestRequest(client *http.Client, body []byte) *Request {
	return &Request{
		Ctx: translator.WithUsageCapture(context.Background()), Client: client,
		Config: &providers.ProviderConfig{BaseURL: "https://server.codeium.com"},
		APIKey: "fixture-key", Body: body, ModelName: "claude-opus-5-low",
	}
}

func devinDesktopTestClient(t *testing.T, chat []byte) (*http.Client, *devinDesktopTestBody, *devinDesktopTestBody) {
	t.Helper()
	authResp, authBody := devinDesktopTestReply(devinDesktopTestString(1, "fixture-jwt"))
	chatResp, chatBody := devinDesktopTestReply(chat)
	calls := 0
	client := &http.Client{Transport: devinDesktopTestTransport(func(req *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			if req.URL.Path != devinDesktopAuthPath {
				t.Errorf("first request path = %s", req.URL.Path)
			}
			return authResp, nil
		}
		if calls != 2 || req.URL.Path != devinDesktopChatPath {
			t.Errorf("chat request %d path = %s", calls, req.URL.Path)
		}
		return chatResp, nil
	})}
	return client, authBody, chatBody
}

type devinDesktopTestBlockingBody struct {
	closed  chan struct{}
	started chan struct{}
	once    sync.Once
	start   sync.Once
}

func (b *devinDesktopTestBlockingBody) Read([]byte) (int, error) {
	b.start.Do(func() { close(b.started) })
	<-b.closed
	return 0, io.ErrClosedPipe
}

func (b *devinDesktopTestBlockingBody) Close() error {
	b.once.Do(func() { close(b.closed) })
	return nil
}
