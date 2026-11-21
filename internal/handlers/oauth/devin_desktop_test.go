// 9RouterX — port and modifications by https://github.com/yanstd888/ — https://t.me/aiclub_id
package oauth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func devinTestAuthorization(t *testing.T, h *OAuthHandler) (string, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.HandleDevinAuthorize(rec, httptest.NewRequest(http.MethodGet, "/api/oauth/devin-desktop/authorize", nil))
	var body struct {
		State    string `json:"state"`
		Verifier string `json:"codeVerifier"`
		URL      string `json:"url"`
		Redirect string `json:"redirectUri"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Verifier) != 128 || body.Redirect != devinRedirectURI {
		t.Fatalf("wrong PKCE shape: %s", rec.Body.String())
	}
	u, err := url.Parse(body.URL)
	if err != nil {
		t.Fatal(err)
	}
	if u.Query().Get("code_challenge") != sha256Base64(body.Verifier) || u.Query().Get("prompt") != "select_account" {
		t.Fatalf("wrong authorize query: %s", body.URL)
	}
	t.Cleanup(func() { devinLogin.Lock(); delete(devinLogin.sessions, body.State); devinLogin.Unlock() })
	return body.State, body.Verifier
}

func TestDevinAuthorizationOneShotAndTokenPrefix(t *testing.T) {
	var hits atomic.Int32
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if len(body) != 2 || body["code"] != "fixture-code" || len(body["code_verifier"]) != 128 {
			t.Errorf("wrong grant: %v", body)
		}
		_, _ = w.Write([]byte(`{"token":"fixture-session-token"}`))
	}))
	defer mock.Close()
	old := devinTokenURL
	devinTokenURL = mock.URL
	t.Cleanup(func() { devinTokenURL = old })
	h := NewOAuthHandler(nil)
	state, verifier := devinTestAuthorization(t, h)
	_, err := h.exchangeDevinCode(t.Context(), devinRedirectURI+"?state=wrong&code=fixture-code", state, verifier, "")
	if err == nil {
		t.Fatal("accepted callback from a different state")
	}
	var group sync.WaitGroup
	var success atomic.Int32
	for range 2 {
		group.Add(1)
		go func() {
			defer group.Done()
			res, err := h.exchangeDevinCode(t.Context(), "fixture-code", state, verifier, "")
			if err == nil {
				if !strings.HasPrefix(res.ConnectionID, "devind-") {
					t.Error("wrong provider persisted")
				}
				success.Add(1)
			}
		}()
	}
	group.Wait()
	if hits.Load() != 1 || success.Load() != 1 {
		t.Fatalf("duplicate callback exchange: hits=%d success=%d", hits.Load(), success.Load())
	}
}

func TestDevinImportPreservesVerbatimCredential(t *testing.T) {
	for _, token := range []string{"fixture-token-long-enough", devinTokenPrefix + "fixture-token-long-enough"} {
		h := NewOAuthHandler(nil)
		res, err := h.saveDevinCredential(token, "", false)
		if err != nil {
			t.Fatal(err)
		}
		if res.ConnectionID != "devind-"+shortHash(token) {
			t.Fatal("import changed the token bytes")
		}
	}
	devinLogin.Lock()
	devinLogin.sessions["expired-fixture"] = devinAuthSession{verifier: "v", created: time.Now().Add(-time.Hour)}
	devinLogin.Unlock()
	if claimDevinState("expired-fixture", "v") {
		t.Fatal("accepted expired session")
	}
	devinLogin.Lock()
	delete(devinLogin.sessions, "expired-fixture")
	devinLogin.Unlock()
}
