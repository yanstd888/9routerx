package executor

import (
	json "encoding/json/v2"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type devinDesktopOutput struct {
	id            string
	model         string
	created       int64
	role          bool
	stop          uint64
	usage         *devinDesktopUsage
	indexes       map[string]int
	last          int
	orphan        string
	identityBytes int
	collected     devinDesktopCollected
}

type devinDesktopChunk struct {
	ID      string                    `json:"id"`
	Object  string                    `json:"object"`
	Created int64                     `json:"created"`
	Model   string                    `json:"model"`
	Choices []devinDesktopChunkChoice `json:"choices"`
	Usage   *devinDesktopUsage        `json:"usage,omitempty"`
}

type devinDesktopChunkChoice struct {
	Index        int               `json:"index"`
	Delta        devinDesktopDelta `json:"delta"`
	FinishReason *string           `json:"finish_reason"`
}

func newDevinDesktopOutput(model string) *devinDesktopOutput {
	return &devinDesktopOutput{
		id: "chatcmpl-devin-desktop-" + uuid.NewString(), model: model, created: time.Now().Unix(),
		indexes: make(map[string]int), last: -1,
	}
}

func (o *devinDesktopOutput) consume(response devinDesktopResponse, emit func(devinDesktopDelta) error) error {
	if response.stop != 0 {
		o.stop = response.stop
	}
	if response.usage != nil {
		o.usage = response.usage
	}
	if !o.role && (response.text != "" || response.thinking != "" || len(response.calls) > 0) {
		o.role = true
		if err := emit(devinDesktopDelta{Role: "assistant"}); err != nil {
			return err
		}
	}
	if response.thinking != "" {
		if err := emit(devinDesktopDelta{Thinking: response.thinking}); err != nil {
			return err
		}
	}
	if response.text != "" {
		if err := emit(devinDesktopDelta{Content: response.text}); err != nil {
			return err
		}
	}
	for _, fragment := range response.calls {
		call, err := o.call(fragment)
		if err != nil {
			return err
		}
		if call != nil {
			if err := emit(devinDesktopDelta{ToolCalls: []devinDesktopDeltaCall{*call}}); err != nil {
				return err
			}
		}
	}
	return nil
}

func (o *devinDesktopOutput) call(fragment devinDesktopCallFragment) (*devinDesktopDeltaCall, error) {
	if fragment.id == "" && o.last < 0 {
		if len(o.orphan)+len(fragment.arguments) > devinDesktopFrameLimit {
			return nil, errors.New("orphan tool arguments exceed safety limit")
		}
		o.orphan += fragment.arguments
		return nil, nil
	}
	index, exists := o.indexes[fragment.id]
	first := fragment.id != "" && !exists
	if fragment.id == "" {
		index = o.last
	}
	if first {
		if len(o.indexes) >= 4096 || o.identityBytes+len(fragment.id) > devinDesktopAuthLimit {
			return nil, errors.New("tool identities exceed safety limit")
		}
		index = len(o.indexes)
		o.indexes[fragment.id] = index
		o.identityBytes += len(fragment.id)
	}
	if fragment.id != "" {
		o.last = index
	}
	call := &devinDesktopDeltaCall{Index: index, Function: devinDesktopDeltaFunction{Name: fragment.name, Arguments: fragment.arguments}}
	if first {
		call.ID, call.Type = fragment.id, "function"
		call.Function.Arguments = o.orphan + call.Function.Arguments
		o.orphan = ""
	}
	return call, nil
}

func (o *devinDesktopOutput) chunk(delta devinDesktopDelta, final bool) ([]byte, error) {
	chunk := devinDesktopChunk{ID: o.id, Object: "chat.completion.chunk", Created: o.created, Model: o.model,
		Choices: []devinDesktopChunkChoice{{Delta: delta}}}
	if final {
		reason := devinDesktopFinish(o.stop, len(o.indexes) > 0)
		chunk.Choices[0].FinishReason, chunk.Usage = &reason, o.usage
	}
	return json.Marshal(chunk)
}

func (o *devinDesktopOutput) collect(response devinDesktopResponse) error {
	return o.consume(response, func(delta devinDesktopDelta) error {
		size := len(delta.Content) + len(delta.Thinking)
		for _, call := range delta.ToolCalls {
			size += len(call.ID) + len(call.Function.Name) + len(call.Function.Arguments)
		}
		if o.collected.bytes+size > devinDesktopFrameLimit {
			return errors.New("completion exceeds safety limit")
		}
		o.collected.bytes += size
		o.collected.text.WriteString(delta.Content)
		o.collected.thinking.WriteString(delta.Thinking)
		for _, call := range delta.ToolCalls {
			if call.Index == len(o.collected.calls) {
				o.collected.calls = append(o.collected.calls, devinDesktopCollectedCall{ID: call.ID, Type: "function"})
			}
			if call.Index < 0 || call.Index >= len(o.collected.calls) {
				return errors.New("invalid tool call index")
			}
			fn := &o.collected.calls[call.Index].Function
			fn.Name += call.Function.Name
			fn.Arguments += call.Function.Arguments
		}
		return nil
	})
}

func (o *devinDesktopOutput) json() ([]byte, error) {
	type message struct {
		Role     string                      `json:"role"`
		Content  *string                     `json:"content"`
		Thinking string                      `json:"reasoning_content,omitempty"`
		Calls    []devinDesktopCollectedCall `json:"tool_calls,omitempty"`
	}
	type choice struct {
		Index        int     `json:"index"`
		Message      message `json:"message"`
		FinishReason string  `json:"finish_reason"`
	}
	text := o.collected.text.String()
	msg := message{Role: "assistant", Content: &text, Thinking: o.collected.thinking.String(), Calls: o.collected.calls}
	if text == "" && len(msg.Calls) > 0 {
		msg.Content = nil
	}
	body := struct {
		ID      string             `json:"id"`
		Object  string             `json:"object"`
		Created int64              `json:"created"`
		Model   string             `json:"model"`
		Choices []choice           `json:"choices"`
		Usage   *devinDesktopUsage `json:"usage,omitempty"`
	}{o.id, "chat.completion", o.created, o.model, []choice{{Message: msg, FinishReason: devinDesktopFinish(o.stop, len(o.indexes) > 0)}}, o.usage}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal completion: %w", err)
	}
	return payload, nil
}
