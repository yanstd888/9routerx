// 9RouterX — offline fixtures for the pinned Replicate executor (OmniRoute 632d84f5).
package executor

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"9routerx/proxy/internal/providers"
	"9routerx/proxy/internal/proxy"
)

const replicateTestModel = "meta/meta-llama-3-70b-instruct"

type replicateCall struct {
	method, path string
	header       http.Header
	body         []byte
}

func replicateEnableLocalDispatch(t *testing.T) {
	t.Helper()
	prior := proxy.AllowPrivateProviderURLs()
	proxy.SetAllowPrivateProviderURLs(true)
	t.Cleanup(func() { proxy.SetAllowPrivateProviderURLs(prior) })
}

func replicateServer(t *testing.T, handler func(http.ResponseWriter, *http.Request, int)) (*httptest.Server, *[]replicateCall) {
	t.Helper()
	calls := &[]replicateCall{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		*calls = append(*calls, replicateCall{r.Method, r.URL.Path, r.Header.Clone(), body})
		handler(w, r, len(*calls)-1)
	}))
	t.Cleanup(srv.Close)
	return srv, calls
}

func replicateJSON(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, body)
}

func replicateRun(t *testing.T, srv *httptest.Server, model, body string, stream bool) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := &Request{
		Ctx: context.Background(), Client: srv.Client(), Config: &providers.ProviderConfig{},
		Provider: "replicate", APIKey: "r8_test", Body: []byte(body),
		IsStream: stream, ModelName: model,
		ConnData: map[string]any{"baseUrl": srv.URL + "/v1"},
	}
	if err := ForwardReplicate(rec, req); err != nil {
		t.Fatalf("ForwardReplicate: %v", err)
	}
	return rec
}

func replicateSetPoll(t *testing.T, interval, deadline time.Duration) {
	t.Helper()
	priorInterval, priorDeadline := replicatePollInterval, replicatePollDeadline
	replicatePollInterval, replicatePollDeadline = interval, deadline
	t.Cleanup(func() { replicatePollInterval, replicatePollDeadline = priorInterval, priorDeadline })
}

func TestReplicateOfficialModelPostAndCompletion(t *testing.T) {
	replicateEnableLocalDispatch(t)
	srv, calls := replicateServer(t, func(w http.ResponseWriter, _ *http.Request, _ int) {
		replicateJSON(w, 200, `{"status":"succeeded","output":["Hello",", ","world"]}`)
	})
	body := `{"messages":[{"role":"system","content":"Be brief."},{"role":"user","content":"Hi"}],"max_tokens":64,"temperature":0.5}`
	rec := replicateRun(t, srv, replicateTestModel, body, false)

	if len(*calls) != 1 {
		t.Fatalf("calls = %d, want 1", len(*calls))
	}
	call := (*calls)[0]
	if call.method != http.MethodPost || call.path != "/v1/models/"+replicateTestModel+"/predictions" {
		t.Fatalf("upstream = %s %s", call.method, call.path)
	}
	if got := call.header.Get("Authorization"); got != "Bearer r8_test" {
		t.Errorf("Authorization = %q", got)
	}
	if got := call.header.Get("Prefer"); got != "wait=60" {
		t.Errorf("Prefer = %q", got)
	}
	wantBody := `{"input":{"prompt":"Hi","system_prompt":"Be brief.","max_tokens":64,"temperature":0.5}}`
	if string(call.body) != wantBody {
		t.Errorf("payload = %s, want %s", call.body, wantBody)
	}
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"content":"Hello, world"`) {
		t.Fatalf("completion = %d %s", rec.Code, rec.Body.String())
	}
}

func TestReplicateVersionedModelUsesGenericPredictionsURL(t *testing.T) {
	replicateEnableLocalDispatch(t)
	srv, calls := replicateServer(t, func(w http.ResponseWriter, _ *http.Request, _ int) {
		replicateJSON(w, 200, `{"status":"succeeded","output":"v"}`)
	})
	rec := replicateRun(t, srv, "owner/custom-model:abc123def456", `{"messages":[{"role":"user","content":"Hi"}]}`, false)
	if rec.Code != 200 {
		t.Fatalf("status = %d", rec.Code)
	}
	call := (*calls)[0]
	if call.path != "/v1/predictions" {
		t.Errorf("path = %s", call.path)
	}
	if !strings.Contains(string(call.body), `"version":"abc123def456"`) {
		t.Errorf("payload = %s", call.body)
	}
}

func TestReplicateUsageMetricsAndLengthFinish(t *testing.T) {
	replicateEnableLocalDispatch(t)
	srv, _ := replicateServer(t, func(w http.ResponseWriter, _ *http.Request, _ int) {
		replicateJSON(w, 200, `{"status":"succeeded","output":["cut off"],"metrics":{"input_token_count":5,"output_token_count":16}}`)
	})
	rec := replicateRun(t, srv, replicateTestModel, `{"messages":[{"role":"user","content":"Hi"}],"max_tokens":16}`, false)
	body := rec.Body.String()
	if !strings.Contains(body, `"prompt_tokens":5`) || !strings.Contains(body, `"completion_tokens":16`) || !strings.Contains(body, `"total_tokens":21`) {
		t.Errorf("usage = %s", body)
	}
	if !strings.Contains(body, `"finish_reason":"length"`) {
		t.Errorf("finish = %s", body)
	}
}

func TestReplicateRejectsInvalidModelAndMissingUser(t *testing.T) {
	replicateEnableLocalDispatch(t)
	tests := []struct {
		name, model, body string
	}{
		{"malformed id", "../../etc/passwd", `{"messages":[{"role":"user","content":"Hi"}]}`},
		{"no user turn", replicateTestModel, `{"messages":[{"role":"system","content":"only system"}]}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, calls := replicateServer(t, func(w http.ResponseWriter, _ *http.Request, _ int) {
				replicateJSON(w, 200, `{}`)
			})
			rec := replicateRun(t, srv, tt.model, tt.body, false)
			if rec.Code != 400 {
				t.Fatalf("status = %d", rec.Code)
			}
			if len(*calls) != 0 {
				t.Errorf("upstream called %d times before validation", len(*calls))
			}
		})
	}
}

