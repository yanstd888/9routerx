### Fixed
- Provider-count claims in README and wiki now come from code instead of hand-maintained literals. `tools/sitegen/providercount` walks `providers.KnownProviders` through `ClassifyProvider` and prints `{"total":344,"apikey":265,"oauth":34,"freeTier":23,"free":11,"webCookie":11}`; the same tool's `-write` mode refreshes every generated span. `make readme-count` is the regeneration entry point.
- Replaced the stale 154/91/28/29/6 figures in README, the wiki home page, the providers page and the comparison page. The icon map follows the heading whose number is no longer baked into the slug.

### Verification
- `go run ./tools/sitegen/providercount` returns the JSON above; `grep -rn '"154' README.md wiki/` returns 0.
- `TestCollectMatchesRegistry`, `TestReplaceMarkers`, `TestReplaceMarkersRejectsUnknownField`, and the full `go test ./tools/sitegen/...` suite pass.
- `tools/p1_gate.sh sec1-h3`: all steps green. The `make` binary is not installed in the local environment, so the target command was verified directly with `go run ./tools/sitegen/providercount -write`.
