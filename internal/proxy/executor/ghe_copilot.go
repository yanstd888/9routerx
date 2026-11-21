package executor

import (
	"bytes"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"9routerx/proxy/internal/constants"
	"9routerx/proxy/internal/providers"
	"9routerx/proxy/internal/proxy"
	"9routerx/proxy/internal/proxy/ghecopilot"
	"9routerx/proxy/internal/translator"
)

// ForwardGHECopilot selects one native lane from catalog metadata, independent
// of the client endpoint. OAuth renewal is done before this request is shaped.
func ForwardGHECopilot(w http.ResponseWriter, req *Request) error {
	var head struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(req.Body, &head); err != nil {
		return fmt.Errorf("decode GHE request: %w", err)
	}
	model := strings.TrimPrefix(head.Model, ghecopilot.Provider+"/")
	target := providers.GHECopilotTargetFormat(model, req.ConnData)
	base, err := ghecopilot.APIBase(req.ConnData)
	if err != nil {
		return fmt.Errorf("GHE API base: %w", err)
	}
	body, err := gheBody(req, model, target)
	if err != nil {
		return err
	}
	path := "/chat/completions"
	if target == providers.FormatClaude {
		path = "/v1/messages"
	}
	if target == providers.FormatOpenAIResponses {
		path = "/responses"
	}
	cfg := *req.Config
	cfg.BaseURL = base + path
	cfg.Format = target
	cfg.AuthHeader, cfg.AuthScheme = "Authorization", "bearer"
	cfg.StaticHeaders = gheHeaders(req, body, target)
	headers := providers.ApplyHeaderOverrides(cfg.StaticHeaders, cfg.HeaderOverrides)
	upstreamReq, err := http.NewRequestWithContext(reqCtx(req), http.MethodPost, cfg.BaseURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build GHE inference request: %w", err)
	}
	for key, value := range headers {
		upstreamReq.Header.Set(key, value)
	}
	upstreamReq.Header.Set("Authorization", "Bearer "+req.APIKey)
	upstreamReq.Header.Set("Accept", "text/event-stream")
	upstreamReq.Header.Del("Cookie")
	resp, err := ghecopilot.Do(reqCtx(req), req.Client, upstreamReq)
	if err != nil {
		return fmt.Errorf("GHE inference: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		return proxy.UpstreamFailure(http.StatusBadGateway, "GHE inference redirects are not allowed")
	}
	if err := upstreamStatusError(resp); err != nil {
		var ue *proxy.UpstreamError
		if errors.As(err, &ue) {
			normalizeGithubContextError(ue)
		}
		return err
	}
	forwardUpstreamResponseHeaders(w, resp.Header)
	copy := *req
	copy.Ctx = reqCtx(req)
	copy.Config = &cfg
	copy.Body = body
	copy.StartTime = req.StartTime
	if copy.StartTime.IsZero() {
		copy.StartTime = time.Now()
	}
	stream := proxy.NewStallReaderWithContext(copy.Ctx, resp.Body, cfg.ResolveStallTimeout(proxy.ResolveDefaultStallTimeout()), "ghe-copilot")
	defer stream.Close()
	_, replay, err := proxy.PeekFirstStreamEvent(copy.Ctx, stream, 0)
	if err != nil {
		return err
	}
	switch target {
	case providers.FormatClaude:
		return gheMessagesResponse(w, &copy, replay)
	case providers.FormatOpenAIResponses:
		return gheResponsesResponse(w, &copy, replay)
	default:
		if copy.IsStream {
			return execSSEStream(w, replay, &copy)
		}
		return jsonResponse(copy.Ctx, w, replay, copy.TranslateResp, copy.ResponseBuf)
	}
}

func gheBody(req *Request, model, target string) ([]byte, error) {
	body := req.Body
	var obj map[string]any
	if err := json.Unmarshal(body, &obj); err != nil {
		return nil, err
	}
	_, responses := obj["input"]
	claudeNative := req.ClaudeClient && !req.TranslateResp && target == providers.FormatClaude
	if responses && target != providers.FormatOpenAIResponses {
		converted, err := translator.ResponsesToChatRequest(body)
		if err != nil {
			return nil, fmt.Errorf("GHE Responses to Chat: %w", err)
		}
		body = converted.Body
	}
	if target == providers.FormatClaude && !claudeNative {
		body = EnsureClaudeMessagesFor(body, ghecopilot.Provider, model)
	}
	if target == providers.FormatOpenAIResponses && !responses {
		converted, _, err := buildResponsesBody(body)
		if err != nil {
			return nil, fmt.Errorf("GHE Chat to Responses: %w", err)
		}
		body = converted
	}
	obj = nil
	if err := json.Unmarshal(body, &obj); err != nil {
		return nil, err
	}
	obj["model"], obj["stream"] = model, true
	if target == providers.FormatOpenAI {
		gheChatQuirks(obj)
	}
	if target == providers.FormatOpenAIResponses {
		delete(obj, "messages")
		delete(obj, "stream_options")
		if obj["max_output_tokens"] == nil {
			obj["max_output_tokens"] = firstNonNil(obj["max_completion_tokens"], obj["max_tokens"])
		}
		if obj["max_output_tokens"] == nil {
			delete(obj, "max_output_tokens")
		}
		delete(obj, "max_completion_tokens")
		delete(obj, "max_tokens")
	}
	return json.Marshal(obj)
}

func gheChatQuirks(obj map[string]any) {
	messages, _ := obj["messages"].([]any)
	for _, raw := range messages {
		msg, _ := raw.(map[string]any)
		if msg["role"] == "assistant" {
			delete(msg, "reasoning_text")
			delete(msg, "reasoning_content")
		}
	}
	for len(messages) > 1 {
		last, _ := messages[len(messages)-1].(map[string]any)
		if last["role"] != "assistant" {
			break
		}
		messages = messages[:len(messages)-1]
	}
	if messages != nil {
		obj["messages"] = messages
	}
	if tools, ok := obj["tools"].([]any); ok && len(tools) > 128 {
		obj["tools"] = tools[:128]
	}
}

func gheHeaders(req *Request, body []byte, target string) map[string]string {
	headers := providers.GitHubCopilotChatHeaders("text/event-stream", "user")
	for key, value := range req.Config.StaticHeaders {
		switch strings.ToLower(key) {
		case "x-initiator", "copilot-integration-id", "copilot-vision-request", "x-client-session-id", "x-agent-task-id", "x-github-repository-nwo", "x-github-repository-host":
			headers[key] = value
		}
	}
	if target == providers.FormatClaude {
		headers["anthropic-version"] = "2023-06-01"
	}
	var obj any
	if json.Unmarshal(body, &obj) == nil && gheHasImage(obj) {
		headers["copilot-vision-request"] = "true"
	}
	return headers
}

func gheHasImage(value any) bool {
	switch v := value.(type) {
	case map[string]any:
		if kind, _ := v["type"].(string); kind == "image" || kind == "image_url" || kind == "input_image" {
			return true
		}
		for key, child := range v {
			if key == "messages" || key == "input" || key == "content" {
				if gheHasImage(child) {
					return true
				}
			}
		}
	case []any:
		for _, child := range v {
			if gheHasImage(child) {
				return true
			}
		}
	}
	return false
}

func gheMessagesResponse(w http.ResponseWriter, req *Request, upstream io.Reader) error {
	if req.IsStream {
		if req.ClaudeClient {
			return gheNativeMessagesStream(w, req, upstream)
		}
		copy := *req
		copy.TranslateResp = false
		return handleClaudeMessagesStream(w, &copy, upstream)
	}
	raw, err := io.ReadAll(io.LimitReader(upstream, constants.MaxUpstreamBodyBytes+1))
	if err != nil {
		return fmt.Errorf("read GHE Messages stream: %w", err)
	}
	if int64(len(raw)) > constants.MaxUpstreamBodyBytes {
		return proxy.UpstreamFailure(http.StatusBadGateway, "GHE Messages response too large")
	}
	body, err := gheAggregateMessages(raw)
	if err != nil {
		return err
	}
	if req.ClaudeClient {
		return jsonResponse(req.Ctx, w, bytes.NewReader(body), false, req.ResponseBuf)
	}
	converted, err := translator.TranslateClaudeResponseToOpenAI(body)
	if err != nil {
		return fmt.Errorf("translate GHE Messages: %w", err)
	}
	return jsonResponse(req.Ctx, w, bytes.NewReader(converted), false, req.ResponseBuf)
}
