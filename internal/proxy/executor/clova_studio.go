// 9RouterX — native executor for Naver CLOVA Studio Chat Completions v3.
// OmniRoute 632d84f5: open-sse/executors/clova-studio.ts and the paired
// openai-to-clova / clova-to-openai translators. CLOVA is a proprietary format
// and always streams upstream, even for non-streaming clients.
package executor

import (
	"bytes"
	json "encoding/json/v2"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"9routerx/proxy/internal/proxy"
	"9routerx/proxy/internal/translator"
)

// ForwardClovaStudio translates OpenAI chat traffic to the pinned v3 endpoint
// and folds the native token/result stream back into OpenAI frames.
func ForwardClovaStudio(w http.ResponseWriter, req *Request) error {
	if req.Config == nil {
		return proxy.UpstreamFailure(http.StatusBadGateway, "CLOVA Studio provider config missing")
	}
	key := strings.TrimSpace(req.APIKey)
	if key == "" {
		return proxy.UpstreamFailure(http.StatusUnauthorized, "CLOVA Studio API key required")
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

	model := strings.TrimSpace(req.ModelName)
	if model == "" {
		var parsed struct {
			Model string `json:"model"`
		}
		if json.Unmarshal(body, &parsed) == nil {
			model = strings.TrimSpace(parsed.Model)
		}
	}
	model = strings.TrimPrefix(strings.TrimPrefix(model, "clova-studio/"), "clova/")
	if model == "" {
		return proxy.UpstreamFailure(http.StatusBadRequest, "CLOVA Studio request is missing a model")
	}
	payload, err := clovaPayload(model, body)
	if err != nil {
		return proxy.UpstreamFailure(http.StatusBadRequest, err.Error())
	}

	cfg := *req.Config
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/") + "/" + url.PathEscape(model)
	cfg.AuthHeader = "Authorization"
	cfg.AuthScheme = "bearer"
	cfg.StaticHeaders = map[string]string{"Accept": "text/event-stream"}
	resp, err := proxy.ForwardOpenAI(ctx, req.Client, &cfg, key, payload, true)
	if err != nil {
		return fmt.Errorf("ForwardClovaStudio upstream: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := proxy.ReadBody(resp)
	if err != nil {
		return fmt.Errorf("read CLOVA Studio response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return proxy.UpstreamFailure(resp.StatusCode, clovaUpstreamError(raw, resp.StatusCode))
	}
	id := fmt.Sprintf("chatcmpl-clova-%x", time.Now().UnixNano())
	created := time.Now().Unix()
	if req.IsStream {
		return execSSEStream(w, strings.NewReader(clovaConvert(raw, id, model, created).frames), req)
	}
	completion, err := clovaCompletion(raw, id, model, created)
	if err != nil {
		return err
	}
	return jsonResponse(req.Ctx, w, bytes.NewReader(completion), req.TranslateResp, req.ResponseBuf)
}

func clovaUpstreamError(raw []byte, status int) string {
	var parsed struct {
		Status struct {
			Message string `json:"message"`
		} `json:"status"`
		Message string `json:"message"`
	}
	if json.Unmarshal(raw, &parsed) == nil {
		if parsed.Status.Message != "" {
			return parsed.Status.Message
		}
		if parsed.Message != "" {
			return parsed.Message
		}
	}
	message := strings.TrimSpace(string(raw))
	if len(message) > 200 {
		message = message[:200]
	}
	if message == "" {
		return fmt.Sprintf("CLOVA Studio failed with status %d", status)
	}
	return message
}
