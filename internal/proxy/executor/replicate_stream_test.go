// 9RouterX — offline SSE fixtures for the pinned Replicate executor.
package executor

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"9routerx/proxy/internal/providers"
)

var replicateDeltaRE = regexp.MustCompile(`"delta":\{"content":"([^"]*)"\}`)

func TestReplicateExecutorRegistered(t *testing.T) {
	if Get("replicate") == nil {
		t.Fatal("replicate executor not registered")
	}
}

func replicateStreamPrediction(t *testing.T, srv *httptest.Server) string {
	t.Helper()
	return fmt.Sprintf(`{"status":"starting","urls":{"get":"%s/v1/predictions/live","cancel":"%s/v1/predictions/live/cancel","stream":"%s/stream"}}`, srv.URL, srv.URL, srv.URL)
}

func TestReplicateRelaysNativeStreamWithRealUsage(t *testing.T) {
	replicateEnableLocalDispatch(t)
	var srv *httptest.Server
	var calls *[]replicateCall
	srv, calls = replicateServer(t, func(w http.ResponseWriter, r *http.Request, _ int) {
		switch {
		case r.Method == http.MethodPost:
			replicateJSON(w, 200, replicateStreamPrediction(t, srv))
		case r.URL.Path == "/stream":
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, "event: output\ndata: Hel\n\nevent: output\ndata:  lo\n\nevent: done\ndata: {}\n\n")
		case r.URL.Path == "/v1/predictions/live":
			replicateJSON(w, 200, `{"status":"succeeded","metrics":{"input_token_count":11,"output_token_count":2}}`)
		default:
			replicateJSON(w, 404, `{"detail":"unexpected path"}`)
		}
	})
	rec := replicateRun(t, srv, replicateTestModel, `{"messages":[{"role":"user","content":"Hi"}]}`, true)
	body := rec.Body.String()
	deltas := replicateDeltaRE.FindAllStringSubmatch(body, -1)
	if len(deltas) != 2 || deltas[0][1] != "Hel" || deltas[1][1] != " lo" {
		t.Fatalf("deltas = %v", deltas)
	}
	for _, want := range []string{`"prompt_tokens":11`, `"completion_tokens":2`, `"total_tokens":13`, `"finish_reason":"stop"`, "data: [DONE]"} {
		if !strings.Contains(body, want) {
			t.Errorf("stream missing %s: %s", want, body)
		}
	}
	for _, call := range *calls {
		if call.path == "/stream" {
			if call.header.Get("Accept") != "text/event-stream" || call.header.Get("Cache-Control") != "no-store" {
				t.Errorf("stream headers = %v", call.header)
			}
			if call.header.Get("Authorization") != "" {
				t.Error("stream request carried credentials")
			}
		}
		if call.method == http.MethodPost && call.header.Get("Prefer") != "" {
			t.Error("streaming POST kept Prefer header")
		}
	}
}

func TestReplicateStreamErrorEventIsSanitized(t *testing.T) {
	replicateEnableLocalDispatch(t)
	var srv *httptest.Server
	srv, _ = replicateServer(t, func(w http.ResponseWriter, r *http.Request, _ int) {
		if r.Method == http.MethodPost {
			replicateJSON(w, 200, replicateStreamPrediction(t, srv))
			return
		}
		if r.URL.Path == "/stream" {
			_, _ = io.WriteString(w, "event: error\ndata: {\"detail\":\"boom at /srv/app/x.js\"}\n\n")
			return
		}
		replicateJSON(w, 200, `{"status":"succeeded"}`)
	})
	rec := replicateRun(t, srv, replicateTestModel, `{"messages":[{"role":"user","content":"Hi"}]}`, true)
	body := rec.Body.String()
	if !strings.Contains(body, "Replicate prediction failed") || !strings.Contains(body, "data: [DONE]") {
		t.Fatalf("error stream = %s", body)
	}
	if strings.Contains(body, "/srv/app") {
		t.Errorf("leaked path: %s", body)
	}
}

