### Fixed

- Zed streaming JSONL path now closes the upstream response body after reading, so the connection is returned to the pool instead of leaking, and bodies over 10 MiB surface an error instead of being silently truncated.

### Validation

- `go vet ./internal/proxy/executor/`, `go build ./internal/proxy/executor/`
