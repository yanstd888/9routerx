### Fixed
- CodeBuddy CN connections now render their quota rows in the usage panel. `fetchProviderUsage` had a case for `codebuddy-intl` but none for `codebuddy-cn`, so every CN connection fell through to the default branch and answered `{"quotas":{}}`. The panel showed "Account active. No quota limits tracked." while the intl lane showed its Monthly / Bonus Pack rows.
- `fetchCodeBuddyIntlUsage` became `fetchCodeBuddyUsage(ctx, provider, ...)`: both deployments answer the same billing meter contract, so the envelope, the refill/bonus split and the row names are parsed once, and only the origin plus the client identity are selected by provider. The intl entry point is kept as a wrapper.
- The CN identity is read from `providers.KnownProviders["codebuddy-cn"].StaticHeaders` rather than hardcoded, so the billing read presents the same CLI build the chat lane sends. `X-Domain` is derived from the billing origin because the executor overrides the registry's `www.codebuddy.cn` per connection.
- Error messages in the intl path said "CodeBuddy CN …" regardless of the lane; they now name the provider that answered.

Verification: `go build ./...`, `go test ./internal/handlers/dashboard/` (7 new cases pinning dispatch, CN identity headers, unchanged intl identity, error naming, the empty-account read, the blank-token path and that the shared registry map is not mutated).
