// 9RouterX — offline error-sanitization fixtures for the pinned Replicate executor.
package executor

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestReplicateUpstreamDetailRedactsCredentials(t *testing.T) {
	replicateEnableLocalDispatch(t)
	detail := "Invalid token from https://user:pass@example.test/x?api_key=SECRET123456&x=1 Bearer tok_ABCDEFGH1234 access_token=ABCD1234EFGH"
	srv, _ := replicateServer(t, func(w http.ResponseWriter, _ *http.Request, _ int) {
		replicateJSON(w, http.StatusUnauthorized, fmt.Sprintf(`{"detail":%q}`, detail))
	})
	rec := replicateRun(t, srv, replicateTestModel, `{"messages":[{"role":"user","content":"Hi"}]}`, false)
	body := rec.Body.String()
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	for _, secret := range []string{"SECRET123456", "tok_ABCDEFGH1234", "ABCD1234EFGH", "user:pass", "pass@example.test"} {
		if strings.Contains(body, secret) {
			t.Errorf("credential %q leaked: %s", secret, body)
		}
	}
	for _, want := range []string{"status 401", "Invalid token", "example.test"} {
		if !strings.Contains(body, want) {
			t.Errorf("response missing %q: %s", want, body)
		}
	}
}

func TestReplicateSanitizeDetailMasksOtherCredentialShapes(t *testing.T) {
	for _, tt := range []struct{ name, detail, secret string }{
		{"quoted key", `api_key="abcDEF123456"`, "abcDEF123456"},
		{"refresh token", `refresh_token=rt_ABCDEF123456`, "rt_ABCDEF123456"},
		{"cookie", `cookie: session=abcdef123456`, "abcdef123456"},
		{"basic", `Basic dXNlcjpwYXNzMTIzNDU2`, "dXNlcjpwYXNzMTIzNDU2"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := replicateSanitizeDetail(tt.detail); strings.Contains(got, tt.secret) {
				t.Errorf("credential leaked: %s", got)
			}
		})
	}
}

func TestReplicateDetailTruncationKeepsValidUTF8(t *testing.T) {
	replicateEnableLocalDispatch(t)
	// 3-byte runes: byte 200 cuts a rune in half under a plain s[:200].
	detail := strings.Repeat("中", 100)
	safe := replicateSanitizeDetail(detail)
	if !utf8.ValidString(safe) || safe == "" {
		t.Fatalf("sanitized detail invalid or empty: %q", safe)
	}
	srv, _ := replicateServer(t, func(w http.ResponseWriter, _ *http.Request, _ int) {
		replicateJSON(w, http.StatusBadRequest, fmt.Sprintf(`{"detail":%q}`, detail))
	})
	rec := replicateRun(t, srv, replicateTestModel, `{"messages":[{"role":"user","content":"Hi"}]}`, false)
	body := rec.Body.String()
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
	if !strings.Contains(body, `"message":"Replicate API failed with status 400:`) {
		t.Errorf("empty message: %s", body)
	}
	if strings.Contains(body, `"internal error"`) {
		t.Errorf("marshal failed: %s", body)
	}
}
