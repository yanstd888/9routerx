// 9RouterX — port and modifications by https://github.com/yanstd888/ — https://t.me/aiclub_id
package oauth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestLoopbackClaimsCallbackBeforeExchange(t *testing.T) {
	proxy := &codexProxy{sessions: map[string]*codexSession{}}
	proxy.register("fixture-state", "fixture-verifier", devinRedirectURI, "")
	started, finish := make(chan struct{}), make(chan struct{})
	var hits atomic.Int32
	proxy.exchange = func(context.Context, string, *codexSession) (pkceExchangeResult, error) {
		hits.Add(1)
		close(started)
		<-finish
		return pkceExchangeResult{ConnectionID: "devind-fixture"}, nil
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		proxy.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/callback?state=fixture-state&code=fixture-code", nil))
	}()
	<-started
	replay := httptest.NewRecorder()
	proxy.ServeHTTP(replay, httptest.NewRequest(http.MethodGet, "/callback?state=fixture-state&code=fixture-code", nil))
	close(finish)
	<-done
	if replay.Code != http.StatusConflict || hits.Load() != 1 {
		t.Fatalf("duplicate callback exchanged: status=%d hits=%d", replay.Code, hits.Load())
	}
	session, _ := proxy.lookup("fixture-state")
	if session.status != "done" {
		t.Fatalf("replay changed terminal status: %s", session.status)
	}
}
