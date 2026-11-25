// 9RouterX — native executor for Poe's documented OpenAI-compatible API.
// OmniRoute 632d84f5: config/providers/registry/poe/index.ts and
// open-sse/executors/default.ts. `poe` is the API-key lane; the web-cookie
// GraphQL transport stays on the separate, unported `poe-web` id.
package executor

import (
	"context"
	json "encoding/json/v2"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"

	"9routerx/proxy/internal/handlerutil"
	"9routerx/proxy/internal/providers"
	"9routerx/proxy/internal/proxy"
	"9routerx/proxy/internal/translator"
)

const (
	poeDefaultBaseURL = "https://api.poe.com/v1"
	poeChatPath       = "/chat/completions"
	poeResponsesPath  = "/responses"
	poeMessagesPath   = "/messages"
)

// ForwardPoe selects the lane from the caller's format and the model's pinned
// target format: Responses clients and Responses-target models use /responses,
// official Claude ids use /messages, every other request uses /chat/completions.
func ForwardPoe(w http.ResponseWriter, req *Request) error {
	if req.Config == nil {
		return proxy.UpstreamFailure(http.StatusBadGateway, "Poe provider config missing")
	}
	if strings.TrimSpace(req.APIKey) == "" {
		return proxy.UpstreamFailure(http.StatusUnauthorized, "Poe API key required")
	}
	ctx := reqCtx(req)
	body := req.Body
	model := poeBodyModel(body)
	if model == "" {
		model = req.ModelName
	}
	model = stripOpenCodePrefix(model, "poe")
	if model == "" {
		return proxy.UpstreamFailure(http.StatusBadRequest, "Poe request is missing a model")
	}
	req.Ctx = ctx

	lane := poeLane(ctx, model)
	switch {
	case lane == "responses" && !translator.IsResponsesClient(ctx):
		transformed, _, err := buildResponsesBody(body)
		if err != nil {
			return proxy.UpstreamFailure(http.StatusBadRequest, "translate Poe Responses request: "+err.Error())
		}
		body = transformed
	case lane != "responses" && translator.IsResponsesClient(ctx):
		converted, err := translator.ResponsesToChatRequest(body)
		if err != nil {
			return proxy.UpstreamFailure(http.StatusBadRequest, "translate Poe Chat request: "+err.Error())
		}
		body = converted.Body
		ctx = translator.WithResponsesBridge(ctx)
		if len(converted.CustomToolNames) > 0 {
			ctx = translator.WithCustomToolNames(ctx, converted.CustomToolNames)
		}
		req.Ctx = ctx
	}
	body = withJSONFields(body, map[string]any{"model": model})

	base, err := poeBaseURL(req)
	if err != nil {
		return err
	}
	cfg := *req.Config
	cfg.StaticHeaders = nil
	path := poeChatPath
	switch lane {
	case "responses":
		path = poeResponsesPath
	case "messages":
		path = poeMessagesPath
		if !req.ClaudeClient {
			body = EnsureClaudeMessagesFor(body, "poe", model)
		}
		// The pinned default executor supplies this header on Anthropic-shaped
		// messages lanes unless the operator already set one.
		cfg.StaticHeaders = map[string]string{"anthropic-version": "2023-06-01"}
	}
	cfg.BaseURL = base + path

	if lane == "chat" {
		req.Config = &cfg
		req.Body = body
		req.ModelName = model
		return ForwardOpenAI(w, req)
	}

	resp, err := proxy.ForwardOpenAI(ctx, req.Client, &cfg, req.APIKey, body, req.IsStream)
	if err != nil {
		return fmt.Errorf("ForwardPoe upstream: %w", err)
	}
	defer resp.Body.Close()
	forwardUpstreamResponseHeaders(w, resp.Header)
	if lane == "messages" {
		if req.IsStream {
			return handleClaudeMessagesStream(w, req, resp.Body)
		}
		return handleClaudeMessagesNonStream(w, req, resp.Body)
	}
	return handleCodexStream(w, req, resp.Body)
}

func poeBodyModel(body []byte) string {
	var parsed struct {
		Model string `json:"model"`
	}
	if err := jsonUnmarshalClientBody(body, &parsed); err != nil {
		return ""
	}
	return strings.TrimSpace(parsed.Model)
}

func jsonUnmarshalClientBody(body []byte, target any) error {
	return json.Unmarshal(body, target, handlerutil.ClientBody)
}

func poeLane(ctx context.Context, model string) string {
	if translator.IsResponsesClient(ctx) {
		return "responses"
	}
	if formats, ok := providers.ProviderModelFormats["poe"]; ok {
		if mf, ok := formats[model]; ok && mf.TargetFormat == providers.FormatClaude {
			return "messages"
		}
	}
	return "chat"
}

// poeBaseURL normalizes a bare host, /v1, or a full lane URL onto the pinned
// /v1 root before the lane path is appended.
func poeBaseURL(req *Request) (string, error) {
	raw := ""
	if req.ConnData != nil {
		raw, _ = req.ConnData["baseUrl"].(string)
	}
	if strings.TrimSpace(raw) == "" && req.Config != nil {
		raw = req.Config.BaseURL
	}
	if strings.TrimSpace(raw) == "" {
		raw = os.Getenv("POE_BASE_URL")
	}
	if strings.TrimSpace(raw) == "" {
		raw = poeDefaultBaseURL
	}
	base := strings.TrimRight(strings.TrimSpace(raw), "/")
	for _, suffix := range []string{poeChatPath, poeResponsesPath, poeMessagesPath} {
		base = strings.TrimSuffix(base, suffix)
	}
	if !strings.HasSuffix(strings.ToLower(base), "/v1") {
		base += "/v1"
	}
	u, err := url.Parse(base)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return "", proxy.UpstreamFailure(http.StatusBadRequest, "Poe base URL must be an http(s) host")
	}
	return base, nil
}
