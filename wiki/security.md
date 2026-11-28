---
title: Security
order: 8
section: Using the Gateway
---

# Security

9routerx holds provider credentials, client API keys, and the prompts that pass
through it. This page covers every control that protects them.

## Dashboard authentication

| Access | Requirement |
|---|---|
| Public | `/health`, dashboard HTML/assets, `/login`, OAuth callbacks |
| Proxy routes (`/v1/*`, `/api/chat`) | Active client API key (`Authorization: Bearer` / `X-API-Key`) |
| Dashboard management APIs | Operator session, local CLI token, or a dashboard access token with a permitted scope; engine keys are forbidden |
| Read-only metadata (`/api/providers/catalog`, `/api/models/caps`) | An active, unexpired engine API key is also accepted |
| Shutdown, update, DB import | Session, local CLI token, or dashboard token with admin permission; never an engine API key |
| DB export with login required | Additional handler check requires a valid session, local CLI token, or verified `x-9r-password`; an admin dashboard token alone is insufficient |
| Vault rotation and automatic-backup restore/delete | Dashboard `write` or `admin` token can reach these routes; do not treat `write` as non-destructive |
| Console log APIs | Full session, local CLI token, or permitted dashboard-token scope; engine API keys are rejected |

### Dashboard access tokens

Dashboard tokens are separate from client/engine keys. `GET /api/dashboard-tokens`
lists token metadata; `POST` accepts a name and scope (`admin`, `write`, or
`read`). Creation shows the plaintext `9rdt_` token once. Send it in the
`x-9r-access-token` header, not as an assumed Bearer token. Do not put it in URLs
or logs.

`read` and `write` permissions are restricted by HTTP method and explicit path
exclusions. Shutdown, update, and DB import are privileged. DB export adds a
handler-level password/session/CLI check when login is required; an admin
access token alone does not satisfy it. Vault rotation and automatic-backup
restore/delete are still in scope for `write`.
Do not equate `write` with safe or non-destructive access. These scopes do not
replace engine-key model/budget policies. Tokens have no expiry option and
remain usable until revoked with `DELETE /api/dashboard-tokens/{id}`. Minting
and revocation invalidate the local cache; a separate process can retain a
cached decision for up to 30 seconds.

**First login.** On localhost the initial password is `123456`; change it in
Settings → Profile. On a remote host the default password is blocked (CVE-2026-56679
— a fresh install with a known default can be taken over publicly), so set
`INITIAL_PASSWORD` or connect through an SSH port-forward first.

## Client IP and rate-limit bucketing

Locality never trusts the request Host header. Provider-node exceptions use the
direct loopback socket peer; default-password checks use the trusted client-IP
resolver below. A reverse proxy without trusted end-client information still
looks like its socket peer, so configure that trust explicitly before exposure.

A forwarded client-IP header is only read when the TCP peer is inside the trusted
set. Otherwise a client could rotate `X-Forwarded-For` and give every login
attempt its own rate-limit bucket.

| Variable | Effect |
|---|---|
| `TRUST_PROXY=true` | Trusts loopback and the RFC1918/link-local ranges |
| `TRUST_CLOUDFLARE=true` | Trusts Cloudflare's forwarded headers |
| `TRUSTED_PROXIES` | Comma-separated CIDRs or IPs that **replace** the defaults |

Behind an untrusted proxy, every request is bucketed by the proxy's own socket
address, so all its users share one login bucket: five failed attempts lock them
all out. `TRUSTED_PROXIES` separates them again.

## Credential vault

Provider credentials are sealed with AES-256-GCM envelope encryption
(`internal/vault/vault.go`). The master key is the root of trust — losing it
loses every stored credential, so the dashboard exposes status and rotation
through `/api/vault/status` and `/api/vault/rotate` rather than leaving it to a
file edit.

Credentials are decrypted in memory only when a dispatch needs them; the database
never holds a plaintext copy after migration.

## API key hashing

Client API keys are stored as Argon2id verifiers with a SHA-256 lookup index
(`internal/keikey/apikey.go`). The plaintext key is shown once at creation.
An auth cache keeps the 64 MiB-per-verification cost off the request path, so a
busy gateway does not spend its CPU re-verifying the same key.

