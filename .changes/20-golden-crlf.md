### Fixed

- Golden fixture comparisons (GCF block and PII mask sections) normalize CRLF line endings before comparing, so a Windows checkout made with `core.autocrlf=true` no longer fails `TestEncodeGoldenBlock` and `TestPIIMaskGolden` with a fixture-drift report that does not exist.

### Validation

- `go vet ./internal/compression/gcf/ ./internal/translator/`, `go test -count=1 ./internal/compression/gcf/ ./internal/translator/` on a Windows CRLF checkout (previously failing, now passing); fixtures unchanged so LF checkouts behave identically.
