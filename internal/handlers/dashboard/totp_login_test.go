// 9RouterX — this file is part of a port maintained by https://github.com/yanstd888/ — https://t.me/aiclub_id
package dashboard

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	json "encoding/json/v2"

	"github.com/pquerna/otp/totp"
	"golang.org/x/crypto/bcrypt"

	"9routerx/proxy/internal/auth"
)

func totpLoginRequest(t *testing.T, h *DashboardHandler, path, password, code string) *httptest.ResponseRecorder {
	t.Helper()
	body := `{"password":` + mustJSON(t, password)
	if code != "" {
		body += `,"code":` + mustJSON(t, code)
	}
	body += `}`
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	rec := httptest.NewRecorder()
	if path == "/api/auth/login/totp" {
		h.HandleAuthLoginTOTP(rec, req)
	} else {
		h.HandleAuthLogin(rec, req)
	}
	return rec
}

func requireNoSession(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	for _, c := range rec.Result().Cookies() {
		if c.Name == auth.CookieName {
			t.Fatalf("second-factor step must not issue a session cookie")
		}
	}
}

func enableTOTPForTest(t *testing.T, secret string, backupCodes []string) {
	t.Helper()
	auth.SetTOTPConfig(auth.TOTPConfig{
		Enabled:     true,
		Secret:      secret,
		BackupCodes: backupCodes,
	})
	t.Cleanup(func() { auth.SetTOTPConfig(auth.TOTPConfig{}) })
}

func TestHandleAuthLogin_TOTPDisabledUsesPasswordAlone(t *testing.T) {
	authTestEnv(t)
	auth.ResetLoginLimiter()
	auth.SetTOTPConfig(auth.TOTPConfig{})
	repo, cleanup := setupTestDB(t)
	defer cleanup()
	storePassword(t, repo, "s3cret-pass")

	rec := totpLoginRequest(t, NewDashboardHandler(repo), "/api/auth/login", "s3cret-pass", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "totpRequired") {
		t.Fatalf("unexpected totpRequired with no enrollment: %s", rec.Body.String())
	}
}

func TestHandleAuthLogin_TOTPRequiredWithoutCodeIssuesNoSession(t *testing.T) {
	authTestEnv(t)
	auth.ResetLoginLimiter()
	secret, _, err := auth.GenerateTOTPSecret("dashboard-owner", "9RouterX")
	if err != nil {
		t.Fatalf("generate secret: %v", err)
	}
	enableTOTPForTest(t, secret, nil)
	repo, cleanup := setupTestDB(t)
	defer cleanup()
	storePassword(t, repo, "s3cret-pass")

	rec := totpLoginRequest(t, NewDashboardHandler(repo), "/api/auth/login", "s3cret-pass", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 prompt, got %d: %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["totpRequired"] != true {
		t.Fatalf("expected totpRequired, got %s", rec.Body.String())
	}
	requireNoSession(t, rec)
}

func TestHandleAuthLoginTOTP_CorrectCodeIssuesSession(t *testing.T) {
	authTestEnv(t)
	auth.ResetLoginLimiter()
	secret, _, err := auth.GenerateTOTPSecret("dashboard-owner", "9RouterX")
	if err != nil {
		t.Fatalf("generate secret: %v", err)
	}
	enableTOTPForTest(t, secret, nil)
	repo, cleanup := setupTestDB(t)
	defer cleanup()
	storePassword(t, repo, "s3cret-pass")
	h := NewDashboardHandler(repo)

	code, err := totp.GenerateCode(secret, time.Now())
	if err != nil {
		t.Fatalf("generate code: %v", err)
	}
	rec := totpLoginRequest(t, h, "/api/auth/login/totp", "s3cret-pass", code)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var session *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == auth.CookieName {
			session = c
		}
	}
	if session == nil || !auth.Verify(session.Value, auth.Secret()) {
		t.Fatalf("expected a verifiable session cookie, got %v", rec.Result().Cookies())
	}
}

func TestHandleAuthLoginTOTP_WrongCodeIsUnauthorized(t *testing.T) {
	authTestEnv(t)
	auth.ResetLoginLimiter()
	secret, _, err := auth.GenerateTOTPSecret("dashboard-owner", "9RouterX")
	if err != nil {
		t.Fatalf("generate secret: %v", err)
	}
	enableTOTPForTest(t, secret, nil)
	repo, cleanup := setupTestDB(t)
	defer cleanup()
	storePassword(t, repo, "s3cret-pass")

	rec := totpLoginRequest(t, NewDashboardHandler(repo), "/api/auth/login/totp", "s3cret-pass", "000000")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d: %s", rec.Code, rec.Body.String())
	}
	requireNoSession(t, rec)
}

func TestHandleAuthLoginTOTP_BackupCodeBurnsOnUse(t *testing.T) {
	authTestEnv(t)
	auth.ResetLoginLimiter()
	hash, err := bcrypt.GenerateFromPassword([]byte("backup-code-1"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hash backup code: %v", err)
	}
	enableTOTPForTest(t, "", []string{string(hash)})
	repo, cleanup := setupTestDB(t)
	defer cleanup()
	storePassword(t, repo, "s3cret-pass")
	h := NewDashboardHandler(repo)

	first := totpLoginRequest(t, h, "/api/auth/login/totp", "s3cret-pass", "backup-code-1")
	if first.Code != http.StatusOK {
		t.Fatalf("expected 200 for backup code, got %d: %s", first.Code, first.Body.String())
	}
	if codes := auth.TOTPConfigLive().BackupCodes; len(codes) != 0 {
		t.Fatalf("backup code was not burned: %d left", len(codes))
	}

	auth.ResetLoginLimiter()
	replay := totpLoginRequest(t, h, "/api/auth/login/totp", "s3cret-pass", "backup-code-1")
	if replay.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 on replay, got %d: %s", replay.Code, replay.Body.String())
	}
	requireNoSession(t, replay)
}
