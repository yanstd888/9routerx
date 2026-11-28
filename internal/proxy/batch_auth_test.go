// 9RouterX — port and modifications by https://github.com/yanstd888/ — https://t.me/aiclub_id
package proxy

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"9routerx/proxy/internal/providers"
)

func TestProviderBatchAuthHeaders(t *testing.T) {
	for _, tc := range []struct {
		name     string
		key      string
		optional bool
		noAuth   bool
		scheme   string
		want     string
	}{
		{"optional anonymous", "public", true, false, "bearer", ""},
		{"optional empty", "", true, false, "bearer", ""},
		{"optional supplied", "fixture-key", true, false, "bearer", "Bearer fixture-key"},
		{"no-auth stays credential-free", "fixture-key", false, true, "bearer", ""},
		{"Maritalk Key scheme", "fixture-key", false, false, "key", "Key fixture-key"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if got := r.Header.Get("Authorization"); got != tc.want {
					t.Errorf("Authorization = %q, want %q", got, tc.want)
				}
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()
			cfg := &providers.ProviderConfig{BaseURL: server.URL, AuthHeader: "Authorization", AuthScheme: tc.scheme, AuthOptional: tc.optional, NoAuth: tc.noAuth}
			resp, err := ForwardOpenAI(WithAllowLoopback(t.Context()), server.Client(), cfg, tc.key, []byte(`{}`), false)
			if err != nil {
				t.Fatal(err)
			}
			resp.Body.Close()
		})
	}
}
