package executor

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"9routerx/proxy/internal/proxy"
)

func TestDevinDesktopAuthFailures(t *testing.T) {
	for _, tc := range []struct {
		name     string
		payload  []byte
		status   int
		declared int64
	}{
		{"no-jwt", []byte{}, 200, 0},
		{"malformed", []byte{0x0a, 0x80}, 200, 0},
		{"wrong-wire", []byte{0x08, 1}, 200, 0},
		{"oversized", bytes.Repeat([]byte("x"), devinDesktopAuthLimit+1), 200, 0},
		{"declared-oversized", []byte{}, 200, devinDesktopAuthLimit + 1},
		{"unauthorized", []byte("fixture-key"), 401, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp, body := devinDesktopTestReply(tc.payload)
			resp.StatusCode = tc.status
			if tc.declared != 0 {
				resp.ContentLength = tc.declared
			}
			calls := 0
			client := &http.Client{Transport: devinDesktopTestTransport(func(*http.Request) (*http.Response, error) { calls++; return resp, nil })}
			w := httptest.NewRecorder()
			err := ForwardDevinDesktop(w, devinDesktopTestRequest(client, []byte(`{"messages":[]}`)))
			var upstream *proxy.UpstreamError
			if !errors.As(err, &upstream) {
				t.Fatalf("want UpstreamError, got %v", err)
			}
			wantStatus := 502
			if tc.status != 200 {
				wantStatus = tc.status
			}
			if upstream.StatusCode != wantStatus || calls != 1 || !body.closed.Load() || w.Body.Len() != 0 {
				t.Errorf("status=%d calls=%d closed=%v", upstream.StatusCode, calls, body.closed.Load())
			}
			if strings.Contains(string(upstream.Body), "fixture-key") {
				t.Error("credential leak")
			}
		})
	}
}

func TestDevinDesktopRedirectsDoNotReplayCredentials(t *testing.T) {
	for _, phase := range []string{"auth", "chat", "models"} {
		t.Run(phase, func(t *testing.T) {
			calls, redirects := 0, 0
			var redirectBody *devinDesktopTestBody
			client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { redirects++; return nil }, Transport: devinDesktopTestTransport(func(req *http.Request) (*http.Response, error) {
				calls++
				if phase == "chat" && calls == 1 {
					resp, _ := devinDesktopTestReply(devinDesktopTestString(1, "jwt"))
					return resp, nil
				}
				resp, body := devinDesktopTestReply([]byte("fixture-key"))
				redirectBody = body
				resp.StatusCode = http.StatusTemporaryRedirect
				resp.Header.Set("Location", "https://other.example/credential-sink")
				return resp, nil
			})}
			var err error
			if phase == "models" {
				_, err = FetchDevinDesktopModels(context.Background(), client, "fixture-key", "")
			} else {
				err = ForwardDevinDesktop(httptest.NewRecorder(), devinDesktopTestRequest(client, []byte(`{"messages":[]}`)))
			}
			if err == nil || redirects != 0 || !redirectBody.closed.Load() {
				t.Fatalf("err=%v redirect callbacks=%d", err, redirects)
			}
			want := 1
			if phase == "chat" {
				want = 2
			}
			if calls != want || client.CheckRedirect == nil {
				t.Error("transport reused/policy changed")
			}
		})
	}
}

func TestDevinDesktopServiceURLGuard(t *testing.T) {
	t.Setenv("ALLOW_PRIVATE_NETWORKING", "false")
	t.Setenv("ALLOW_PRIVATE_PROVIDER_URLS", "false")
	for _, base := range []string{"http://127.0.0.1", "http://169.254.169.254", "https://user:secret@server.codeium.com", "file:///tmp", "https://server.codeium.com?target=private", "https://server.codeium.com#private", "https://[invalid"} {
		t.Run(base, func(t *testing.T) {
			calls := 0
			client := &http.Client{Transport: devinDesktopTestTransport(func(*http.Request) (*http.Response, error) { calls++; return nil, io.ErrUnexpectedEOF })}
			req := devinDesktopTestRequest(client, []byte(`{"messages":[]}`))
			req.Config.BaseURL = base
			err := ForwardDevinDesktop(httptest.NewRecorder(), req)
			var upstream *proxy.UpstreamError
			if !errors.As(err, &upstream) || upstream.StatusCode != 403 || calls != 0 {
				t.Fatalf("err=%v calls=%d", err, calls)
			}
		})
	}
}

func TestDevinDesktopCancellationClosesRead(t *testing.T) {
	for _, phase := range []string{"auth", "chat", "models"} {
		t.Run(phase, func(t *testing.T) {
			body := &devinDesktopTestBlockingBody{closed: make(chan struct{}), started: make(chan struct{})}
			calls := 0
			client := &http.Client{Transport: devinDesktopTestTransport(func(*http.Request) (*http.Response, error) {
				calls++
				if phase == "chat" && calls == 1 {
					resp, _ := devinDesktopTestReply(devinDesktopTestString(1, "jwt"))
					return resp, nil
				}
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: body, ContentLength: -1}, nil
			})}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			result := make(chan error, 1)
			go func() {
				if phase == "models" {
					_, err := FetchDevinDesktopModels(ctx, client, "fixture-key", "")
					result <- err
					return
				}
				req := devinDesktopTestRequest(client, []byte(`{"messages":[]}`))
				req.Ctx = ctx
				req.IsStream = phase == "chat"
				result <- ForwardDevinDesktop(httptest.NewRecorder(), req)
			}()
			select {
			case <-body.started:
			case <-time.After(2 * time.Second):
				t.Fatal("read did not start")
			}
			cancel()
			select {
			case err := <-result:
				if !errors.Is(err, context.Canceled) {
					t.Errorf("error=%v", err)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("cancel blocked")
			}
			select {
			case <-body.closed:
			default:
				t.Error("body not closed")
			}
		})
	}
}

func TestDevinDesktopMissingKeyDoesNotDispatch(t *testing.T) {
	req := devinDesktopTestRequest(&http.Client{Transport: devinDesktopTestTransport(func(*http.Request) (*http.Response, error) { t.Fatal("dispatched without key"); return nil, nil })}, []byte(`{"messages":[]}`))
	req.APIKey = ""
	var upstream *proxy.UpstreamError
	if err := ForwardDevinDesktop(httptest.NewRecorder(), req); !errors.As(err, &upstream) || upstream.StatusCode != 401 {
		t.Error(err)
	}
}
