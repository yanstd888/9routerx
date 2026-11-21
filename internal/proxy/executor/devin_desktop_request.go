package executor

import (
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"fmt"
	"strings"

	"9routerx/proxy/internal/translator"
	"github.com/google/uuid"
)

type devinDesktopSource uint64

const (
	devinDesktopUser       devinDesktopSource = 1
	devinDesktopAssistant  devinDesktopSource = 2
	devinDesktopToolResult devinDesktopSource = 4
)

type devinDesktopFunction struct {
	Name        string         `json:"name,omitempty"`
	Description string         `json:"description,omitempty"`
	Parameters  jsontext.Value `json:"parameters,omitempty"`
	Arguments   string         `json:"arguments,omitempty"`
	Strict      bool           `json:"strict,omitempty"`
}

type devinDesktopToolCall struct {
	ID       string               `json:"id,omitempty"`
	Type     string               `json:"type,omitempty"`
	Function devinDesktopFunction `json:"function"`
}

type devinDesktopMessage struct {
	Role       string                 `json:"role"`
	Content    jsontext.Value         `json:"content"`
	ToolCalls  []devinDesktopToolCall `json:"tool_calls"`
	ToolCallID string                 `json:"tool_call_id"`
}

type devinDesktopChatInput struct {
	Model             string                 `json:"model"`
	Messages          []devinDesktopMessage  `json:"messages"`
	Tools             []devinDesktopToolCall `json:"tools"`
	ToolChoice        jsontext.Value         `json:"tool_choice"`
	ParallelToolCalls *bool                  `json:"parallel_tool_calls"`
	ConversationID    string                 `json:"conversation_id"`
}

type devinDesktopPrompt struct {
	id     string
	source devinDesktopSource
	text   string
	calls  []devinDesktopToolCall
	callID string
}

type devinDesktopRequestInput struct {
	model           string
	system          string
	sessionID       string
	cascadeID       string
	prompts         []devinDesktopPrompt
	tools           []devinDesktopToolCall
	choice          []byte
	disableParallel bool
}

type devinDesktopMetadata struct {
	apiKey    string
	sessionID string
	jwt       string
}

func (m devinDesktopMetadata) encode() []byte {
	var payload []byte
	for _, field := range []struct {
		number int
		text   string
	}{
		{1, "windsurf"}, {2, "1.48.2"}, {3, m.apiKey}, {4, "en-US"},
		{7, "3.6.27"}, {10, m.sessionID}, {12, "windsurf"}, {21, m.jwt},
	} {
		if field.text != "" {
			payload = append(payload, wsStr(field.number, field.text)...)
		}
	}
	return payload
}

func devinDesktopInput(req *Request, ctx context.Context) (devinDesktopRequestInput, context.Context, error) {
	body := req.Body
	var err error
	if req.ClaudeClient {
		body, err = translator.TranslateClaudeToOpenAI(body)
		if err != nil {
			return devinDesktopRequestInput{}, ctx, fmt.Errorf("translate Claude request: %w", err)
		}
	}
	if translator.IsResponsesClient(ctx) {
		converted, err := translator.ResponsesToChatRequest(body)
		if err != nil {
			return devinDesktopRequestInput{}, ctx, fmt.Errorf("translate Responses request: %w", err)
		}
		body = converted.Body
		ctx = translator.WithResponsesBridge(ctx)
		if len(converted.CustomToolNames) > 0 {
			ctx = translator.WithCustomToolNames(ctx, converted.CustomToolNames)
		}
	}
	var chat devinDesktopChatInput
	if err := json.Unmarshal(body, &chat); err != nil {
		return devinDesktopRequestInput{}, ctx, fmt.Errorf("parse chat request: %w", err)
	}
	input := devinDesktopRequestInput{
		model: chat.Model, sessionID: uuid.NewString(), cascadeID: chat.ConversationID,
		tools: chat.Tools, choice: devinDesktopChoice(chat.ToolChoice),
		disableParallel: chat.ParallelToolCalls != nil && !*chat.ParallelToolCalls,
	}
	if req.ModelName != "" {
		input.model = req.ModelName
	}
	if input.cascadeID == "" {
		input.cascadeID = uuid.NewString()
	}
	var system []string
	for _, msg := range chat.Messages {
		text := devinDesktopText(msg.Content)
		if msg.Role == "system" || msg.Role == "developer" {
			if text != "" {
				system = append(system, text)
			}
			continue
		}
		prompt := devinDesktopPrompt{id: uuid.NewString(), source: devinDesktopUser, text: text}
		switch msg.Role {
		case "assistant":
			prompt.source, prompt.id, prompt.calls = devinDesktopAssistant, "bot-"+prompt.id, msg.ToolCalls
		case "tool":
			prompt.source, prompt.callID = devinDesktopToolResult, msg.ToolCallID
		}
		input.prompts = append(input.prompts, prompt)
	}
	input.system = strings.Join(system, "\n\n")
	if len(input.prompts) == 0 {
		input.prompts = []devinDesktopPrompt{{id: uuid.NewString(), source: devinDesktopUser}}
	}
	return input, ctx, nil
}

