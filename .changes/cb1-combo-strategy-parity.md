### Fixed
- Both combo lanes now call one `routeComboModels` dispatcher. The messages/responses lane used to honour only `smart` and `tokenmaxxing`, so `cache-optimized`, `auto`, `quota-weighted`, `reset-aware` and `lkgp` silently behaved like stored order despite being offered in the UI.
- Added `TestRouteComboModelsParity` for all seven strategies, using fresh state per case and member-set comparison for clock-scored strategies.

### Verification
- `go test ./internal/handlers/chat/ -run TestRouteComboModelsParity -count=5` stable.
- `tools/p1_gate.sh sec3-cb1`: all steps green. A later full gate surfaced a clock-order flake in the new test; its tolerance was fixed in a follow-up `test(chat)` commit before this branch was finalized.
