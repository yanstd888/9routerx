---
title: Comparison
order: 22
section: Under the Hood
---

# Comparison

How 9RouterX relates to **9Router** (decolua — the JavaScript project it was ported from) and **OmniRoute** (cheaper-inference). Facts only; where no public benchmark exists the wording is *on paper*. Star counts checked live on Nov 14, 2026 (9Router 32,422 · OmniRoute 87,366 · 9RouterX 68) and move constantly — judge by the feature rows.

## At a glance

| Metric | 9RouterX | 9Router (decolua) | OmniRoute (cheaper-inference) |
|---|---|---|---|
| Runtime | Single native Go binary + embedded Svelte 5 dashboard | JavaScript / Node.js + Next.js | TypeScript / Node.js |
| Install | One binary (~48 MB linux-amd64), zero runtime deps | `npm i -g 9router` (6 deps, needs Node) | npm (~105 deps) / Docker / pnpm |
| Providers | 155 (91 API · 28 OAuth · 29 free · 4 cookie) | 40+ | 375–380 (150+ free) |
| Free models | 248 across 36 providers (24 keyless) | 100+ | 1,400+ models, ~1.62B free tokens/mo |
| Dashboard | Svelte 5 + Vite + Tailwind 4, embedded (1 dashboard dep) | Next.js React app | Desktop / PWA |
| Database | Embedded SQLite, drop-in schema-compatible | Same SQLite schema | SQLite |
| Usage under heavy traffic | *On paper:* compiled native process, no V8/GC overhead as queues grow — no public benchmark; measure on your workload | *On paper:* Node.js event loop; depends on Node version and warm-up | *On paper:* Node.js, same caveats |

## What 9RouterX adds over 9Router

9RouterX is a Go port of 9Router with its own layer on top: full per-key governance (daily USD cutoffs, request limits, IP whitelists, PII masking) where upstream has basic key auth; a semantic cache + prompt cache layer where upstream passes through; the Token Saver suite (20–40% RTK compression, 50+ per-tool filters, GCF headroom, tool-call repair); a catalog of 248 free models across 36 providers (24 keyless); and MCP, A2A, TLS stealth, MITM inspector, webhooks, Telegram bot and Vault modules. Both share the same SQLite schema — a 9Router database drops straight in (see [Parity](parity)).

## Where OmniRoute is bigger

OmniRoute's catalog is larger, stated plainly: ~375–380 providers (150+ free), 1,400+ models, a pooled ~1.62B free tokens/month allowance, 20 routing strategies and quota-aware fallback. That breadth is their strength. 9RouterX's difference is shape: one Go binary with zero runtime dependencies, light enough for small self-hosted machines, deeper per-key financial governance, and the Svelte 5 dashboard embedded in the same binary. Pick by what you host.

## Go + Svelte vs a Node stack

*On paper — no public benchmark; measure on your workload.* A Go binary distributes as one file per platform (linux x64/arm64, macOS x64/arm64, Windows x64) instead of `npm install` with a runtime and `node_modules`; it serves the first request right after boot instead of warming up a runtime; and `9routerx update` swaps a single file instead of a package-manager update plus restart. Baseline memory of a comparable-scope Go process typically sits lower than a Node one — actual numbers depend on traffic shape and cache sizes.
