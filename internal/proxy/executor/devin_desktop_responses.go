package executor

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"net/http"

	"9routerx/proxy/internal/guardrails"
	"9routerx/proxy/internal/proxy"
	"9routerx/proxy/internal/translator"
)

type devinDesktopResponsesBridge struct {
	state   *translator.ResponsesState
	indexes map[string]int
	items   []map[string]any
}

func newDevinDesktopResponsesBridge(model string, names []string) *devinDesktopResponsesBridge {
	state := translator.InitResponsesState(model, true)
	state.SetCustomToolNames(names)
	return &devinDesktopResponsesBridge{state: state, indexes: make(map[string]int)}
}

func (b *devinDesktopResponsesBridge) feed(chunk *translator.OpenAIChunk) []translator.ResponsesEvent {
	events := translator.TranslateOpenAIToResponses(chunk, b.state)
	for _, event := range events {
		b.reindex(event)
	}
	return events
}

func (b *devinDesktopResponsesBridge) reindex(event translator.ResponsesEvent) {
	// The shared translator uses choice/tool index zero for several item kinds.
	// Reindex its events by item ID before they leave this native transport.
	item, _ := event.Data["item"].(map[string]any)
	id, _ := event.Data["item_id"].(string)
	if item != nil {
		id, _ = item["id"].(string)
	}
	if id != "" {
		index, exists := b.indexes[id]
		if !exists {
			index = len(b.items)
			b.indexes[id] = index
			b.items = append(b.items, nil)
		}
		event.Data["output_index"] = index
		if event.Event == "response.output_item.done" {
			b.items[index] = item
		}
	}
	if event.Event == "response.completed" || event.Event == "response.incomplete" {
		if response, ok := event.Data["response"].(map[string]any); ok {
			items := make([]map[string]any, 0, len(b.items))
			for _, item := range b.items {
				if item != nil {
					items = append(items, item)
				}
			}
			response["output"] = items
		}
	}
}

func devinDesktopResponsesJSON(ctx context.Context, w http.ResponseWriter, req *Request, body []byte) error {
	if err := proxy.EmptyUpstreamError(body); err != nil {
		return err
	}
	filtered, err := guardrails.ApplyBuffered(ctx, body)
	if err != nil {
		return guardrailBlockFailure(err)
	}
	var chat translator.OpenAIResponse
	if err := json.Unmarshal(filtered, &chat); err != nil {
		return fmt.Errorf("devin desktop Responses completion: %w", err)
	}
	if len(chat.Choices) != 1 {
		return errors.New("devin desktop completion has no choice")
	}
	b := newDevinDesktopResponsesBridge(chat.Model, translator.CustomToolNamesFrom(ctx))
	choice := chat.Choices[0]
	makeChunk := func(delta translator.OpenAIDelta) *translator.OpenAIChunk {
		return &translator.OpenAIChunk{ID: chat.ID, Model: chat.Model, Choices: []translator.OpenAIChoice{{Delta: delta}}}
	}
	b.feed(makeChunk(translator.OpenAIDelta{Role: "assistant", ReasoningContent: choice.Message.ReasoningContent}))
	b.feed(makeChunk(translator.OpenAIDelta{Content: choice.Message.Content}))
	b.feed(makeChunk(translator.OpenAIDelta{ToolCalls: choice.Message.ToolCalls}))
	terminal := makeChunk(translator.OpenAIDelta{})
	terminal.Usage, terminal.Choices[0].FinishReason = chat.Usage, choice.FinishReason
	events := append(b.feed(terminal), b.feed(nil)...)
	for _, event := range events {
		if event.Event != "response.completed" && event.Event != "response.incomplete" {
			continue
		}
		encoded, err := json.Marshal(event.Data["response"])
		if err != nil {
			return fmt.Errorf("marshal Devin Responses object: %w", err)
		}
		if req.ResponseBuf != nil {
			if _, err := req.ResponseBuf.Write(filtered); err != nil {
				return fmt.Errorf("capture Devin response: %w", err)
			}
		}
		if usage := translator.ParseResponseUsage(filtered); usage != nil {
			translator.SetUsage(ctx, usage)
		}
		w.Header().Set("Content-Type", "application/json")
		_, err = w.Write(encoded)
		return err
	}
	return proxy.UpstreamFailure(http.StatusBadGateway, "Devin Desktop response could not be translated")
}