func TestReplicateUntrustedStreamURLFallsBackToPolling(t *testing.T) {
	replicateEnableLocalDispatch(t)
	replicateSetPoll(t, time.Millisecond, time.Second)
	var srv *httptest.Server
	var calls *[]replicateCall
	srv, calls = replicateServer(t, func(w http.ResponseWriter, r *http.Request, _ int) {
		if r.Method == http.MethodPost {
			replicateJSON(w, 200, fmt.Sprintf(`{"status":"starting","urls":{"get":"%s/v1/predictions/p1","stream":"https://attacker.example/stream"}}`, srv.URL))
			return
		}
		replicateJSON(w, 200, `{"status":"succeeded","output":["ok"]}`)
	})
	rec := replicateRun(t, srv, replicateTestModel, `{"messages":[{"role":"user","content":"Hi"}]}`, true)
	if !strings.Contains(rec.Body.String(), `"content":"ok"`) {
		t.Fatalf("fallback body = %s", rec.Body.String())
	}
	for _, call := range *calls {
		if strings.Contains(string(call.body), "attacker.example") || call.path == "/stream" {
			t.Errorf("untrusted stream host was contacted: %+v", call)
		}
	}
}

func TestReplicateSynthesizesStreamForFinishedPrediction(t *testing.T) {
	replicateEnableLocalDispatch(t)
	srv, calls := replicateServer(t, func(w http.ResponseWriter, _ *http.Request, _ int) {
		replicateJSON(w, 200, `{"status":"succeeded","output":["streamed text"]}`)
	})
	rec := replicateRun(t, srv, replicateTestModel, `{"messages":[{"role":"user","content":"Hi"}]}`, true)
	body := rec.Body.String()
	for _, want := range []string{`"delta":{"role":"assistant"}`, `"content":"streamed text"`, `"finish_reason":"stop"`, "data: [DONE]"} {
		if !strings.Contains(body, want) {
			t.Errorf("synthesized stream missing %s: %s", want, body)
		}
	}
	if !strings.Contains(string((*calls)[0].body), `"stream":true`) {
		t.Errorf("payload = %s", (*calls)[0].body)
	}
}

func TestReplicateNeverSendsCredentialWithoutOne(t *testing.T) {
	replicateEnableLocalDispatch(t)
	srv, calls := replicateServer(t, func(w http.ResponseWriter, _ *http.Request, _ int) {
		replicateJSON(w, 200, `{"status":"succeeded","output":"ok"}`)
	})
	rec := httptest.NewRecorder()
	req := &Request{
		Ctx: context.Background(), Client: srv.Client(), Config: &providers.ProviderConfig{},
		Provider: "replicate", Body: []byte(`{"messages":[{"role":"user","content":"Hi"}]}`),
		ModelName: replicateTestModel,
		ConnData:  map[string]any{"baseUrl": srv.URL + "/v1"},
	}
	if err := ForwardReplicate(rec, req); err != nil {
		t.Fatalf("ForwardReplicate: %v", err)
	}
	if rec.Code != 200 {
		t.Fatalf("status = %d", rec.Code)
	}
	if (*calls)[0].header.Get("Authorization") != "" {
		t.Error("executor invented a credential")
	}
}

func TestIsTrustedReplicateStreamURL(t *testing.T) {
	requestURL := "https://api.replicate.com/v1/predictions"
	tests := []struct {
		url  string
		want bool
	}{
		{"https://stream.replicate.com/v1/streams/abc", true},
		{"https://replicate.com/stream", true},
		{"https://api.replicate.com/v1/streams/x", true},
		{"https://stream.replicate.com.evil.test/x", false},
		{"http://stream.replicate.com/x", false},
		{"https://attacker.example/stream", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := isTrustedStreamURL(tt.url, requestURL); got != tt.want {
			t.Errorf("isTrustedStreamURL(%q) = %v, want %v", tt.url, got, tt.want)
		}
	}
}
