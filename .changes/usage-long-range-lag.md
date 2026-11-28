### Fixed
- Daily usage windows (7d/30d/60d/all) are aggregated from raw history through the window cache instead of re-reading and JSON-unmarshalling every usageDaily row on each poll; the aggregate is built once, advanced by delta, and rebuilt when the rollup changes. A rollup-only change can no longer stack on top of the history fold, and daily breakdowns now carry LastUsed timestamps.
- The daily chart, burn rate, cache-hit totals, Details-tab page count, prompt-cache analytics, compression summary and lifetime token-quota spend are served from cached aggregates advanced by watermarks, so repeated dashboard reads stop rescanning the ledger. Unknown periods fall back to 24h on both resolvers, and response labels report the window actually served.
- The Overview slows its polls to 15s/60s on periods of a week or more and caps the breakdown table at 200 rendered rows with a Show more control, keeping the existing merge semantics.

### Validation
- `tools/p1_gate.sh FULL_SERIAL_GATE_USAGE`: go build, vet, `go test ./... -count=1`, `bun test src scripts` (933 pass), `bun run build`, svelte-check ratchet (63 = baseline).
- Uncapped `golangci-lint --new-from-rev=origin/main --max-same-issues=0 --max-issues-per-linter=0`: 0 issues.
- Regression tests cover daily cache hit/advance/rewind and rollup invalidation, the one-scan-per-entry chart, resolver parity, LastUsed, the error-count delta, analytics parity and advance, the burn-rate delta, the EXPLAIN plan for the quota spend, the Details count watermark, frontend pacing and the render cap.
- On a 50k-row ledger, cached polls measured ~10ms (30d), ~12ms (60d), ~20ms (all) in isolation; loaded full-suite runs reached 33-49ms for `all` and missed the absolute 20ms target there. The handover reports the miss with numbers; the aggregate fold itself fell from 100-480ms to under a millisecond.
