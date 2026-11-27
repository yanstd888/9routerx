### Fixed

- `POST /admin/health/reset` (all arms) now clears per-connection model locks for model names containing a dot — `gpt-3.5-turbo`, `gemini-2.5-pro`, … — by quoting the `json_set` path the same way the lock write does, and no longer writes junk nested keys into the connection data blob.

### Validation

- `go vet ./internal/db/`, `go test -count=1 ./internal/db/`, including new regression tests that lock a dotted model, reset per-model and provider-wide, and assert the blob keeps every `modelLock_*` key flat.
