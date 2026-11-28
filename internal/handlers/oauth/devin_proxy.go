// 9RouterX — port and modifications by https://github.com/yanstd888/ — https://t.me/aiclub_id
package oauth

import (
	"context"
	"net/http"

	"9routerx/proxy/internal/handlerutil"
)

var devinLoopback = &codexProxy{port: devinLoopbackPort, sessions: map[string]*codexSession{}}

func (h *OAuthHandler) HandleDevinStartProxy(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	state, verifier := q.Get("state"), q.Get("code_verifier")
	devinLogin.Lock()
	session, ok := devinLogin.sessions[state]
	devinLogin.Unlock()
	if !ok || session.verifier != verifier || q.Get("app_port") == "" || q.Get("redirect_uri") != devinRedirectURI {
		handlerutil.WriteJSONError(w, 400, "invalid Devin loopback session")
		return
	}
	err := devinLoopback.start(q.Get("app_port"), func(ctx context.Context, code string, sess *codexSession) (pkceExchangeResult, error) {
		return h.exchangeDevinCode(ctx, code, sess.state, sess.codeVerifier, sess.name)
	})
	if err != nil {
		reason := "start_failed"
		if isAddrInUse(err) {
			reason = "port_busy"
		}
		handlerutil.WriteJSON(w, 200, map[string]any{"success": false, "reason": reason})
		return
	}
	devinLoopback.register(state, verifier, devinRedirectURI, q.Get("name"))
	handlerutil.WriteJSON(w, 200, map[string]any{"success": true, "port": devinLoopbackPort})
}

func (h *OAuthHandler) HandleDevinPollStatus(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	if state == "" {
		handlerutil.WriteJSONError(w, 400, "missing state")
		return
	}
	sess, ok := devinLoopback.lookup(state)
	if !ok {
		handlerutil.WriteJSON(w, 200, map[string]any{"status": "unknown"})
		return
	}
	if sess.status == "done" || sess.status == "error" {
		devinLoopback.clear(state)
	}
	handlerutil.WriteJSON(w, 200, map[string]any{"status": sess.status, "connectionId": sess.connectionID, "error": sess.errMsg})
}

func (h *OAuthHandler) HandleDevinStopProxy(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	devinLogin.Lock()
	delete(devinLogin.sessions, state)
	devinLogin.Unlock()
	devinLoopback.stop()
	handlerutil.WriteJSON(w, 200, map[string]any{"success": true})
}
