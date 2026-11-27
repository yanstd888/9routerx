### Added
- **Keenable web search and web fetch provider (`keenable`).** `/v1/search` with `model: keenable/search` calls `POST https://api.keenable.ai/v1/search` (`max_results` capped at the API's limit of 50), and `/v1/web/fetch` with `model: keenable/fetch` calls `POST https://api.keenable.ai/v1/fetch` and returns the page as Markdown. Requests and answers are adapted in `internal/handlers/media/keenable_search.go`, the same way as the AI/ML API Toolbox lane.

  Both ids are listed by `/v1/models/web` from the registry entry. No key is needed: with no connection the lanes use Keenable's keyless paths (`/v1/search/public`, `/v1/fetch/public`, 1,000 requests per hour per IP). A connection with an API key moves the calls to the keyed paths, which only lift the rate limit. Every request names the app with `X-Keenable-Title: 9RouterX`; the key is attached only when the target is exactly `https://api.keenable.ai`, redirects are not followed, and the caller's own headers are not relayed upstream.

  Nothing changes for existing connections: no default, combo or fallback was touched.

### Verification

- `go test -count=1 -run Keenable ./internal/providers/ ./internal/handlers/media/`: registry entry, keyless and keyed paths, request/response mapping for search and fetch, per-page fetch errors, exact-host check, no redirect follow, no caller-header relay, keyless 429 status, `HandleSearch` and `HandleWebFetch` dispatch with no connection, bare `keenable` with a stored key.
- `cd web && bun test src/lib/providers.test.ts`: catalog wiring.
