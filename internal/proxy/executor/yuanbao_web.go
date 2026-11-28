// 9RouterX — derived from the work of https://github.com/yanstd888/ — port and modifications by the maintainers — https://t.me/aiclub_id
package executor

import (
	"net/http"

	"9routerx/proxy/internal/proxy"
)

// ForwardYuanbaoWeb serves the Tencent Yuanbao web lane: an OpenAI-format
// passthrough over yuanbao.tencent.com's /api with the browser cookie (it
// must include hy_user + hy_token) as the credential (xrouter registry
// transport, pinned 7fd5698). The transport itself is the generic OpenAI
// forwarder — this wrapper exists only to put the lane behind the
// web-executor policy gate, like every other web-cookie session lane.
func ForwardYuanbaoWeb(w http.ResponseWriter, req *Request) error {
	if !CheckWebExecutorAllowed(w, "yuanbao-web") {
		return &proxy.UpstreamError{
			StatusCode: http.StatusForbidden,
			Body:       []byte(`{"error":{"code":"web_executors_disabled","message":"yuanbao-web is disabled by policy","type":"policy_error"}}`),
		}
	}
	return ForwardOpenAI(w, req)
}
