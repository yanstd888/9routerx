### Fixed
- Removed the shadowed `mimocode` provider row from the display registry, wire configuration, registry aliases and executor registration. `ResolveAlias("mimocode")` has always returned `mimo-free`, so that row could never be reached.
- Kept the compatibility aliases `mimocode` and `mimocode-free` pointing at `mimo-free`, and added `TestNoShadowedRegistryEntries` for registry ids that actually have wire configuration.

### Verification
- Red before the fix: the new test reported `mimocode` resolving to `mimo-free`. Green after removing the row.
- `TestMimocodeProviderCatalogAndAliases` updated to assert the canonical provider only; `go test ./internal/providers/ ./internal/proxy/executor/` pass; `tools/p1_gate.sh sec2-pb2` green.