Keys issued while `#199` kept the plaintext are still revealed to a dashboard
session so their clients can be migrated; rotating one replaces that plaintext
with a verifier for good. Newly issued keys cannot be revealed at all — the
dashboard disables the reveal control and the only recovery is rotation.

## Guardrails

PII and prompt-injection detection runs before a request leaves and on the
response stream (`internal/guardrails/`). Layers resolve global → provider →
model → chain → API key, most-specific wins, and actions are
`log_only` / `warn` / `mask` / `block`. Policy CRUD and the audit log live behind
`/api/guardrails/policies` and `/api/guardrails/logs`.

A `json_schema` custom pattern carries a JSON Schema document in place of a
regex. It is enforced on inbound JSON bodies and buffered responses; a request
that violates it is refused with the offending field named in the 400 response.
A schema-only policy is active without any other detector, while streaming
responses stay unblocked because bytes are already on the wire before the whole
answer exists to validate.

> **Guardrails and Persona pull in opposite directions.** Persona can instruct a
> model to ignore its own rules; guardrails inspect what leaves your gateway for
> exactly that. Turning guardrails off does not stop the provider from seeing
> the same request — it only stops *you* from seeing it first. If an account
> matters, read the warning on the
> [Advanced Features](advanced-features.html) page before using Persona to
> bypass a model's limits.

## Reversible PII vault

`chat.pii_vault` accepts `off` (default) or `on`. This is an in-memory,
session-scoped placeholder map, separate from encrypted credential storage.
It recognizes a subset of emails, public IP addresses, NIK, card numbers, and
IBAN patterns on inbound Chat/Messages paths; it does not cover the Responses
path or every kind of sensitive data.

Restoration is best effort and requires the exact placeholder to survive the
model response. JSON transformations and placeholders split across SSE chunks
are not universally restored. Excess matches or collisions can pass through
raw after 256 mappings per session. Session eviction is pressure-based; the
30-minute TTL is not a guaranteed deletion timer. Do not treat this feature as
an anonymization guarantee or compliance boundary.

## SSRF protection

Requests the gateway makes on your behalf — provider validation, proxy tests,
web fetch — are checked against SSRF rules before dispatch
(`internal/handlerutil/ssrf.go`), so a custom node cannot be pointed at a
link-local or metadata address and have the gateway fetch it.

## Netguard and fetch gate

`internal/netguard` and `internal/fetchgate` gate outbound fetches, so a
provider URL or a model-catalog sync cannot reach an address it should not.

## Security Log and Audit Log

Two sidebar pages record what happened, at two levels:

- **Security Log** (`/api/security-log`, `internal/securitylog/`) — per-event
  security records: login successes and failures, default-password rotation
  attempts, lockouts, and classified anomalies. Each entry carries severity,
  actor, IP, method, path, and status, so an incident review starts here rather
  than in the raw request log.
- **Audit Log** (`/api/audit`, `internal/db/auditlog.go`) records dashboard
  actions for operational review. This is not a comprehensive tamper-evident
  ledger: ordinary writes are not fully wired to the internal hash-chain
  helpers. There is no public verification endpoint or independent anchor
  that proves completeness. Protect the database and retain independent logs
  when stronger audit evidence is required.

## TLS transport boundaries

TLS fingerprinting support lives in `internal/tlsstealth`. The global switch is
not consumed by every outbound transport, and provider-specific fingerprints
are configured independently. A profile's advertised browser version can also
differ from the underlying library version. Do not assume one toggle changes
all requests.

Some transports retry certificate failures with verification disabled unless
`STRICT_SSL=true` or `STRICT_SSL=1`. Set strict verification for deployments
that must reject such failures, and test the transport actually used. TLS
fingerprinting is not a security certification or a substitute for certificate
verification.

## Session and CLI tokens

Dashboard sessions live in `internal/auth/session.go`; the local CLI token that
lets `9routerx` talk to its own daemon lives in `internal/auth/clitoken.go`.
The CLI token is what makes destructive operations work from a terminal without a
browser session.

## Further reading

- Security panel UI: `web/src/components/security/SecurityPanel.svelte`
- Security log classification: `internal/securitylog/classify.go`
- Audit log: `internal/db/auditlog.go`
- Vault internals: `internal/vault/vault.go`, `internal/db/vault.go`
- Guardrails: `internal/guardrails/`
