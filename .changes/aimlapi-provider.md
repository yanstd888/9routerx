### Added
- **AI/ML API provider (`aimlapi`, alias `aiml`).** An OpenAI-compatible aggregator at `https://api.aimlapi.com/v1/chat/completions`, Bearer key, routed through the shared OpenAI executor like Together and SumoPod. It shows up in the dashboard's API-key providers with a built-in list of eleven chat models (`openai/gpt-5.4-mini`, `anthropic/claude-sonnet-4.6`, `google/gemini-3.8-flash`, `deepseek/deepseek-v4-flash`, …). **Check Latest Models** reads `https://api.aimlapi.com/v1/models` and keeps only the `type: openai/chat-completions` rows, since the same catalogue also lists image, video, speech and Responses/Messages routes.

  The same key serves the **web search** and **web fetch** lanes through the AI/ML API Toolbox (`POST https://tools.aimlapi.com/v1/tools/run`): `/v1/search` with `model: aimlapi` runs `web/search` (`max_results` capped at the tool's limit of 20), `/v1/web/fetch` runs `litescrape/web-fetch` and returns the page as Markdown. Requests and answers are adapted in `internal/handlers/media/aimlapi_tools.go`, because the Toolbox takes `{"tool","input"}` on one endpoint rather than a vendor-shaped body.

  Key handling on the Toolbox lane: the key is attached only when the target is exactly `https://tools.aimlapi.com`, redirects are not followed, and the caller's own headers are not relayed upstream.

  Nothing changes for existing connections: no default, combo or fallback was touched.

### Verification

- `go test -count=1 -run Aimlapi ./internal/providers/ ./internal/handlers/media/`: registry entry, chat-row filter, request/response mapping for search and fetch, exact-host check, no redirect follow, no caller-header relay, upstream error text, `HandleSearch` dispatch.
- `cd web && bun test src/lib/providers.test.ts`: catalog wiring and artwork.
