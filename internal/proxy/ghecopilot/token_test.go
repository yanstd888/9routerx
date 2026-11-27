package ghecopilot

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"testing"
	"time"
)

type testTransport func(*http.Request) (*http.Response, error)

func (f testTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func response(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}
}
func validToken(t *testing.T, api, proxy string) string {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"token": "derived", "expires_at": time.Now().Add(time.Hour).Unix(), "endpoints": map[string]any{"api": api, "proxy": proxy}})
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestGHEExpiry(t *testing.T) {
	stamp := time.Now().Truncate(time.Second)
	for _, value := range []any{stamp.Unix(), float64(stamp.Unix()), stamp.UnixMilli(), stamp.Format(time.RFC3339), stamp.Format(time.RFC3339Nano)} {
		got, ok := Expiry(value)
		if !ok || !got.Equal(stamp) {
			t.Errorf("Expiry(%v)=%v,%v", value, got, ok)
		}
	}
	for _, value := range []any{nil, "not-a-date", float64(-1), true} {
		if _, ok := Expiry(value); ok {
			t.Errorf("accepted %v", value)
		}
	}
}

func TestGHEURLAndRedirectIsolation(t *testing.T) {
	for _, value := range []string{"http://enterprise.example", "https://user:pass@enterprise.example", "https://enterprise.example/path", "https://enterprise.example?secret=x", "https://enterprise.example#fragment", "https://[fe80::1%25eth0]"} {
		if _, err := InstanceURL(value); err == nil {
			t.Errorf("accepted instance %q", value)
		}
	}
	for _, endpoint := range []string{"https://169.254.169.254", "https://168.63.129.16", "https://127.0.0.1", "https://10.0.0.1"} {
		if _, err := Guard(t.Context(), endpoint); err == nil {
			t.Errorf("accepted %s", endpoint)
		}
	}
	base, err := APIBase(map[string]any{"copilotApiUrl": "https://api.enterprise.example/responses/", "copilotProxyUrl": "https://nes.enterprise.example", "gheUrl": "https://enterprise.example"})
	if err != nil || base != "https://api.enterprise.example" {
		t.Fatalf("APIBase=%s %v", base, err)
	}
	jar, _ := cookiejar.New(nil)
	origin, _ := url.Parse("https://enterprise.example")
	jar.SetCookies(origin, []*http.Cookie{{Name: "session", Value: "owner"}})
	calls := 0
	client := &http.Client{Jar: jar, Transport: testTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Header.Get("Cookie") != "" {
			t.Fatal("cookie leaked")
		}
		resp := response(http.StatusTemporaryRedirect, "redirect")
		resp.Header.Set("Location", "https://evil.example/token")
		return resp, nil
	})}
	req, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, origin.String(), strings.NewReader("refresh=fixture"))
	req.Header.Set("Cookie", "explicit=owner")
	if _, err := Do(t.Context(), client, req); err == nil || calls != 1 {
		t.Fatalf("redirect err=%v calls=%d", err, calls)
	}
}

func TestGHERenewCachedAndWithoutGrant(t *testing.T) {
	for _, cached := range []bool{true, false} {
		t.Run(map[bool]string{true: "cached", false: "renew no refresh grant"}[cached], func(t *testing.T) {
			psd := map[string]any{"gheUrl": "https://enterprise.example", "copilotApiUrl": "https://old.api.example", "alias": "mine", "copilotToken": "old", "copilotTokenExpiresAt": time.Now().Add(-time.Hour).Unix()}
			if cached {
				psd["copilotTokenExpiresAt"] = time.Now().Add(time.Hour).Format(time.RFC3339)
			}
			data := map[string]any{"accessToken": "github-oauth", "apiKey": "independent", "providerSpecificData": psd}
			calls := 0
			client := &http.Client{Transport: testTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.URL.String() != "https://enterprise.example/api/v3/copilot_internal/v2/token" || r.Header.Get("Authorization") != "Bearer github-oauth" {
					t.Fatalf("bad exchange %v", r)
				}
				return response(200, validToken(t, "https://new.api.example", "https://nes.example")), nil
			})}
			changed, err := Renew(t.Context(), client, data, false)
			if err != nil || changed == cached || calls != map[bool]int{true: 0, false: 1}[cached] {
				t.Fatalf("changed=%v err=%v calls=%d", changed, err, calls)
			}
			if data["accessToken"] != "github-oauth" || data["apiKey"] != "independent" || psd["alias"] != "mine" {
				t.Fatalf("credential or alias changed: %v", data)
			}
			if !cached && psd["copilotApiUrl"] != "https://new.api.example" {
				t.Fatalf("endpoint not updated: %v", psd)
			}
		})
	}
}

func TestGHEFallbackRefreshAndExchange(t *testing.T) {
	for _, mirrored := range []bool{true, false} {
		t.Run(map[bool]string{true: "mirrored key", false: "independent key"}[mirrored], func(t *testing.T) {
			apiKey := "independent"
			if mirrored {
				apiKey = "old-oauth"
			}
			psd := map[string]any{"gheUrl": "https://enterprise.example", "copilotToken": "stale", "copilotTokenExpiresAt": 0, "githubLogin": "fixture"}
			data := map[string]any{"accessToken": "old-oauth", "refreshToken": "old-refresh", "apiKey": apiKey, "alias": "my-alias", "providerSpecificData": psd}
			var paths []string
			client := &http.Client{Transport: testTransport(func(r *http.Request) (*http.Response, error) {
				paths = append(paths, r.URL.Path)
				if r.URL.Path == "/login/oauth/access_token" {
					_ = r.ParseForm()
					if r.Form.Get("grant_type") != "refresh_token" || r.Form.Get("refresh_token") != "old-refresh" || r.Form.Get("client_secret") != "" {
						t.Fatalf("bad form %v", r.Form)
					}
					return response(200, `{"access_token":"new-oauth","refresh_token":"new-refresh","expires_in":3600}`), nil
				}
				if r.Header.Get("Authorization") == "Bearer old-oauth" {
					return response(401, `{}`), nil
				}
				if r.Header.Get("Authorization") != "Bearer new-oauth" {
					t.Fatalf("wrong OAuth bearer")
				}
				return response(200, validToken(t, "https://api.enterprise.example", "https://nes.enterprise.example")), nil
			})}
			changed, err := Renew(t.Context(), client, data, false)
			if err != nil || !changed || len(paths) != 3 {
				t.Fatalf("changed=%v err=%v paths=%v", changed, err, paths)
			}
			if data["accessToken"] != "new-oauth" || data["refreshToken"] != "new-refresh" || data["alias"] != "my-alias" || psd["copilotToken"] != "derived" || psd["githubLogin"] != "fixture" {
				t.Fatalf("bad credentials: %v", data)
			}
			wantKey := "independent"
			if mirrored {
				wantKey = "new-oauth"
			}
			if data["apiKey"] != wantKey {
				t.Fatalf("key=%v want=%s", data["apiKey"], wantKey)
			}
		})
	}
}

func TestGHECanceledExchangeAndUnsafeEndpoint(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	client := &http.Client{Transport: testTransport(func(r *http.Request) (*http.Response, error) { return nil, r.Context().Err() })}
	if _, err := Exchange(ctx, client, "https://enterprise.example", "fixture"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel=%v", err)
	}
	client.Transport = testTransport(func(r *http.Request) (*http.Response, error) {
		return response(200, validToken(t, "https://169.254.169.254", "https://nes.example")), nil
	})
	if _, err := Exchange(t.Context(), client, "https://enterprise.example", "fixture"); err == nil {
		t.Fatal("metadata endpoint accepted")
	}
}
