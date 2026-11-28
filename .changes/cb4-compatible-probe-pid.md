### Fixed
- `handleTestCompatibleModel` now captures `providerId` before awaiting and refuses to write latency, status or error when the panel has switched. The guard is present in both the success path and the catch path.

### Verification
- `bun run ratchet:svelte`: 63 errors, baseline 63. `bunx oxlint --deny-warnings`: 0 warnings.
- `tools/p1_gate.sh sec3-cb4`: all steps green.
