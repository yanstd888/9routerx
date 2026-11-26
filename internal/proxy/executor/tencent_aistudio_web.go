// 9RouterX — native executor for the free Tencent AI Studio web chat lane.
// OmniRoute 632d84f5: open-sse/executors/tencent-aistudio-web.ts. The session
// cookie goes on the /api/chat/<model> hop; no API key or OAuth is involved.
package executor

import (
	"bytes"
	"context"
	json "encoding/json/v2"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"9routerx/proxy/internal/proxy"
	"9routerx/proxy/internal/translator"
)

var tencentAistudioModels = map[string]string{
	"hy3-g":          "HunyuanDefault",
	"hunyuan-default": "HunyuanDefault",
	"hunyuan-3d":     "Hunyuan3D",
}

func tencentAistudioGuard(ctx context.Context, raw string) (context.Context, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return ctx, proxy.UpstreamFailure(http.StatusBadRequest, "Tencent AI Studio endpoint has an invalid URL")
	}
	allowLocal := proxy.AllowPrivateProviderURLs() || proxy.IsPrivateNetworkingAllowed() || proxy.IsLoopbackAllowed(ctx)
	if !allowLocal && parsed.Scheme != "https" {
		return ctx, proxy.UpstreamFailure(http.StatusBadRequest, "Tencent AI Studio endpoint must use HTTPS")
	}
	if err := proxy.AssertDispatchURLAllowed(raw, allowLocal); err != nil {
		return ctx, proxy.UpstreamFailure(http.StatusBadRequest, "Tencent AI Studio endpoint blocked by SSRF guard")
	}
	if allowLocal {
		ctx = proxy.WithAllowLoopback(ctx)
	}
	return ctx, nil
}

func readAllBounded(r io.Reader) []byte {
	data, _ := io.ReadAll(io.LimitReader(r, 10<<20))
	return data
}

func ForwardTencentAistudioWeb(w http.ResponseWriter, req *Request) error {
	if req.Config == nil {
		return proxy.UpstreamFailure(http.StatusBadGateway, "Tencent AI Studio provider config missing")
	}
	cookie := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(req.APIKey), "Cookie:"))
	if cookie == "" {
		return proxy.UpstreamFailure(http.StatusUnauthorized, "Tencent AI Studio Cookie is required")
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

	model := strings.TrimPrefix(strings.TrimSpace(req.ModelName), "tencent-aistudio-web/")
	if model == "" {
		var parsed struct {
			Model string `json:"model"`
		}
		if json.Unmarshal(body, &parsed) == nil {
			model = strings.TrimSpace(parsed.Model)
		}
	}
	mapped, ok := tencentAistudioModels[model]
	if !ok {
		mapped = "HunyuanDefault"
	}
	var parsedBody struct {
		Messages []any `json:"messages"`
	}
	if json.Unmarshal(body, &parsedBody) != nil {
		return proxy.UpstreamFailure(http.StatusBadRequest, "invalid Tencent AI Studio request")
	}
	payload, err := json.Marshal(map[string]any{"model": mapped, "messages": parsedBody.Messages})
	if err != nil {
		return proxy.UpstreamFailure(http.StatusInternalServerError, "encode Tencent AI Studio request")
	}
	base := strings.TrimRight(req.Config.BaseURL, "/")
	if base == "" {
		base = "https://aistudio.tencent.ai"
	}
	ctx, err = tencentAistudioGuard(ctx, base)
	if err != nil {
		return err
	}
	client := req.Client
	if client == nil {
		client = http.DefaultClient
	}
	response, err := proxy.DoRequest(ctx, client, http.MethodPost, base+"/api/chat/"+mapped, map[string]string{
		"Content-Type": "application/json", "Cookie": cookie,
		"Origin": base, "Referer": base + "/",
		"User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
	}, payload)
	if err != nil {
		return proxy.UpstreamFailure(http.StatusBadGateway, "Tencent AI Studio request failed")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		detail, _ := io.ReadAll(io.LimitReader(response.Body, 1024))
		return proxy.UpstreamFailure(response.StatusCode, fmt.Sprintf("Tencent AI Studio failed with status %d: %s", response.StatusCode, strings.TrimSpace(string(detail))))
	}
	if req.IsStream {
		return execSSEStream(w, response.Body, req)
	}
	return jsonResponse(req.Ctx, w, bytes.NewReader(readAllBounded(response.Body)), req.TranslateResp, req.ResponseBuf)
}
