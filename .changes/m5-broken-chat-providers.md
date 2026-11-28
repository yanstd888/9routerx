### Fixed
- Cursor is hidden with a TODO_OWNER note until its checksum/machineId wire executor exists. Posting to the bare `api2.cursor.sh` host could never serve chat, so leaving it visible made Check All mark all fourteen models dead.
- Removed `vertex` and `vertex-partner` from the registry, wire configuration, model catalogs, seeded free-tier list and web catalog. No Vertex executor is registered and no project/service-account provisioning exists for the generic lane, so advertising the entries was misleading; the stale comment claiming a shared Vertex executor was removed too.
- `huggingface` is unchanged: its registry kinds are `image` and `stt`, not chat, so the reported bare chat endpoint is not a live chat lane. The new test pins that it must not start advertising `llm` without a dispatcher.
- `gemini-cli` now declares `Format: "gemini-native"`, so fallback dispatches through the native handler that already probes a project id and calls `ForwardGemini` instead of POSTing to a bare `/v1internal` URL.

### Scope note
`TestM5DispatchLane` is scoped to the five ids named in the audit. A global "every llm provider has a lane" assertion also fails on unrelated web/cookie sessions, placeholder URLs and other pre-existing special lanes outside this task; those are not changed here.

### Verification
- Red before the fix: cursor not hidden, both Vertex ids present, gemini-cli format empty. Green after the dispositions above.
- `go test ./internal/providers/ ./internal/proxy/executor/ ./internal/handlers/chat/` and `bun test src/lib` pass.
