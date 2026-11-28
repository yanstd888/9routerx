// 9RouterX — port and modifications by https://github.com/yanstd888/ — https://t.me/aiclub_id
package oauth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWorkbuddyPollPreservesRefreshAndExpiry(t *testing.T) {
	original := deviceClient.Transport
	deviceClient.Transport = deviceRoundTripperFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "www.workbuddy.ai" || r.URL.Path != "/v2/plugin/auth/token" || r.URL.Query().Get("state") != "fixture-state" || r.Header.Get("X-Product") != "SaaS" {
			t.Errorf("unexpected token poll: %s %v", r.URL, r.Header)
		}
		rec := httptest.NewRecorder()
		_, _ = rec.WriteString(`{"code":0,"data":{"accessToken":"fixture-access","refreshToken":"fixture-refresh","expiresIn":7200}}`)
		return rec.Result(), nil
	})
	t.Cleanup(func() { deviceClient.Transport = original })
	tokens, err := workbuddyPoll(t.Context(), "fixture-state")
	if err != nil {
		t.Fatal(err)
	}
	if tokens.access != "fixture-access" || tokens.refresh != "fixture-refresh" || tokens.expiresIn != 7200 {
		t.Fatalf("device token metadata discarded: %+v", tokens)
	}
}
