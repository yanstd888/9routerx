package executor

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

const qoderPipeExitTimeout = 2 * time.Second

var qoderPipeClientFailure = errors.New("Qoder test client stopped writing")

type qoderPipeFailWriter struct{ header http.Header }

func (w *qoderPipeFailWriter) Header() http.Header { return w.header }
func (w *qoderPipeFailWriter) WriteHeader(int)     {}
func (w *qoderPipeFailWriter) Write([]byte) (int, error) {
	return 0, qoderPipeClientFailure
}
func (w *qoderPipeFailWriter) Flush() {}

type qoderPipeSignalWriter struct {
	io.Writer
	started chan struct{}
	onWrite sync.Once
}

func (w *qoderPipeSignalWriter) Write(p []byte) (int, error) {
	w.onWrite.Do(func() { close(w.started) })
	return w.Writer.Write(p)
}

type qoderPipeErrorReader struct{ err error }

func (r qoderPipeErrorReader) Read([]byte) (int, error) { return 0, r.err }

func qoderPipeAwait(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(qoderPipeExitTimeout):
		t.Fatalf("%s did not complete within %s", what, qoderPipeExitTimeout)
	}
}

func TestQoderRewindPipe_ProducerExit(t *testing.T) {
	for _, tc := range []struct {
		name string
		mode string
	}{
		{name: "early_consumer_return", mode: "return"},
		{name: "client_writer_failure", mode: "write_failure"},
		{name: "context_cancel_while_writer_blocked", mode: "cancel_write"},
		{name: "context_cancel_while_upstream_read_blocked", mode: "cancel_read"},
		{name: "normal_completion_stream", mode: "stream"},
		{name: "normal_completion_nonstream", mode: "nonstream"},
		{name: "producer_read_error_preserved", mode: "read_error"},
		{name: "producer_peek_error_preserved", mode: "peek_error"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			// Exceed SSECopy's read buffer so a failed client leaves the same
			// producer Write partially consumed, rather than relying on scheduling.
			raw := qoderSSE(qoderChunk(`{"content":"`+strings.Repeat("x", 8192)+`"}`, `null`),
				qoderChunk(`{}`, `"stop"`), "[DONE]")
			body := newQoderPipeTestBody(raw)
			defer body.Close()
			var source io.Reader = body
			producerErr := errors.New("Qoder test upstream read failed")
			if tc.mode == "cancel_read" {
				body.prefix = strings.NewReader("")
			}
			if tc.mode == "read_error" {
				source = io.MultiReader(strings.NewReader(envelope(t, 200, qoderChunk(`{"content":"hello"}`, `null`))),
					qoderPipeErrorReader{err: producerErr})
			}
			writing, exited := make(chan struct{}), make(chan struct{})
			reader, cleanup := qoderRewindPipe(ctx, body, func(out io.Writer) error {
				defer close(exited)
				if tc.mode == "peek_error" {
					return producerErr
				}
				return qoderSSERewrite(source, &qoderPipeSignalWriter{Writer: out, started: writing}, "model")
			})
			defer cleanup()
			// Rescue the deliberately broken ownership mutation after recording
			// failure, so no producer from this case survives into the next one.
			defer func() {
				_ = reader.Close()
				_ = body.Close()
				qoderPipeAwait(t, exited, "rescued producer exit")
			}()

			switch tc.mode {
			case "return":
				qoderPipeAwait(t, writing, "producer write start")
				if _, err := io.ReadFull(reader, make([]byte, 1)); err != nil {
					t.Fatalf("early consumer read: %v", err)
				}
				cleanup()
			case "write_failure":
				qoderPipeAwait(t, writing, "producer write start")
				err := execSSEStream(&qoderPipeFailWriter{header: make(http.Header)}, reader, &Request{Ctx: ctx, IsStream: true})
				if !errors.Is(err, qoderPipeClientFailure) {
					t.Fatalf("client failure = %v, want %v", err, qoderPipeClientFailure)
				}
				cleanup()
			case "cancel_write", "cancel_read":
				if tc.mode == "cancel_read" {
					qoderPipeAwait(t, body.reading, "upstream read start")
				} else {
					qoderPipeAwait(t, writing, "producer write start")
				}
				cancel()
			case "stream", "nonstream":
				rec := httptest.NewRecorder()
				req := &Request{Ctx: ctx, IsStream: tc.mode == "stream"}
				result := make(chan error, 1)
				go func() {
					if req.IsStream {
						result <- execSSEStream(rec, reader, req)
						return
					}
					result <- qoderNonStream(rec, req, reader)
				}()
				select {
				case err := <-result:
					if err != nil {
						t.Fatalf("normal completion: %v", err)
					}
				case <-time.After(qoderPipeExitTimeout):
					t.Fatal("normal consumer completion blocked")
				}
				if !strings.Contains(rec.Body.String(), strings.Repeat("x", 8192)) {
					t.Error("normal completion lost content")
				}
				if req.IsStream && !strings.Contains(rec.Body.String(), strings.TrimSpace(qoderSSEDone)) {
					t.Error("normal stream lost DONE")
				}
				if !req.IsStream && rec.Header().Get("Content-Type") != "application/json" {
					t.Error("normal nonstream completion is not JSON")
				}
				cleanup()
				cancel()
			case "read_error", "peek_error":
				result := make(chan error, 1)
				go func() {
					_, err := io.ReadAll(reader)
					result <- err
				}()
				select {
				case err := <-result:
					if !errors.Is(err, producerErr) {
						t.Fatalf("producer error = %v, want %v", err, producerErr)
					}
				case <-time.After(qoderPipeExitTimeout):
					t.Fatal("producer error was not delivered")
				}
				cleanup()
			}
			qoderPipeAwait(t, exited, "producer exit")
			if tc.mode == "cancel_write" || tc.mode == "cancel_read" {
				_, err := reader.Read(make([]byte, 1))
				if !errors.Is(err, io.ErrClosedPipe) {
					t.Errorf("canceled reader = %v, want io.ErrClosedPipe after reader close", err)
				}
			}
			qoderPipeAwait(t, body.closed, "original upstream close")
		})
	}
}

