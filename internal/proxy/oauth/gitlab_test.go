// 9RouterX — port and modifications by https://github.com/yanstd888/ — https://t.me/aiclub_id
package oauth

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestGitlabRefreshUsesStoredInstanceAndClient(t *testing.T) {
	client := &http.Client{Transport: amazonQRoundTrip(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != "https://gitlab.fixture.example/oauth/token" || r.Method != http.MethodPost {
			t.Errorf("unexpected refresh: %s %s", r.Method, r.URL)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if r.Form.Get("client_id") != "fixture-client" || r.Form.Get("client_secret") != "fixture-secret" || r.Form.Get("refresh_token") != "fixture-refresh" {
			t.Errorf("account-specific grant was lost: %v", r.Form)
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"access_token":"new-access","refresh_token":"new-refresh","expires_in":3600}`))}, nil
	})}
	refresh := Get("gitlab")
	if refresh == nil {
		t.Fatal("GitLab refresher is not registered")
	}
	result, err := refresh(t.Context(), &Params{Client: client, Provider: "gitlab", RefreshToken: "fixture-refresh", ProviderSpecificData: map[string]string{"baseUrl": "https://gitlab.fixture.example", "clientId": "fixture-client", "clientSecret": "fixture-secret"}})
	if err != nil {
		t.Fatal(err)
	}
	if result.AccessToken != "new-access" || result.RefreshToken != "new-refresh" {
		t.Fatalf("result = %+v", result)
	}
}
