### Fixed

- Streaming requests for `gemini-*` models on the opencode-go route translate again: the executor's Gemini stream no longer drops every SSE event (its `data:` prefix gate and first-event skip predated the scanner that strips the prefix), so clients receive deltas, a finish reason, and stream termination instead of headers and silence.

### Validation

- `go vet ./internal/proxy/executor/`, `go test -count=1 ./internal/proxy/executor/`, including a new regression test streaming two SSE events through `geminiStream`.
