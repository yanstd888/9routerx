### Fixed

- Bound `CompressContainerOutput` for small positive line budgets without panicking on short tables; preserve fitting output and existing large-budget behavior (issue #18).
- Replay unknown-length request bodies completely after the bounded TPM token scan, while retaining the original body closer and prefix-based charge (issue #14; duplicate #24).
- Close abandoned Qoder rewrite pipes and upstream reads on consumer return or cancellation, preserving normal completion and producer errors (issue #26).
- Retain all announced item identities and final snapshots in the Responses bridge's terminal output array (issue #16, terminal-only scope).
- Force termination and confirm exit of an unhealthy startup child before removing its matching PID claim; retain claims and cleanup errors when termination fails (issue #19).

### Verification

- Each fix passed `go build ./...`, `go vet ./...`, and `go test ./internal/... -count=1` serially with real exit codes.
- Each new guard was proven by a core mutation that failed, followed by restoration and passing tests.
- Daemon lifecycle guards executed on Windows/amd64 with isolated child processes and data directories; Linux/amd64 and Darwin/arm64 daemon tests cross-compiled but were not executed locally.
- No tracked frontend files changed; genuine baseline assets were built only for Go embed preparation. No live provider accounts or production databases were used.

### Scope notes

- The permitted `json_guard.go` number-precision path for issue #17 was already protected in the baseline. An unnecessary parser prototype was rejected and restored; no generic-decoder behavior was changed.
- The separate `sanitize.go` precision path, Responses non-terminal output-index collisions, and follow-up cases noted on issues #10/#15 remain outside this batch. These local fixes do not imply those issues are fully resolved.
- Issue replies and closure notes are owner-reviewed drafts only; no issue was posted to or closed by this batch.
