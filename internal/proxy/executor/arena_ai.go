// 9RouterX — derived from the work of https://github.com/yanstd888/ — port and modifications by the maintainers — https://t.me/aiclub_id
package executor

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	json "encoding/json/v2"

	"9routerx/proxy/internal/handlerutil"
	"9routerx/proxy/internal/proxy"
	"9routerx/proxy/internal/translator"
)

const arenaAIEvalURL = "https://arena.ai/nextjs-api/stream/create-evaluation"
const arenaAIBaseURL = "https://arena.ai"
const arenaAIUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"

// arenaAIModelMap maps the public model ids to arena.ai's internal evaluation
// model UUIDs (ported verbatim from the xrouter executor, pinned 7fd5698).
// A model id outside the map passes through unchanged: the upstream accepts
// raw ids it knows.
var arenaAIModelMap = map[string]string{
	"gpt-5.5-instant":               "019e71ea-1e1d-740f-9c2d-dab5869ff108",
	"gpt-5.5":                       "019e71ea-1e1d-740f-9c2d-dab5869ff108",
	"gemini-3.1-pro-preview":        "019c7820-5480-78b6-9fef-04c0d7004054",
	"gemini-3.1-pro":                "019c7820-5480-78b6-9fef-04c0d7004054",
	"gemini-3.6-flash":              "019f90b1-c0ac-71ce-b295-487f261bf0f4",
	"gemini-3-flash":                "019b47da-49b9-7295-906c-ce44ccd30d74",
	"grok-4.20-beta-0309-reasoning": "019ce35a-fa6f-7262-8ee2-ed4442821ce7",
	"grok-4.20":                     "019ce35a-fa6f-7262-8ee2-ed4442821ce7",
	"glm-5":                         "019c45d7-96f0-7d39-8143-9d57941b5523",
	"qwen3.7-plus":                  "019e86fe-167d-77bd-94a8-df7aee4f4551",
	"gpt-5.1-high":                  "019a8548-a2b1-70ce-b1be-eba096d41f58",
	"gemini-3.5-flash-lite":         "019f90b1-cae9-786a-91a2-f3ee4c6cbf61",
	"gpt-5.4-mini-high":             "019cfcdd-5426-777e-8314-04619cb92cc4",
	"gemini-2.5-pro":                "0199f060-b306-7e1f-aeae-0ebb4e3f1122",
	"glm-4.7":                       "019becd0-af81-7883-a7ca-5c4a4e42ff7a",
	"minimax-m3":                    "019e809d-f62d-7192-bb7f-1657e066b5f2",
	"kimi-k2-thinking-turbo":        "019a59bc-8bb8-7933-92eb-fe143770c211",
	"o3-2025-04-16":                 "cb0f1e24-e8e9-4745-aabc-b926ffde7475",
	"o3":                            "cb0f1e24-e8e9-4745-aabc-b926ffde7475",
	"claude-haiku-4-5-20251001":     "0199e8e9-01ed-73e0-96ba-cf43b286bf10",
	"deepseek-v3":                   "019b9784-470e-75b9-b2fb-f005d78972e1",
	"deepseek-r1":                   "019a59bc-8bb8-7933-92eb-fe143770c211",
}

