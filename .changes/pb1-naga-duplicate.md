### Fixed
- Collapsed the two equivalent Naga entries into `naga-ac`. Both rows shared the same endpoint, auth header, passthrough flag and models fetcher, so operators had to install one credential twice and health/usage split across two ids.
- Kept `naga-ai` as a compatibility alias to `naga-ac` in Go and in the web alias map, removed the duplicate registry/config/executor entries, and added `TestNagaAIAliasResolvesToNagaAC`.

### Verification
- `grep -rn '"naga-ai"' internal/providers` returns only the alias shim.
- `go test ./internal/providers/ ./internal/proxy/executor/` and `bun test src/lib` pass; `tools/p1_gate.sh sec2-pb1-rerun` green.
