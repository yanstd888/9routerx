// 9RouterX — port and modifications by https://github.com/yanstd888/ — https://t.me/aiclub_id
package oauth

import (
	json "encoding/json/v2"
	"io"
	"net/http"
	"strings"
	"testing"
)

type amazonQRoundTrip func(*http.Request) (*http.Response, error)

func (f amazonQRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestAmazonQUsesAWSDeviceRefresher(t *testing.T) {
	refresh := Get("amazon-q")
	if refresh == nil {
		t.Fatal("Amazon Q device tokens have no registered refresher")
	}
	client := &http.Client{Transport: amazonQRoundTrip(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodPost || r.URL.String() != "https://oidc.us-east-1.amazonaws.com/token" {
			t.Errorf("refresh request = %s %s", r.Method, r.URL)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		var payload map[string]string
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatal(err)
		}
		if payload["grantType"] != "refresh_token" || payload["clientId"] != "fixture-client" || payload["clientSecret"] != "fixture-secret" || payload["refreshToken"] != "fixture-refresh" {
			t.Errorf("AWS device credentials not preserved: %s", body)
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"accessToken":"fixture-access","refreshToken":"fixture-rotated","expiresIn":3600}`))}, nil
	})}
	result, err := refresh(t.Context(), &Params{Client: client, Provider: "amazon-q", RefreshToken: "fixture-refresh", ProviderSpecificData: map[string]string{"clientId": "fixture-client", "clientSecret": "fixture-secret", "region": "us-east-1"}})
	if err != nil {
		t.Fatal(err)
	}
	if result.AccessToken != "fixture-access" || result.RefreshToken != "fixture-rotated" {
		t.Fatalf("refresh result = %+v", result)
	}
}
