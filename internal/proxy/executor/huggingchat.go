// 9RouterX — native executor for HuggingChat's session-cookie JSONL API.
// OmniRoute 632d84f5: open-sse/executors/huggingchat.ts and
// open-sse/executors/huggingchat/jsonlStream.ts.
package executor

import (
	"bytes"
	"context"
	json "encoding/json/v2"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"9routerx/proxy/internal/proxy"
	"9routerx/proxy/internal/translator"
)

const (
	huggingchatDefaultBaseURL = "https://huggingface.co/chat"
	huggingchatBodyMax        = 10 << 20
)

// ForwardHuggingChat creates a conversation, posts the flattened prompt as
// multipart form data, and folds the JSONL reply into OpenAI chunks.
func ForwardHuggingChat(w http.ResponseWriter, req *Request) error {
	if req.Config == nil {
		return proxy.UpstreamFailure(http.StatusBadGateway, "HuggingChat provider config missing")
	}
	cookie := huggingchatCookie(req.APIKey)
	if cookie == "" {
		return proxy.UpstreamFailure(http.StatusUnauthorized, "HuggingChat session cookie is required")
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
		model = huggingchatBodyModel(body)
	}
	model = strings.TrimPrefix(model, "huggingchat/")
	if model == "" {
		return proxy.UpstreamFailure(http.StatusBadRequest, "HuggingChat request is missing a model")
	}
	prompt, system := huggingchatPrompt(body)
	if prompt == "" {
		return proxy.UpstreamFailure(http.StatusBadRequest, "HuggingChat requires at least one user message")
	}

	base := huggingchatBaseURL(req)
	guarded, err := huggingchatGuard(ctx, base)
	if err != nil {
		return err
	}
	createPayload := map[string]any{"model": model}
	if system != "" {
		createPayload["preprompt"] = system
	}
	createBody, _ := json.Marshal(createPayload)
	created, err := proxy.DoRequest(guarded, req.Client, http.MethodPost, base+"/conversation", map[string]string{
		"Cookie": cookie, "Content-Type": "application/json", "Accept": "application/json",
	}, createBody)
	if err != nil {
		return proxy.UpstreamFailure(http.StatusBadGateway, "HuggingChat conversation request failed")
	}
	raw, err := io.ReadAll(io.LimitReader(created.Body, huggingchatBodyMax))
	_ = created.Body.Close()
	if err != nil || created.StatusCode < 200 || created.StatusCode >= 300 {
		return proxy.UpstreamFailure(http.StatusBadGateway, "HuggingChat conversation request failed")
	}
	var conversation struct {
		ConversationID string `json:"conversationId"`
	}
	if json.Unmarshal(raw, &conversation) != nil || conversation.ConversationID == "" {
		return proxy.UpstreamFailure(http.StatusBadGateway, "HuggingChat returned no conversationId")
	}
	cookie = huggingchatMergeCookies(cookie, created.Header.Values("Set-Cookie"))

	parent, err := huggingchatParentMessage(guarded, req.Client, base, conversation.ConversationID, cookie)
	if err != nil {
		return proxy.UpstreamFailure(http.StatusBadGateway, err.Error())
	}
	formBody := huggingchatForm(prompt, system, parent)
	response, err := proxy.DoRequest(guarded, req.Client, http.MethodPost, base+"/conversation/"+conversation.ConversationID, map[string]string{
		"Cookie": cookie, "Content-Type": formBody.contentType, "Accept": "text/event-stream",
	}, formBody.data)
	if err != nil {
		return proxy.UpstreamFailure(http.StatusBadGateway, "HuggingChat message request failed")
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		detail, _ := io.ReadAll(io.LimitReader(response.Body, 2048))
		return proxy.UpstreamFailure(response.StatusCode, huggingchatError(string(detail)))
	}
	stream, err := io.ReadAll(io.LimitReader(response.Body, huggingchatBodyMax))
	if err != nil {
		return proxy.UpstreamFailure(http.StatusBadGateway, "read HuggingChat response")
	}
	id := fmt.Sprintf("chatcmpl-huggingchat-%x", time.Now().UnixNano())
	frames, completion := huggingchatConvert(stream, id, model)
	if req.IsStream {
		return execSSEStream(w, strings.NewReader(frames), req)
	}
	return jsonResponse(req.Ctx, w, bytes.NewReader(completion), req.TranslateResp, req.ResponseBuf)
}

func huggingchatParentMessage(ctx context.Context, client *http.Client, base, id, cookie string) (string, error) {
	response, err := proxy.DoRequest(ctx, client, http.MethodGet, base+"/api/v2/conversations/"+id, map[string]string{
		"Cookie": cookie, "Accept": "application/json",
	}, nil)
	if err != nil {
		return "", fmt.Errorf("HuggingChat conversation lookup failed")
	}
	defer func() { _ = response.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(response.Body, huggingchatBodyMax))
	if err != nil || response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", fmt.Errorf("HuggingChat conversation lookup failed")
	}
	var parsed struct {
		RootMessageID string `json:"rootMessageId"`
	}
	if json.Unmarshal(raw, &parsed) != nil {
		return "", nil
	}
	return parsed.RootMessageID, nil
}

func huggingchatError(detail string) string {
	detail = strings.TrimSpace(detail)
	if len(detail) > 200 {
		detail = detail[:200]
	}
	if detail == "" {
		return "HuggingChat upstream failure"
	}
	return detail
}