func TestReplicateUpstreamStatusRetryAfterAndSanitizing(t *testing.T) {
	replicateEnableLocalDispatch(t)
	tests := []struct {
		name, detail, retryAfter string
		status                   int
	}{
		{"unauthorized", "Invalid token at /srv/app/secret.js", "3", 401},
		{"rate limited", "Too many requests", "7", 429},
		{"verbose detail", strings.Repeat("x", 5000), "", 400},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, _ := replicateServer(t, func(w http.ResponseWriter, _ *http.Request, _ int) {
				w.Header().Set("Retry-After", tt.retryAfter)
				replicateJSON(w, tt.status, fmt.Sprintf(`{"detail":%q}`, tt.detail))
			})
			rec := replicateRun(t, srv, replicateTestModel, `{"messages":[{"role":"user","content":"Hi"}]}`, false)
			if rec.Code != tt.status {
				t.Fatalf("status = %d, want %d", rec.Code, tt.status)
			}
			if tt.retryAfter != "" && rec.Header().Get("Retry-After") != tt.retryAfter {
				t.Errorf("Retry-After = %q", rec.Header().Get("Retry-After"))
			}
			if strings.Contains(rec.Body.String(), "/srv/app") {
				t.Errorf("leaked path: %s", rec.Body.String())
			}
			if len(rec.Body.String()) >= 400 {
				t.Errorf("detail not bounded: %d bytes", len(rec.Body.String()))
			}
		})
	}
}

