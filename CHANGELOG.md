# Changelog

All notable changes to **9RouterX** are documented in this file. The format
follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and this
project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Release history up to the fork point is archived in the upstream repo's
[CHANGELOG.md](https://github.com/luqman-v1/9router-go/blob/main/CHANGELOG.md).

## [Unreleased]

- Release sections carry the `v` prefix (`## [v1.0.0]`), matching what the
  release workflow's notes extraction and the dashboard changelog both expect;
  the pending-fragment splice now lands under this Unreleased heading instead
  of above the file title.

## [v1.5.64] - 2026-11-28

### Added
- **NoTrack Web, ChatPlayground, SYNTX.ai, Adapta One and Conol web-chat providers** — native executors ported from the pinned registry, with per-provider catalog metadata, artwork and frontend tests.
- **Eleven API-key providers** — UncloseAI (free anonymous gateway), FriendliAI, Databricks Model Serving, Snowflake Cortex, Charm Hyper, Token Kiosk, Token Market, TokenReply, Unifically, Gitlawb Opengateway (GMI Cloud) and Cheaper Inference. Cheaper Inference carries the 39-model catalog, the dual chat/Responses surface with a hard `store:false`, the CI payload transport (gzip tier, chunked reservation above 4MiB, 20MiB ceiling) and the fail-closed media guard.

### Fixed
- **Gitlawb MiMo lane chat route** — the config posted to the bare `/v1/xiaomi-mimo` prefix, which the gateway answers with 404 `Not found. Use POST /v1/chat/completions.`; the suffixed `/chat/completions` route is verified live and now registered.
- **Model lookup test flake** — catalog probe ids are percent-encoded, so display names containing spaces (baseten's "Kimi K2.7 Code") can no longer randomly break the synthesized request line.
- **golangci-lint debt** — 143 findings across the never-CI-linted provider batches resolved: unchecked closes, capitalized error strings, misformatted files, dead code, bodyclose and errorlint wrap-style fixes.

## [v1.5.63] - 2026-11-27

### Fixed
- **CodeBuddy token refresh** — CN and International use their site-specific JSON refresh API headers and endpoints; successful device authorization persists token expiry.
- **OpenCode Free overload handling** — retry overload responses with bounded backoff while preserving usage-limit rotation; retire a newly rotated stable session if upstream rejects it too.

## [v1.5.6] - 2026-11-21

### Added
- **OMP connect from an empty models.yml** — a fresh oh-my-pi install's empty (0-byte), whitespace-only, BOM-prefixed or comment-only `~/.omp/agent/models.yml` no longer fails connect with the raw `yaml: line 1: did not find expected <document start>` parser error: the patcher builds the whole nested `providers.9router` document from scratch (flow object value written verbatim, verified by re-decoding), and a file whose first content line is genuinely not a mapping is refused with an actionable message naming the file (issue #7).
- **Dual-cookie browser capture** — throwaway-browser and import-chrome results fill both dual-cookie fields for gemini-web and grok-web (the captured session used to be discarded on submit), the combined header keeps exact cookie names through Base64-padded values, and a missing timestamp/`sso-rw` cookie keeps the form open with a paste-it-manually note.
- **ZCode plan quota for pasted connections** — the quota tracker resolves the billing JWT from provider data (`zcodeJwtToken` → `zaiBusinessToken` → `accessToken` → `apiKey`) so legacy plan-JWT connections read their balance, and billing rejections (401/403, codes 3001/3012/401, `token_version`) answer with reconnect-via-OAuth guidance instead of a bare error.
- **Credential encryption guidance** — the OFF banner links "How to set it" straight to the README's Credential encryption section (32-byte padded-Base64 key, gateway-process env, persistence, `ROUTER_MASTER_KEY_PREVIOUS` rotation), and the command palette finds the section via `ROUTER_MASTER_KEY` (issue #8).
- **Confirmed `service take-over` migration** — shared port/process diagnostics, first-run alternative-port selection and actionable noninteractive conflict errors; legacy autostart is inspected read-only and disabled only on exact legacy gateway path matches, preserving unrelated Node apps and 9RouterX entries, with per-OS autostart discovery and migration documented.

### Fixed
- **Wiki navbar spacing** — the published wiki's top navigation regains flex spacing and 44px link targets after the homepage menu moved to `.site-nav`, without touching the mobile sheet behavior.
- **Space-safe daemon pidfiles** — executable paths containing spaces are preserved in daemon pidfiles.
- **Service autostart details** — the effective service port/data paths persist, supervised Windows launches stay out of the terminal picker, long scheduled-task commands import via UTF-16 XML, and cron/desktop global flags precede `start`.
- **Site icon sync** — the site renderer's icon annotations match the current README, with every current section pinned in a regression test so the Pages gate keeps holding.

## [v1.5.5] - 2026-11-20

### Added
- **Perplexity-web executor** — perplexity.ai's schematized SSE ask endpoint served by a browser session cookie: OpenAI-shaped streaming and buffered replies with reasoning_content for the pro-search narration, FNV-1a session continuation, and the reference-only tools hint.
- **Generic per-provider web-login** — one WebLoginConfig per provider (page, cookie domain, required token sources); POST /api/providers/{id}/browser-login and /import-chrome serve every configured provider and 404-name the rest. New configs: chatgpt-web and muse-web (Baidu Yiyan).
- **Persistent browser profiles** — per-provider profiles under <DATA_DIR>/browser-profiles/ (0700, locked per launch, wiped by POST /api/providers/{id}/reset-profile) keep re-logins warm; gemini-web deliberately stays throwaway.
- **Xvfb virtual display** — web-logins on displayless Linux hosts launch against an Xvfb display automatically for persistent-profile providers.

### Fixed
- **muse-web display identity** — the provider advertises Baidu Yiyan Web (Free) with the AuthHint naming Baidu Yiyan/ERNIE (historical-compatibility id), plus a brand guard walking every web-cookie provider.
- **UI hygiene sweep (audit Hallmark + MWG)** — svg-only provider artwork resolves as .svg (atmorouter broken image), emoji replaced with icons, autocomplete tokens on password inputs, explicit rel=noopener, instant focus ring, 100vw removal, dead manifest duplicate removed, AbortSignal.timeout, field-edge contrast token, cli-tools LCP priority, color-scheme meta, button nowrap, passive scroll listeners, toSorted conversions, named transition properties, service-worker waitUntil, decimal inputmode, sidebar/row/aria-labels, skip link, modal focus trap, toast aria-live, reduced-motion fallback, and view-level h1 headings.

## [v1.5.4] - 2026-11-19

### Added
- **Proxy pool harvest** — POST /api/proxy-pools/{id}/harvest ingests a batch of candidate proxies (up to 500), normalizes and deduplicates them, probes the new entries concurrently through the same dual-target probe the test button uses, and writes back only the survivors. Relay pools ingest without probing. The pool card grows a Harvest panel: paste, ingest & probe, one summary line.
- **Least-known-good provider strategy (lkgp)** — the combo ladder reorders by each member provider's ledger workload: the provider that has served the least in the last hour goes first, spreading traffic across the ladder. A failed read keeps the stored order; same-provider members keep their relative order.
- **Cache hit-rate telemetry** — GET /api/cache answers a hitRates block: the prompt-cache share of requests carrying cached tokens in the window, and the semantic cache's process-lifetime hit share.
- **Model arena** — POST /api/arena/run runs one prompt against up to six combo members side by side; the combos view grows the arena panel with member pills and a results grid (output, latency, tokens). API-key and compatible connections dispatch directly; OAuth and web-cookie members are reported as skipped with the reason.

## [v1.5.3] - 2026-11-19

### Added
- **PII vault with streaming detokenize** — `chat.pii_vault=on` replaces personal data in inbound requests with deterministic placeholders (`[PII:EMAIL_ADDRESS:ab12cd34]`) before the model or provider sees it, and restores the originals on the way back out — buffered answers and streamed frames alike, with a carry buffer for tokens the model emits across a frame boundary. Deterministic per value, so the conversation stays consistent; the mapping is memory-only and ages out.
- **Remote auto-backup** — each backup can leave the machine: a Telegram bot document upload or a GitHub contents-API write, best-effort, with the outcome on the same status row. The auto-backup scheduler also actually started at boot now — it existed but was never wired, so the schedule never fired.
- **Browser login for grok-web, perplexity-web and claude-web** — the throwaway-browser sign-in flow is parameterized into per-provider recipes (page, cookie domain, required cookie names) and three more providers run it. CDP reads HttpOnly cookies, so no DevTools needed.
- **Model-family grouping and cross-family flagging** — the family definition lives in one shared function; GET /api/combos derives `families` + `crossFamily`, create/update responses warn on a mixed ladder, and dispatch records each boundary crossing on the routing-explain panel. The boundary is the model line: gpt→gpt-mini is not a crossing, gpt→claude is.
- **Onboarding tour + doctor** — a first-run card with the three setup steps checked live, a health-check button, and a dismissal persisted in the settings blob; GET /api/doctor answers named, graded checks (database, login posture, connections, keys, backup destination) with fix hints.

## [v1.5.2] - 2026-11-19

### Added
- **Tool-loop watchdog** — `chat.tool_loop_watch` (off default | log | break) tracks tool-call signatures per session across requests; a response repeating the same call past `chat.tool_loop_threshold` (default 3) is logged and recorded on the explain panel, or withheld behind a clean 429 the agent can read. A break keeps the session record: a retry that changes nothing breaks again immediately, only a different call resets.
- **Combo spending ceiling** — a combo can carry `budgetLimitUsd` + `budgetPeriod` (daily | weekly | monthly | total), enforced once per dispatch before the member ladder is walked. Breach follows the budget breach mode: reject answers the generic 402; downgrade restricts the ladder to the cheapest priced member and flags it with `X-Budget-Downgraded`. The combo editor grows the field.
- **Fair-share admission** — under account-queue contention the freed slot goes to the client key that has been served the least (rolling in-memory ledger fed by the usage logger), with skip-credit aging so nobody starves past their timeout. Zero shares drain in the exact FIFO order the queue always ran.
- **Scoped dashboard access tokens** — mint `admin` | `write` | `read` bearer tokens (x-9r-access-token header) for scripting callers instead of sharing the dashboard session. Enforced per route and method, failing closed: an unknown, revoked or out-of-scope token never degrades to the anonymous path. Plaintext shown once at mint; stored under the engine-key hashing. Managed from the profile page.

### Fixed
- **Settings restart-loss** — the section normalizers (context guard, budget breach mode, session affinity, account TTLs, retry caps) applied their value live and then refused to persist it, so every one of those toggles silently reverted on the next restart. They now keep the section in the stored blob; only malformed payloads are stripped.
- **Shipped-but-404 dashboard routes** — thirteen routes existed only in the tests-only route table: the resilience panel and its reset, routing explain, the savings summary and burn rate (both shipped 404 in v1.5.1), settings history and rollback, plans apply-keys, tunnel repair, and the custom-model wildcard delete. All mounted on the real server now; the live smoke gate caught the first one and the route audit found the rest.
- **elevenlabs voices test** — the test stubbed the media package var while calling the chat handler, which reads the provider registry: it hit the real api.elevenlabs.io and only passed with network. It stubs the registry entry now.

## [v1.5.1] - 2026-11-18

### Added
- **Combo strategies exposed** — `cache-optimized`, `auto` (5-factor) and `reset-aware` now selectable from the dashboard combo dropdown; the backend supported them but the UI never offered the choice.
- **Audit log hash-chain** — every audit entry carries sha256(prevHash + canonical) binding it to the chain. GET /api/audit/verify walks the log and reports the first tampered row.
- **Burn-rate analytics** — GET /api/burn-rate returns the 24-hour sliding token and cost burn rate, with budget projections for keys with an armed daily cost limit.
- **gzip unwrap** — upstream responses that force `Content-Encoding: gzip` despite an identity request are transparently decoded before parsing.

## [v1.5.0] - 2026-11-18

### Added
- **Cross-provider session affinity** — `routing.session_affinity` (off default | combo | global) binds native client sessions (Claude Code, Codex) to the connection that served them through a persistent lease with the vendor's prompt-cache window TTL (Claude 5m, Gemini 60m, others 30m).
- **Connect command** — `9routerx connect <client>` patches the named AI client's config to point at this gateway. Dry-run shows the diff without writing; the real run backs up and writes atomically. A hooks-shaped payload is refused (CVE-2026-108593 guard). A dashboard Connect card drives the same patcher.
- **Savings Ledger** — one USD figure aggregating prompt-cache and compression savings for the window, with a per-source breakdown and top-5 list. Compression USD is computed per model from the pricing table at the input rate.
- **TOTP 2FA + backup codes** — the dashboard login can require a six-digit authenticator code (±90s skew) with ten bcrypt-hashed one-time backup codes. `9routerx reset-2fa` clears the enrollment after a dashboard-password confirmation.
- **Settings history + rollback** — every write snapshots changed keys (old/new/source) into a settingsHistory table capped at 100 per key. GET /api/settings/history and POST /api/settings/history/{id}/rollback. Credential keys are masked in both images and their rollback is refused.
- **JSON schema guardrail** — a custom pattern named json_schema validates buffered responses against a JSON Schema document; mismatches produce findings at the policy's action. Streaming responses skip schema validation.
- **Hermes adapter** — the tenth clientconfig adapter writes the gateway endpoint and credential into ~/.hermes/config.yaml.

### Fixed
- **muse-web provider registered** — the executor, alias and models were in place but the registry entry was missing, making the provider unreachable from every catalog surface.

## [v1.4.0] - 2026-11-17

### Added
- **Zero-retry dedup + attempt caps** — a body that just failed a transient status is remembered (LRU 512, TTL 5 menit); request berikutnya dengan body sama melewati retry ladder langsung ke fallback, dan cap attempt fallback kini bertingkat exact > provider > global lewat `retry.attempt_caps`. Semua default = perilaku lama.
- **Context window guard** — kandidat fallback yang window-nya tidak muat estimasi prompt (chars/4, tanpa tokenizer) di-skip dan rantai lanjut ke provider berikutnya; `fallback.context_guard` = skip (default) | truncate | off.
- **Budget downgrade** — `budget.breach_mode` = reject (default) | downgrade: breach tidak langsung 402 melainkan reroute ke model termurah yang lolos context guard, dengan header `X-Budget-Downgraded`; 402 generik hanya saat tidak ada kandidat.
- **Routing explain** — `GET /api/routing-explain?limit=N` (ring 200 entri in-memory) + panel dashboard menampilkan kandidat yang dipertimbangkan dan alasannya.
- **Health radar** — agregasi p50/p95/uptime 24 jam per provider (tabel freeProviderRadar), tie-breaker fallback gabungan health+latency di bawah urutan config, kolom p50/uptime di panel Free Provider Health.
- **Recovery notification** — provider yang keluar dari cooldown menghasilkan notifikasi recovery dengan dedupe key `<provider>#recovery`.
- **Account blacklist TTL** — `accounts.blacklist_ttl` (menit; 0 = perilaku lama) memarkir koneksi 401/403/402 dari rotasi sesuai TTL operator.
- **Web Cookie Providers section** — kategori lane cookie tampil sebagai seksi sendiri di halaman Providers; claude-web tetap terlihat dengan badge disabled saat gate-nya off.

### Changed
- **Auto-prune dead keys** — koneksi yang menjawab 401/403/402 ditandai `disabledReason` + timestamp (badge merah di daftar koneksi); sweep berkala melepasnya dari pool aktif setelah `keys.autoprune_ttl` (0 = sweep off).

## [v1.3.0] - 2026-11-17

### Added
- **Claude Web (unofficial, opt-in)** — provider `claude-web` untuk claude.ai lewat sessionKey browser. Nonaktif secara default: butuh `CLAUDE_WEB_ENABLED=true` atau toggle di bagian Web Cookie Providers. Melanggar ToS Anthropic dan bisa berhenti kapan saja; tidak ada jaminan dukungan.

### Changed
- **Dashboard dimuat per tab** — view besar (Endpoint, Token Saver, Jailbreak, Quota Tracker, Analytics, CLI Tools) sekarang di-lazy-load, jadi bundle awal tidak lagi membawa seluruh dashboard.

### Fixed
- **Tabel lebar di mobile** — tabel 540-850px kini scroll di dalam kartunya sendiri, kolom sekunder disembunyikan di layar kecil sehingga halaman tidak lagi melebar.
- **Polling saat tab tidak aktif** — semua timer view berhenti menghitung ketika tab browser disembunyikan.

## [v1.2.2] - 2026-11-15

### Fixed

- **Usage page froze on real data** — a 30-day window whose live data carried a real
  `other` model family inside the top six, plus smaller families below the cutoff,
  produced two chart series named `other` once the tail fold ran. The duplicate key
  crashed the chart render mid-update, which froze every reactive update on the page:
  the period picker and the section menu stopped responding until a hard reload, and
  the remembered period made it look permanently stuck on Last 30 days. Both the
  analytics trend chart and the overview usage chart now merge the tail total into the
  existing `other` entry instead of pushing a second one.

## [v1.2.1] - 2026-11-14

### Added

- **Three-way comparison docs** — a `9RouterX vs 9Router vs OmniRoute` section in the
  README (facts-first table with live-checked star counts, on-paper wording where no
  public benchmark exists) and a matching wiki page with the OmniRoute column.

### Fixed

- **Usage analytics batch** — the period picker no longer locks up while the 5-second
  usage polls run; request-details pagination uses one page size everywhere and resets
  on a period switch (a stale page no longer strands an empty list); the usage chart
  falls back to raw usage history when the daily rollup is empty (tokens, cost and the
  all-time window span), so the chart no longer draws zeros next to real data; buckets
  that carry totals without a model split render as a single series; the Est. Cost card
  renders a compact amount (`Rp 25,3 rb`, `$25.3K`) instead of truncating, with the full
  figure on hover.
- **Usage stats fallback** — day+ windows (7d/30d/60d/90d/all/custom Nd) recompute from
  raw history when the `usageDaily` rollup carries nothing, so the stats cards agree
  with the chart instead of reading zero. A populated rollup still wins; an empty
  window stays empty.
- **Test stability** — the elevenlabs voices test stubs its upstream URL instead of
  calling the real API; the CI test job runs with outbound network sealed
  (`ROUTERX_BLOCK_NET=1`), so an un-stubbed upstream fails with an explicit message;
  the bootfx integration test polls briefly for asynchronously written usage rows;
  real-upstream smoke tests get a `//go:build network` tag run weekly instead of on
  every push.

### Notes

- The Details tab reads `requestDetails` after the retention pruner has kept the newest
  5,000 rows, so very long windows report the retained total rather than everything the
  ledger ever wrote; `usageHistory` retention is separate.
- "Today" on the dashboard is the server's local calendar day (local midnight), not a
  UTC day — comparisons against a UTC-range SQL query will differ by the UTC offset.

## [v1.2.0] - 2026-11-13

### Added

- **Cache-aware sticky TTL** — combo routing gains a `ttl` mode where the same
  account stays pinned for a configurable window, matching vendor prompt-cache
  lifetimes so cached tokens are reused instead of re-billed.
- **Auto-prune dead connections** — connections accumulating consecutive
  permanent credential failures (401/403-banned/402) are deactivated from
  rotation automatically. Ships OFF with dry-run ON for observation.
- **Dual-column cookie input** — Gemini Web and Grok Web session cookies get
  separate primary/secondary fields with auto-split paste detection.
- **Burn-in protection** — accounts accumulating 429/5xx failures walk a
  suspect → heal (1-in-5 throttle) → rescue (out of rotation) cycle. Ships OFF.
- **Windowed-LRU** — `ROTATION_WINDOW=N` restricts round-robin selection to the
  top N candidates to prevent pool-wide burn-in. Default 0 (off).

### Fixed

- **Standalone homepage** — the landing page is pure design; the full README
  renders at `readme.html` instead of being embedded under the hero.
- **Footer polish** — brand row, link columns, legal line, and the wordmark
  finale with the dashboard's accent glow.
- **Responsive navbar** — sticky glass bar with a mobile hamburger menu that
  closes on Escape, link click, and resize.
- **Provider catalog** — grid of real registry names with per-auth breakdown.
- **Security** — dashboard management routes restricted to operator credentials;
  API keys stored hash-only (plaintext reveal legacy-only); locality guard
  ignores the Host header; updater archive extraction bounded; A2A fail-closed
  when enabled without a token.

## [v1.1.4] - 2026-11-09

### Security

- **Dashboard management is operator-only** — client engine keys can no longer
  create, rotate, delete or re-policy API keys, change settings or trigger
  updates; those routes answer `403` unless the caller holds a dashboard session
  or the local CLI token. Two read-only metadata endpoints
  (`/api/providers/catalog`, `/api/models/caps`) still accept an engine key.
- **API keys are stored hashed again** — create and rotate keep only an
  Argon2id verifier plus a SHA-256 lookup index; the secret is returned once and
  never written to the database. Rows from the plaintext era stay revealable
  until they are rotated, and spend accounting follows the row so quotas,
  budgets and the ledger prune keep working across a rotation.
- **Locality no longer trusts the Host header** — the default-password guard and
  provider-node exceptions classify callers by socket peer (and the trusted
  client-IP resolver for proxied logins), so a spoofed `Host: 127.0.0.1` gains
  nothing.
- **Updater is decompression-bomb-proof** — release archives are capped at 64
  entries / 256 MiB per entry / 512 MiB expanded, ZIP symlinks are refused, and
  the extracted bytes must parse as an executable for the running platform
  (ELF / Mach-O / PE) before anything replaces the binary.
- **Fail-closed defaults** — an enabled A2A server without a configured token
  refuses every request; the gateway binds `127.0.0.1` unless `HOST` says
  otherwise; key-reveal responses carry no-store headers and an audit line.
- **Plaintext storage is announced** — without `ROUTER_MASTER_KEY`, the boot
  log and the dashboard both say credential encryption is OFF.

### Changed

- **403 on operator-only routes** carries the explicit `forbidden` error code
  instead of the quota-shaped one the status mapping produced.
- **Budget cutoff wording** — README, wiki and the Plans dialog now say the
  ceiling is checked when a request starts (a request already in flight can
  overshoot slightly) instead of implying a continuous limit.

## [v1.1.3] - 2026-11-08

### Added

- **llms.txt** — the Pages site now publishes the LLM-crawler site map at
  `https://yanstd888.github.io/9routerx/llms.txt`: every documentation page,
  the registry facts (155 providers, 248 free models / 24 keyless) and the
  quick facts AI assistants need to drive the gateway.
- **JSON-LD structured data** — the homepage carries a schema.org
  SoftwareApplication block (name, platforms, description, download URL) so
  search engines render richer results for the project.

### Changed

- **npm package metadata** — the `9routerx` package description and keywords
  now describe what the gateway actually does (token saver, Claude Code /
  Cursor / Cline / Codex routing, keyless free models, MCP Server, A2A) instead
  of five generic words.

## [v1.1.2] - 2026-11-08

### Fixed

- **Releases-API fallback could replace the gateway with the tray binary** —
  caught by the v1.1.0→v1.1.1 release E2E. GitHub lists release assets
  alphabetically, so `9routerx-tray-windows-amd64.exe` sorts before
  `9routerx-windows-amd64.exe`, and the updater's asset matcher — which
  substring-matches OS and arch — returned the tray. The install then
  verified cleanly (the digest was the tray's own SHA256SUMS entry) and
  replaced `9routerx.exe` with the systray companion. The matcher now skips
  tray assets in both its scan and its non-checksum fallback, so the fallback
  can only ever select the main binary, wherever the tray asset sits in the
  list. The manifest path was never affected: `downloadUrls` names the main
  binary per platform explicitly. Windows installs still running v1.1.0 carry
  the old matcher in their own binary — update those manually (release page
  or `install.ps1`) rather than through their Releases-API fallback.

## [v1.1.1] - 2026-11-08

### Fixed

- **Tray companion name** — the Windows tray tooltip and menu header still
  rendered the old product name ("9Router Go (Stopped)", "9Router Go (Port
  20128)", …) on released v1.1.0 binaries. The state titles in `internal/tray`
  now say **9RouterX** in every state (Running / Stopped / Port conflict /
  Unknown); no user-visible "9Router Go" string remains in the tray tree.
- **Tray and executable icons** — the tray icon and the Windows executables
  still carried the old 9router-go artwork. `tools/icongen` (new) builds the
  multi-size `.ico` (16/24/32/48/64/128/256 px frames, 32-bit BGRA + AND
  mask) straight from the 9RouterX logo (`web/public/logo-9routerx.png`);
  `cmd/9routerx-tray/tray.ico` is regenerated (the go:embed'd runtime tray
  icon) and both `9routerx.exe` and `9routerx-tray.exe` now embed it as a
  Windows resource (`rsrc_windows_amd64.syso`, generated with go-winres and
  linked automatically by `go build`), so Explorer, the taskbar and the tray
  all show the new icon. Regenerate after a logo change with `make icons`.
- **Auto Update path** — `version.json` advertised 1.1.0 without a sha256,
  and since #72 a digest-less manifest is refused by design, so every install
  fell through to the GitHub Releases API — which returned HTTP 500 for some
  users, ending in "manifest error (manifest carries no sha256 for 1.1.0),
  github releases error (github releases HTTP 500)". The release workflow now
  publishes per-platform `sha256s` (plus per-platform `downloadUrls`) into
  `version.json` from SHA256SUMS.txt at release time, and `checkManifest`
  selects the digest for the running platform, so the manifest alone carries
  everything an install needs and the release-API fallback is no longer on
  the critical path.

## [v1.1.0] - 2026-11-08

### Added

- **Interactive CLI menu** — bare `9routerx` on a real terminal now shows an
  arrow-key picker (Bubble Tea + lipgloss, MIT; highlight uses the dashboard
  accent `#2f5bd3`): ↑/↓ or j/k navigate, Enter selects, q/Esc/Ctrl+C quit.
  Number keys `1`-`6` with `0` exit keep the old direct-select behaviour and
  Enter still defaults to item 1 (open dashboard). Pipe/CI/service stdin
  automatically falls back to the plain numbered menu, exactly as before, and
  the default port line in the header is `http://127.0.0.1:20128`.

### Changed

- BREAKING-ish: default port 888 → 20128 for ecosystem compatibility with
  9router / 9router-go; set `PORT=888` to keep the old behavior.
- docs: npm install as primary install path; npm badges
- docs: acknowledgments table with live star badges; attribution fixes (headroom source, frontend deps)
- CLI output is emoji-free: the update notice prints as
  `[UPDATE] v<x.y.z> available — run '9routerx update'`, success lines print
  `[OK]`, warnings print `[WARN]`.
- Errors from menu actions (start against a busy port, failed stop) render
  inside the interactive picker instead of being lost on the redrawn screen.
- Binaries grow ~1 MB from the TUI dependency (windows-amd64 `-ldflags "-s -w"`:
  49.98 MB → 51.04 MB).

## [v1.0.0] - 2026-11-06

Forked from 9router-go (luqman-v1), upstream decolua/9router — attribution and thanks.

### Added

- **Per-key governance** — daily cost limit, request limit, IP whitelist, per-key system prompt, endpoint permissions, and policy templates applied across multiple keys (serenhope columns).
- **Token saver suite** — RTK input compression plus anti-slop injector, context squeezer, tool-call fallback, JSON guard, and Responses compact verification.
- **Output styles** — Caveman and Ponytail directive modes for compressed output.
- **Security event log** — per-event security log with alerting, TLS stealth hardening, and audit log v2.
- **Notifications** — notification feed, Telegram release notifications, and multi-target webhooks.
- **i18n** — Indonesian/English dashboard localization.
- **Free-tier catalog** — free-tier provider catalog entries with usage/cost tracking, plus custom credits tracking.
- **Routing strategies** — cache-optimized, auto, quota-weighted, and reset-aware combo routing strategies.
- **3-tier circuit breaker** — strike-breaker quota handling across subscription, cheap, and free tiers.
- **GCF compression** — pre-compressed `.br`/`.gz` dashboard asset serving.
- **Teams billing** — multi-seat/team billing support.
- **MCP server and A2A** — MCP (Model Context Protocol) server endpoint and agent-to-agent endpoints.
- **Config bundle** — import/export of configuration bundles.
- **Web executors** — ChatGPT web executor and browser-based provider executors.
- **Dashboard phase 4** — API key metrics, search/filter, bulk create, success rate and loop detection.
- **9RouterX rebranding** — full rebrand across README, wiki, skills, and docs; binary `9routerx` (`9routerx-tray` for the tray companion); module path `9routerx/proxy`; repository `github.com/yanstd888/9routerx`; wiki `yanstd888.github.io/9routerx`.

### Changed

- **BREAKING**: environment variables are renamed from `NINEROUTER_*` / `9ROUTER_*` to the `9ROUTERX_*` prefix (e.g. `9ROUTERX_URL`, `9ROUTERX_KEY`). Migration: rename the variables in your `.env`, shell profile, and deployment manifests; legacy spellings are still accepted for one release via the `internal/envcompat` fallback with a deprecation warning, and are dropped afterwards.
- **BREAKING**: product name `9router-go` becomes **9RouterX** — repository links now point at `github.com/yanstd888/9routerx`, the binary and CLI commands are `9routerx`, the module path is `9routerx/proxy`, and installers are fetched from `raw.githubusercontent.com/yanstd888/9routerx`.
- **BREAKING**: default gateway port `20130` → `888`. Migration: set `PORT=20130` in your `.env` to keep the old port.

[1.1.0]: https://github.com/yanstd888/9routerx/releases/tag/v1.1.0
[1.0.0]: https://github.com/yanstd888/9routerx/releases/tag/v1.0.0
