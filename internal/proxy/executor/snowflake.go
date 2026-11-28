// 9RouterX — native executor for Snowflake Cortex inference.
// OmniRoute 632d84f5: open-sse/executors/default.ts routes snowflake through
// normalizeSnowflakeChatUrl (account host -> /api/v2/cortex/inference:complete)
// and buildHeaders, which classifies the credential: a "pat/" prefixed
// PROGRAMMATIC ACCESS TOKEN has its prefix stripped and advertises
// X-Snowflake-Authorization-Token-Type: PROGRAMMATIC_ACCESS_TOKEN, while any
// other token is treated as a keypair JWT. The pinned provider baseUrl is an
// {account} placeholder, so the connection must carry the account host and the
// executor refuses the placeholder before any payload is built.
package executor

import (
	"net/http"
	"strings"

	"9routerx/proxy/internal/proxy"
)

const (
	snowflakePlaceholderHost = "{account}"
	snowflakePATPrefix       = "pat/"
)

// normalizeSnowflakeChatURL mirrors the pinned normalizeSnowflakeChatUrl:
// trim trailing slashes, strip a pasted /cortex/inference:complete or /api/v2
// suffix, then always land on /api/v2/cortex/inference:complete.
func normalizeSnowflakeChatURL(base string) string {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if base == "" {
		return ""
	}
	base = strings.TrimSuffix(base, "/cortex/inference:complete")
	base = strings.TrimSuffix(base, "/api/v2")
	base = strings.TrimRight(base, "/")
	if base == "" {
		return ""
	}
	return base + "/api/v2/cortex/inference:complete"
}

// ForwardSnowflake resolves the connection Base URL into the Cortex
// inference:complete endpoint and classifies the credential, refusing the
// {account} placeholder or a missing URL/token with actionable errors so a
// configuration problem cannot trip provider cooldown or circuit-breaker
// logic. With both present it delegates to the shared OpenAI forwarder, which
// applies the token-type static header after the Bearer authorization.
func ForwardSnowflake(w http.ResponseWriter, req *Request) error {
	base := ""
	if req.ConnData != nil {
		base, _ = req.ConnData["baseUrl"].(string)
	}
	base = normalizeSnowflakeChatURL(base)
	if base == "" || strings.Contains(base, snowflakePlaceholderHost) {
		return proxy.UpstreamFailure(http.StatusBadRequest, "Snowflake requires a Base URL: set it to your account host, e.g. https://acme-org.snowflakecomputing.com.")
	}
	if req.Config == nil {
		return proxy.UpstreamFailure(http.StatusBadGateway, "Snowflake provider config missing")
	}
	token := strings.TrimSpace(req.APIKey)
	if token == "" {
		return proxy.UpstreamFailure(http.StatusUnauthorized, "Snowflake API key required: paste a programmatic access token (pat/...) or a keypair JWT.")
	}
	tokenType := "KEYPAIR_JWT"
	if strings.HasPrefix(token, snowflakePATPrefix) {
		tokenType = "PROGRAMMATIC_ACCESS_TOKEN"
		token = strings.TrimPrefix(token, snowflakePATPrefix)
	}
	cfg := *req.Config
	cfg.BaseURL = base
	cfg.StaticHeaders = map[string]string{"X-Snowflake-Authorization-Token-Type": tokenType}
	req.Config = &cfg
	req.APIKey = token
	return ForwardOpenAI(w, req)
}