// ForwardArenaAI serves the arena.ai evaluation lane (the canonical id;
// "lmarena" resolves here as an alias). One POST opens a direct evaluation
// against the chosen model and answers with an SSE-ish body whose data
// frames carry plain text deltas (xrouter executor, pinned 7fd5698).
func ForwardArenaAI(w http.ResponseWriter, req *Request) error {
	if !CheckWebExecutorAllowed(w, "arena-ai") {
		return &proxy.UpstreamError{
			StatusCode: http.StatusForbidden,
			Body:       []byte(`{"error":{"code":"web_executors_disabled","message":"arena-ai is disabled by policy","type":"policy_error"}}`),
		}
	}

	var parsed struct {
		Model    string                     `json:"model"`
		Messages []translator.OpenAIMessage `json:"messages"`
		Prompt   string                     `json:"prompt"`
		Stream   bool                       `json:"stream"`
	}
	if err := json.Unmarshal(req.Body, &parsed); err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "arena-ai: invalid request body")
		return err
	}

	userToken := resolveArenaAIToken(req)
	if userToken == "" {
		handlerutil.WriteJSONErrorCode(w, http.StatusUnauthorized, "missing_session",
			"arena-ai: missing user token or evaluation_session cookie")
		return &proxy.UpstreamError{
			StatusCode: http.StatusUnauthorized,
			Body:       []byte(`{"error":{"code":"missing_session","message":"arena-ai: missing user token or evaluation_session cookie","type":"auth_error"}}`),
		}
	}

	modelName := strings.ToLower(parsed.Model)
	modelName = strings.TrimPrefix(modelName, "arena-ai/")
	modelName = strings.TrimPrefix(modelName, "arena/")
	targetModelAID := modelName
	if mapped, ok := arenaAIModelMap[modelName]; ok {
		targetModelAID = mapped
	} else if mapped, ok := arenaAIModelMap[parsed.Model]; ok {
		targetModelAID = mapped
	}

	prompt := renderArenaAIPrompt(parsed.Messages, parsed.Prompt)

	sessionID, err := arenaAUUID()
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, "arena-ai: uuid error")
		return err
	}
	userMsgID, err := arenaAUUID()
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, "arena-ai: uuid error")
		return err
	}
	modelMsgID, err := arenaAUUID()
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, "arena-ai: uuid error")
		return err
	}

	payload := map[string]any{
		"id":              sessionID,
		"mode":            "direct",
		"modelAId":        targetModelAID,
		"userMessageId":   userMsgID,
		"modelAMessageId": modelMsgID,
		"userMessage":     map[string]any{"content": prompt, "experimental_attachments": []any{}},
		"modality":        "text",
	}
	payloadBytes, err := json.Marshal(payload, json.Deterministic(true))
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, "arena-ai: marshal error")
		return err
	}

	endpointURL := arenaAIEvalURL
	if req.Config != nil && req.Config.BaseURL != "" {
		endpointURL = req.Config.BaseURL
	}
	if req.ConnData != nil {
		if u, ok := req.ConnData["baseUrl"].(string); ok && strings.TrimSpace(u) != "" {
			endpointURL = strings.TrimSpace(u)
		}
	}

	ctx := req.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpointURL, bytes.NewReader(payloadBytes))
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream, application/json, text/plain, */*")
	httpReq.Header.Set("User-Agent", arenaAIUserAgent)
	httpReq.Header.Set("Origin", arenaAIBaseURL)
	httpReq.Header.Set("Referer", arenaAIBaseURL+"/")
	// A pasted Cookie header rides verbatim; a bare token becomes the
	// evaluation/agentic session pair plus a Bearer credential, exactly as
	// the upstream executor splits the two shapes.
	if strings.Contains(userToken, "=") {
		httpReq.Header.Set("Cookie", userToken)
	} else {
		httpReq.Header.Set("Cookie", "evaluation_session="+userToken+"; agentic_session="+userToken)
		httpReq.Header.Set("Authorization", "Bearer "+userToken)
	}

	client := req.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadGateway, "arena-ai: upstream error: "+err.Error())
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
		msg := string(bodyBytes)
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			msg = "Arena AI user token is expired or invalid; re-authenticate and paste a fresh token"
		}
		handlerutil.WriteJSONError(w, resp.StatusCode, fmt.Sprintf("arena-ai: upstream error %d: %s", resp.StatusCode, msg))
		return &proxy.UpstreamError{StatusCode: resp.StatusCode, Body: bodyBytes}
	}

	return streamArenaAIResponse(w, resp.Body, parsed.Model, req.IsStream)
}

func resolveArenaAIToken(req *Request) string {
	if req.APIKey != "" && req.APIKey != "anonymous" {
		return strings.TrimSpace(req.APIKey)
	}
	if req.ConnData != nil {
		for _, key := range []string{"token", "evaluation_session", "cookie", "apiKey", "accessToken"} {
			if v, ok := req.ConnData[key].(string); ok && strings.TrimSpace(v) != "" {
				return strings.TrimSpace(v)
			}
		}
	}
	return ""
}

// renderArenaAIPrompt folds the OpenAI messages into the labeled text prompt
// the evaluation endpoint expects (ported from the upstream executor).
func renderArenaAIPrompt(messages []translator.OpenAIMessage, fallback string) string {
	if len(messages) == 0 {
		return fallback
	}
	parts := make([]string, 0, len(messages))
	for _, m := range messages {
		role := "User"
		switch m.Role {
		case "assistant":
			role = "Assistant"
		case "system":
			role = "System"
		}
		content := ""
		if s, ok := m.Content.(string); ok {
			content = s
		} else if m.Content != nil {
			if b, err := json.Marshal(m.Content, json.Deterministic(true)); err == nil {
				content = string(b)
			}
		}
		parts = append(parts, fmt.Sprintf("%s: %s", role, content))
	}
	return strings.Join(parts, "\n\n")
}

// arenaAUUID returns a random RFC 4122-shaped UUID string for the protocol's
// id fields (the upstream uses crypto.randomUUID).
func arenaAUUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

// streamArenaAIResponse folds arena.ai's evaluation stream into OpenAI
// chunks. Frames arrive as `data:` lines carrying {"text"|"content"|"delta"}
// strings; bare lines are raw text deltas. The upstream body ends the turn
// without a [DONE] marker, so the terminator is emitted when the reader ends.
func streamArenaAIResponse(w http.ResponseWriter, upstream io.Reader, model string, isStream bool) error {
	scanner := bufio.NewScanner(upstream)
	scanner.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)

	flusher, isFlusher := w.(http.Flusher)
	if isStream {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(http.StatusOK)
		if isFlusher {
			flusher.Flush()
		}
	}

	var accumulated strings.Builder
	id := fmt.Sprintf("chatcmpl-arn-%d", time.Now().UnixMilli())

	emit := func(textChunk string) {
		accumulated.WriteString(textChunk)
		if !isStream {
			return
		}
		chunk := map[string]any{
			"id":      id,
			"object":  "chat.completion.chunk",
			"created": time.Now().Unix(),
			"model":   model,
			"choices": []any{
				map[string]any{
					"index":         0,
					"delta":         map[string]string{"content": textChunk},
					"finish_reason": nil,
				},
			},
		}
		chunkBytes, _ := json.Marshal(chunk, json.Deterministic(true))
		_, _ = fmt.Fprintf(w, "data: %s\n\n", string(chunkBytes))
		if isFlusher {
			flusher.Flush()
		}
	}

	for scanner.Scan() {
		trimmed := strings.TrimSpace(scanner.Text())
		if trimmed == "" || strings.HasPrefix(trimmed, ":") {
			continue
		}
		if strings.HasPrefix(trimmed, "data:") {
			dataStr := strings.TrimSpace(strings.TrimPrefix(trimmed, "data:"))
			if dataStr == "" || dataStr == "[DONE]" {
				continue
			}
			var event map[string]any
			if err := json.Unmarshal([]byte(dataStr), &event); err != nil {
				emit(dataStr)
				continue
			}
			textChunk := ""
			if t, ok := event["text"].(string); ok {
				textChunk = t
			} else if c, ok := event["content"].(string); ok {
				textChunk = c
			} else if d, ok := event["delta"].(string); ok {
				textChunk = d
			}
			if textChunk != "" {
				emit(textChunk)
			}
			continue
		}
		emit(trimmed)
	}
	if err := scanner.Err(); err != nil {
		return err
	}

	if isStream {
		termChunk := map[string]any{
			"id":      id,
			"object":  "chat.completion.chunk",
			"created": time.Now().Unix(),
			"model":   model,
			"choices": []any{
				map[string]any{
					"index":         0,
					"delta":         map[string]string{},
					"finish_reason": "stop",
				},
			},
		}
		termBytes, _ := json.Marshal(termChunk, json.Deterministic(true))
		_, _ = fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", string(termBytes))
		if isFlusher {
			flusher.Flush()
		}
		return nil
	}

	fullResp := map[string]any{
		"id":      id,
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   model,
		"choices": []any{
			map[string]any{
				"index": 0,
				"message": map[string]string{
					"role":    "assistant",
					"content": accumulated.String(),
				},
				"finish_reason": "stop",
			},
		},
	}
	handlerutil.WriteJSON(w, http.StatusOK, fullResp)
	return nil
}
