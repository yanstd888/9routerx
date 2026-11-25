package executor

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"9routerx/proxy/internal/translator"
)

func opencodeServerPump(ctx context.Context, state *opencodeServerStreamState, sub *opencodeServerSubscription, sessionID string) error {
	assistantID := ""
	seen := map[string]bool{}
	toolDeltas := map[string]string{}
	var tokens opencodeServerTokenCounts
	hasTokens := false
	flushTools := func() error {
		for partID, text := range toolDeltas {
			delete(toolDeltas, partID)
			if err := opencodeServerEmitToolText(state, text); err != nil {
				return err
			}
		}
		return nil
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-sub.bus.done:
			return state.failure(fmt.Errorf("event stream closed: %v", sub.bus.err))
		case event, ok := <-sub.ch:
			if !ok {
				return state.failure(errors.New("event stream closed"))
			}
			if event.Type == "session.error" {
				return state.failure(fmt.Errorf("%s", event.Error))
			}
			if event.Type == "server.connected" || event.SessionID != sessionID {
				continue
			}
			if event.Type == "message.updated" {
				if event.Role != "assistant" {
					continue
				}
				if assistantID == "" {
					assistantID = event.MessageID
				}
				if event.HasTokens {
					tokens, hasTokens = event.Tokens, true
				}
				continue
			}
			if assistantID == "" || (event.MessageID != "" && event.MessageID != assistantID) {
				continue
			}
			if event.HasTokens {
				tokens, hasTokens = event.Tokens, true
			}
			switch event.Type {
			case "message.part.delta":
				if event.Kind == "tool" {
					toolDeltas[event.PartID] += event.Delta
					continue
				}
				if event.Delta != "" {
					seen[event.PartID] = true
					if err := opencodeServerEmitDelta(state, event.Kind, event.Delta); err != nil {
						return err
					}
				}
			case "message.part.updated":
				if event.Kind == "tool" {
					delete(toolDeltas, event.PartID)
					if event.Text != "" {
						if err := opencodeServerEmitToolText(state, event.Text); err != nil {
							return err
						}
					}
					continue
				}
				if !seen[event.PartID] {
					if event.Text != "" || event.Reasoning != "" {
						seen[event.PartID] = true
						kind := event.Kind
						text := event.Text
						if kind == "reasoning" {
							text = event.Reasoning
						}
						if err := opencodeServerEmitDelta(state, kind, text); err != nil {
							return err
						}
					}
				}
			case "session.idle":
				if err := flushTools(); err != nil {
					return err
				}
				if hasTokens {
					if err := state.usage(tokens); err != nil {
						return err
					}
				}
				if state.responses != nil {
					for _, frame := range state.responses.flush() {
						if err := state.write([]byte(translator.FormatResponsesSSE(frame))); err != nil {
							return err
						}
					}
					return nil
				}
				return state.write([]byte("data: [DONE]\n\n"))
			}
		}
	}
}

func opencodeServerEmitDelta(state *opencodeServerStreamState, kind, text string) error {
	delta := translator.OpenAIDelta{Content: text}
	if kind == "reasoning" {
		delta, delta.Content = translator.OpenAIDelta{ReasoningContent: text}, ""
	}
	return state.feed(&translator.OpenAIChunk{
		ID: state.id, Model: state.model,
		Choices: []translator.OpenAIChoice{{Index: 0, Delta: delta}},
	})
}

func opencodeServerEmitToolText(state *opencodeServerStreamState, text string) error {
	clean, calls := opencodeServerExtractCalls(text)
	for index, call := range calls {
		idx := index
		delta := translator.OpenAIDelta{ToolCalls: []translator.OpenAIToolCallStream{{
			Index: &idx, ID: call.ID, Type: "function",
			Function: &translator.OpenAIFunctionStream{Name: call.Function.Name, Arguments: call.Function.Arguments},
		}}}
		if err := state.feed(&translator.OpenAIChunk{
			ID: state.id, Model: state.model,
			Choices: []translator.OpenAIChoice{{Index: 0, Delta: delta}},
		}); err != nil {
			return err
		}
	}
	if clean != "" {
		return opencodeServerEmitDelta(state, "text", clean)
	}
	return nil
}

// opencodeServerAnswer folds the blocking message into one chat.completion.
func opencodeServerAnswer(ctx context.Context, w http.ResponseWriter, req *Request, result opencodeServerMessageResponse, model string) error {
	var text, reasoning strings.Builder
	var calls []translator.OpenAIToolCall
	for _, part := range result.Parts {
		switch part.Type {
		case "text":
			clean, found := opencodeServerExtractCalls(part.Text)
			text.WriteString(clean)
			calls = append(calls, found...)
		case "reasoning":
			reasoning.WriteString(part.Text)
		case "tool":
			clean, found := opencodeServerExtractCalls(part.Text)
			text.WriteString(clean)
			calls = append(calls, found...)
		}
	}
	finish := "stop"
	if len(calls) > 0 {
		finish = "tool_calls"
	}
	usage := result.Info.Tokens.usage()
	if usage != nil {
		translator.SetUsage(ctx, usage)
	}
	response := map[string]any{
		"id":      "ocs-" + result.Info.ID,
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   model,
		"choices": []any{map[string]any{
			"index": 0,
			"message": map[string]any{
				"role":              "assistant",
				"content":           text.String(),
				"reasoning_content": reasoning.String(),
				"tool_calls":        calls,
			},
			"finish_reason": finish,
		}},
		"usage": usage,
	}
	body, err := json.Marshal(response)
	if err != nil {
		return fmt.Errorf("encode OpenCode completion: %w", err)
	}
	if req.ResponseBuf != nil {
		if _, err := req.ResponseBuf.Write(body); err != nil {
			return fmt.Errorf("capture OpenCode completion: %w", err)
		}
	}
	if translator.IsResponsesClient(ctx) {
		return jsonResponseAsResponses(ctx, w, body)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, err = w.Write(body)
	return err
}
