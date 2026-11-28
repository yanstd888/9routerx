<div align="center">

![9RouterX — Local AI Router, LLM Gateway, and Token Saver Single Binary](docs/9routerx-readme-hero-motion.gif)

# 9RouterX — FREE AI Router & Token Saver (Single Binary)

**High-performance local AI gateway and OpenAI/Anthropic-compatible LLM router compiled into a single Go binary.**

Cut tool-output token consumption by **20–40%**, automate multi-tier failover (**Subscription → Cheap → Free**), and route across **<!-- GEN:PROVIDERCOUNT total -->344<!-- /GEN --> AI providers** and **248 free models** with zero Node.js runtime overhead.

**9RouterX is the self-hosted 9Router & LiteLLM alternative** — one native Go binary you own: per-key financial governance, semantic cache, and 248 free models (24 keyless) running locally on port `20128`.

If 9RouterX helps your workflow, please give the repository a star — it helps other developers discover the project.

<a href="https://trendshift.io/repositories/348263?utm_source=trendshift-badge&amp;utm_medium=badge&amp;utm_campaign=badge-trendshift-348263" target="_blank" rel="noopener noreferrer"><img src="https://trendshift.io/api/badge/trendshift/repositories/348263/daily?language=Go" alt="yanstd888%2F9routerx | Trendshift" width="250" height="55"/></a> <br>
[![CI](https://github.com/yanstd888/9routerx/actions/workflows/ci.yml/badge.svg)](https://github.com/yanstd888/9routerx/actions/workflows/ci.yml)
[![Release CI](https://github.com/yanstd888/9routerx/actions/workflows/release.yml/badge.svg)](https://github.com/yanstd888/9routerx/actions/workflows/release.yml)
[![npm version](https://img.shields.io/npm/v/9routerx.svg?style=flat-square&logo=npm)](https://www.npmjs.com/package/9routerx)
[![npm downloads](https://img.shields.io/npm/dm/9routerx.svg?style=flat-square&label=downloads&color=22c55e)](https://www.npmjs.com/package/9routerx)
[![GHCR Container](https://img.shields.io/badge/GHCR-yanstd888%2F9routerx-2496ED?style=flat-square&logo=github&logoColor=white)](https://github.com/yanstd888/9routerx/pkgs/container/9routerx)
[![Container & Binary Pulls](https://img.shields.io/github/downloads/yanstd888/9routerx/total?style=flat-square&logo=docker&logoColor=blue&label=Docker%20pulls&color=0ea5e9)](https://github.com/yanstd888/9routerx/pkgs/container/9routerx)
[![Telegram Community](https://img.shields.io/badge/Telegram-%40aiclub__id-26A5E4?style=flat-square&logo=telegram)](https://t.me/aiclub_id)

[Quick Start](#quick-start) • [Why 9RouterX?](#why-9routerx) • [Comparison](#9routerx-vs-9router-vs-omniroute) • [Key Features](#key-features) • [Supported Clients](#supported-ai-coding-clients) • [Free Providers](#free-ai-providers-and-keyless-models-catalog) • [Database Sharing](#sharing-a-database-with-9router) • [Screenshots](#screenshots) • [Setup Guide](#setup-guide-summary-and-wiki-documentation) • [Architecture Matrix](#architecture-comparison) • [FAQ](#frequently-asked-questions-faq) • [Docs](#docs) • [Wiki](https://yanstd888.github.io/9routerx/)

**<!-- GEN:PROVIDERCOUNT total -->344<!-- /GEN -->** Built-in Providers · **<!-- GEN:PROVIDERCOUNT oauth -->34<!-- /GEN -->** OAuth-category Entries · **<!-- GEN:PROVIDERCOUNT freeAll -->34<!-- /GEN -->** Free-category Entries · **248** Cataloged Free Models (**24** Keyless) · **5** Build Targets · **1** Binary

</div>

---

> **What is 9RouterX?**
>
> **9RouterX** is an open-source, self-hosted LLM gateway written in Go with an embedded Svelte 5 + Vite + Tailwind 4 dashboard, listening on loopback port `20128` by default. It exposes Chat Completions, Anthropic Messages, Responses, embeddings, model catalogs, Ollama-compatible chat, MCP, and A2A routes. The registry contains **<!-- GEN:PROVIDERCOUNT total -->344<!-- /GEN --> built-in provider entries** grouped as **<!-- GEN:PROVIDERCOUNT apikey -->265<!-- /GEN --> API-key**, **<!-- GEN:PROVIDERCOUNT oauth -->34<!-- /GEN --> OAuth**, **<!-- GEN:PROVIDERCOUNT freeAll -->34<!-- /GEN --> free**, and **<!-- GEN:PROVIDERCOUNT webCookie -->11<!-- /GEN --> web-cookie** entries. These are catalog categories, not guaranteed authentication-flow counts: auth modes vary by provider. The static free catalog lists **248 models across 36 providers**, including **24 keyless models**; Antigravity still requires Google OAuth. See [Providers](https://yanstd888.github.io/9routerx/providers.html) for actual browser-login and credential support.

## Why 9RouterX?

AI-assisted software engineering workflows suffer from fragmented monthly subscriptions, sudden upstream rate limits, runaway token billing, and bloated CLI tool context windows. 9RouterX resolves these operational bottlenecks locally at the gateway layer before requests leave your machine:

* **Unused Monthly Quotas & Rate-Limit Interruptions → Smart 3-Tier Cascade (`Subscription → Cheap → Free`):** Stop losing unused monthly subscription credits or hitting `429 Too Many Requests` in the middle of a complex multi-file refactor. 9RouterX prioritizes your active **Subscription** tier first (via connected OAuth accounts or web-cookie sessions), overflows seamlessly to **Cheap API** providers (such as DeepSeek, Groq, Fireworks, or Nebius), and automatically falls back to **Free-Tier / Keyless** models (`248 free models across 36 providers`, including `24 keyless models`) when primary limits are exhausted.
* **Verbose CLI & Tool-Call Outputs → 20–40% Token Saver Suite:** Autonomous coding agents regularly burn tens of thousands of input tokens reading raw terminal logs, directory trees, build outputs, and repetitive JSON responses. Built-in RTK compression (`internal/tokensaver`, `internal/compression`), **50+ ready-made per-tool output filters** (`git diff`, `git log`, `grep`, `ripgrep`, `docker logs`, `npm install`, `pnpm`, `cargo build`, `go test`, `terraform plan`, `kubectl`, `pytest`, and friends), GCF/tabular headroom compression (`internal/compression/gcf`, `internal/headroom`), context squeezing, and output style directives (`Caveman`, `Ponytail`, `ADHD`, and `Anti-Slop` modes) strip redundant noise before forwarding requests upstream.
* **Uncontrolled API Spend → Per-Key Financial Governance & Guardrails:** Enforce strict daily USD spending ceilings with hard cutoffs checked when each request starts (`internal/budget`), per-key request rate limits, CIDR/IP whitelists, per-key system prompt injection, and scoped endpoint permissions (`internal/policy`, `internal/guardrails`, `internal/keikey`) before distributing local gateway keys (`sk-...`) to autonomous agents, CI pipelines, or team members.
* **Silent Endpoint Failures & Broken Tool Calls → Active Health Probing & Auto-Repair:** Upstream LLM providers frequently return malformed JSON tool-call arguments, truncated schemas, or suffer silent latency degradation. 9RouterX repairs broken tool-call structures in transit (`tool-call repair fallback`), guards against SSRF dispatch and leaks via PII masking, and continuously tracks live endpoint health (`internal/modelhealth`, `internal/resilience`) with a 3-tier circuit breaker to bypass degraded nodes automatically.
* **Redundant Repeated Prompts → Semantic, Prompt & Sticky TTL Caching:** Reduce round-trip latency and API token billing through local vector-similarity semantic caching (`internal/semanticcache`), automatic upstream prompt cache header passthrough (`internal/promptcache`), **cache-aware sticky TTL routing** (saving up to ~90% on cached input reads), and zero-allocation FastJSON parsing (`internal/fastjson`).
* **Heavy Runtime Dependencies → Single Self-Contained Go Binary:** No Node.js runtime memory bloat, no Python virtual environments, and no external PostgreSQL or Redis containers required. Everything—including the reactive Svelte 5 dashboard, SQLite storage engine, MCP server, and MITM inspector—runs from a single native binary across Linux (`x64`/`arm64`), macOS (`x64`/`arm64`), and Windows (`x64`).

## 9RouterX vs 9Router vs OmniRoute

9RouterX is an independent Go + Svelte 5 evolution of 9Router (`decolua/9router` and `luqman-v1/9router-go`) that expanded its own enterprise governance and token-saving architecture, sitting in the same self-hosted gateway space as OmniRoute (`cheaper-inference/OmniRoute`). Every row in the table below states a verifiable fact—where an architectural trait has no standardized public benchmark behind it, it is explicitly labeled *On paper* rather than given a synthetic number. Star counts were checked live on **Nov 14, 2026**; evaluate each gateway by its operational requirements and feature rows.

| Metric / Capability | **9RouterX** (This Repo) | **9Router** (`decolua`) | **OmniRoute** (`cheaper-inference`) |
| :--- | :--- | :--- | :--- |
| **GitHub Stars (Nov 14, 2026)** | 68 | 32,422 | 87,366 |
| **Language / Runtime** | **Go, single native binary + Svelte 5 dashboard** | JavaScript / Node.js + Next.js | TypeScript / Node.js |
| **Installation Footprint** | **One native binary (~48 MB linux-amd64), zero runtime dependencies** | `npm i -g 9router` (6 deps, requires Node.js runtime) | npm (~105 deps) / Docker / pnpm |
| **Supported Providers** | **<!-- GEN:PROVIDERCOUNT total -->344<!-- /GEN -->** (`<!-- GEN:PROVIDERCOUNT apikey -->265<!-- /GEN --> API` · `<!-- GEN:PROVIDERCOUNT oauth -->34<!-- /GEN --> OAuth` · `<!-- GEN:PROVIDERCOUNT freeAll -->34<!-- /GEN --> free` · `<!-- GEN:PROVIDERCOUNT webCookie -->11<!-- /GEN --> cookie`) | 40+ | 375–380 (150+ free) |
| **Cataloged Free Models** | **248 across 36 providers (24 keyless)** | 100+ | 1,400+ models (~1.62B free tokens/mo) |
| **Embedded Dashboard** | **Svelte 5 + Vite + Tailwind 4, embedded in binary** | Next.js React web app | Desktop / PWA |
| **Database Engine** | **Embedded SQLite, drop-in schema-compatible** | Same SQLite schema | SQLite |
| **Behavior Under Heavy Traffic** | *On paper:* a compiled native Go process—one process for the gateway, dashboard, and SQLite engine, with no V8 heap or GC pauses growing with the request queue. Measure on your own workload. | *On paper:* Node.js runtime (V8 + single-threaded event loop); sustained throughput depends on Node version and warm-up state. | *On paper:* Node.js runtime, same event-loop and V8 heap considerations. |
| **Latency Profile** | *On paper:* zero cold-start tax after boot; hot request paths use allocation-conscious Go routines and FastJSON. | *On paper:* first-request warm-up on the Node/Next.js side. | *On paper:* first-request warm-up on the Node side. |

### What 9RouterX Adds on Top of 9Router

9RouterX preserves 100% SQLite database compatibility with upstream 9Router while adding a dedicated optimization and security stack:
* **Full Per-Key Governance:** Daily USD hard cutoffs checked at request start, per-key rate limits, IP whitelists, reusable plan templates, SSRF dispatch protection, and PII masking (versus basic key authentication).
* **Token Saver Suite:** 20–40% RTK compression, 50+ per-tool CLI output filters, GCF/tabular headroom compression, context squeezer, output style directives (`Caveman`, `Ponytail`, `ADHD`, `Anti-Slop`), and automated tool-call repair fallback.
* **Multi-Layer Caching & Resilience:** Semantic cache + prompt cache passthrough + cache-aware sticky TTL routing, windowed-LRU rotation, auto-prune for dead credentials, and burn-in account recovery.
* **Expanded Free & Keyless Catalog:** 248 free models across 36 providers (including 24 keyless models).
* **Advanced Protocols & Ops Modules:** Built-in MCP Server, A2A protocol, TLS stealth fingerprinting, MITM traffic inspector, multi-target webhooks, Telegram bot alerts, encrypted credential Vault, and one-command service migration (`9routerx service take-over`).

### Where OmniRoute is Bigger

OmniRoute's upstream catalog is larger, stated plainly: roughly 375–380 providers (150+ of them free), 1,400+ models, a pooled ~1.62B free tokens/month allowance, 20 routing strategies, and quota-aware fallback. That catalog breadth is their core strength. Where **9RouterX** differs: it ships as **one compiled Go binary with zero runtime dependencies** (versus an npm installation pulling ~105 packages), stays lightweight enough for **small self-hosted VPS instances or edge hardware**, enforces **granular per-key financial governance and RTK/GCF tool compression**, and embeds the **Svelte 5 dashboard directly inside the binary**. Compare features against your infrastructure requirements—neither replaces the other.

### Go + Svelte 5 vs a Node.js Stack

The architectural difference manifests in day-to-day operations (*on paper throughout — measure on your own workload*):

* **Single-File Distribution:** One native executable per platform (`linux-x64`, `linux-arm64`, `win32-x64`, `darwin-x64`, `darwin-arm64`) versus `npm install` pulling a Node.js runtime dependency tree and `node_modules`.
* **Instant Cold Start:** The compiled Go gateway serves its first completion request immediately upon process boot without waiting for JIT/module graph warm-up.
* **Predictable Memory Footprint:** Compiled Go services of comparable scope typically idle with a smaller resident memory footprint than Node.js/Next.js servers.
* **Concurrency Under Load:** Concurrent streaming connections are multiplexed across lightweight Go goroutines rather than competing on a single-threaded V8 event loop.
* **Atomic Self-Updates:** Updating swaps a single binary (`9routerx update` or container image pull) without rebuilding local package trees.

## System Architecture

```text
  AI Coding Clients (Claude Code, Cursor, Codex, Cline, OpenCode, Gemini CLI, OpenClaw)
                                           │
                   http://localhost:20128/v1 (OpenAI / Anthropic / MCP / A2A)
                                           ▼
┌─────────────────────────────────────────────────────────────────────────────────────┐
│                             9RouterX Single Go Binary                               │
│  ┌────────────────────────┐  ┌────────────────────────┐  ┌───────────────────────┐  │
│  │  Governance & Policy   │→ │   Token Saver Suite    │→ │  Semantic & Prompt    │  │
│  │  • Daily Cost Cutoff   │  │   • RTK Compression    │  │       Caching         │  │
│  │  • Rate Limit / IP WL  │  │   • 50+ Tool Filters   │  │  • Vector Similarity  │  │
│  │  • Guardrails & Vault  │  │   • GCF / Headroom     │  │  • Sticky TTL (~90%)  │  │
│  │  • PII Mask / SSRF Grd │  │   • Tool-Call Repair   │  │  • FastJSON Engine    │  │
│  └────────────────────────┘  └────────────────────────┘  └───────────────────────┘  │
│                                           │                                         │
│                                           ▼                                         │
│  ┌───────────────────────────────────────────────────────────────────────────────┐  │
│  │          Smart Combo Router & Resilience Engine (Model Health Probes)         │  │
│  │   Strategies: Fallback │ Round-Robin │ Sticky TTL │ Capacity │ Fusion │ Auto  │  │
│  └───────────────┬────────────────────────┬────────────────────────┬─────────────┘  │
└──────────────────┼────────────────────────┼────────────────────────┼────────────────┘
                   ▼                        ▼                        ▼
       [Tier 1: Subscription]      [Tier 2: Cheap API]     [Tier 3: Free / Keyless]
        <!-- GEN:PROVIDERCOUNT oauth -->34<!-- /GEN --> OAuth-category Entries  <!-- GEN:PROVIDERCOUNT apikey -->265<!-- /GEN --> API-category Entries  248 Free Models across
        <!-- GEN:PROVIDERCOUNT webCookie -->11<!-- /GEN --> Web Cookie Providers      (DeepSeek, Groq...)     36 Providers (24 Keyless)
```

## Key Features

| Feature Group | Capabilities & Internal Modules |
| :--- | :--- |
| **Routing & Fallback** | **Auto 3-Tier Fallback** (`Subscription → Cheap → Free`), **Combo Routing** (`fallback`, `round-robin`, `sticky`, `capacity`, `fusion`, `cache-optimized`, `auto`, `quota-weighted`, `reset-aware` via `internal/routing`, `internal/combos` & `internal/combopresets`), multi-account round-robin load balancing, active **Model Health** probing (`internal/modelhealth`), and 3-stage circuit-breaker resilience (`internal/resilience`). |
| **Token Saver Suite** | **20–40% tool-output token savings** (`internal/tokensaver`, `internal/compression`), **RTK Compression**, **50+ ready-made per-tool output filters** (for `git diff`, `grep`, `docker logs`, `npm install`, `terraform plan`, and friends), **GCF/Tabular Headroom** (`internal/compression/gcf`, `internal/headroom`), **Context Squeezer** for long histories, **Output Style Directives** (`Caveman`, `Ponytail`, `ADHD`), **Anti-Slop Injector** (`internal/tokensaver/antislop`), and automated **Tool-Call Repair Fallback**. |
| **Governance per Key** | **Daily Cost Ceiling + Hard Cutoff** (`internal/budget`, checked per request), per-key request rate limits, **IP Whitelist**, per-key custom system prompts, granular endpoint permissions, reusable **Plan Templates** (`internal/policy`, `internal/guardrails`, `internal/keikey`), SSRF dispatch protection, PII masking, encrypted credential storage (`internal/vault`, `internal/sessionvault`), and audit logs (`internal/securitylog`). |
| **Providers & Auth** | **<!-- GEN:PROVIDERCOUNT total -->344<!-- /GEN --> built-in entries**, grouped as <!-- GEN:PROVIDERCOUNT apikey -->265<!-- /GEN --> API-key, <!-- GEN:PROVIDERCOUNT oauth -->34<!-- /GEN --> OAuth, <!-- GEN:PROVIDERCOUNT freeAll -->34<!-- /GEN --> free, and <!-- GEN:PROVIDERCOUNT webCookie -->11<!-- /GEN --> web-cookie. Categories are not auth-flow counts. Custom endpoints, custom headers, Kiro JSON import, cookie paste, and supported browser capture/import are available; persistent profiles and Xvfb apply only to the generic runner. See the [provider support matrix](https://yanstd888.github.io/9routerx/providers.html#browser-based-login). |
| **Free Tier & Keyless** | **<!-- GEN:PROVIDERCOUNT freeAll -->34<!-- /GEN --> free-tier providers** (<!-- GEN:PROVIDERCOUNT freeTier -->23<!-- /GEN --> `CategoryFreeTier` + <!-- GEN:PROVIDERCOUNT free -->11<!-- /GEN --> `CategoryFree`) and **248 free models cataloged across 36 providers**, including **24 cataloged keyless models** without a user-supplied upstream API key (`antigravity`, `opencode`, `blackbox`); Antigravity requires Google OAuth login once. Tracks daily, monthly, credit, one-time, and uncapped quota regimes automatically. |
| **Cache & Performance** | **Semantic Cache** (`internal/semanticcache`) for vector-matched query deduplication, upstream **Prompt Cache** optimization (`internal/promptcache`), **Cache-Aware Sticky TTL** (reduces cached input costs by up to ~90%), and high-throughput **FastJSON** serialization (`internal/fastjson`). |
| **Observability** | **Extended Logging + Payload Inspector**, custom usage analytics windows (`12h`, `14d`, custom date ranges), Tremor-inspired data visualizations (`bits-ui` + `layerchart`), distributed **Tracing** (`internal/tracing`, `internal/observ`), **Uptime** monitoring (`internal/uptime`), **Security Log**, multi-target **Webhooks + Notifications** (`internal/notifications`), and **Telegram Bot** alerts (`internal/telegram`). |
| **Infra & Extras** | **Single Go Binary** across 5 OS/arch targets, background daemon CLI (`start`/`stop`/`restart`/`status`/`logs`), one-command migration (`9routerx service take-over`), Docker container (`ghcr.io`), desktop **Tray App**, built-in **Tunnel** (`internal/tunnel`), self-**Updater** (`internal/updater`), encrypted **Sync Bundle** backup/restore (`internal/syncbundle`), **TLS Stealth** (`internal/tlsstealth`), **MITM Traffic Inspection** (`internal/mitm`), **MCP Server** (`internal/mcpserver`), **A2A Protocol** (`internal/a2a`), and **Media TTS/STT**. |

### Current routing and management additions

- [Combos](https://yanstd888.github.io/9routerx/combos.html): `lkgp` usage-based ranking, combo budgets, derived family warnings, and a limited direct API-key arena. Multi-member fusion does not use the combo-budget gate.
- [Security](https://yanstd888.github.io/9routerx/security.html) and [Token Saver](https://yanstd888.github.io/9routerx/token-saver.html): scoped dashboard tokens, opt-in reversible PII placeholders, and tool-loop watch. Coverage is protocol-dependent; masking is not guaranteed anonymization.
- [Operations](https://yanstd888.github.io/9routerx/operations.html): automatic SQLite snapshots, partial/sensitive remote JSON backups, and user-supplied proxy harvest batches. Browser profiles are excluded from built-in DB backups.
- [Dashboard](https://yanstd888.github.io/9routerx/dashboard.html) and [Analytics](https://yanstd888.github.io/9routerx/usage-analytics.html): local doctor/onboarding status, mixed-window cache metrics, estimated savings, and rough gateway-wide burn-rate projections. These are not live upstream readiness or financial forecasts.

These pages describe the reviewed source. Check `9routerx version` and the release history before assuming every change is in an older binary.

### v1.2 Cost & Resilience Controls (Conservative Opt-In Defaults)

All advanced v1.2 cost-reduction and account-protection mechanisms ship **disabled (`OFF`) by default** to guarantee predictable baseline behavior until explicitly enabled by the operator:

| Control Feature | Environment / Config Flag | Default | Operational Impact |
| :--- | :--- | :---: | :--- |
| **Cache-Aware Sticky TTL** | Combo strategy `ttl` mode (`5m` Claude default, up to `60m` Gemini) | `OFF` | Pins consecutive requests to the same warm upstream account within the vendor's prompt-cache window so cached prefixes are reused instead of re-billed—delivering **up to ~90% input-cost reduction on cache hits** (e.g., Anthropic bills cached reads at ~10% of standard input pricing). |
| **Windowed-LRU Rotation** | `ROTATION_WINDOW=N` | `OFF` | Restricts round-robin selection to the top `N` active candidate connections to prevent pool-wide rate-limit burn-in across large account pools. |
| **Dead-Credential Auto-Prune** | `AUTOPRUNE_ENABLED` (supports dry-run mode) | `OFF` | Automatically deactivates upstream connections after consecutive permanent credential failures (`401 Unauthorized`, `403 Banned`, `402 Payment Required`). |
| **Account Burn-In Protection** | `BURNIN_ENABLED` | `OFF` | Guides degraded accounts through an automated **Suspect → Heal (1-in-5 request throttle) → Rescue (temporary rotation quarantine)** recovery cycle. |

## Quick Start

Install 9RouterX, create your local gateway API key in the dashboard on port `20128`, and execute your first routed completion in 3 steps.

### 1. Install 9RouterX

> **Upgrading from 9Router?** Your existing SQLite data carries over automatically. Run `9routerx service take-over` to replace the legacy gateway process and its system autostart service in a single step (see the [Installation & Migration Guide](https://yanstd888.github.io/9routerx/installation.html)).

**Option A — Via npm (Recommended for Linux, macOS, and Windows; requires Node.js >= 18 for wrapper install only):**

```bash
npm install -g 9routerx
```

**Option B — Via Official Installer Script (Linux / macOS / Windows):**

```bash
# Linux & macOS
curl -fsSL https://raw.githubusercontent.com/yanstd888/9routerx/main/install.sh | bash
```

```powershell
# Windows (PowerShell)
irm https://raw.githubusercontent.com/yanstd888/9routerx/main/install.ps1 | iex
```

**Option C — Prebuilt Standalone Binaries:**

Download `9routerx-linux-amd64`, `9routerx-linux-arm64`, `9routerx-darwin-amd64`, `9routerx-darwin-arm64`, or `9routerx-windows-amd64.exe` from [GitHub Releases](https://github.com/yanstd888/9routerx/releases). These are executable assets, not archives; verify them against `SHA256SUMS.txt`.

### 2. Start & Create API Key

```bash
9routerx
# Dashboard: http://localhost:20128 (Default password: 123456 — change in Settings → Profile)
# Next: Settings → API Keys → Create Key → copy your key (sk-...)
```

* **Default Login:** First login uses the default compatibility password `123456`. Change it immediately in **Settings → Profile**.
* **Gateway Auth Note:** `RequireApiKey` (`internal/middleware/auth.go`) validates client keys against your local SQLite database (`0 keys configured = 401 Authentication required`). Always create at least one gateway API key (`sk-...`) in **Settings → API Keys** before sending requests.

### 3. Send Your First Request (OpenCode Keyless, Zero Upstream Login)

Once you have created your local gateway key (`sk-YOUR-KEY`), test immediately with `oc/big-pickle` from OpenCode Free without any upstream login. To test `antigravity/gemini-3.7-flash-medium`, sign in with Google through Antigravity once first (free); no user-supplied upstream API key is needed.

```bash
curl http://127.0.0.1:20128/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-YOUR-KEY" \
  -d '{
    "model": "oc/big-pickle",
    "messages": [
      {"role": "user", "content": "Verify 9RouterX local gateway on port 20128 in one sentence."}
    ]
  }'
```

### Keep the Terminal Free (Background Mode)

Run 9RouterX as a background daemon without tying up an active terminal session:

```bash
9routerx start               # launch in background (writes DATA_DIR/run/gateway.pid)
9routerx status              # check running PID and active port
9routerx restart             # restart background instance
9routerx logs -n 100         # tail the last 100 lines of DATA_DIR/run/gateway.log
9routerx stop                # stop the background daemon
9routerx service take-over   # migrate from legacy 9Router service & take over autostart
9routerx version             # print running binary version & build metadata
```

* **PID & Logs:** Background state is stored in `DATA_DIR/run/gateway.pid` and daemon output is written to `DATA_DIR/run/gateway.log`.
* **Windows Note:** On Windows, `9routerx stop` uses `TerminateProcess` (which skips graceful in-flight drain). For a graceful drain on Windows, trigger **Shutdown** from the web dashboard or use `9routerx restart`.

### Running with Docker (GHCR)

Run 9RouterX in a container with persistent SQLite storage. The explicit host setting makes the published port reachable; replace the password placeholder and keep authentication enabled:

```bash
docker run -d \
  --name 9routerx \
  --restart unless-stopped \
  -p 20128:20128 \
  -v ~/.9router:/data \
  -e DATA_DIR=/data \
  -e HOST=0.0.0.0 \
  -e INITIAL_PASSWORD=your-secure-password \
  ghcr.io/yanstd888/9routerx:latest
```

* **Experimental Builds:** To test bleeding-edge commits from the experimental branch, pull `ghcr.io/yanstd888/9routerx:exp` (`docker pull ghcr.io/yanstd888/9routerx:exp`). Experimental workflow builds only publish to the `exp` tag and never overwrite `latest`.

### Install Alternatives & Build from Source

**Release binary:** download from [GitHub Releases](https://github.com/yanstd888/9routerx/releases/latest) and verify against `SHA256SUMS.txt`.

**Experimental builds:** unpublished work ships as experimental builds — GitHub Releases marks them **Pre-release** (so `9routerx update` keeps pointing at stable), and GHCR tags them `exp` (`ghcr.io/yanstd888/9routerx:exp`), never `latest`. Opt in only to help shake out bugs.

**Build from source:** prerequisites Go 1.27 and Bun 1.x (dashboard is embedded into the binary, so build web first) — then `make build`. Full flags and steps: [wiki — installation](https://yanstd888.github.io/9routerx/installation.html).

**Start on boot:** autostart/service installation is documented in the wiki — [installation](https://yanstd888.github.io/9routerx/installation.html).

## Sharing a Database with 9Router

9RouterX is engineered as a drop-in compatible upgrade for existing **9Router** and **9router-go** installations. It listens on the exact same default port (`20128`) and opens the same `DATA_DIR/db/data.sqlite` database—meaning your configured **providers, OAuth connections, combos, client API keys, and usage history carry over automatically without migration**.

| Deployment Mode | Default `DATA_DIR` / SQLite Database Path |
| :--- | :--- |
| **Linux / macOS (Native)** | `~/.9router` (opens `~/.9router/db/data.sqlite`) |
| **Windows (Native)** | `%APPDATA%\9router` (opens `%APPDATA%\9router\db\data.sqlite`) |
| **Docker Container** | Mount host `~/.9router:/data` with `-e DATA_DIR=/data` |

* **Caveat:** Do not run both 9Router and 9RouterX against the same SQLite file simultaneously. Stop the old process first (or run `9routerx service take-over` / back up `data.sqlite` before switching) so only one binary holds the SQLite write lock.

## Screenshots

Every screen ships in dark and light themes, at desktop and mobile widths. The
full gallery — with per-page commentary — lives on the wiki's
[**Screenshots**](https://yanstd888.github.io/9routerx/screenshots.html) page.

| | Dark | Light |
|---|---|---|
| **Providers** — connect AI providers and manage custom OpenAI/Anthropic-compatible endpoints | ![Providers view, dark theme — the provider catalog and connection list in the 9RouterX dashboard](docs/screenshots/providers.png) | ![Providers view, light theme — the same provider catalog and connection list in light mode](docs/screenshots/providers-light.png) |

## Supported AI Coding Clients

9RouterX works out of the box with any OpenAI-compatible, Anthropic-compatible, Responses API, Ollama-compatible, MCP, or A2A client:

* **Claude Code** (`@anthropic-ai/claude-code`)
* **OpenAI Codex CLI**
* **Cursor**
* **Antigravity**
* **Gemini CLI**
* **OpenCode**
* **Cline / Roo Code**
* **OpenClaw**

> **IPv6 Resolution Note (`localhost` vs `127.0.0.1`):** Some CLI tools (upstream documentation specifically notes **OpenClaw** and certain Node/Go CLIs) may resolve `localhost` to IPv6 `::1` first and fail to connect. If a client refuses to connect via `localhost`, use explicit IPv4 **`http://127.0.0.1:20128`** instead.

### Example 1: Claude Code Setup

Route Claude Code through `http://localhost:20128` to enable 20–40% tool-output token compression, 50+ per-tool filters, and multi-provider failover:

```bash
export ANTHROPIC_BASE_URL="http://localhost:20128"
export ANTHROPIC_API_KEY="sk-YOUR-KEY"
claude
```

### Example 2: Codex / OpenCode / OpenClaw (Explicit IPv4 `127.0.0.1`)

```bash
export OPENAI_BASE_URL="http://127.0.0.1:20128/v1"
export OPENAI_API_KEY="sk-YOUR-KEY"
```

### Example 3: Cline / Cursor JSON Configuration

```json
{
  "apiProvider": "openai-compatible",
  "openAiBaseUrl": "http://127.0.0.1:20128/v1",
  "openAiApiKey": "sk-YOUR-KEY",
  "openAiModelId": "oc/big-pickle"
}
```

### Example 4: Gemini CLI / Custom Anthropic-Compatible Clients

```bash
export GEMINI_API_BASE_URL="http://127.0.0.1:20128/v1"
export GEMINI_API_KEY="sk-YOUR-KEY"
```

## Free AI Providers and Keyless Models Catalog

9RouterX includes a built-in catalog of **248 free models across 36 providers** (verified in `freemodels.go`), featuring **24 cataloged keyless models** (`antigravity`, `opencode`, `blackbox`) without a user-supplied upstream API key. Antigravity requires Google OAuth login once (free); OpenCode offers zero-login access. Gemini Web requires a browser login/session cookie.

### Supported Free Quota Regimes

* **`keyless`**: No user-supplied upstream provider API key (`antigravity`, `opencode`, `blackbox`). Antigravity requires Google OAuth login once; OpenCode and the cataloged Blackbox free endpoint do not require an upstream login.
* **`recurring-daily`**: Free request or token allowance resets automatically every 24 hours.
* **`recurring-monthly`**: Free quota refreshes each calendar or billing month.
* **`recurring-credit`**: Periodic free credit grants replenished by the provider.
* **`recurring-uncapped`**: Rate-limited free endpoints without hard lifetime exhaustion caps.
* **`one-time-initial`**: Initial trial credit pool granted upon account creation.

### Complete Free Provider Directory (36 Providers · 248 Free Models)

| Provider | Model free | Keyless | Quota / Access Regime |
| :--- | :---: | :---: | :--- |
| `bazaarlink` | 32 | — | Free API tier (`recurring-daily` / `recurring-uncapped`) |
| `bluesminds` | 22 | — | Free API tier (`recurring-daily` / `recurring-uncapped`) |
| `vertex` | 21 | — | Google Cloud credits / free tier (`one-time-initial` / `recurring-credit`) |
| `kilo-gateway` | 12 | — | Free gateway tier (`recurring-daily`) |
| `fireworks` | 10 | — | Initial / recurring free inference credits (`one-time-initial`) |
| `antigravity` | 9 | 9 | **Free after Google OAuth login once; no user-supplied upstream API key (e.g. `gemini-3.7-flash-high/medium/low`)** |
| `opencode` | 9 | 9 | **Keyless (`keyless` — zero upstream setup, e.g. `oc/big-pickle`)** |
| `cloudflare-ai` | 9 | — | Recurring daily free Workers AI neurons (`recurring-daily`) |
| `qoder` | 9 | — | One-click OAuth free coding tier (`recurring-daily` / `recurring-monthly`) |
| `opencode-zen` | 9 | — | Free developer coding tier (`recurring-daily`) |
| `ollama` | 8 | — | Local / free cloud endpoints (`recurring-uncapped`) |
| `hyperbolic` | 8 | — | Initial trial inference credits (`one-time-initial`) |
| `nous` | 7 | — | Nous Research free API tier (`recurring-credit` / `one-time-initial`) |
| `api-airforce` | 7 | — | Recurring daily / uncapped free tier (`recurring-daily`) |
| `kiro` | 7 | — | Free developer tier + `external_idp` auth support (`recurring-monthly`) |
| `blackbox` | 6 | 6 | **Keyless (`keyless` — zero upstream setup)** |
| `cohere` | 6 | — | Recurring monthly trial evaluation key (`recurring-monthly`) |
| `huggingface` | 6 | — | Serverless inference API free tier (`recurring-daily`) |
| `groq` | 5 | — | Recurring daily high-speed LPU free tier (`recurring-daily`) |
| `mistral` | 5 | — | La Plateforme experiment free tier (`recurring-monthly`) |
| `sambanova` | 5 | — | Recurring free developer cloud tier (`recurring-daily`) |
| `glm-cn` | 4 | — | One-click OAuth / Zhipu CN free models (`recurring-daily`) |
| `gemini` | 4 | — | Google AI Studio recurring daily free tier (`recurring-daily`) |
| `nvidia` | 4 | — | NVIDIA NIM developer free credit tier (`one-time-initial` / `recurring-credit`) |
| `llm7` | 4 | — | One-click OAuth free routing tier (`recurring-daily`) |
| `openrouter` | 3 | — | Recurring `:free` and `openrouter/auto` model endpoints (`recurring-daily`) |
| `agnes` | 3 | — | Free API inference tier (`recurring-daily`) |
| `glm` | 2 | — | Zhipu GLM global free models (`recurring-daily`) |
| `cerebras` | 2 | — | Recurring daily wafer-scale free inference tier (`recurring-daily`) |
| `deepseek` | 2 | — | Initial credit / off-peak developer tier (`one-time-initial`) |
| `morph` | 2 | — | Fast code-apply free developer tier (`recurring-monthly`) |
| `tokenharbor` | 2 | — | Free API routing tier (`recurring-daily`) |
| `baidu` | 1 | — | Ernie / Qianfan free model (`recurring-daily`) |
| `nebius` | 1 | — | Nebius AI Studio trial credit free model (`one-time-initial`) |
| `sensenova` | 1 | — | SenseTime free model endpoint (`recurring-daily`) |
| `tencent` | 1 | — | Tencent Hunyuan free model endpoint (`recurring-daily`) |
| **Total (36 Providers)** | **248** | **24** | **Verified in `freemodels.go`** |

## Provider Categories Breakdown

9RouterX groups **<!-- GEN:PROVIDERCOUNT total -->344<!-- /GEN --> built-in entries** into four catalog categories. These are not four mutually exclusive authentication architectures; follow each provider's credential form:

1. **<!-- GEN:PROVIDERCOUNT apikey -->265<!-- /GEN --> API-key category entries:** Includes OpenAI, Anthropic, Gemini, DeepSeek, and other API services. Header requirements vary by upstream.
2. **<!-- GEN:PROVIDERCOUNT oauth -->34<!-- /GEN --> OAuth-category entries:** Includes browser, device-code, token-import, and API-key forms. For example, `muse` uses API-key auth despite its category; this is not a promise of <!-- GEN:PROVIDERCOUNT oauth -->34<!-- /GEN --> one-click flows.
3. **<!-- GEN:PROVIDERCOUNT freeAll -->34<!-- /GEN --> free-category entries:** <!-- GEN:PROVIDERCOUNT freeTier -->23<!-- /GEN --> `CategoryFreeTier` and <!-- GEN:PROVIDERCOUNT free -->11<!-- /GEN --> `CategoryFree`. Free quota and eligibility belong to the upstream service and can change.
4. **<!-- GEN:PROVIDERCOUNT webCookie -->11<!-- /GEN --> web-cookie entries:** `claude-web`, `deepseek-web`, `gemini-web`, `grok-web`, `muse-web` (Baidu Yiyan/ERNIE, not Meta Muse), and `perplexity-web`. Generic Grok/Perplexity/Yiyan capture uses persistent profiles; dedicated Gemini/DeepSeek/Claude login uses temporary profiles. Import/reset support is not universal. See [Providers](https://yanstd888.github.io/9routerx/providers.html#browser-based-login).

### Adding a Custom Provider

You can register any custom OpenAI-compatible or Anthropic-compatible upstream server directly in the dashboard (`http://localhost:20128` → **Providers** → **Add Custom**):

* Set custom base URLs for local or private inference servers (vLLM, Ollama, llama.cpp, LM Studio, LiteLLM, or enterprise gateways).
* Inject custom HTTP authentication or routing headers per provider.
* Attach dedicated per-key outbound HTTP/SOCKS5 proxy pools and configure SSRF dispatch guard rules.

## Embedded Dashboard Overview

9RouterX compiles a reactive **Svelte 5 + Vite + Tailwind 4** management interface (built with `shadcn-svelte`, `bits-ui`, and `layerchart`) directly into the Go binary at `http://localhost:20128` (default password: `123456`):

* **Token Saver & Compression Analytics:** Monitor real-time 20–40% token reduction ratios, RTK compression metrics, **50+ ready-made per-tool output filters**, GCF/tabular headroom stats, output style directives (`Caveman`, `Ponytail`, `ADHD`, `Anti-Slop`), context squeezer activity, and semantic cache hit rates.
* **Connections & Authentication:** Configure catalog entries with their supported credential forms, including Kiro JSON import, cookie paste, and provider-specific browser capture/import. Persistent-profile support is limited to the generic runner. Inspect model health and build combos with the [documented strategies and budgets](https://yanstd888.github.io/9routerx/combos.html).
* **Per-Key Governance & Custom Usage Ranges:** Issue client API keys (`sk-...`) with daily USD cutoffs enforced at request start, request rate limits, IP whitelists, and custom system prompts; inspect Tremor-style usage charts across `12h`, `14d`, or custom date ranges; and inspect full request/response JSON payloads in the Extended Logging & Payload Inspector.

## Setup Guide Summary and Wiki Documentation

For advanced deployment and module configuration, refer to the [**Official 9RouterX Documentation & Wiki**](https://yanstd888.github.io/9routerx/):

* [**Installation, Background CLI & Docker Guide**](https://yanstd888.github.io/9routerx/installation.html) — Custom port configuration (default `20128`), daemon commands (`start`/`stop`/`restart`/`status`/`logs`), one-step `9routerx service take-over`, autostart on boot, desktop tray mode, and GHCR containers.
* [**Client Setup Guide**](https://yanstd888.github.io/9routerx/client-setup.html) — Ready-to-copy snippets for Claude Code, Cursor, Codex CLI, Cline, OpenCode, and OpenClaw.
* [Combo Routing & Budgets](https://yanstd888.github.io/9routerx/combos.html): fallback chains, strategy IDs, timeout/grace, family warnings, and arena limitations.
* [Token Saver & Tool-Loop Watch](https://yanstd888.github.io/9routerx/token-saver.html): RTK filters, compression, output styles, and protocol-specific loop detection.
* [Key Governance](https://yanstd888.github.io/9routerx/governance.html) and [Security](https://yanstd888.github.io/9routerx/security.html): per-key budgets, scheduling hints, dashboard scopes, PII and TLS boundaries.
* [Operations & Backups](https://yanstd888.github.io/9routerx/operations.html): automatic snapshots, sensitive remote JSON artifacts, proxy harvest, and runtime maintenance.
* [API Reference](https://yanstd888.github.io/9routerx/api.html): mounted endpoint groups, including MCP/A2A and the recent management routes.

## Architecture Comparison

Factual capability matrix comparing **9RouterX**, upstream **9router-go**, and the original **9Router (Next.js)**:

| Capability / Metric | **9RouterX** (This Repository) | **9router-go** (Upstream Port) | **9Router** (Original Next.js) |
| :--- | :--- | :--- | :--- |
| **Runtime Binary** | **Single Native Go Binary** | Single Native Go Binary | Node.js + Next.js Runtime |
| **Default Port** | **`20128`** | `20128` | `20128` |
| **Database Compatibility** | **Drop-in `DATA_DIR/db/data.sqlite` sharing** | `DATA_DIR/db/data.sqlite` | `~/.9router/db/data.sqlite` |
| **Embedded Dashboard** | **Svelte 5 + Vite + Tailwind 4 (`shadcn-svelte` + `layerchart`)** | Embedded Web UI | Next.js React Web App |
| **Supported Providers** | **<!-- GEN:PROVIDERCOUNT total -->344<!-- /GEN --> Providers (`<!-- GEN:PROVIDERCOUNT apikey -->265<!-- /GEN --> API` · `<!-- GEN:PROVIDERCOUNT oauth -->34<!-- /GEN --> OAuth` · `<!-- GEN:PROVIDERCOUNT freeAll -->34<!-- /GEN --> Free` · `<!-- GEN:PROVIDERCOUNT webCookie -->11<!-- /GEN --> Cookie`)** | Core Provider Subset | Core JS Providers |
| **Cataloged Free Models** | **248 Free Models across 36 Providers (24 Keyless)** | Static Subset | Manual Configuration |
| **Token Saver Suite** | **20–40% RTK Compression, 50+ Tool Filters, GCF Headroom, Context Squeezer, Tool Repair** | Not Included | Not Included |
| **Per-Key Governance** | **Daily Cost Cutoff, Request Limits, IP Whitelist, Plan Templates, PII Masking** | Basic Key Auth | Basic Key Auth |
| **Caching Layer** | **Semantic Cache + Prompt Cache + Sticky TTL (~90% Read Savings) + FastJSON** | Passthrough | Passthrough |
| **Resilience Controls** | **3-Tier Circuit Breaker, Windowed-LRU, Auto-Prune, Burn-In Protection** | Basic Failover | Basic Failover |
| **Advanced Modules** | **MCP Server, A2A, TLS Stealth, MITM Inspector, Multi-Target Webhooks, Telegram Bot, Vault** | Core Proxy Routing | Core Proxy Routing |
| **Supported Platforms** | **5 Platforms** (`linux-x64`, `linux-arm64`, `win32-x64`, `darwin-x64`, `darwin-arm64`) | Go Target Builds | Requires Node.js Environment |

## Credential encryption

The "Credential encryption: OFF" banner means provider credentials and browser sessions are stored unencrypted. An absent or invalid `ROUTER_MASTER_KEY` disables sealing.

1. Generate **32 random bytes** on your own machine and encode them with standard padded Base64 (for example, `openssl rand -base64 32`). Keep the result private; never paste it into chat or logs.
2. Set `ROUTER_MASTER_KEY` in the environment of the **gateway process**. On Linux/macOS, use `export ROUTER_MASTER_KEY='<BASE64_VALUE>'`; in PowerShell, use `$env:ROUTER_MASTER_KEY='<BASE64_VALUE>'`. These are placeholders, not usable keys.
3. Persist the value in your protected service/container environment or secret store, then restart the gateway. A shell export does not change an already running service.
4. Check Dashboard → Security → Credential Vault: sealing must show enabled. Keep the same key on later restarts and protect a separate backup of it alongside your database recovery plan.

Do not replace the key casually. Losing it can make sealed credentials unrecoverable. The vault supports re-wrapping and an optional `ROUTER_MASTER_KEY_PREVIOUS`; preserve the old key during a planned rotation and persist the new current key before restarting. Without a usable old key, affected credentials must be re-entered. See [Security](https://yanstd888.github.io/9routerx/security.html) for the vault boundaries.

## Claude Web (Unofficial, Opt-in)

9RouterX can also drive claude.ai's web interface with the `sessionKey` browser cookie through a provider named `claude-web`. **It is disabled by default and it is not part of the free-tier catalog.** Enable it with `CLAUDE_WEB_ENABLED=true` or the toggle in Dashboard → Web Cookie Providers; `CLAUDE_WEB_ENABLED=false` is the operator kill-switch that always wins.

Be clear about what this lane is before enabling it:

- It violates [Anthropic's terms of service](https://www.anthropic.com/legal/consumer-terms) — the account you paste the cookie from risks restriction or ban.
- It is unofficial and unverified against a live session: claude.ai can change or remove the endpoints at any time, and when it breaks there is no support and no fix on any schedule.
- The session key is a full-account credential: it never leaves the gateway except inside the Cookie header, but anyone with dashboard access can read the stored connection, so treat it like a password.
- Usage is estimated locally (characters per token); claude.ai reports no counts on this lane.

If you enable it anyway: paste the `sessionKey` cookie from a logged-in claude.ai browser into the connection's API-key field, keep the toggle on, and point your client's model at `claude-web`.

## Frequently Asked Questions (FAQ)

<details>
<summary><strong>1. Does 9RouterX support Claude Code, Cursor, Codex, and Cline?</strong></summary>

Yes. 9RouterX exposes Anthropic-compatible (`http://localhost:20128`), OpenAI-compatible (`http://localhost:20128/v1`), Responses API, and Ollama-compatible endpoints, alongside built-in MCP Server and A2A protocol support. You can connect Claude Code, Cursor, OpenAI Codex CLI, Cline, Roo Code, OpenCode, OpenClaw (use explicit IPv4 `http://127.0.0.1:20128`), Antigravity, and Gemini CLI directly to port `20128`.
</details>

<details>
<summary><strong>2. Is 9RouterX free, and how do the 24 keyless models work?</strong></summary>

Yes. 9RouterX is free and open-source software (MIT License) that runs locally as a single binary with no proxy markup fees. It ships with **<!-- GEN:PROVIDERCOUNT freeAll -->34<!-- /GEN --> free-tier providers** and **248 cataloged free models across 36 providers**, including **24 cataloged keyless models** (`antigravity`, `opencode`, `blackbox`) without a user-supplied upstream API key. Antigravity (for example `antigravity/gemini-3.7-flash-medium`) requires a Google OAuth login once, which is free. Use `oc/big-pickle` from OpenCode for a zero-login test. Create a local 9RouterX gateway key in Settings → API Keys → Create Key (`http://localhost:20128`, default password `123456`) before making API requests; 0 keys returns `401 Authentication required`.
</details>

<details>
<summary><strong>3. How is 9RouterX different from LiteLLM or OpenRouter?</strong></summary>

* **Versus OpenRouter:** OpenRouter is a cloud intermediary that adds per-token markup fees and routes prompts through external third-party servers. 9RouterX runs **100% locally on your machine** as a single binary with zero markup.
* **Versus LiteLLM:** LiteLLM requires a Python environment; advanced budget/usage tracking needs external databases (PostgreSQL/Redis). **9RouterX** compiles a full AI gateway, **Svelte 5 dashboard**, **20–40% Token Saver suite (with 50+ per-tool filters)**, **semantic cache**, **per-key USD budget cutoffs checked at request start**, and **provider-specific authentication forms** into a **single zero-dependency Go binary** backed by an embedded SQLite file.
</details>

<details>
<summary><strong>4. How does the 20–40% Token Saver work without breaking code generation?</strong></summary>

AI coding agents consume massive context windows when reading CLI outputs, directory listings, and tool responses. 9RouterX's `tokensaver` and `compression` modules apply RTK compression, **50+ ready-made per-tool output filters** (`git diff`, `grep`, `docker logs`, `npm install`, `terraform plan`), GCF tabular headroom compression, context squeezing, output style directives (`Caveman`, `Ponytail`, `ADHD`), and anti-slop rules to tool outputs in transit—cutting 20–40% of token volume while repairing malformed JSON tool calls automatically.
</details>

<details>
<summary><strong>5. Can I migrate from 9Router or 9router-go without losing my keys and combos?</strong></summary>

Yes. 9RouterX shares the exact same default port (`20128`) and SQLite database schema (`DATA_DIR/db/data.sqlite`). Run `9routerx service take-over` (or stop your existing 9Router instance and start 9RouterX)—all providers, OAuth connections, combos, client API keys, and usage statistics carry over immediately without migration scripts.
</details>

<details>
<summary><strong>6. What happens when my primary AI subscription hits a rate limit?</strong></summary>

When a request hits a `429` rate limit, quota exhaustion, or upstream `5xx` error, 9RouterX's `resilience` circuit breaker and `modelhealth` probes automatically cascade the request through your configured combo tier (**Subscription OAuth → Cheap API Key → Free-Tier / Keyless**) without dropping your active coding session.
</details>

<details>
<summary><strong>7. How do I run 9RouterX in the background or update to the latest release?</strong></summary>

Use the built-in daemon commands: `9routerx start`, `9routerx status`, `9routerx restart`, `9routerx logs -n 100`, and `9routerx stop`. To update, run `npm install -g 9routerx@latest`, `9routerx update`, or pull the latest GHCR container (`ghcr.io/yanstd888/9routerx:latest`).
</details>

## Contributing

Issues and pull requests use templates, so pick the right one rather than opening a blank report:

- **Bug report** — something behaves incorrectly. Include a reproduction, `9routerx version`, OS, and the log excerpt around the failure.
- **Feature request** — the problem you cannot solve today, the surface it touches, and whether upstream already has it.
- **Question** — configuration and usage help.

PRs follow the checks CI runs: `go vet ./...`, `go test -count=1 ./...`, `make test-integration`, plus `cd web && bun test` and `make vet-svelte` for anything touching the dashboard. Before writing code, skim [`AGENTS.md`](AGENTS.md) — provider isolation and the Go/Svelte conventions keep reviews fast.

## Docs

**Full documentation, searchable:** [**9routerx Wiki**](https://yanstd888.github.io/9routerx/) — installation, usage, API reference, dashboard, providers, governance, architecture, database, CLI, and a screenshot gallery. Every page supports `?q=` search and `#section` deep links.

Repository references:

- [`ARCHITECTURE.md`](ARCHITECTURE.md) — routing, providers, runtime layout
- [`DATABASE.md`](DATABASE.md) — SQLite schema & operator contract
- [`CHANGELOG.md`](CHANGELOG.md) — release history. Work merged since the last tag lives in `.changes/` fragments and is folded in at the next release; the dashboard's changelog modal shows both. Check what you are running with `9routerx version`.

## Acknowledgments

| Repository | Stars | Contribution to 9RouterX |
|---|---|---|
| [decolua/9router](https://github.com/decolua/9router) | ![stars](https://img.shields.io/github/stars/decolua/9router?style=social) | the original Next.js gateway & dashboard that 9RouterX preserves compatibility with (same SQLite data dir, no-migration upgrade path) |
| [luqman-v1/9router-go](https://github.com/luqman-v1/9router-go) | ![stars](https://img.shields.io/github/stars/luqman-v1/9router-go?style=social) | the Go + Svelte port 9RouterX continues from; gateway core, provider lanes, CLI |
| [serenhope/9router](https://github.com/serenhope/9router) | ![stars](https://img.shields.io/github/stars/serenhope/9router?style=social) | per-key governance, token-saver suite, security log, web executors, dashboard features ported into 9RouterX |
| [cheaper-inference/OmniRoute](https://github.com/cheaper-inference/OmniRoute) | ![stars](https://img.shields.io/github/stars/cheaper-inference/OmniRoute?style=social) | SSRF dispatch guard + PII masking, routing strategies, circuit breaker, multi-target webhooks, Tremor-style data-viz patterns |
| [router-for-me/CLIProxyAPI](https://github.com/router-for-me/CLIProxyAPI) | ![stars](https://img.shields.io/github/stars/router-for-me/CLIProxyAPI?style=social) | Kiro `external_idp` auth JSON importer (port of their auth format, see `internal/handlers/oauth/kiro_external_idp.go`) |
| [rtk-ai/rtk](https://github.com/rtk-ai/rtk) | ![stars](https://img.shields.io/github/stars/rtk-ai/rtk?style=social) | the request-side token-compression concept behind the RTK Token Saver (20-40% input savings) |
| [headroomlabs-ai/headroom](https://github.com/headroomlabs-ai/headroom) | ![stars](https://img.shields.io/github/stars/headroomlabs-ai/headroom?style=social) | GCF/tabular compression concepts ported into `internal/compression/gcf` and `internal/headroom` |
| [JuliusBrussee/caveman](https://github.com/JuliusBrussee/caveman) | ![stars](https://img.shields.io/github/stars/JuliusBrussee/caveman?style=social) | the terse-output style directive (Caveman mode, `internal/tokensaver/caveman.go`) |
| [DietrichGebert/ponytail](https://github.com/DietrichGebert/ponytail) | ![stars](https://img.shields.io/github/stars/DietrichGebert/ponytail?style=social) | the structured-output style directive (Ponytail mode) |
| [ayghri/i-have-adhd](https://github.com/ayghri/i-have-adhd) | ![stars](https://img.shields.io/github/stars/ayghri/i-have-adhd?style=social) | the scannable/bulleted output style (ADHD mode) |
| [miqdadbadjuber/anti-slop](https://github.com/miqdadbadjuber/anti-slop) | ![stars](https://img.shields.io/github/stars/miqdadbadjuber/anti-slop?style=social) | antislop injector rules bundled in `internal/tokensaver/antislop` with their original text and attribution preserved |
| [huntabyte/shadcn-svelte](https://github.com/huntabyte/shadcn-svelte) | ![stars](https://img.shields.io/github/stars/huntabyte/shadcn-svelte?style=social) | the dashboard component layer (chart, table, tooltip, separator via bits-ui + layerchart) |

9RouterX stands on this lineage — thank you to the upstream maintainers and their communities for the gateway, the port, and the ideas this project builds on. The complete three-layer ownership inventory (inherited / ported / original) lives in [ATTRIBUTION.md](ATTRIBUTION.md), and everyone whose commit landed in the repository is credited on the wiki's [**Owner & Contributors**](https://yanstd888.github.io/9routerx/team.html) page.

## Star History

[![Star History Chart](https://api.star-history.com/chart?repos=yanstd888/9routerx&type=date&legend=top-left)](https://www.star-history.com/?repos=yanstd888%2F9routerx&type=date&legend=top-left)

## License

MIT License — see [`LICENSE`](https://github.com/yanstd888/9routerx/blob/main/LICENSE).

---

## Attribution

**9RouterX — all source code port and modification by [yanstd888](https://github.com/yanstd888/) — [t.me/aiclub_id](https://t.me/aiclub_id).**

This repository is an independent Go + Svelte port of [9Router](https://github.com/decolua/9router) (MIT); all port and modification work is by Muhammad Dian (yanstd888). Re-hosting this code without the LICENSE copyright notice and this attribution violates the MIT license terms. For the full file-by-file ownership breakdown, see [`ATTRIBUTION.md`](ATTRIBUTION.md).
