// 9RouterX — native executor for Cerebrium deployment endpoints.
// OmniRoute 632d84f5: open-sse/executors/cerebrium.ts. Cerebrium has no shared
// endpoint; each deployment exposes its own OpenAI-compatible /run URL, which
// the connection must save as its Base URL.
package executor

import (
	"net/http"
	"strings"

	"9routerx/proxy/internal/proxy"
)

// ForwardCerebrium refuses a request without a deployment URL before any
// payload is built or sent, so a configuration error cannot trip provider
// cooldown or circuit-breaker logic. With a URL present it delegates to the
// shared OpenAI forwarder.
func ForwardCerebrium(w http.ResponseWriter, req *Request) error {
	base := ""
	if req.ConnData != nil {
		base, _ = req.ConnData["baseUrl"].(string)
	}
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if base == "" {
		return proxy.UpstreamFailure(http.StatusBadRequest, "Cerebrium requires a Base URL: set it to your deployment's /run endpoint.")
	}
	if req.Config == nil {
		return proxy.UpstreamFailure(http.StatusBadGateway, "Cerebrium provider config missing")
	}
	cfg := *req.Config
	cfg.BaseURL = base
	req.Config = &cfg
	return ForwardOpenAI(w, req)
}
