// 9RouterX — port and modifications by https://github.com/yanstd888/ — https://t.me/aiclub_id
package oauth

import (
	json "encoding/json/v2"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpenferenceUsesFixedRedirect(t *testing.T) {
	h := NewOAuthHandler(nil)
	rec := httptest.NewRecorder()
	h.HandlePKCEAuthorize(rec, httptest.NewRequest(http.MethodGet, "/api/oauth/pkce/authorize?provider=openference&redirect_uri=http://example.com/callback", nil))
	var response map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response["redirectUri"] != openferenceRedirectURI || !strings.Contains(response["authUrl"], "56123") {
		t.Fatalf("fixed redirect lost: %s", rec.Body.String())
	}
}

func TestOpenferenceLoopbackCompletesOwnProvider(t *testing.T) {
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if r.Form.Get("client_id") != "omniroute" || r.Form.Get("code_verifier") != "fixture-verifier" || r.Form.Get("redirect_uri") != openferenceRedirectURI {
			t.Errorf("Openference grant mixed with another provider: %v", r.Form)
		}
		_, _ = w.Write([]byte(`{"access_token":"fixture-access","refresh_token":"fixture-refresh","expires_in":3600}`))
	}))
	defer mock.Close()
	old := pkceProviders["openference"]
	cfg := *old
	cfg.tokenURL = mock.URL
	pkceProviders["openference"] = &cfg
	oldLoopback := openferenceLoopback
	openferenceLoopback = &codexProxy{port: 0, sessions: map[string]*codexSession{}}
	t.Cleanup(func() {
		openferenceLoopback.stop()
		openferenceLoopback = oldLoopback
		pkceProviders["openference"] = old
	})
	h := NewOAuthHandler(nil)
	rec := httptest.NewRecorder()
	h.HandleOpenferenceStartProxy(rec, httptest.NewRequest(http.MethodGet, "/api/oauth/openference/start-proxy?app_port=20128&state=fixture-state&code_verifier=fixture-verifier&redirect_uri="+openferenceRedirectURI, nil))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"success":true`) {
		t.Fatalf("listener did not start: %s", rec.Body.String())
	}
	callback := httptest.NewRecorder()
	openferenceLoopback.ServeHTTP(callback, httptest.NewRequest(http.MethodGet, "/callback?state=fixture-state&code=fixture-code", nil))
	poll := httptest.NewRecorder()
	h.HandleOpenferencePollStatus(poll, httptest.NewRequest(http.MethodGet, "/api/oauth/openference/poll-status?state=fixture-state", nil))
	if !strings.Contains(poll.Body.String(), `"status":"done"`) || !strings.Contains(poll.Body.String(), `"connectionId":"of-`) {
		t.Fatalf("wrong exchange outcome: %s", poll.Body.String())
	}
}
