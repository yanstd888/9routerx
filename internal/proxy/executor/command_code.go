// 9RouterX — native executor for the OmniRoute 632d84f5 `command-code` lane.
// The pinned Provider API endpoint is OpenAI-shaped and Bearer-authenticated;
// it is distinct from the legacy `commandcode` CLI /alpha/generate lane.
package executor

import (
	"fmt"
	"net/http"
	"strings"

	"9routerx/proxy/internal/proxy"
	"9routerx/proxy/internal/translator"
)

const (
	commandCodeChatPath      = "/provider/v1/chat/completions"
	commandCodeMessagesPath  = "/provider/v1/messages"
	commandCodeResponsesPath = "/provider/v1/responses"
)

// ForwardCommandCode routes OpenAI-shaped models to the pinned chat endpoint
// and the six targetFormat:"claude" models to the sibling Messages endpoint.
// No CLI impersonation headers or fallback to /alpha/generate are sent.
func ForwardCommandCode(w http.ResponseWriter, req *Request) error {
	if req.Config == nil {
		return proxy.UpstreamFailure(http.StatusBadGateway, "Command Code provider config missing")
	}
	if strings.TrimSpace(req.APIKey) == "" {
		return proxy.UpstreamFailure(http.StatusUnauthorized, "Command Code API key required")
	}
	ctx := reqCtx(req)
	body := req.Body
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

	model := req.ModelName
	if model == "" {
		model = commandCodeBodyModel(body)
	}
	wire := commandCodeWireModel(model)
	claudeLane := commandCodeClaudeModel(wire)

	normalized, _, err := commandCodeNormalizeBody(body, wire, req.IsStream)
	if err != nil {
		return proxy.UpstreamFailure(http.StatusBadRequest, "invalid Command Code request: "+err.Error())
	}
	if claudeLane && !req.ClaudeClient {
		normalized = EnsureClaudeMessagesFor(normalized, "command-code", wire)
	}

	base, err := commandCodeBaseURL(req)
	if err != nil {
		return err
	}
	cfg := *req.Config
	cfg.StaticHeaders = nil
	path := commandCodeChatPath
	if claudeLane {
		path = commandCodeMessagesPath
		// Pinned buildProviderHeaders adds this only on the Messages lane.
		cfg.StaticHeaders = map[string]string{"anthropic-version": "2023-06-01"}
	}
	cfg.BaseURL = base + path

	if !claudeLane {
		req.Config = &cfg
		req.Body = normalized
		req.ModelName = wire
		return ForwardOpenAI(w, req)
	}

	resp, err := proxy.ForwardOpenAI(ctx, req.Client, &cfg, req.APIKey, normalized, req.IsStream)
	if err != nil {
		return fmt.Errorf("ForwardCommandCode upstream: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	forwardUpstreamResponseHeaders(w, resp.Header)
	if req.IsStream {
		return handleClaudeMessagesStream(w, req, resp.Body)
	}
	return handleClaudeMessagesNonStream(w, req, resp.Body)
}
