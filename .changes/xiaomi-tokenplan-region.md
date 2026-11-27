### Fixed
- **Xiaomi Token Plan connections created with a non-default region were routed to the wrong host and answered `401 Invalid API Key` for a valid key.** The registry names one origin (`token-plan-sgp`), and the dashboard's connection probe already resolved the full region map — but the chat lane read only the registry, so a connection stored with `region: cn` was probed against `token-plan-cn` by **Test connection** and then chat-routed to Singapore. The visible symptom was a connection that passed its own test and failed every real request.

  The region table moved to `internal/providers/xiaomi_tokenplan.go` and is now the single source for both lanes: `getProviderConfig` resolves the chat origin from the connection's `providerSpecificData.region`, and `validate.go` references the same map instead of keeping a second copy. An operator-supplied `baseUrl` still wins.

  Unknown or empty regions fall back to `sgp`, which is what the registry already named, so an unconfigured connection keeps the behaviour it had.

  Also normalised on the way: `CN`, ` cn ` and `cn` resolve identically; a non-string `region` is ignored rather than crashing the type assertion.

### Verification

- `go build ./...`, `go vet ./internal/providers/ ./internal/handlers/chat/`.
- `go test ./internal/providers/ -run XiaomiTokenplan` — 10 cases: each region, case and whitespace normalisation, unknown and empty fallback, the chat route carrying exactly one `/v1`, the registry entry matching the default region, and `XiaomiTokenplanRegionOf` over a nil map, an absent key, a string and a non-string.
- `go test ./internal/handlers/dashboard/` and `./internal/handlers/chat/` — both pass.
- Live check against a real deployment: the connection's log line changed from
  `forward to https://token-plan-sgp.xiaomimimo.com/v1/chat/completions` (401) to
  `forward to https://token-plan-cn.xiaomimimo.com/v1/chat/completions`, and the same request then completed.

### Not in this change

The `xiaomi-tokenplan` / `xmtp` model lists in `registry_models.go` still name `mimo-v2.5-pro`, `mimo-v2.5`, `mimo-v2-pro` and `mimo-v2-omni`, which the upstream now rejects with `400 Unsupported model`. `mimo-v2.6-pro` works. That is a separate data fix — bundled here it would make the diff look like a model-catalog update and hide the routing bug being repaired.