func TestReplicatePollsUntilSucceeded(t *testing.T) {
	replicateEnableLocalDispatch(t)
	replicateSetPoll(t, time.Millisecond, time.Second)
	var gets int
	var srv *httptest.Server
	var calls *[]replicateCall
	srv, calls = replicateServer(t, func(w http.ResponseWriter, r *http.Request, _ int) {
		if r.Method == http.MethodPost {
			replicateJSON(w, 200, fmt.Sprintf(`{"status":"processing","urls":{"get":"%s/v1/predictions/abc"}}`, srv.URL))
			return
		}
		gets++
		if gets == 1 {
			replicateJSON(w, 200, `{"status":"processing"}`)
			return
		}
		replicateJSON(w, 200, `{"status":"succeeded","output":["done"]}`)
	})
	rec := replicateRun(t, srv, replicateTestModel, `{"messages":[{"role":"user","content":"Hi"}]}`, false)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"content":"done"`) {
		t.Fatalf("completion = %d %s", rec.Code, rec.Body.String())
	}
	if len(*calls) != 3 || (*calls)[1].method != http.MethodGet {
		t.Fatalf("calls = %+v", *calls)
	}
	if (*calls)[1].header.Get("Prefer") != "" {
		t.Error("polling GET kept Prefer header")
	}
}

func TestReplicatePollRejectsForeignGetURL(t *testing.T) {
	replicateEnableLocalDispatch(t)
	replicateSetPoll(t, time.Millisecond, 20*time.Millisecond)
	srv, calls := replicateServer(t, func(w http.ResponseWriter, _ *http.Request, _ int) {
		replicateJSON(w, 200, `{"status":"processing","urls":{"get":"https://attacker.example/steal"}}`)
	})
	rec := replicateRun(t, srv, replicateTestModel, `{"messages":[{"role":"user","content":"Hi"}]}`, false)
	if rec.Code != 502 {
		t.Fatalf("status = %d", rec.Code)
	}
	if len(*calls) != 1 {
		t.Errorf("followed foreign URL: %+v", *calls)
	}
}

func TestReplicateTimeoutCancelsPrediction(t *testing.T) {
	replicateEnableLocalDispatch(t)
	replicateSetPoll(t, time.Millisecond, 5*time.Millisecond)
	var srv *httptest.Server
	var calls *[]replicateCall
	srv, calls = replicateServer(t, func(w http.ResponseWriter, r *http.Request, _ int) {
		if r.Method == http.MethodPost && !strings.HasSuffix(r.URL.Path, "/cancel") {
			replicateJSON(w, 200, fmt.Sprintf(`{"status":"processing","urls":{"get":"%s/v1/predictions/slow","cancel":"%s/v1/predictions/slow/cancel"}}`, srv.URL, srv.URL))
			return
		}
		if strings.HasSuffix(r.URL.Path, "/cancel") {
			replicateJSON(w, 200, `{}`)
			return
		}
		replicateJSON(w, 200, fmt.Sprintf(`{"status":"processing","urls":{"cancel":"%s/v1/predictions/slow/cancel"}}`, srv.URL))
	})
	rec := replicateRun(t, srv, replicateTestModel, `{"messages":[{"role":"user","content":"Hi"}]}`, false)
	if rec.Code != 504 {
		t.Fatalf("status = %d", rec.Code)
	}
	cancelled := false
	for _, call := range *calls {
		if call.method == http.MethodPost && strings.HasSuffix(call.path, "/cancel") {
			cancelled = true
		}
	}
	if !cancelled {
		t.Error("prediction was not cancelled on timeout")
	}
}

func TestReplicateFailedPredictionStatus(t *testing.T) {
	replicateEnableLocalDispatch(t)
	for _, status := range []string{"failed", "canceled"} {
		t.Run(status, func(t *testing.T) {
			srv, _ := replicateServer(t, func(w http.ResponseWriter, _ *http.Request, _ int) {
				replicateJSON(w, 200, fmt.Sprintf(`{"status":%q,"error":"model crashed"}`, status))
			})
			rec := replicateRun(t, srv, replicateTestModel, `{"messages":[{"role":"user","content":"Hi"}]}`, false)
			if rec.Code != 502 || !strings.Contains(rec.Body.String(), "model crashed") {
				t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestReplicateFlattensToolsAndFirstImage(t *testing.T) {
	replicateEnableLocalDispatch(t)
	srv, calls := replicateServer(t, func(w http.ResponseWriter, _ *http.Request, _ int) {
		replicateJSON(w, 200, `{"status":"succeeded","output":"ok"}`)
	})
	body := `{"messages":[{"role":"user","content":[{"type":"text","text":"What is this?"},{"type":"image_url","image_url":{"url":"https://example.com/a.png"}}]},{"role":"assistant","content":"","tool_calls":[{"function":{"name":"lookup","arguments":"{\"q\":1}"}}]},{"role":"tool","content":"result-42"}]}`
	replicateRun(t, srv, replicateTestModel, body, false)
	payload := string((*calls)[0].body)
	for _, want := range []string{`"image":"https://example.com/a.png"`, "[tool call lookup(", "Tool result: result-42", `Assistant:`} {
		if !strings.Contains(payload, want) {
			t.Errorf("payload missing %s: %s", want, payload)
		}
	}
}
