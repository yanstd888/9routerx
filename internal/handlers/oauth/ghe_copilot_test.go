package oauth

import (
	json "encoding/json/v2"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type gheDeviceTransport func(*http.Request) (*http.Response, error)

func (f gheDeviceTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestGHEDeviceStartPollAndPending(t *testing.T) {
	original := deviceClient
	t.Cleanup(func() { deviceClient = original })
	var paths []string
	pending := true
	deviceClient = &http.Client{Transport: gheDeviceTransport(func(r *http.Request) (*http.Response, error) {
		paths = append(paths, r.URL.Path)
		if r.URL.Host != "enterprise.example" || r.Header.Get("Cookie") != "" {
			t.Fatalf("wrong device destination %s", r.URL)
		}
		status, body := 200, "{}"
		switch r.URL.Path {
		case "/login/device/code":
			_ = r.ParseForm()
			if r.Form.Get("scope") != "read:user" || r.Form.Get("client_id") == "" {
				t.Fatalf("form=%v", r.Form)
			}
			body = `{"device_code":"fixture-device","user_code":"FIXTURE","verification_uri":"https://enterprise.example/login/device","expires_in":900,"interval":5,"owner_secret":"never-relay"}`
		case "/login/oauth/access_token":
			_ = r.ParseForm()
			if r.Form.Get("device_code") != "fixture-device" || r.Form.Get("grant_type") != "urn:ietf:params:oauth:grant-type:device_code" {
				t.Fatalf("form=%v", r.Form)
			}
			if pending {
				body = `{"error":"slow_down","interval":10}`
			} else {
				body = `{"access_token":"oauth-fixture","refresh_token":"refresh-fixture","expires_in":3600}`
			}
		case "/api/v3/copilot_internal/v2/token":
			if r.Header.Get("Authorization") != "Bearer oauth-fixture" {
				t.Fatal("not OAuth bearer")
			}
			raw, _ := json.Marshal(map[string]any{"token": "derived-fixture", "expires_at": time.Now().Add(time.Hour).Unix(), "endpoints": map[string]any{"api": "https://api.enterprise.example", "proxy": "https://nes.enterprise.example"}})
			body = string(raw)
		case "/api/v3/user":
			body = `{"id":42,"login":"fixture-user","name":"Fixture","email":"fixture@example.invalid"}`
		default:
			t.Fatalf("invented endpoint %s", r.URL)
		}
		return &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	h := &OAuthHandler{}
	rec := httptest.NewRecorder()
	h.HandleDeviceStart(rec, httptest.NewRequest(http.MethodPost, "/api/oauth/device/start", strings.NewReader(`{"provider":"ghe-copilot","gheUrl":"https://enterprise.example/"}`)))
	if rec.Code != 200 || strings.Contains(rec.Body.String(), "never-relay") {
		t.Fatalf("start=%d %s", rec.Code, rec.Body)
	}
	var start map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &start)
	session, _ := start["session"].(map[string]any)
	if session["gheUrl"] != "https://enterprise.example" {
		t.Fatalf("session=%v", session)
	}
	tokens, status, msg := devicePoll(t.Context(), "ghe-copilot", "fixture-device", session)
	if status != "pending" || msg != "" || tokens.interval != 10 {
		t.Fatalf("pending=%s %s %+v", status, msg, tokens)
	}
	pending = false
	tokens, status, msg = devicePoll(t.Context(), "ghe-copilot", "fixture-device", session)
	if status != "authorized" || msg != "" || tokens.access != "oauth-fixture" || tokens.refresh != "refresh-fixture" || tokens.expiresIn != 3600 {
		t.Fatalf("poll=%s %s %+v", status, msg, tokens)
	}
	if tokens.extra["copilotToken"] != "derived-fixture" || tokens.extra["copilotApiUrl"] != "https://api.enterprise.example" || tokens.extra["copilotProxyUrl"] != "https://nes.enterprise.example" || tokens.extra["githubLogin"] != "fixture-user" {
		t.Fatalf("PSD=%v", tokens.extra)
	}
	if len(paths) != 5 {
		t.Fatalf("paths=%v", paths)
	}
}

func TestGHEDeviceRejectUnsafeInstance(t *testing.T) {
	original := deviceClient
	t.Cleanup(func() { deviceClient = original })
	deviceClient = &http.Client{Transport: gheDeviceTransport(func(r *http.Request) (*http.Response, error) { t.Fatal("unsafe URL dispatched"); return nil, nil })}
	for _, instance := range []string{"http://enterprise.example", "https://169.254.169.254", "https://user:password@enterprise.example", "https://enterprise.example/api/v3"} {
		if _, err := gheStart(t.Context(), instance); err == nil {
			t.Errorf("accepted %s", instance)
		}
	}
}
