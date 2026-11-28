// 9RouterX — port and modifications by https://github.com/yanstd888/ — https://t.me/aiclub_id
package oauth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGitlabDuoCanonicalScopeAndPersistence(t *testing.T) {
	if cfg := pkceLookup("gitlab-duo"); cfg != pkceLookup("gitlab") || !strings.Contains(cfg.scope, "ai_features") {
		t.Fatal("GitLab Duo login does not share canonical identity and inference scope")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusUnauthorized) }))
	defer server.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/oauth/pkce/exchange", nil)
	ex, fail := resolvePKCEExchange(pkceExchangeBody{Provider: "gitlab-duo", Code: "fixture-code", CodeVerifier: "fixture-verifier", BaseURL: server.URL, ClientID: "fixture-client", ClientSecret: "fixture-secret"}, req)
	if fail != nil {
		t.Fatal(fail)
	}
	if ex.provider != "gitlab" {
		t.Fatalf("connection provider = %q", ex.provider)
	}
	data, _ := ex.buildConnectionData(t.Context(), &pkceTokens{AccessToken: "fixture-access", RefreshToken: "fixture-refresh", ExpiresIn: 3600})
	psd, ok := data["providerSpecificData"].(map[string]any)
	if !ok || psd["baseUrl"] != server.URL || psd["clientId"] != "fixture-client" || psd["clientSecret"] != "fixture-secret" {
		t.Fatal("GitLab OAuth client credentials depend on optional profile lookup")
	}
}
