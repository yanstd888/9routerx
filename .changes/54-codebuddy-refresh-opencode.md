### Fixed

- CodeBuddy CN and International refresh now use the documented JSON `{}` POST with the single-use refresh token in `X-Refresh-Token`, site-specific `X-Domain`/User-Agent, `X-Auth-Refresh-Source: plugin`, and `X-Product: SaaS`. The International refresh URL now targets `www.codebuddy.ai`; successful device authorization persists `expiresIn` to `expiresAt`.
- OpenCode free inference retries overload responses up to eight total attempts with the pinned linear/capped backoff and jitter; cancellation stops the wait. `FreeUsageLimitError` still rotates once rather than entering the overload loop. If the freshly rotated stable session is also rejected, that exact session is expired so the next request mints a new one.

### Validation

- `go build ./...`
- `go vet ./...`
- `go test ./internal/... -count=1 -timeout=5m`
- Focused regression tests cover CN/International CodeBuddy headers and endpoint separation, error/refresh-token/expiry handling, device expiry persistence, overload success/eight-attempt cap/abort/backoff, preserved FreeTierError retry behavior, and second rejected stable-session expiry.
