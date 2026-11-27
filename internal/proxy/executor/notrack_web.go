// 9RouterX — native executor for NoTrack's session-cookie dispatch API.
// OmniRoute 632d84f5: open-sse/executors/notrack-web.ts and its support files.
// The public JSONL/SSE stream is folded into OpenAI chat completions.
package executor

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"9routerx/proxy/internal/proxy"
	"9routerx/proxy/internal/translator"
)

const (
	notrackDefaultBaseURL = "https://notrack.ai"
	notrackMaxInputChars  = 3800
	notrackBodyMax        = 10 << 20
	notrackUserAgent      = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/150.0.0.0 Safari/537.36 Edg/150.0.0.0"
)

// ForwardNotrackWeb posts one flattened prompt to /api/dispatch and converts
// the upstream notrack events into OpenAI frames or one completion.
func ForwardNotrackWeb(w http.ResponseWriter, req *Request) error {
	if req.Config == nil {
		return proxy.UpstreamFailure(http.StatusBadGateway, "NoTrack provider config missing")
	}
	ctx := reqCtx(req)
	body := req.Body
	if req.ClaudeClient {
		converted, err := translator.TranslateClaudeToOpenAI(body)
		if err != nil {
			return proxy.UpstreamFailure(http.StatusBadRequest, "translate Claude request: "+err.Error())
		}
		body = converted
	}
	if translator.IsResponsesClient(ctx) {
		converted, err := translator.ResponsesToChatRequest(body)
		if err != nil {
			return proxy.UpstreamFailure(http.StatusBadRequest, "translate Responses request: "+err.Error())
		}
		body = converted.Body
		ctx = translator.WithResponsesBridge(ctx)
		if len(converted.CustomToolNames) > 0 {
			ctx = translator.WithCustomToolNames(ctx, converted.CustomToolNames)
		}
	}
	req.Ctx = ctx

	cookie := notrackCookie(req.APIKey)
	if cookie == "" {
		return proxy.UpstreamFailure(http.StatusUnauthorized, "NoTrack session cookie is required")
	}
	payload, err := notrackPayload(body)
	if err != nil {
		return proxy.UpstreamFailure(http.StatusBadRequest, err.Error())
	}
	base := notrackBaseURL(req)
	ctx, err = notrackGuard(ctx, base)
	if err != nil {
		return err
	}
	client := req.Client
	if client == nil {
		client = http.DefaultClient
	}
	response, err := proxy.DoRequest(ctx, client, http.MethodPost, base+"/api/dispatch", map[string]string{
		"content-type": "application/json", "accept": "*/*",
		"accept-language": "zh-TW,zh;q=0.9,en-US;q=0.8,en;q=0.7",
		"origin": notrackDefaultBaseURL, "referer": notrackDefaultBaseURL + "/zh-CN/chat",
		"user-agent": notrackUserAgent, "cookie": cookie,
	}, payload)
	if err != nil {
		return proxy.UpstreamFailure(http.StatusBadGateway, "NoTrack request failed")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return proxy.UpstreamFailure(notrackStatus(response.StatusCode), notrackErrorText(response.StatusCode))
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, notrackBodyMax))
	if err != nil {
		return proxy.UpstreamFailure(http.StatusBadGateway, "read NoTrack response")
	}
	id := fmt.Sprintf("chatcmpl-notrack-%x", time.Now().UnixNano())
	model := notrackModel(req.ModelName)
	created := time.Now().Unix()
	if req.IsStream {
		return execSSEStream(w, strings.NewReader(notrackFrames(raw, id, model, created)), req)
	}
	completion, err := notrackCompletion(raw, id, model, created)
	if err != nil {
		return err
	}
	return jsonResponse(req.Ctx, w, bytes.NewReader(completion), req.TranslateResp, req.ResponseBuf)
}

func notrackStatus(status int) int {
	if status >= 500 {
		return http.StatusBadGateway
	}
	return status
}

func notrackErrorText(status int) string {
	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return "NoTrack auth failed — refresh uid/si_usr_id/si_ses_id cookies"
	case status == http.StatusTooManyRequests:
		return "NoTrack rate limited; retry briefly"
	case status >= 500:
		return "NoTrack upstream unavailable"
	default:
		return fmt.Sprintf("NoTrack returned HTTP %d", status)
	}
}
