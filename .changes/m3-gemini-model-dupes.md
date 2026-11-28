### Fixed
- Removed the second copy of `gemini-2.5-pro`, `gemini-2.5-flash` and `gemini-2.5-flash-lite` from the Gemini catalog in both the Go registry and the web model list. The first entries stay; no model id disappears from the catalog.
- Added `TestProviderModelsNoDuplicateIDs` so a future port cannot reintroduce duplicate ids inside one provider.

### Verification
- Red before the fix: the new test named the three duplicated Gemini ids. Green after removing the second block in `registry_models.go` and `web/src/lib/models.ts`.
- `go test ./internal/providers/` and `bun test src/lib` pass; `tools/p1_gate.sh sec2-m3` green.
