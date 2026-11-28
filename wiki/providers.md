---
title: Providers
order: 6
section: Using the Gateway
---

# Providers

9RouterX has <!-- GEN:PROVIDERCOUNT total -->344<!-- /GEN --> built-in provider entries: <!-- GEN:PROVIDERCOUNT apikey -->265<!-- /GEN --> in the API-key category, <!-- GEN:PROVIDERCOUNT oauth -->34<!-- /GEN --> in
OAuth, <!-- GEN:PROVIDERCOUNT freeAll -->34<!-- /GEN --> in the free categories, and <!-- GEN:PROVIDERCOUNT webCookie -->11<!-- /GEN --> in web-cookie. These are catalog groups,
not counts of guaranteed authentication flows; for example, `muse` is grouped
under OAuth but uses API-key authentication. The grouped catalog also includes
two custom endpoint templates, which are not built-in providers.

The static catalog contains 248 free models across 36 providers, including 24
keyless models. Antigravity requires one Google OAuth login; OpenCode offers
zero-login access. Catalog inclusion does not guarantee current upstream
availability or quota. Provider-specific credentials and dispatch paths remain
separate.

## Connecting

Dashboard → Providers → pick a provider → Connect.

OpenCode Free is cataloged as a no-upstream-auth lane. Kiro still needs a
supported credential/login flow; free credits do not mean no signup or login.

## OAuth flows

Different providers authenticate differently, and the dashboard offers the right
one per provider:

| Flow | Providers |
|---|---|
| Social / authorize-exchange | Kiro, Antigravity, Cline, Trae, Windsurf, Zed, Kimchi, Xiaomi MiMo |
| Device code | Any provider exposing a device flow |
| Local proxy callback | Codex |
| Bulk import | Codex, Grok CLI |
| API key paste | Kiro |
| Cookie / PAT paste | iFlow (cookie), GitLab (PAT) |
| Session polling | Freebuff — initiate, poll, inspect, and switch between sessions |

Built-in public client credentials exist for antigravity, gemini-cli, xai, codex,
iflow and others, so most operators never set an OAuth client ID. Override per
provider with `{PROVIDER}_OAUTH_CLIENT_ID` / `{PROVIDER}_OAUTH_CLIENT_SECRET`
when you run your own OAuth app.

Tokens are refreshed automatically, and a request that fails with `401` is retried
once after a refresh.

## Browser-based login

Browser capture and import are management operations, not OAuth flows. Use
`POST /api/providers/{id}/browser-login` to launch a browser, or
`POST /api/providers/{id}/import-chrome` to read an existing Chromium profile.
Support depends on the actual route:

| Provider ID | Browser login | Import | Profile and reset |
|---|---|---|---|
| `grok-web` | Generic, captures `sso` | Domain cookies | Persistent; reset supported |
| `perplexity-web` | Generic, captures `__Secure-next-auth.session-token` | Domain cookies | Persistent; reset supported |
| `muse-web` | Generic Baidu Yiyan/ERNIE, captures `BDUSS` on `baidu.com` | Domain cookies | Persistent; reset supported |
| `chatgpt-web` | Generic runtime lane | Domain cookies | Persistent; reset supported; not a seventh catalog entry |
| `gemini-web` | Dedicated, captures `__Secure-1PSID` and `__Secure-1PSIDTS` | Dedicated | Temporary; reset returns `400` |
| `deepseek-web` | Dedicated, captures user token and cookies | Dedicated | Temporary, including renewal; reset does not clear the profile used by this flow |
| `claude-web` | Dedicated, captures `sessionKey` | Not configured (`404`) | Temporary; reset returns `400` |

`muse-web` is a historical compatibility ID for **Baidu Yiyan/ERNIE**, not Meta
Muse. `chatgpt-web` has runtime configuration and an executor, but no
`ProviderRegistry` entry in the dashboard catalog. Claude Web is unofficial,
opt-in, and unverified against a live session.

### Persistent profiles and Xvfb

Generic persistent profiles live under `<DATA_DIR>/browser-profiles/<provider>/`.
The runner requests `0700` permissions on Unix; Windows permissions follow the
host ACLs. The built-in database export and remote backup do not include these
directories. A manual filesystem backup can include them, so exclude them
explicitly. Treat the profile as an account credential.

`POST /api/providers/{id}/reset-profile` removes a configured persistent
profile and returns `{provider,status:"reset"}`. Gemini and Claude have no such
persistent profile. DeepSeek has a reference config, but its dedicated route
still uses a temporary directory; resetting the generic directory does not
reset that browser session.

