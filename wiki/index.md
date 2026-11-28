---
title: Home
order: 1
section: Getting Started
---

# 9RouterX

**Never stop coding. Save 20-40% tokens with RTK + auto-fallback to FREE & cheap AI models.**

9routerx is a single Go binary that serves both the proxy APIs and the dashboard — no Node.js at runtime. This page is the entry point to the whole wiki.

## Start here

| You want to… | Open |
|---|---|
| Install and run | [Installation](installation.html) |
| Use it from a CLI tool (Claude Code, Codex, Cursor…) | [Client Setup](client-setup.html) |
| Browse the main endpoint groups | [API](api.html) |
| Get to know the dashboard | [Dashboard](dashboard.html) |
| Connect AI providers | [Providers](providers.html) |
| Build fallback chains | [Combos](combos.html) |
| Cut token spend | [Token Saver](token-saver.html) |
| See what RTK actually saved | [Usage & Analytics](usage-analytics.html) |
| Limit who spends what | [Governance](governance.html) |
| Protect credentials and prompts | [Security](security.html) |
| Understand routing internals | [Architecture](architecture.html) |
| See how data is stored | [Database](database.html) |
| Run it in production | [Operations](operations.html) |
| Configure everything in the dashboard | [Settings](settings.html) |
| Compare with upstream 9Router | [Parity](parity.html) |
| See what was ported, and from where | [Advanced Features](advanced-features.html) |
| Every CLI command and env var | [CLI](cli.html) |
| Contribute | [Contributing](contributing.html) |
| Browse screenshots | [Screenshots](screenshots.html) |

## Running in 30 seconds

```bash
9routerx
# Dashboard: http://localhost:20128 (initial password: 123456)
```

Dashboard → Providers → connect a supported provider, then create a local
engine API key in Settings → API Keys before pointing your client at
`http://127.0.0.1:20128/v1`. OpenCode Free is cataloged as a no-upstream-login
lane; Kiro still requires a supported credential flow.

## Which version am I on?

```bash
9routerx version
```

Some features on this site arrived in a specific release. Where that matters, the
page says so — for example the [Advanced Features](advanced-features.html) page
lists which release each port landed in, and flags the ones that are still on a
branch rather than in a release you can download.

## What is in the box

| Area | What you get |
|---|---|
| Proxy APIs | OpenAI Chat, Claude Messages, Responses, embeddings, images, video, audio, search, batches |
| Routing | [Combo strategies](combos.html), including `lkgp`, budgets and family warnings; arena uses a limited direct path |
| Token savers | RTK and compression/output styles; opt-in [tool-loop watch](token-saver.md#tool-loop-watch) with protocol-dependent coverage |
| Governance | Per-key rate limits, budgets, plans, allowlists, and a [fair-share scheduling hint](governance.md#fair-share-scheduling), not a reserved quota |
| Security | Credential vault, key hashing, dashboard-token scopes, reversible PII placeholders, guardrails, and audit records with documented limits |
| Providers | <!-- GEN:PROVIDERCOUNT total -->344<!-- /GEN --> built-in entries (<!-- GEN:PROVIDERCOUNT apikey -->265<!-- /GEN --> API category, <!-- GEN:PROVIDERCOUNT oauth -->34<!-- /GEN --> OAuth category, <!-- GEN:PROVIDERCOUNT freeAll -->34<!-- /GEN --> free, <!-- GEN:PROVIDERCOUNT webCookie -->11<!-- /GEN --> web-cookie); auth modes vary. 248 static free models across 36 providers, including 24 keyless; custom endpoint templates are separate |
| Agents | MCP server endpoint, A2A endpoints, multi-target webhooks, team cost centers |
| Operations | Docker, tunnels, updates, automatic snapshots, sensitive remote JSON backups, supplied proxy harvest, and local doctor/onboarding status |

## Search

The search box at the top left searches **titles, headings, and endpoints** across
every wiki page. Results are also shareable: the URL becomes `?q=...` (query) and
`#...` (a specific section inside a page), so whoever you send it to opens
exactly the part you meant.