func devinDesktopText(raw jsontext.Value) string {
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &blocks) != nil {
		return ""
	}
	var result strings.Builder
	for _, block := range blocks {
		if block.Type == "text" {
			result.WriteString(block.Text)
		}
	}
	return result.String()
}

func devinDesktopChoice(raw jsontext.Value) []byte {
	var choice string
	if json.Unmarshal(raw, &choice) == nil {
		if choice == "auto" || choice == "none" || choice == "required" {
			return wsStr(1, choice)
		}
		return nil
	}
	var named devinDesktopToolCall
	if json.Unmarshal(raw, &named) == nil && named.Type == "function" && named.Function.Name != "" {
		return wsStr(2, named.Function.Name)
	}
	return nil
}

func (input devinDesktopRequestInput) encode(metadata devinDesktopMetadata) []byte {
	payload := wsMsg(1, metadata.encode())
	if input.system != "" {
		payload = append(payload, wsStr(2, input.system)...)
	}
	for _, prompt := range input.prompts {
		p := append(wsStr(1, prompt.id), wsVarintField(2, uint64(prompt.source))...)
		if prompt.text != "" {
			p = append(p, wsStr(3, prompt.text)...)
		}
		for _, call := range prompt.calls {
			if call.Type != "function" || call.ID == "" || call.Function.Name == "" {
				continue
			}
			c := append(wsStr(1, call.ID), wsStr(2, call.Function.Name)...)
			c = append(c, wsStr(3, call.Function.Arguments)...)
			p = append(p, wsMsg(6, c)...)
		}
		if prompt.callID != "" {
			p = append(p, wsStr(7, prompt.callID)...)
		}
		payload = append(payload, wsMsg(3, p)...)
	}
	payload = append(payload, wsVarintField(7, 5)...)
	for _, tool := range input.tools {
		if tool.Type != "function" || tool.Function.Name == "" {
			continue
		}
		f := tool.Function
		t := wsStr(1, f.Name)
		if f.Description != "" {
			t = append(t, wsStr(2, f.Description)...)
		}
		schema := f.Parameters
		if len(schema) == 0 || string(schema) == "null" {
			schema = jsontext.Value(`{}`)
		}
		t = append(t, wsStr(3, string(schema))...)
		if f.Strict {
			t = append(t, wsVarintField(12, 1)...)
		}
		payload = append(payload, wsMsg(10, t)...)
	}
	if input.disableParallel {
		payload = append(payload, wsVarintField(11, 1)...)
	}
	if len(input.choice) > 0 {
		payload = append(payload, wsMsg(12, input.choice)...)
	}
	payload = append(payload, wsStr(14, input.model)...)
	payload = append(payload, wsStr(16, input.cascadeID)...)
	return append(payload, wsStr(21, input.model)...)
}
