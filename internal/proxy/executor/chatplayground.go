// 9RouterX — native executor for ChatPlayground's Clerk-authenticated chat API.
// OmniRoute 632d84f5: open-sse/executors/chatplayground.ts and
// open-sse/services/chatplaygroundAuth.ts.
package executor

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"9routerx/proxy/internal/proxy"
	"9routerx/proxy/internal/translator"
)

const (
	chatplaygroundDefaultBaseURL = "https://app.chatplayground.ai/api/chat"
	chatplaygroundWebOrigin      = "https://web.chatplayground.ai"
	chatplaygroundClerkHost      = "https://clerk.chatplayground.ai"
	chatplaygroundMaxMessage     = 15000
)

type chatplaygroundJWT struct {
	token  string
	expiry time.Time
}

var (
	chatplaygroundJWTMu sync.Mutex
	chatplaygroundJWTs  = map[string]chatplaygroundJWT{}
)

func ForwardChatplayground(w http.ResponseWriter, req *Request) error {
	if req.Config == nil {
		return proxy.UpstreamFailure(http.StatusBadGateway, "ChatPlayground provider config missing")
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

	modelData, ok := chatplaygroundModel(req.ModelName, body)
	if !ok {
		return proxy.UpstreamFailure(http.StatusBadRequest, "ChatPlayground model not found or invalid")
	}
	token, err := chatplaygroundToken(ctx, req)
	if err != nil {
		return proxy.UpstreamFailure(http.StatusUnauthorized, err.Error())
	}
	payload, err := chatplaygroundPayload(body, modelData)
	if err != nil {
		return proxy.UpstreamFailure(http.StatusBadRequest, err.Error())
	}
	base := strings.TrimRight(req.Config.BaseURL, "/")
	if strings.TrimSpace(base) == "" {
		base = chatplaygroundDefaultBaseURL
	}
	ctx, err = chatplaygroundGuard(ctx, base)
	if err != nil {
		return err
	}
	client := req.Client
	if client == nil {
		client = http.DefaultClient
	}
	target := base + "/" + modelData.Endpoint
	headers := map[string]string{
		"accept": "*/*", "content-type": "application/json",
		"origin": chatplaygroundWebOrigin, "referer": chatplaygroundWebOrigin + "/",
		"user-agent": chatplaygroundUserAgent, "authorization": "Bearer " + token,
	}
	response, err := proxy.DoRequest(ctx, client, http.MethodPost, target, headers, payload)
	if err != nil {
		return proxy.UpstreamFailure(http.StatusBadGateway, "ChatPlayground request failed")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		detail, _ := io.ReadAll(io.LimitReader(response.Body, 1024))
		return proxy.UpstreamFailure(response.StatusCode, chatplaygroundError(response.StatusCode, string(detail)))
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, 10<<20))
	if err != nil {
		return proxy.UpstreamFailure(http.StatusBadGateway, "read ChatPlayground response")
	}
	id := fmt.Sprintf("chatcmpl-cpl-%x", time.Now().UnixNano())
	created := time.Now().Unix()
	content := chatplaygroundStripID(string(raw), true)
	if req.IsStream {
		return execSSEStream(w, strings.NewReader(chatplaygroundFrames(content, id, req.ModelName, created)), req)
	}
	completion := chatplaygroundCompletion(content, id, req.ModelName, created)
	return jsonResponse(req.Ctx, w, bytes.NewReader(completion), req.TranslateResp, req.ResponseBuf)
}
