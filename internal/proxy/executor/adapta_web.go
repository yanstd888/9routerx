// 9RouterX — native executor for Adapta One's Clerk-authenticated stream API.
// OmniRoute 632d84f5: open-sse/executors/adapta-web.ts.
package executor

import (
	"bytes"
	json "encoding/json/v2"
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
	adaptaDefaultBaseURL   = "https://agent.adapta.one"
	adaptaDefaultClerkURL  = "https://clerk.agent.adapta.one"
	adaptaUserAgent        = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/149.0.0.0 Safari/537.36"
	adaptaModelID          = 14
	adaptaPublicStreamErr  = "\n\n[Erro: Adapta upstream error]"
)

var adaptaModels = map[string]bool{
	"adapta-one": true, "adapta-gpt": true, "adapta-claude": true, "adapta-gemini": true,
	"adapta-grok": true, "adapta-deepseek": true, "adapta-llama": true,
}

type adaptaJWT struct {
	token  string
	expiry time.Time
}

var (
	adaptaJWTMu sync.Mutex
	adaptaJWTs  = map[string]adaptaJWT{}
)

func ForwardAdaptaWeb(w http.ResponseWriter, req *Request) error {
	if req.Config == nil {
		return proxy.UpstreamFailure(http.StatusBadGateway, "Adapta provider config missing")
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

	model := strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(req.ModelName), "adapta-web/"), "adp-web/")
	if model == "" {
		model = adaptaBodyModel(body)
	}
	model = strings.TrimPrefix(strings.TrimPrefix(model, "adapta-web/"), "adp-web/")
	if !adaptaModels[model] {
		return proxy.UpstreamFailure(http.StatusBadRequest, "Adapta model not found or invalid")
	}
	messages, err := adaptaMessages(body)
	if err != nil {
		return proxy.UpstreamFailure(http.StatusBadRequest, err.Error())
	}
	token, err := adaptaSessionJWT(ctx, req)
	if err != nil {
		return proxy.UpstreamFailure(http.StatusUnauthorized, err.Error())
	}
	base := strings.TrimRight(req.Config.BaseURL, "/")
	if base == "" {
		base = adaptaDefaultBaseURL
	}
	ctx, err = adaptaGuard(ctx, base)
	if err != nil {
		return err
	}
	payload, _ := json.Marshal(map[string]any{"messages": messages, "aiModelId": adaptaModelID})
	client := req.Client
	if client == nil {
		client = http.DefaultClient
	}
	response, err := proxy.DoRequest(ctx, client, http.MethodPost, base+"/api/chat/stream/v1", map[string]string{
		"Authorization": "Bearer " + token, "Content-Type": "application/json",
		"Accept": "text/event-stream", "User-Agent": adaptaUserAgent,
		"Origin": base, "Referer": base + "/agentic-chat",
	}, payload)
	if err != nil {
		return proxy.UpstreamFailure(http.StatusBadGateway, "Adapta request failed")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return proxy.UpstreamFailure(response.StatusCode, "Adapta upstream error")
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, 10<<20))
	if err != nil {
		return proxy.UpstreamFailure(http.StatusBadGateway, "read Adapta stream")
	}
	text, streamErr := adaptaStream(raw)
	if streamErr {
		return proxy.UpstreamFailure(http.StatusBadGateway, "Adapta upstream stream error")
	}
	id := fmt.Sprintf("chatcmpl-adp-%x", time.Now().UnixNano())
	created := time.Now().Unix()
	if req.IsStream {
		return execSSEStream(w, strings.NewReader(adaptaFrames(text, id, model, created)), req)
	}
	return jsonResponse(req.Ctx, w, bytes.NewReader(adaptaCompletion(text, id, model, created)), req.TranslateResp, req.ResponseBuf)
}
