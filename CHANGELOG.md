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
