// 9RouterX — port and modifications by https://github.com/yanstd888/ — https://t.me/aiclub_id
package oauth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	json "encoding/json/v2"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"9routerx/proxy/internal/handlerutil"
)

const devinRedirectURI = "http://127.0.0.1:59653/callback"
const devinLoopbackPort = 59653
const devinTokenPrefix = "devin-session-token$"

var devinAuthorizeURL = "https://app.devin.ai/auth/cli/continue"
var devinTokenURL = "https://api.devin.ai/auth/cli/token"
var devinOAuthClient = &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

// The session store shares the bounded lifetime of the fixed-port listener;
// claiming a state is atomic across manual and loopback exchanges.
var devinLogin = struct {
	sync.Mutex
	sessions map[string]devinAuthSession
}{sessions: make(map[string]devinAuthSession)}

type devinAuthSession struct {
	verifier string
	created  time.Time
}

func (h *OAuthHandler) HandleDevinAuthorize(w http.ResponseWriter, r *http.Request) {
	var raw [96]byte
	if _, err := rand.Read(raw[:]); err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, "could not create Devin authorization verifier")
		return
	}
	verifier := base64.RawURLEncoding.EncodeToString(raw[:])
	state := randomString(32)
	devinLogin.Lock()
	for key, session := range devinLogin.sessions {
		if time.Since(session.created) > 10*time.Minute {
			delete(devinLogin.sessions, key)
		}
	}
	if len(devinLogin.sessions) >= 64 {
		devinLogin.Unlock()
		handlerutil.WriteJSONError(w, http.StatusTooManyRequests, "too many pending Devin authorizations")
		return
	}
	devinLogin.sessions[state] = devinAuthSession{verifier: verifier, created: time.Now()}
	devinLogin.Unlock()
	q := url.Values{"redirect_uri": {devinRedirectURI}, "state": {state}, "prompt": {"select_account"}, "code_challenge": {sha256Base64(verifier)}, "code_challenge_method": {"S256"}}
	handlerutil.WriteJSON(w, http.StatusOK, map[string]any{"url": devinAuthorizeURL + "?" + q.Encode(), "state": state, "codeVerifier": verifier, "redirectUri": devinRedirectURI})
}

func claimDevinState(state, verifier string) bool {
	devinLogin.Lock()
	defer devinLogin.Unlock()
	session, ok := devinLogin.sessions[state]
	if !ok || session.verifier != verifier || time.Since(session.created) > 10*time.Minute {
		return false
	}
	delete(devinLogin.sessions, state)
	return true
}

func (h *OAuthHandler) exchangeDevinCode(ctx context.Context, raw, state, verifier, name string) (pkceExchangeResult, error) {
	code := strings.TrimSpace(raw)
	if strings.HasPrefix(code, "http://") || strings.HasPrefix(code, "https://") {
		u, err := url.Parse(code)
		if err != nil || u.Query().Get("state") != state {
			return pkceExchangeResult{}, fmt.Errorf("Devin callback state mismatch")
		}
		code = u.Query().Get("code")
	}
	if code == "" || !claimDevinState(state, verifier) {
		return pkceExchangeResult{}, fmt.Errorf("invalid, expired or already used Devin authorization session")
	}
	payload, err := json.Marshal(map[string]string{"code": code, "code_verifier": verifier})
	if err != nil {
		return pkceExchangeResult{}, fmt.Errorf("encode Devin grant: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, devinTokenURL, strings.NewReader(string(payload)))
	if err != nil {
		return pkceExchangeResult{}, fmt.Errorf("create Devin grant: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := devinOAuthClient.Do(req)
	if err != nil {
		return pkceExchangeResult{}, fmt.Errorf("Devin token exchange unavailable")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return pkceExchangeResult{}, fmt.Errorf("Devin token exchange returned HTTP %d", resp.StatusCode)
	}
	var body struct {
		Token string `json:"token"`
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil || json.Unmarshal(b, &body) != nil || strings.TrimSpace(body.Token) == "" {
		return pkceExchangeResult{}, fmt.Errorf("Devin token exchange returned no token")
	}
	token := strings.TrimSpace(body.Token)
	if !strings.HasPrefix(token, devinTokenPrefix) {
		token = devinTokenPrefix + token
	}
	return h.saveDevinCredential(token, name, true)
}

func (h *OAuthHandler) saveDevinCredential(token, name string, browser bool) (pkceExchangeResult, error) {
	if len(strings.TrimSpace(token)) < 16 {
		return pkceExchangeResult{}, fmt.Errorf("Devin credential must contain at least 16 characters")
	}
	id := "devind-" + shortHash(token)
	data := map[string]any{"apiKey": token, "accessToken": token}
	if browser {
		data["providerSpecificData"] = map[string]any{"authMethod": "browser"}
	}
	if name == "" {
		name = "Devin Desktop"
	}
	if err := h.persistConnection(id, "devin-desktop", name, data, ""); err != nil {
		return pkceExchangeResult{}, fmt.Errorf("save Devin connection: %w", err)
	}
	return pkceExchangeResult{ConnectionID: id, Name: name}, nil
}

func (h *OAuthHandler) HandleDevinImport(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Token string `json:"token"`
		Name  string `json:"name"`
	}
	if json.Unmarshal(readDevinBody(r), &body) != nil {
		handlerutil.WriteJSONError(w, 400, "invalid Devin credential request")
		return
	}
	res, err := h.saveDevinCredential(strings.TrimSpace(body.Token), body.Name, false)
	if err != nil {
		handlerutil.WriteJSONError(w, 400, err.Error())
		return
	}
	handlerutil.WriteJSON(w, 200, map[string]any{"status": "authorized", "connectionId": res.ConnectionID})
}

func (h *OAuthHandler) HandleDevinExchange(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Code     string `json:"code"`
		State    string `json:"state"`
		Verifier string `json:"codeVerifier"`
		Name     string `json:"name"`
	}
	if json.Unmarshal(readDevinBody(r), &body) != nil {
		handlerutil.WriteJSONError(w, 400, "invalid Devin authorization request")
		return
	}
	res, err := h.exchangeDevinCode(r.Context(), body.Code, body.State, body.Verifier, body.Name)
	if err != nil {
		handlerutil.WriteJSONError(w, 400, err.Error())
		return
	}
	handlerutil.WriteJSON(w, 200, map[string]any{"status": "authorized", "connectionId": res.ConnectionID})
}

func readDevinBody(r *http.Request) []byte {
	defer r.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	return b
}
