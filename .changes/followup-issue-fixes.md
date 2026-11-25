### Fixed

- `EnsureTrailingUserTurnBody` now recognizes a same-length replacement of an emptied trailing user turn, so the `Continue.` placeholder is preserved through the encoded Claude Messages body path (#10).
- Zed whole-body reads inspect one byte beyond the 10 MiB limit. Exact-cap responses pass; larger responses return a sanitized 502 instead of silently forwarding truncated output (#15).
- `sanitizeToolArgs` reuses `unmarshalNumbered`, preserving integer arguments above 2^53. Numbered Read `limit`/`offset` values are clamped before unsafe integer conversion (#17).

### Verification

- Each fix has a regression guard, an expected-red mutation check, and a restored green run.
- `go build ./...`, `go vet ./...`, `go test ./internal/... -count=1`, and `go test ./... -count=1` all passed locally with exit 0.
- No tracked frontend, dependency, provider registry, or CI files changed.
