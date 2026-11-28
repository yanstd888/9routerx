// 9RouterX — port and modifications by https://github.com/yanstd888/ — https://t.me/aiclub_id
package executor

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"9routerx/proxy/internal/providers"
	"9routerx/proxy/internal/proxy"
)

func TestWorkbuddyUpstreamStreamPreservesDownstreamMode(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(map[bool]string{false: "buffered", true: "streamed"}[stream], func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("X-Product") != "SaaS" || r.Header.Get("Authorization") != "Bearer fixture-key" {
					t.Errorf("required request headers missing: %v", r.Header)
				}
				var body map[string]any
				if err := json.Unmarshal(mustReadT5Body(t, r.Body), &body); err != nil {
					t.Fatal(err)
				}
				if body["stream"] != true {
					t.Errorf("upstream stream was not forced: %v", body)
				}
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, "data: {\"id\":\"fixture\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"fixture-ok\"}}]}\n\ndata: [DONE]\n\n")
			}))
			defer server.Close()
			cfg := providers.KnownProviders["workbuddy"]
			cfg.BaseURL = server.URL + "/v2/chat/completions"
			body := []byte(`{"model":"fixture-model","messages":[{"role":"user","content":"hello"}],"stream":false}`)
			rec := httptest.NewRecorder()
			if err := ForwardWorkbuddy(rec, &Request{Ctx: proxy.WithAllowLoopback(t.Context()), Client: server.Client(), Config: &cfg, APIKey: "fixture-key", Body: body, IsStream: stream, Provider: "workbuddy"}); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(rec.Body.String(), "fixture-ok") {
				t.Fatalf("empty response: %s", rec.Body.String())
			}
			if stream && !strings.Contains(rec.Header().Get("Content-Type"), "text/event-stream") {
				t.Fatal("streamed client did not receive SSE")
			}
			if !stream && !jsontext.Value(rec.Body.Bytes()).IsValid() {
				t.Fatalf("buffered client received non-JSON: %s", rec.Body.String())
			}
			if !strings.Contains(string(body), `"stream":false`) {
				t.Fatal("original request mutated")
			}
		})
	}
}
