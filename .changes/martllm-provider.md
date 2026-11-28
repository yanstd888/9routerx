### Added
- **MartLLM provider (`martllm`).** An OpenAI-compatible API at `https://api.martllm.com/v1/chat/completions`, Bearer key, routed through the shared OpenAI executor like SumoPod and AI/ML API. It shows up in the dashboard's API-key providers with eleven built-in chat models (`anthropic/claude-sonnet-5.5`, `google/gemini-3.8-flash`, `deepseek/deepseek-v4-flash`, `z-ai/glm-5.3`, …). **Check Latest Models** reads `https://api.martllm.com/v1/models`, which is public, so the list loads before a key is added.

  Nothing changes for existing connections: no default, combo or fallback was touched.

### Verification

- `go test -count=1 ./internal/providers/ ./internal/proxy/executor/ ./internal/handlers/dashboard/`: registry entry, transport, executor registration, catalogue and models-list URL (`TestAggregatorProviders/martllm`).
- `cd web && bun test src/lib/providers.test.ts`: catalog wiring and artwork.
