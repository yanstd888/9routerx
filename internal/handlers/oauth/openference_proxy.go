// 9RouterX — port and modifications by https://github.com/yanstd888/ — https://t.me/aiclub_id
package oauth

import (
	"context"
	"fmt"
	"net/http"

	"9routerx/proxy/internal/handlerutil"
)

const openferenceRedirectURI = "http://127.0.0.1:56123/callback"
const openferenceLoopbackPort = 56123

// Reuse the state-bound listener lifecycle, not Codex credentials or endpoints.
var openferenceLoopback = &codexProxy{port: openferenceLoopbackPort, sessions: map[string]*codexSession{}}

func (h *OAuthHandler) HandleOpenferenceStartProxy(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	state, verifier := q.Get("state"), q.Get("code_verifier")
	if state == "" || verifier == "" || q.Get("app_port") == "" {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "missing app_port, state or code_verifier")
		return
	}
	if q.Get("redirect_uri") != openferenceRedirectURI {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "Openference requires "+openferenceRedirectURI)
		return
	}
	err := openferenceLoopback.start(q.Get("app_port"), func(ctx context.Context, code string, sess *codexSession) (pkceExchangeResult, error) {
		cfg := pkceLookup("openference")
		ex := &pkceExchange{cfg: cfg, provider: "openference", code: code, codeVerifier: sess.codeVerifier, redirectURI: sess.redirectURI, name: sess.name, clientID: cfg.clientID, tokenURL: cfg.tokenURL}
		res, fail := h.completePKCEExchange(ctx, ex)
		if fail != nil {
			return pkceExchangeResult{}, fmt.Errorf("openference exchange: %s", fail.message)
		}
		return res, nil
	})
	if err != nil {
		reason := "start_failed"
		if isAddrInUse(err) {
			reason = "port_busy"
		}
		handlerutil.WriteJSON(w, http.StatusOK, map[string]any{"success": false, "reason": reason})
		return
	}
	openferenceLoopback.register(state, verifier, openferenceRedirectURI, q.Get("name"))
	handlerutil.WriteJSON(w, http.StatusOK, map[string]any{"success": true, "serverSide": true, "port": openferenceLoopbackPort})
}

func (h *OAuthHandler) HandleOpenferencePollStatus(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	if state == "" {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "missing state")
		return
	}
	sess, ok := openferenceLoopback.lookup(state)
	if !ok {
		handlerutil.WriteJSON(w, http.StatusOK, map[string]any{"status": "unknown"})
		return
	}
	if sess.status == "done" || sess.status == "error" {
		openferenceLoopback.clear(state)
	}
	handlerutil.WriteJSON(w, http.StatusOK, map[string]any{"status": sess.status, "connectionId": sess.connectionID, "email": sess.email, "error": sess.errMsg})
}

func (h *OAuthHandler) HandleOpenferenceStopProxy(w http.ResponseWriter, r *http.Request) {
	openferenceLoopback.stop()
	handlerutil.WriteJSON(w, http.StatusOK, map[string]any{"success": true})
}
