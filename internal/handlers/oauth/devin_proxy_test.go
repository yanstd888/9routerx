// 9RouterX — port and modifications by https://github.com/yanstd888/ — https://t.me/aiclub_id
package oauth

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestDevinLoopbackCompletesMatchingAuthorization(t *testing.T) {
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"token":"fixture-session-token"}`))
	}))
	defer mock.Close()
	oldURL := devinTokenURL
	devinTokenURL = mock.URL
	oldLoopback := devinLoopback
	devinLoopback = &codexProxy{port: 0, sessions: map[string]*codexSession{}}
	t.Cleanup(func() { devinLoopback.stop(); devinLoopback = oldLoopback; devinTokenURL = oldURL })
	h := NewOAuthHandler(nil)
	state, verifier := devinTestAuthorization(t, h)
	q := url.Values{"app_port": {"20130"}, "state": {state}, "code_verifier": {verifier}, "redirect_uri": {devinRedirectURI}}
	rec := httptest.NewRecorder()
	h.HandleDevinStartProxy(rec, httptest.NewRequest(http.MethodGet, "/api/oauth/devin-desktop/start-proxy?"+q.Encode(), nil))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"success":true`) {
		t.Fatalf("listener failed: %s", rec.Body.String())
	}
	cb := httptest.NewRecorder()
	devinLoopback.ServeHTTP(cb, httptest.NewRequest(http.MethodGet, "/callback?state="+state+"&code=fixture-code", nil))
	poll := httptest.NewRecorder()
	h.HandleDevinPollStatus(poll, httptest.NewRequest(http.MethodGet, "/api/oauth/devin-desktop/poll-status?state="+state, nil))
	if !strings.Contains(poll.Body.String(), `"status":"done"`) {
		t.Fatalf("callback did not complete: %s", poll.Body.String())
	}
}
