// 9RouterX — this file is part of a port maintained by https://github.com/yanstd888/ — https://t.me/aiclub_id
package dashboard

import (
	json "encoding/json/v2"
	"net/http"
	"strings"
	"time"

	"9routerx/proxy/internal/auth"
	"9routerx/proxy/internal/handlerutil"
	"9routerx/proxy/internal/log"
)

// TOTP 2FA (E2): the settings endpoints manage the enrollment, and the login
// handler requires a code (or a backup code) when the enrollment is enabled.
// Backup codes are bcrypt-hashed; the plain list is shown once at enroll time
// and never stored.

// readTOTPConfig reads the stored enrollment state (an empty config when the
// operator never enrolled). The live push is the fallback so a settings write
// that has not been flushed still gates the login.
func (h *DashboardHandler) readTOTPConfig() auth.TOTPConfig {
	live := auth.TOTPConfigLive()
	if live.Enabled {
		return live
	}
	raw := settingsOrEmpty(h)
	if blob, ok := raw["totp"].(map[string]any); ok {
		blobBytes, err := json.Marshal(blob)
		if err == nil {
			var cfg auth.TOTPConfig
			if json.Unmarshal(blobBytes, &cfg) == nil {
				return cfg
			}
		}
	}
	return auth.TOTPConfig{}
}

// writeTOTPConfig persists the enrollment state and applies it live to the
// login gate.
func (h *DashboardHandler) writeTOTPConfig(cfg auth.TOTPConfig) error {
	if err := h.Repo.UpdateSettingsRaw(map[string]any{"totp": cfg}); err != nil {
		return err
	}
	auth.SetTOTPConfig(cfg)
	return nil
}

// HandleTOTPStatus serves GET /api/auth/totp: the enrollment state without
// any secret material.
func (h *DashboardHandler) HandleTOTPStatus(w http.ResponseWriter, r *http.Request) {
	cfg := h.readTOTPConfig()
	handlerutil.WriteJSON(w, http.StatusOK, map[string]any{
		"enabled":     cfg.Enabled,
		"backupCount": len(cfg.BackupCodes),
		"enrolledAt":  cfg.EnrolledAt,
	})
}

// HandleTOTPEnrollStart serves POST /api/auth/totp/enroll-start: generates a
// secret + otpauth URI and stages it (not enabled yet) until the first code
// verifies.
func (h *DashboardHandler) HandleTOTPEnrollStart(w http.ResponseWriter, r *http.Request) {
	secret, uri, err := auth.GenerateTOTPSecret("dashboard-owner", "9RouterX")
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	staged := auth.TOTPConfig{Secret: secret, EnrolledAt: time.Now().UTC()}
	if err := h.writeTOTPConfig(staged); err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	handlerutil.WriteJSON(w, http.StatusOK, map[string]any{
		"otpauthUri": uri,
		"secret":     secret,
	})
}

// HandleTOTPEnrollVerify serves POST /api/auth/totp/enroll-verify: the first
// code verifies the staged secret, generates the backup codes, and flips the
// enrollment enabled. The plain backup codes are returned exactly once.
func (h *DashboardHandler) HandleTOTPEnrollVerify(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Code string `json:"code"`
	}
	if err := json.UnmarshalRead(r.Body, &body); err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	cfg := h.readTOTPConfig()
	if cfg.Secret == "" {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "no enrollment staged — start one first")
		return
	}
	if !auth.VerifyTOTPCode(cfg.Secret, body.Code) {
		handlerutil.WriteJSONError(w, http.StatusUnauthorized, "code does not match — try the next code or re-scan")
		return
	}
	plain, hashed, err := auth.GenerateBackupCodes()
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	cfg.Enabled = true
	cfg.BackupCodes = hashed
	if err := h.writeTOTPConfig(cfg); err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	handlerutil.WriteJSON(w, http.StatusOK, map[string]any{
		"enabled":     true,
		"backupCodes": plain,
	})
}

// HandleTOTPDisable serves POST /api/auth/totp/disable: clears the enrollment
// (the operator's dashboard session already passed the step-up auth gate).
func (h *DashboardHandler) HandleTOTPDisable(w http.ResponseWriter, r *http.Request) {
	if err := h.writeTOTPConfig(auth.TOTPConfig{}); err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	handlerutil.WriteJSON(w, http.StatusOK, map[string]any{"disabled": true})
}

// totpLoginGate is the login handler's second factor: when the enrollment is
// enabled, a correct password alone is not enough. The code may be a TOTP
// code or one of the one-time backup codes (which burns on use).
//
// The gate is deliberately pure: it never writes a response. The login handler
// owns the wire shape — a missing code is a 200 "totpRequired" prompt, a wrong
// code is a generic 401 — so both the first and second login step share one
// decision without double-writing the response.
func (h *DashboardHandler) totpLoginGate(w http.ResponseWriter, r *http.Request, payload struct {
	Password    string `json:"password"`
	NewPassword string `json:"newPassword"`
	Code        string `json:"code"`
}, ip string) (secondFactorRequired bool, proceed bool) {
	cfg := h.readTOTPConfig()
	if !cfg.Enabled {
		return false, true
	}
	code := strings.TrimSpace(payload.Code)
	if code == "" {
		return true, false
	}
	if auth.VerifyTOTPCode(cfg.Secret, code) {
		return false, true
	}
	if idx, matched := auth.MatchBackupCode(cfg.BackupCodes, code); matched {
		cfg.BackupCodes = auth.BurnBackupCode(cfg.BackupCodes, idx)
		if err := h.writeTOTPConfig(cfg); err != nil {
			log.Warn("auth", "totp backup code could not be burned", "error", err)
			return false, false
		}
		return false, true
	}
	return false, false
}
