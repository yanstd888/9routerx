// 9RouterX — native executor for Conol's cookie-authenticated session chat API.
// OmniRoute 632d84f5: open-sse/executors/conol-web.ts and
// open-sse/services/conol{Auth,Models,SessionModel}.ts.
// Image upload and persisted session bindings are not fabricated.
package executor

import (
	"bytes"
	"fmt"
	"net/http"
	"strings"
	"time"

	"9routerx/proxy/internal/proxy"
	"9routerx/proxy/internal/translator"
)

const (
	conolDefaultBaseURL = "https://conol.ai/api/sessions"
	conolWebOrigin       = "https://conol.ai"
	conolUserAgent       = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 Chrome/120.0.0.0 Safari/537.36"
)

func ForwardConolWeb(w http.ResponseWriter, req *Request) error {
	if req.Config == nil {
		return proxy.UpstreamFailure(http.StatusBadGateway, "Conol provider config missing")
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

	cookie := conolCookie(req.APIKey)
	if cookie == "" {
		return proxy.UpstreamFailure(http.StatusUnauthorized, "Conol session cookie is required")
	}
	prompt := conolPrompt(body)
	if prompt == "" {
		return proxy.UpstreamFailure(http.StatusBadRequest, "No user message found")
	}
	model, effort := conolModel(req.ModelName, body)
	base := strings.TrimRight(req.Config.BaseURL, "/")
	if base == "" {
		base = conolDefaultBaseURL
	}
	ctx, err := conolGuard(ctx, base)
	if err != nil {
		return err
	}
	client := req.Client
	if client == nil {
		client = http.DefaultClient
	}
	sessionID, err := conolCreateSession(ctx, client, base, cookie)
	if err != nil {
		return proxy.UpstreamFailure(http.StatusBadGateway, err.Error())
	}
	if err := conolPinModel(ctx, client, base, cookie, sessionID, model, effort); err != nil {
		return proxy.UpstreamFailure(http.StatusBadGateway, err.Error())
	}
	if err := conolSubmit(ctx, client, base, cookie, sessionID, prompt); err != nil {
		return proxy.UpstreamFailure(http.StatusBadGateway, err.Error())
	}
	text, err := conolPoll(ctx, client, base, cookie, sessionID)
	if err != nil {
		return proxy.UpstreamFailure(http.StatusBadGateway, err.Error())
	}
	if text == "" {
		return proxy.UpstreamFailure(http.StatusBadGateway, "Conol returned no assistant response")
	}
	id := fmt.Sprintf("chatcmpl-conol-%s-%x", sessionID, time.Now().UnixNano())
	created := time.Now().Unix()
	if req.IsStream {
		return execSSEStream(w, strings.NewReader(conolFrames(text, id, model, created)), req)
	}
	return jsonResponse(req.Ctx, w, bytes.NewReader(conolCompletion(text, id, model, created)), req.TranslateResp, req.ResponseBuf)
}
