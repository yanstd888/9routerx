// 9RouterX — native executor for NLP Cloud's proprietary chatbot API.
// OmniRoute 632d84f5: open-sse/executors/nlpcloud.ts and
// config/providers/registry/nlpcloud/index.ts. The wire uses Token auth and
// input/context/history, not OpenAI chat/completions.
package executor

import (
	"bytes"
	json "encoding/json/v2"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"9routerx/proxy/internal/proxy"
	"9routerx/proxy/internal/translator"
)

const nlpcloudDefaultBaseURL = "https://api.nlpcloud.io/v1"

// ForwardNLPCloud adapts OpenAI-shaped requests to the pinned chatbot endpoint
// and folds the response back into OpenAI Chat Completions before the shared
// response bridges run.
func ForwardNLPCloud(w http.ResponseWriter, req *Request) error {
	if req.Config == nil {
		return proxy.UpstreamFailure(http.StatusBadGateway, "NLP Cloud provider config missing")
	}
	key := strings.TrimSpace(req.APIKey)
	if key == "" {
		return proxy.UpstreamFailure(http.StatusUnauthorized, "NLP Cloud API key required")
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
		model = nlpcloudBodyModel(body)
	}
	model = strings.TrimPrefix(strings.TrimPrefix(model, "nlpcloud/"), "nlpc/")
	if model == "" {
		return proxy.UpstreamFailure(http.StatusBadRequest, "NLP Cloud request is missing a model")
	}
	payload, err := nlpcloudPayload(body, model, req.IsStream)
	if err != nil {
		return proxy.UpstreamFailure(http.StatusBadRequest, err.Error())
	}

	base, err := nlpcloudBaseURL(req)
	if err != nil {
		return err
	}
	cfg := *req.Config
	cfg.BaseURL = base + "/" + url.PathEscape(model) + "/chatbot"
	cfg.AuthHeader = "Authorization"
	cfg.AuthScheme = "raw"
	cfg.StaticHeaders = nil
	// The pinned header is `Authorization: Token <key>`; the raw auth scheme
	// sends the prefixed string verbatim without echoing it into logs.
	resp, err := proxy.ForwardOpenAI(ctx, req.Client, &cfg, "Token "+key, payload, req.IsStream)
	if err != nil {
		return fmt.Errorf("ForwardNLPCloud upstream: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := proxy.ReadBody(resp)
	if err != nil {
		return fmt.Errorf("read NLP Cloud response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return proxy.UpstreamFailure(resp.StatusCode, nlpcloudErrorMessage(raw, resp.StatusCode))
	}

	id := fmt.Sprintf("chatcmpl-nlpcloud-%x", time.Now().UnixNano())
	created := time.Now().Unix()
	if req.IsStream {
		frames := nlpcloudFrames(raw, id, model, created)
		return execSSEStream(w, strings.NewReader(frames), req)
	}
	completion, err := nlpcloudCompletion(raw, id, model, created)
	if err != nil {
		return err
	}
	return jsonResponse(req.Ctx, w, bytes.NewReader(completion), req.TranslateResp, req.ResponseBuf)
}

func nlpcloudBodyModel(body []byte) string {
	var parsed struct {
		Model string `json:"model"`
	}
	if json.Unmarshal(body, &parsed) != nil {
		return ""
	}
	return strings.TrimSpace(parsed.Model)
}

func nlpcloudBaseURL(req *Request) (string, error) {
	raw := ""
	if req.ConnData != nil {
		raw, _ = req.ConnData["baseUrl"].(string)
	}
	if strings.TrimSpace(raw) == "" && req.Config != nil {
		raw = req.Config.BaseURL
	}
	if strings.TrimSpace(raw) == "" {
		raw = os.Getenv("NLPCLOUD_BASE_URL")
	}
	if strings.TrimSpace(raw) == "" {
		raw = nlpcloudDefaultBaseURL
	}
	base := strings.TrimRight(strings.TrimSpace(raw), "/")
	if marker := strings.LastIndex(base, "/chatbot"); marker >= 0 {
		if slash := strings.LastIndex(base[:marker], "/"); slash > 0 {
			base = base[:slash]
		}
	}
	u, err := url.Parse(base)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return "", proxy.UpstreamFailure(http.StatusBadRequest, "NLP Cloud base URL must be an http(s) host")
	}
	return base, nil
}

func nlpcloudErrorMessage(raw []byte, status int) string {
	var parsed struct {
		Detail string `json:"detail"`
		Error  string `json:"error"`
	}
	if json.Unmarshal(raw, &parsed) == nil {
		if parsed.Detail != "" {
			return parsed.Detail
		}
		if parsed.Error != "" {
			return parsed.Error
		}
	}
	message := strings.TrimSpace(string(raw))
	if len(message) > 200 {
		message = message[:200]
	}
	if message == "" {
		return fmt.Sprintf("NLP Cloud API failed with status %d", status)
	}
	return message
}
