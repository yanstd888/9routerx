// 9RouterX — derived from the work of https://github.com/yanstd888/ — port and modifications by the maintainers — https://t.me/aiclub_id
package executor

import (
	"net/http"

	"9routerx/proxy/internal/proxy"
)

// ForwardNotionWeb serves the Notion AI web lane: an OpenAI-format
// passthrough over notion.so's internal /api/v3 with the browser cookie
// (token_v2 and friends) as the credential (xrouter registry transport,
// pinned 7fd5698). UNOFFICIAL/EXPERIMENTAL: Notion's terms do not sanction
// automated AI lanes, so the operator risks an account ban. The transport
// itself is the generic OpenAI forwarder — this wrapper exists only to put
// the lane behind the web-executor policy gate, like every other web-cookie
// session lane.
func ForwardNotionWeb(w http.ResponseWriter, req *Request) error {
	if !CheckWebExecutorAllowed(w, "notion-web") {
		return &proxy.UpstreamError{
			StatusCode: http.StatusForbidden,
			Body:       []byte(`{"error":{"code":"web_executors_disabled","message":"notion-web is disabled by policy","type":"policy_error"}}`),
		}
	}
	return ForwardOpenAI(w, req)
}
