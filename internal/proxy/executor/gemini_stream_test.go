// 9RouterX — source port and modifications by https://github.com/yanstd888/ — https://t.me/aiclub_id
package executor

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
)

// geminiStream is fed by proxy.ScanStream, which hands over each SSE event's
// payload with the "data:" prefix already stripped. The copy this replaces
// still checked chunks for that prefix and skipped the first event outright,
// so every event was dropped before translation and the client read nothing
// but the response headers until the connection closed.
func TestGeminiStream_TranslatesScanStreamPayloads(t *testing.T) {
	var b strings.Builder
	b.WriteString("data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"Hello\"}],\"role\":\"model\"}}]}\n\n")
	b.WriteString("data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\" world\"}],\"role\":\"model\"},\"finishReason\":\"STOP\"}]}\n\n")

	rec := httptest.NewRecorder()
	if err := geminiStream(rec, strings.NewReader(b.String()), context.Background()); err != nil {
		t.Fatalf("geminiStream: %v", err)
	}

	out := rec.Body.String()
	for _, want := range []string{"Hello", "world"} {
		if !strings.Contains(out, want) {
			t.Fatalf("stream output missing %q; got: %s", want, out)
		}
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type = %q, want text/event-stream", ct)
	}
}
