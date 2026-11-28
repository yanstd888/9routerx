// 9RouterX — derived from the work of https://github.com/yanstd888/ — port and modifications by the maintainers — https://t.me/aiclub_id
package executor

import (
	"net/http"

	"9routerx/proxy/internal/proxy"
)

// ForwardZaiWeb serves the Z.ai web lane: an OpenAI-format passthrough over
// chat.z.ai's web completions endpoint with the site's Local Storage "token"
// value as the Bearer credential (xrouter registry transport, pinned
// 7fd5698). The transport itself is the generic OpenAI forwarder — this
// wrapper exists only to put the lane behind the web-executor policy gate,
// like every other web-cookie session lane. (OmniRoute additionally mints a
// per-request CAPTCHA proof through its browser transport; this lane ports
// the xrouter transport, which sends the pasted token as-is.)
func ForwardZaiWeb(w http.ResponseWriter, req *Request) error {
	if !CheckWebExecutorAllowed(w, "zai-web") {
		return &proxy.UpstreamError{
			StatusCode: http.StatusForbidden,
			Body:       []byte(`{"error":{"code":"web_executors_disabled","message":"zai-web is disabled by policy","type":"policy_error"}}`),
		}
	}
	return ForwardOpenAI(w, req)
}
