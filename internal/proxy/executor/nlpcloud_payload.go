package executor

import (
	json "encoding/json/v2"
	"fmt"
	"strings"
)

type nlpcloudHistory struct {
	Input    string `json:"input"`
	Response string `json:"response"`
}

type nlpcloudPayloadBody struct {
	Input       string            `json:"input"`
	Context     string            `json:"context,omitempty"`
	History     []nlpcloudHistory `json:"history,omitempty"`
	Stream      bool              `json:"stream,omitempty"`
	Temperature *float64          `json:"temperature,omitempty"`
	TopP        *float64          `json:"top_p,omitempty"`
	Model       string            `json:"model"`
}

func nlpcloudPayload(body []byte, model string, stream bool) ([]byte, error) {
	var request map[string]any
	if err := json.Unmarshal(body, &request); err != nil || request == nil {
		return nil, fmt.Errorf("decode NLP Cloud request")
	}
	var system []string
	type turn struct{ role, text string }
	var turns []turn
	if messages, ok := request["messages"].([]any); ok {
		for _, item := range messages {
			message, ok := item.(map[string]any)
			if !ok {
				continue
			}
			role, _ := message["role"].(string)
			text := nlpcloudContentText(message["content"])
			if text == "" {
				continue
			}
			switch role {
			case "system", "developer":
				system = append(system, text)
			case "user", "assistant":
				turns = append(turns, turn{role: role, text: text})
			}
		}
	}
	lastUser := -1
	for i := len(turns) - 1; i >= 0; i-- {
		if turns[i].role == "user" {
			lastUser = i
			break
		}
	}
	if lastUser < 0 {
		return nil, fmt.Errorf("NLP Cloud requests require at least one user message")
	}
	payload := nlpcloudPayloadBody{Input: turns[lastUser].text, Model: model, Stream: stream}
	if len(system) > 0 {
		payload.Context = strings.Join(system, "\n\n")
	}
	var pending string
	for _, item := range turns[:lastUser] {
		if item.role == "user" {
			pending = item.text
			continue
		}
		if pending != "" {
			payload.History = append(payload.History, nlpcloudHistory{Input: pending, Response: item.text})
			pending = ""
		}
	}
	if temperature, ok := numberField(request, "temperature"); ok {
		payload.Temperature = &temperature
	}
	if topP, ok := numberField(request, "top_p"); ok {
		payload.TopP = &topP
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode NLP Cloud payload: %w", err)
	}
	return encoded, nil
}

func nlpcloudContentText(content any) string {
	switch value := content.(type) {
	case string:
		return value
	case []any:
		var parts []string
		for _, raw := range value {
			part, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			if text, ok := part["text"].(string); ok {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, "\n")
	}
	return ""
}

func numberField(payload map[string]any, key string) (float64, bool) {
	value, ok := payload[key].(float64)
	return value, ok
}
