package executor

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"testing"
)

type devinDesktopTestSplitReader struct {
	reader *bytes.Reader
	size   int
}

func (r devinDesktopTestSplitReader) Read(p []byte) (int, error) {
	return r.reader.Read(p[:min(len(p), r.size)])
}

func TestDevinDesktopConnectFrames(t *testing.T) {
	valid := devinDesktopTestFrame(devinDesktopTestString(3, "hello"), 0)
	terminal := devinDesktopTestFrame([]byte(`{"metadata":{"trace":["ok"]}}`), 2)
	oversize := []byte{0, 0, 0, 0, 0}
	binary.BigEndian.PutUint32(oversize[1:], devinDesktopFrameLimit+1)
	bomb := devinDesktopTestGzip(t, bytes.Repeat([]byte("x"), devinDesktopFrameLimit+1))
	cases := []struct {
		name      string
		payload   []byte
		encoding  string
		wantError bool
	}{
		{"plain", append(bytes.Clone(valid), terminal...), "", false},
		{"gzip", append(devinDesktopTestFrame(devinDesktopTestGzip(t, devinDesktopTestString(3, "hello")), 1), terminal...), "gzip", false},
		{"gzip-terminal", append(bytes.Clone(valid), devinDesktopTestFrame(devinDesktopTestGzip(t, []byte(`{}`)), 3)...), "gzip", false},
		{"no-trailer", valid, "", true},
		{"empty", nil, "", true},
		{"truncated-header", []byte{0, 0, 0}, "", true},
		{"truncated-body", []byte{0, 0, 0, 0, 2, 0}, "", true},
		{"oversized", oversize, "", true},
		{"flags", devinDesktopTestFrame(nil, 0x80), "", true},
		{"empty-trailer", devinDesktopTestFrame(nil, 2), "", true},
		{"invalid-trailer", devinDesktopTestFrame([]byte(`{bad}`), 2), "", true},
		{"array-trailer", devinDesktopTestFrame([]byte(`[]`), 2), "", true},
		{"null-trailer", devinDesktopTestFrame([]byte(`null`), 2), "", true},
		{"invalid-metadata", devinDesktopTestFrame([]byte(`{"metadata":{"trace":1}}`), 2), "", true},
		{"error-trailer", devinDesktopTestFrame([]byte(`{"error":{"code":"permission_denied","message":"fixture-key"}}`), 2), "", true},
		{"null-error", devinDesktopTestFrame([]byte(`{"error":null}`), 2), "", true},
		{"duplicate-terminal", append(bytes.Clone(terminal), terminal...), "", true},
		{"data-after-terminal", append(bytes.Clone(terminal), valid...), "", true},
		{"bad-gzip", devinDesktopTestFrame([]byte("no gzip"), 1), "gzip", true},
		{"wrong-compression", append(devinDesktopTestFrame(devinDesktopTestGzip(t, []byte{}), 1), terminal...), "br", true},
		{"gzip-bomb", devinDesktopTestFrame(bomb, 1), "gzip", true},
		{"zero-tag", append(devinDesktopTestFrame([]byte{0}, 0), terminal...), "", true},
		{"bad-varint", append(devinDesktopTestFrame(bytes.Repeat([]byte{0x80}, 11), 0), terminal...), "", true},
		{"truncated-proto", append(devinDesktopTestFrame([]byte{0x1a, 3, 1}, 0), terminal...), "", true},
		{"wrong-known-wire", append(devinDesktopTestFrame([]byte{0x18, 1}, 0), terminal...), "", true},
		{"unsupported-proto-wire", append(devinDesktopTestFrame([]byte{0x1b}, 0), terminal...), "", true},
		{"bad-nested-tool", append(devinDesktopTestFrame([]byte{0x32, 1, 0}, 0), terminal...), "", true},
		{"bad-nested-usage", append(devinDesktopTestFrame([]byte{0x3a, 1, 0}, 0), terminal...), "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			count := 0
			err := devinDesktopFrames(context.Background(), bytes.NewReader(tc.payload), tc.encoding, func(response devinDesktopResponse) error { count++; return nil })
			if (err != nil) != tc.wantError {
				t.Fatalf("error=%v, wantError=%v", err, tc.wantError)
			}
			if !tc.wantError && count != 1 {
				t.Errorf("frames=%d", count)
			}
		})
	}
}

func TestDevinDesktopSplitFrames(t *testing.T) {
	payload := append(devinDesktopTestFrame(devinDesktopTestString(3, "hello"), 0), devinDesktopTestFrame([]byte(`{}`), 2)...)
	for _, size := range []int{1, 2, 3, 5, 7} {
		t.Run(string(rune('0'+size)), func(t *testing.T) {
			text := ""
			err := devinDesktopFrames(context.Background(), devinDesktopTestSplitReader{bytes.NewReader(payload), size}, "", func(response devinDesktopResponse) error { text += response.text; return nil })
			if err != nil || text != "hello" {
				t.Fatalf("text=%q err=%v", text, err)
			}
		})
	}
}

func TestDevinDesktopUnknownProtoFields(t *testing.T) {
	payload := append(devinDesktopTestString(3, "ok"), []byte{0xa1, 0x06, 0, 0, 0, 0, 0, 0, 0, 0, 0xad, 0x06, 0, 0, 0, 0}...)
	chat := append(devinDesktopTestFrame(payload, 0), devinDesktopTestFrame([]byte(`{}`), 2)...)
	if err := devinDesktopFrames(context.Background(), bytes.NewReader(chat), "", func(response devinDesktopResponse) error {
		if response.text != "ok" {
			t.Error(response.text)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := devinDesktopFrames(ctx, bytes.NewReader(nil), "", func(devinDesktopResponse) error { return io.ErrClosedPipe }); !errors.Is(err, context.Canceled) {
		t.Error(err)
	}
}
