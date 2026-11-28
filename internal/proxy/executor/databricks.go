// 9RouterX — native executor for Databricks Model Serving endpoints.
// OmniRoute 632d84f5: open-sse/executors/default.ts routes databricks through
// normalizeDatabricksChatUrl, which appends /chat/completions to the workspace
// serving-endpoints host. The pinned provider baseUrl is a placeholder
// (adb-0000000000000000.0) that no real workspace answers, so the connection
// must carry the workspace host and the executor refuses the placeholder
// before any payload is built.
package executor

import (
	"net/http"
	"strings"

	"9routerx/proxy/internal/proxy"
)

// databricksPlaceholderHost is the pinned registry placeholder workspace. It
// mirrors the upstream dashboard example host and never serves a real account.
const databricksPlaceholderHost = "adb-0000000000000000.0.azuredatabricks.net"

// normalizeDatabricksChatURL mirrors the pinned normalizeDatabricksChatUrl:
// trim, then append /chat/completions unless the base already ends with it.
func normalizeDatabricksChatURL(base string) string {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if base == "" {
		return ""
	}
	if strings.HasSuffix(base, "/chat/completions") {
		return base
	}
	return base + "/chat/completions"
}

// ForwardDatabricks resolves the connection Base URL into the workspace chat
// endpoint, refusing the pinned placeholder and any missing URL with an
// actionable 400 so a configuration error cannot trip provider cooldown or
// circuit-breaker logic. With a real workspace URL present it delegates to the
// shared OpenAI forwarder.
func ForwardDatabricks(w http.ResponseWriter, req *Request) error {
	base := ""
	if req.ConnData != nil {
		base, _ = req.ConnData["baseUrl"].(string)
	}
	base = normalizeDatabricksChatURL(base)
	if base == "" || strings.Contains(base, databricksPlaceholderHost) {
		return proxy.UpstreamFailure(http.StatusBadRequest, "Databricks requires a Base URL: set it to your workspace serving-endpoints host, e.g. https://adb-1234567890123456.7.azuredatabricks.net/serving-endpoints.")
	}
	if req.Config == nil {
		return proxy.UpstreamFailure(http.StatusBadGateway, "Databricks provider config missing")
	}
	cfg := *req.Config
	cfg.BaseURL = base
	req.Config = &cfg
	return ForwardOpenAI(w, req)
}
