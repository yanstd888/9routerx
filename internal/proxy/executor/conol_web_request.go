package executor

import (
	"bufio"
	"bytes"
	"context"
	json "encoding/json/v2"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

func conolDo(ctx context.Context, client *http.Client, method, target, cookie, sessionID, contentType string, payload []byte) ([]byte, error) {
	httpReq, err := http.NewRequestWithContext(ctx, method, target, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("create Conol request")
	}
	for key, value := range conolHeaders(cookie, sessionID, contentType) {
		httpReq.Header.Set(key, value)
	}
	response, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("conol request failed")
	}
	defer func() { _ = response.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil || response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("conol upstream HTTP %d", response.StatusCode)
	}
	return raw, nil
}

func conolCreateSession(ctx context.Context, client *http.Client, base, cookie string) (string, error) {
	payload, _ := json.Marshal(map[string]any{"source": map[string]any{"type": "home"}, "messages": []any{}, "timezone": "UTC"})
	raw, err := conolDo(ctx, client, http.MethodPost, base, cookie, "", "application/json", payload)
	if err != nil {
		return "", err
	}
	var parsed struct {
		SessionID string `json:"sessionId"`
	}
	if json.Unmarshal(raw, &parsed) != nil || parsed.SessionID == "" {
		return "", fmt.Errorf("conol returned no session id")
	}
	return parsed.SessionID, nil
}

func conolPinModel(ctx context.Context, client *http.Client, base, cookie, sessionID, model, effort string) error {
	target := base + "/" + url.PathEscape(sessionID) + "/model"
	steps := []map[string]any{
		{"modelPreset": "pro", "hasImageHistory": false},
		{"agentModel": model, "agentEffort": nil},
	}
	if effort != "" {
		steps = append(steps, map[string]any{"agentEffort": effort})
	}
	for _, step := range steps {
		payload, _ := json.Marshal(step)
		if _, err := conolDo(ctx, client, http.MethodPost, target, cookie, sessionID, "application/json", payload); err != nil {
			return err
		}
	}
	return nil
}

func conolSubmit(ctx context.Context, client *http.Client, base, cookie, sessionID, prompt string) error {
	payload, _ := json.Marshal(map[string]any{
		"messages": []any{map[string]any{"type": "text", "content": prompt}}, "timezone": "UTC",
	})
	_, err := conolDo(ctx, client, http.MethodPost, base+"/"+url.PathEscape(sessionID)+"/messages", cookie, sessionID, "application/json", payload)
	return err
}

func conolPoll(ctx context.Context, client *http.Client, base, cookie, sessionID string) (string, error) {
	target := base + "/" + url.PathEscape(sessionID) + "/messages?logDeltas=1"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return "", fmt.Errorf("create Conol stream request")
	}
	for key, value := range conolHeaders(cookie, sessionID, "text/event-stream, application/x-ndjson") {
		httpReq.Header.Set(key, value)
	}
	response, err := client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("conol stream request failed")
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", fmt.Errorf("conol stream HTTP %d", response.StatusCode)
	}
	scanner := bufio.NewScanner(io.LimitReader(response.Body, 10<<20))
	scanner.Buffer(make([]byte, 64<<10), 1<<20)
	var finalized, preview, streamed strings.Builder
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, ":") || strings.HasPrefix(line, "event:") {
			continue
		}
		line = strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		line = strings.TrimSpace(strings.TrimPrefix(line, "message\t"))
		if line == "" || !strings.HasPrefix(line, "{") {
			continue
		}
		var event struct {
			Type    string `json:"type"`
			Stages  []any  `json:"stages"`
			Content any    `json:"content"`
			Message any    `json:"message"`
			Text    any    `json:"text"`
			Delta   any    `json:"delta"`
		}
		if json.Unmarshal([]byte(line), &event) != nil {
			continue
		}
		if event.Type == "done" {
			break
		}
		if text := conolStagedText(event.Stages, "logs"); text != "" {
			finalized.Reset()
			finalized.WriteString(text)
		}
		if text := conolStagedText(event.Stages, "preview"); text != "" {
			preview.Reset()
			preview.WriteString(text)
		}
		switch event.Type {
		case "assistant":
			for _, value := range []any{event.Content, event.Message, event.Text} {
				if text := conolText(value); text != "" {
					finalized.Reset()
					finalized.WriteString(text)
				}
			}
		case "stream_event":
			for _, value := range []any{event.Delta, event.Content, event.Text} {
				if text := conolText(value); text != "" {
					streamed.WriteString(text)
				}
			}
		}
	}
	if finalized.Len() > 0 {
		return finalized.String(), nil
	}
	if preview.Len() > 0 {
		return preview.String(), nil
	}
	return streamed.String(), nil
}

func conolStagedText(stages []any, field string) string {
	result := ""
	for _, raw := range stages {
		stage, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		entries, ok := stage[field].([]any)
		if !ok {
			continue
		}
		for _, entry := range entries {
			if text := conolText(entry); text != "" {
				result = text
			}
		}
	}
	return result
}
