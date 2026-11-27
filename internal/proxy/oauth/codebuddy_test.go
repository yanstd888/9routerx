package oauth

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCodeBuddyCustomRefreshesAreRegistered(t *testing.T) {
	for _, provider := range []string{"codebuddy-cn", "codebuddy-intl"} {
		if Get(provider) == nil {
			t.Errorf("%s custom refresher is not registered", provider)
		}
	}
	RegisterAll()
	for _, provider := range []string{"codebuddy-cn", "codebuddy-intl"} {
		if Get(provider) == nil {
			t.Errorf("RegisterAll removed %s custom refresher", provider)
		}
	}
}

func TestRefreshCodeBuddyUsesHeaderContractPerSite(t *testing.T) {
	for _, tc := range []struct {
		provider string
		endpoint string
		domain   string
		ua       string
	}{
		{"codebuddy-cn", "https://copilot.tencent.com/v2/plugin/auth/token/refresh", "copilot.tencent.com", "CLI/2.63.2 CodeBuddy/2.63.2"},
		{"codebuddy-intl", "https://www.codebuddy.ai/v2/plugin/auth/token/refresh", "www.codebuddy.ai", "IDE/2.63.2 CodeBuddy/2.63.2"},
	} {
		t.Run(tc.provider, func(t *testing.T) {
			var gotPath, gotDomain, gotUA, gotRefresh, gotBody string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath, gotDomain, gotUA, gotRefresh = r.URL.Path, r.Header.Get("X-Domain"), r.Header.Get("User-Agent"), r.Header.Get("X-Refresh-Token")
				body, _ := io.ReadAll(r.Body)
				gotBody = string(body)
				if r.Header.Get("X-Auth-Refresh-Source") != "plugin" || r.Header.Get("X-Product") != "SaaS" || r.Header.Get("X-Requested-With") != "XMLHttpRequest" {
					t.Errorf("refresh headers incomplete: %v", r.Header)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"code":0,"data":{"accessToken":"new-access","refreshToken":"new-refresh","expiresIn":3600}}`))
			}))
			defer srv.Close()
			codebuddySites[tc.provider] = codebuddySite{url: srv.URL + "/v2/plugin/auth/token/refresh", domain: tc.domain, userAgent: tc.ua}
			t.Cleanup(func() { delete(codebuddySites, tc.provider) })
			result, err := Refresh(context.Background(), &Params{Client: srv.Client(), Provider: tc.provider, RefreshToken: "old-refresh"})
			if err != nil {
				t.Fatalf("Refresh: %v", err)
			}
			if gotPath != "/v2/plugin/auth/token/refresh" || gotDomain != tc.domain || gotUA != tc.ua || gotRefresh != "old-refresh" || gotBody != "{}" {
				t.Fatalf("path=%q domain=%q ua=%q refresh=%q body=%q", gotPath, gotDomain, gotUA, gotRefresh, gotBody)
			}
			if result.AccessToken != "new-access" || result.RefreshToken != "new-refresh" || result.ExpiresIn != 3600 {
				t.Fatalf("result = %+v", result)
			}
			if !strings.HasPrefix(result.AccessToken, "new-") {
				t.Fatal("access token missing")
			}
		})
	}
}

func TestRefreshCodeBuddyRetainsRefreshTokenAndRejectsFailureCode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"data":{"accessToken":"rotated-access","expiresIn":120}}`))
	}))
	defer srv.Close()
	codebuddySites["codebuddy-cn"] = codebuddySite{url: srv.URL, domain: "copilot.tencent.com", userAgent: "test"}
	t.Cleanup(func() { delete(codebuddySites, "codebuddy-cn") })
	result, err := refreshCodebuddyCN(context.Background(), &Params{Client: srv.Client(), Provider: "codebuddy-cn", RefreshToken: "keep-me"})
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if result.RefreshToken != "keep-me" || result.ExpiresIn != 120 {
		t.Fatalf("result = %+v", result)
	}

	failed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":11217,"msg":"pending"}`))
	}))
	defer failed.Close()
	codebuddySites["codebuddy-cn"] = codebuddySite{url: failed.URL, domain: "copilot.tencent.com", userAgent: "test"}
	if _, err := refreshCodebuddyCN(context.Background(), &Params{Client: failed.Client(), Provider: "codebuddy-cn", RefreshToken: "failed"}); err == nil {
		t.Fatal("nonzero provider code succeeded")
	}
}

func TestCodeBuddyBuildConnectionUpdateHasExpiresAt(t *testing.T) {
	before := time.Now()
	update := BuildConnectionUpdate(&TokenResult{AccessToken: "a", RefreshToken: "r2", ExpiresIn: 3600})
	expires, ok := update["expiresAt"].(string)
	if !ok || expires == "" {
		t.Fatalf("expiresAt missing: %#v", update)
	}
	parsed, err := time.Parse(time.RFC3339, expires)
	if err != nil || parsed.Before(before.Add(59*time.Minute)) {
		t.Fatalf("expiresAt = %q err=%v", expires, err)
	}
	if update["refreshToken"] != "r2" {
		t.Fatalf("refresh token update = %#v", update)
	}
}
