// 9RouterX — native executor for SYNTX.ai's create-chat/generate/stream API.
// OmniRoute 632d84f5: open-sse/executors/syntx.ts, syntxExecute.ts and
// syntxChat.ts. Image upload and session rollover are not fabricated.
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

const syntxDefaultBaseURL = "https://api.syntx.ai"

func ForwardSyntx(w http.ResponseWriter, req *Request) error {
	if req.Config == nil {
		return proxy.UpstreamFailure(http.StatusBadGateway, "SYNTX provider config missing")
	}
	token := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(req.APIKey), "Bearer "))
	if token == "" {
		return proxy.UpstreamFailure(http.StatusUnauthorized, "SYNTX Bearer JWT is required")
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
	}
	req.Ctx = ctx

	model, ok := syntxModel(req.ModelName, body)
	if !ok {
		return proxy.UpstreamFailure(http.StatusBadRequest, "SYNTX model not found or invalid")
	}
	prompt, title, thinking := syntxPrompt(body, model.Thinking)
	base := syntxBaseURL(req)
	ctx, err := syntxGuard(ctx, base)
	if err != nil {
		return err
	}
	client := req.Client
	if client == nil {
		client = http.DefaultClient
	}
	chatID, err := syntxCreateChat(ctx, client, base, token, title)
	if err != nil {
		return proxy.UpstreamFailure(http.StatusBadGateway, err.Error())
	}
	streamURL, err := syntxGenerate(ctx, client, base, token, chatID, model, prompt, thinking)
	if err != nil {
		return proxy.UpstreamFailure(http.StatusBadGateway, err.Error())
	}
	streamURL, err = syntxResolveStreamURL(streamURL, base)
	if err != nil {
		return err
	}
	ctx, err = syntxGuard(ctx, streamURL)
	if err != nil {
		return err
	}
	response, err := proxy.DoRequest(ctx, client, http.MethodGet, streamURL, syntxAuthHeaders(token), nil)
	if err != nil {
		return proxy.UpstreamFailure(http.StatusBadGateway, "SYNTX stream request failed")
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return proxy.UpstreamFailure(response.StatusCode, "SYNTX stream failed")
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, 10<<20))
	if err != nil {
		return proxy.UpstreamFailure(http.StatusBadGateway, "read SYNTX stream")
	}
	text, usage := syntxStream(raw)
	id := fmt.Sprintf("chatcmpl-syntx-%x", time.Now().UnixNano())
	created := time.Now().Unix()
	if req.IsStream {
		return execSSEStream(w, strings.NewReader(syntxFrames(text, id, model.ID, created, usage)), req)
	}
	return jsonResponse(req.Ctx, w, bytes.NewReader(syntxCompletion(text, id, model.ID, created, usage)), req.TranslateResp, req.ResponseBuf)
}
