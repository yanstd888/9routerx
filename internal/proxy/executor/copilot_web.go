// 9RouterX — derived from the work of https://github.com/yanstd888/ — port and modifications by the maintainers — https://t.me/aiclub_id
package executor

import (
	"net/http"

	"9routerx/proxy/internal/proxy"
)

// ForwardCopilotWeb serves the Microsoft Copilot web lane: an OpenAI-format
// passthrough over copilot.microsoft.com's /c/api with the browser cookie as
// the credential (xrouter registry transport, pinned 7fd5698). The transport
// itself is the generic OpenAI forwarder — this wrapper exists only to put
// the lane behind the web-executor policy gate, like every other web-cookie
// session lane.
func ForwardCopilotWeb(w http.ResponseWriter, req *Request) error {
	if !CheckWebExecutorAllowed(w, "copilot-web") {
		return &proxy.UpstreamError{
			StatusCode: http.StatusForbidden,
			Body:       []byte(`{"error":{"code":"web_executors_disabled","message":"copilot-web is disabled by policy","type":"policy_error"}}`),
		}
	}
	return ForwardOpenAI(w, req)
}
