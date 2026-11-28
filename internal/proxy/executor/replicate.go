// 9RouterX — ported from OmniRoute open-sse/executors/replicate.ts (pin 632d84f5).
// Replicate's language models take a single prediction input, not Chat
// Completions: this executor owns the request rebuild, the native stream relay
// and the bounded polling lifecycle.
package executor

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	json "encoding/json/v2"

	"9routerx/proxy/internal/handlerutil"
	"9routerx/proxy/internal/providers"
)

// ForwardReplicate serves the apikey Replicate lane.
func ForwardReplicate(w http.ResponseWriter, req *Request) error {
	ctx := req.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	client := req.Client
	if client == nil {
		client = http.DefaultClient
	}

	model := strings.TrimSpace(req.ModelName)
	if model == "" {
		model = replicateModelFromBody(req.Body)
	}
	parsed, ok := parseReplicateModel(model)
	if !ok {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "Replicate model IDs must look like owner/name or owner/name:version.")
		return nil
	}
	built, ok := replicateBuildPayload(req.Body, parsed, req.IsStream)
	if !ok {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "Replicate requests require at least one user message.")
		return nil
	}

	requestURL := replicatePredictionsURL(replicateBaseURL(req), parsed)
	headers := replicateHeaders(req)
	payload, err := json.Marshal(built.payload, json.Deterministic(true))
	if err != nil {
		writeReplicateError(w, &replicateHTTPError{status: http.StatusBadGateway, message: "Replicate request failed"})
		return nil
	}

	resp, err := replicateDo(ctx, client, http.MethodPost, requestURL, headers, payload)
	if err != nil {
		writeReplicateError(w, err)
		return nil
	}
	forwardUpstreamResponseHeaders(w, resp.Header)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		failure := replicateUpstreamFailure(resp)
		_ = resp.Body.Close()
		writeReplicateError(w, failure)
		return nil
	}
	created, err := replicateReadPrediction(resp)
	_ = resp.Body.Close()
	if err != nil {
		writeReplicateError(w, &replicateHTTPError{status: http.StatusBadGateway, message: "Replicate request failed"})
		return nil
	}

	status, _ := created["status"].(string)
	streamURL, _ := replicateMap(created["urls"])["stream"].(string)
	if req.IsStream && !replicateTerminalStatuses[status] && isTrustedStreamURL(streamURL, requestURL) {
		return relayReplicateStream(ctx, w, client, req, streamURL, created, requestURL, headers, built, model)
	}

	prediction, err := pollReplicateUntilDone(ctx, client, created, requestURL, headers)
	if err != nil {
		writeReplicateError(w, err)
		return nil
	}
	return replicateFinishedResponse(w, req, prediction, built, model)
}

func replicateBaseURL(req *Request) string {
	if req.ConnData != nil {
		if custom, ok := req.ConnData["baseUrl"].(string); ok {
			if trimmed := strings.TrimSpace(custom); trimmed != "" {
				return strings.TrimRight(trimmed, "/")
			}
		}
	}
	return replicateDefaultBaseURL
}

func replicatePredictionsURL(base string, parsed replicateParsedModel) string {
	if parsed.version != "" {
		return base + "/predictions"
	}
	segments := strings.Split(parsed.path, "/")
	for i, segment := range segments {
		segments[i] = url.PathEscape(segment)
	}
	return base + "/models/" + strings.Join(segments, "/") + "/predictions"
}

func replicateHeaders(req *Request) map[string]string {
	headers := map[string]string{"Content-Type": "application/json"}
	// Streaming returns the prediction immediately; only blocking calls ask the
	// upstream to hold the connection with Prefer: wait.
	if !req.IsStream {
		headers["Prefer"] = fmt.Sprintf("wait=%d", replicateWaitSeconds)
	}
	key := strings.TrimSpace(req.APIKey)
	if key == "" && req.ConnData != nil {
		if token, ok := req.ConnData["accessToken"].(string); ok {
			key = strings.TrimSpace(token)
		}
	}
	if key != "" {
		headers["Authorization"] = "Bearer " + key
	}
	if req.Config != nil {
		headers = providers.ApplyHeaderOverrides(headers, req.Config.HeaderOverrides)
	}
	return headers
}

func writeReplicateError(w http.ResponseWriter, err error) {
	status := http.StatusBadGateway
	message := "Replicate request failed"
	retryAfter := ""
	var failure *replicateHTTPError
	if errors.As(err, &failure) {
		status = failure.status
		message = failure.message
		retryAfter = failure.retryAfter
	}
	if retryAfter != "" {
		w.Header().Set("Retry-After", retryAfter)
	}
	handlerutil.WriteJSONError(w, status, message)
}

type replicateCompletionMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type replicateCompletionChoice struct {
	Index        int                        `json:"index"`
	Message      replicateCompletionMessage `json:"message"`
	FinishReason string                     `json:"finish_reason"`
}

type replicateCompletion struct {
	ID      string                      `json:"id"`
	Object  string                      `json:"object"`
	Created int64                       `json:"created"`
	Model   string                      `json:"model"`
	Choices []replicateCompletionChoice `json:"choices"`
	Usage   replicateUsage              `json:"usage"`
}

func replicateFinishedResponse(w http.ResponseWriter, req *Request, prediction map[string]any, built replicateBuilt, model string) error {
	status, _ := prediction["status"].(string)
	if status != "succeeded" {
		if status == "failed" || status == "canceled" {
			reason := ""
			if detail, ok := prediction["error"].(string); ok {
				reason = ": " + replicateSanitizeDetail(detail)
			}
			writeReplicateError(w, &replicateHTTPError{status: http.StatusBadGateway, message: "Replicate prediction " + status + reason})
			return nil
		}
		writeReplicateError(w, &replicateHTTPError{status: http.StatusGatewayTimeout, message: "Replicate prediction did not finish in time"})
		return nil
	}

	content := replicateOutputToText(prediction["output"])
	usage := replicateResolveUsage(prediction, built.promptText, content)
	finish := replicateFinishReason(usage, built.maxTokens)
	if req.IsStream {
		synthesizeReplicateStream(w, content, model, usage, finish)
		return nil
	}
	chunks := newReplicateChunks(model)
	completion := replicateCompletion{
		ID:      chunks.id,
		Object:  "chat.completion",
		Created: chunks.created,
		Model:   model,
		Choices: []replicateCompletionChoice{{
			Index:        0,
			Message:      replicateCompletionMessage{Role: "assistant", Content: content},
			FinishReason: finish,
		}},
		Usage: usage,
	}
	handlerutil.WriteJSON(w, http.StatusOK, completion)
	return nil
}
