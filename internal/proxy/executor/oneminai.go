// 9RouterX — native executor for 1min.ai's proprietary Chat with AI API.
// OmniRoute 632d84f5: open-sse/executors/oneminai.ts. The wire takes one
// promptObject.prompt string and an API-KEY header, not OpenAI messages.
package executor

import (
	"bytes"
	json "encoding/json/v2"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"9routerx/proxy/internal/proxy"
	"9routerx/proxy/internal/translator"
)

const oneminaiDefaultBaseURL = "https://api.1min.ai/api/chat-with-ai"

func ForwardOneminai(w http.ResponseWriter, req *Request) error {
	if req.Config == nil {
		return proxy.UpstreamFailure(http.StatusBadGateway, "1min.ai provider config missing")
	}
	key := strings.TrimSpace(req.APIKey)
	if key == "" {
		return proxy.UpstreamFailure(http.StatusUnauthorized, "1min.ai API key required")
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
	model := strings.TrimPrefix(strings.TrimSpace(req.ModelName), "oneminai/")
	if model == "" {
		model = oneminaiBodyModel(body)
	}
	model = strings.TrimPrefix(model, "oneminai/")
	if model == "" {
		return proxy.UpstreamFailure(http.StatusBadRequest, "1min.ai request is missing a model")
	}
	prompt := oneminaiPrompt(body)
	if prompt == "" {
		return proxy.UpstreamFailure(http.StatusBadRequest, "1min.ai requires at least one message")
	}
	payload, err := json.Marshal(map[string]any{
		"type": "UNIFY_CHAT_WITH_AI", "model": model,
		"promptObject": map[string]any{"prompt": prompt},
	})
	if err != nil {
		return proxy.UpstreamFailure(http.StatusInternalServerError, "encode 1min.ai request")
	}
	base := strings.TrimRight(req.Config.BaseURL, "/")
	if base == "" {
		base = oneminaiDefaultBaseURL
	}
	if req.IsStream {
		base += "?isStreaming=true"
	}
	ctx, err = oneminaiGuard(ctx, base)
	if err != nil {
		return err
	}
	client := req.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := proxy.DoRequest(ctx, client, http.MethodPost, base, map[string]string{
		"Content-Type": "application/json", "API-KEY": key,
	}, payload)
	if err != nil {
		return proxy.UpstreamFailure(http.StatusBadGateway, "1min.ai request failed")
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		detail, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return proxy.UpstreamFailure(resp.StatusCode, oneminaiError(string(detail), resp.StatusCode))
	}
	id := fmt.Sprintf("chatcmpl-oneminai-%x", time.Now().UnixNano())
	created := time.Now().Unix()
	if req.IsStream {
		raw, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
		if err != nil {
			return proxy.UpstreamFailure(http.StatusBadGateway, "read 1min.ai stream")
		}
		return execSSEStream(w, strings.NewReader(oneminaiFrames(raw, id, model, created)), req)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return proxy.UpstreamFailure(http.StatusBadGateway, "read 1min.ai response")
	}
	completion, err := oneminaiCompletion(raw, id, model, created)
	if err != nil {
		return err
	}
	return jsonResponse(req.Ctx, w, bytes.NewReader(completion), req.TranslateResp, req.ResponseBuf)
}
