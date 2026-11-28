### Fixed
- Dashboard login no longer issues a session from a correct password alone when TOTP enrollment is enabled. The previously dead `totpLoginGate` now runs on the login path: a missing code receives `200 {"totpRequired": true}` with no cookie, a wrong code receives a generic 401, and a valid TOTP or backup code passes. Backup codes still burn on use, and failed second factors count toward the same per-client lockout as password failures.
- Added the public `POST /api/auth/login/totp` step-up route plus a second-step code form in `LoginView.svelte`, so the operator finishes sign-in on the same screen. The API client now shares one login POST helper between both steps.

### Verification
- `TestHandleAuthLogin_TOTPDisabledUsesPasswordAlone`, `TestHandleAuthLogin_TOTPRequiredWithoutCodeIssuesNoSession`, `TestHandleAuthLoginTOTP_CorrectCodeIssuesSession`, `TestHandleAuthLoginTOTP_WrongCodeIsUnauthorized`, `TestHandleAuthLoginTOTP_BackupCodeBurnsOnUse`.
- `tools/p1_gate.sh sec1-h1`: go build, vet, full `go test ./... -count=1`, `bun test src scripts`, `bun run build`, svelte-check ratchet and post-web Go gates all green.
