// 9RouterX — port and modifications by https://github.com/yanstd888/ — https://t.me/aiclub_id
package proxy

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"9routerx/proxy/internal/providers"
)

func TestGatewayBatchPreservesPinnedEndpointAndHeaders(t *testing.T) {
	for _, tc := range []struct {
		provider string
		path     string
		headers  map[string]string
	}{
		{"gitlawb", "/v1/xiaomi-mimo", map[string]string{"User-Agent": "OpenClaude/1.0 (linux; x86_64)", "X-Title": "OpenClaude CLI", "HTTP-Referer": "https://github.com/Gitlawb/openclaude"}},
		{"orcarouter", "/v1/chat/completions", map[string]string{"X-Title": "Endpoint Proxy", "HTTP-Referer": "https://endpoint-proxy.local"}},
	} {
		t.Run(tc.provider, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != tc.path {
					t.Errorf("path = %q, want %q", r.URL.Path, tc.path)
				}
				for header, want := range tc.headers {
					if got := r.Header.Get(header); got != want {
						t.Errorf("%s = %q, want %q", header, got, want)
					}
				}
				if r.Header.Get("Authorization") != "Bearer fixture-key" {
					t.Error("Bearer key lost")
				}
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()
			cfg := providers.KnownProviders[tc.provider]
			cfg.BaseURL = server.URL + tc.path
			resp, err := ForwardOpenAI(WithAllowLoopback(t.Context()), server.Client(), &cfg, "fixture-key", []byte(`{}`), false)
			if err != nil {
				t.Fatal(err)
			}
			resp.Body.Close()
		})
	}
}
