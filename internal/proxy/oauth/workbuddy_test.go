// 9RouterX — port and modifications by https://github.com/yanstd888/ — https://t.me/aiclub_id
package oauth

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestWorkbuddyRefreshHeadersAndRotation(t *testing.T) {
	for _, replacement := range []string{"", "new-refresh"} {
		t.Run("replacement="+replacement, func(t *testing.T) {
			client := &http.Client{Transport: amazonQRoundTrip(func(r *http.Request) (*http.Response, error) {
				if r.URL.String() != "https://www.workbuddy.ai/v2/plugin/auth/token/refresh" || r.Method != http.MethodPost {
					t.Errorf("refresh request = %s %s", r.Method, r.URL)
				}
				for header, want := range map[string]string{"X-Refresh-Token": "old-refresh", "X-Product": "SaaS", "X-Domain": "www.workbuddy.ai", "User-Agent": "WorkBuddy/5.5.6"} {
					if got := r.Header.Get(header); got != want {
						t.Errorf("%s = %q, want %q", header, got, want)
					}
				}
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Fatal(err)
				}
				if string(body) != `{}` {
					t.Errorf("refresh token must not be sent in body: %s", body)
				}
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"code":0,"data":{"accessToken":"new-access","refreshToken":"` + replacement + `","expiresIn":7200}}`))}, nil
			})}
			refresh := Get("workbuddy")
			if refresh == nil {
				t.Fatal("custom refresher is not registered")
			}
			result, err := refresh(t.Context(), &Params{Client: client, Provider: "workbuddy", RefreshToken: "old-refresh"})
			if err != nil {
				t.Fatal(err)
			}
			want := replacement
			if want == "" {
				want = "old-refresh"
			}
			if result.AccessToken != "new-access" || result.RefreshToken != want || result.ExpiresIn != 7200 {
				t.Fatalf("refresh result = %+v", result)
			}
		})
	}
}