type qoderPipeTestBody struct {
	prefix  *strings.Reader
	reading chan struct{}
	closed  chan struct{}
	onRead  sync.Once
	onClose sync.Once
}

func newQoderPipeTestBody(prefix string) *qoderPipeTestBody {
	return &qoderPipeTestBody{
		prefix: strings.NewReader(prefix), reading: make(chan struct{}), closed: make(chan struct{}),
	}
}

func (b *qoderPipeTestBody) Read(p []byte) (int, error) {
	if b.prefix.Len() > 0 {
		return b.prefix.Read(p)
	}
	b.onRead.Do(func() { close(b.reading) })
	<-b.closed
	return 0, io.ErrClosedPipe
}

func (b *qoderPipeTestBody) Close() error {
	b.onClose.Do(func() { close(b.closed) })
	return nil
}

type qoderPipeTestTransport func(*http.Request) (*http.Response, error)

func (f qoderPipeTestTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func qoderPipeForwardRequest(t *testing.T, body io.ReadCloser) *Request {
	t.Helper()
	req := qoderRequest(t, qoderTestServer(t, ""))
	req.Client = &http.Client{Transport: qoderPipeTestTransport(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodPost || !strings.HasPrefix(r.URL.String(), req.Config.BaseURL+"?") {
			return nil, errors.New("unexpected Qoder test target")
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: body}, nil
	})}
	return req
}

func TestForwardQoder_PipeCancelBlockedRead(t *testing.T) {
	for _, tc := range []struct {
		name   string
		stream bool
	}{
		{name: "stream", stream: true},
		{name: "nonstream", stream: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := newQoderPipeTestBody(envelope(t, 200, qoderChunk(`{"content":"hello"}`, `null`)))
			defer body.Close()
			req := qoderPipeForwardRequest(t, body)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			req.Ctx, req.IsStream = ctx, tc.stream
			result := make(chan error, 1)
			go func() { result <- ForwardQoder(httptest.NewRecorder(), req) }()
			select {
			case <-body.reading:
			case err := <-result:
				t.Fatalf("ForwardQoder exited before blocked read: %v", err)
			case <-time.After(qoderPipeExitTimeout):
				t.Fatal("rewriter never reached the blocked upstream read")
			}
			cancel()
			select {
			case err := <-result:
				if err == nil {
					t.Error("cancellation returned success")
				}
			case <-time.After(qoderPipeExitTimeout):
				t.Error("ForwardQoder did not exit after context cancellation")
				_ = body.Close()
				select {
				case <-result:
				case <-time.After(qoderPipeExitTimeout):
					t.Fatal("ForwardQoder did not exit after test rescue")
				}
			}
			select {
			case <-body.closed:
			default:
				t.Error("original upstream body was not closed")
			}
		})
	}
}