On Linux without `DISPLAY` or `WAYLAND_DISPLAY`, the generic persistent runner
can use an installed Xvfb. It provides an invisible display for a profile that
already has a usable session. It does not supply a visible login UI or solve
MFA, CAPTCHA, or cookie decryption. Dedicated Gemini, DeepSeek, and Claude
flows do not use this fallback. Repeated or concurrent Xvfb-backed sessions are
not guaranteed by the current lifecycle implementation.

### Import and saving credentials

Import supports Chromium-family browsers (Chrome, Chromium, Brave, Edge,
Vivaldi, and Arc), not Firefox or Safari. An optional `{profile}` selector can
choose a browser profile or path; the first `Default` profile may belong to a
different account. Close the source browser to reduce locks and concurrent
session changes; the returned warning is advisory, not a guarantee against
logout or session invalidation.

Windows App-Bound encryption is not bypassed. Supported legacy DPAPI/v10/v11
cookies can still be unavailable on a particular host; unavailable decryption
or keyring support can return `501`. DeepSeek local-storage scanning reads
`.ldb`/`.log` files rather than reconstructing every LevelDB state.

Generic login returns `{provider,cookie,tokens,userAgent}`; generic import
returns `{provider,cookie,tokens,warning}`. Gemini/Claude login returns
`{cookie}`, while DeepSeek returns `{userToken,cookie}`. Dedicated imports add
a `warning`. Capture/import reads credentials and leaves connection saving to
the caller. A `200` response does not prove live upstream authentication;
generic import can return partially decrypted cookies without the required
token. Do not log or share these payloads.

`POST /api/connections/{id}/renew-deepseek` is different: it persists renewed
credentials and returns `{renewed,userToken,cookie}`. Browser capture does not
enable an executor. Web executors use `ENABLE_WEB_EXECUTORS` / `enableWebExecutors`;
Claude also has the separate `CLAUDE_WEB_ENABLED` / `claudeWebEnabled` gate.

## Custom providers

One shared dialog adds an OpenAI- or Anthropic-compatible endpoint. One custom
provider is one `providerNodes` row, and a node can have many connections; a
connection points at its node through `provider = node.id`.

| Field | Purpose |
|---|---|
| Name | Display name |
| Prefix | Wire syntax prefix (`provider/model`) |
| Base URL | Upstream endpoint |
| Headers | Your own request headers |

Validate an endpoint before saving with `POST /api/providers/validate`.

## Per-provider limits

Each custom node can carry a capacity budget:

| # | Limit | Unit | Meaning |
|---|---|---|---|
| L1 | RPM | requests/minute | Request rate this endpoint tolerates |
| L2 | TPM | tokens/minute | Input (prompt) token rate this endpoint tolerates |
| L3 | Concurrency | parallel requests | Number of simultaneous requests |
| L4 | Lifetime quota | total tokens | Cumulative cap before the node is declared exhausted |
| L5 | Queue timeout | ms | How long a request may wait in the FIFO queue for L1/L3 |
| L6 | Contract expiry | RFC3339 UTC date | Validity period of the node's contract |

`0` means unlimited; an empty string means no expiry — the same convention as
API keys.

**Behaviour when exceeded**: congestion (RPM/TPM/concurrency) marks the candidate
as full for that attempt only, and the router tries the next candidate; `429` is
returned only when every candidate is exhausted. Node state (quota exhausted,
contract ended) skips all of that node's connections and returns a typed `403`.

**A congestion burst writes no cooldown.** One request over a limit must not
globally park a healthy node — only a debug trace is written. The `lastError` row
the dashboard's node-health table reads is still written, so you can see the
cause without the node being locked.

Set limits with `PUT /api/provider-nodes/{id}/limits` and lift a spent quota with
`POST /api/provider-nodes/{id}/quota/reset` (it moves the watermark, it does not
delete history).

## Models

Browse a provider's models with `GET /api/providers/{id}/models`, export keys
with `GET /api/providers/{id}/export-keys`, and remove many at once with
`DELETE /api/providers/{id}/models`.

**Custom models** are stored in the `customModels` key-value scope, not as columns
on `providerNodes`. Add one at a time or paste many (separated by spaces, commas
or newlines) — one request, atomic, per-row results.

**Aliases** (`/api/models/alias`) let a client write a short name for a long
model id. **Deprecations** (`/api/models/deprecations`) surface models upstream
has retired, and **auto-disable** (`/api/models/auto-disable`) turns off the ones
that stopped working.

## Important notes

- **No cross-provider aliasing.** `antigravity` is not `opencode`; a model
  belonging to one provider is never redirected to another.
- **No substring-based model rewriting.** Routing decisions are driven by catalog
  registration, database aliases, custom node prefixes, or standard
  `provider/model` wire syntax — never by ad-hoc string inspection.
- **The Import from /models button** only appears for providers that genuinely
  support dynamic catalog discovery upstream.
