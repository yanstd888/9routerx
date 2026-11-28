package executor

import (
	"bytes"
	json "encoding/json/v2"
	"fmt"
	"io"
	"net/http"
	"sort"

	"9routerx/proxy/internal/proxy"
	"9routerx/proxy/internal/translator"
)

func gheNativeMessagesStream(w http.ResponseWriter, req *Request, upstream io.Reader) error {
	proxy.WriteSSEHeaders(w)
	flusher, _ := w.(http.Flusher)
	var writeErr, eventErr error
	terminal := false
	usage := map[string]any{}
	err := proxy.ScanStreamEvents(upstream, func(event proxy.SSEEvent) bool {
		if req.Ctx.Err() != nil {
			writeErr = req.Ctx.Err()
			return false
		}
		if bytes.Equal(bytes.TrimSpace(event.Data), []byte("[DONE]")) {
			return true
		}
		var payload struct {
			Type    string `json:"type"`
			Message struct {
				Usage map[string]any `json:"usage"`
			} `json:"message"`
			Usage map[string]any `json:"usage"`
		}
		if json.Unmarshal(event.Data, &payload) == nil {
			for key, value := range payload.Message.Usage {
				usage[key] = value
			}
			for key, value := range payload.Usage {
				usage[key] = value
			}
			if payload.Type == "message_stop" {
				terminal = true
			}
			if payload.Type == "error" {
				eventErr = proxy.UpstreamFailure(http.StatusBadGateway, "GHE Messages upstream error")
			}
		}
		frame := []byte{}
		if event.Event != "" {
			frame = fmt.Appendf(frame, "event: %s\n", event.Event)
		}
		frame = fmt.Appendf(frame, "data: %s\n\n", event.Data)
		if req.ResponseBuf != nil {
			_, _ = req.ResponseBuf.Write(frame)
		}
		_, writeErr = w.Write(frame)
		if flusher != nil {
			flusher.Flush()
		}
		return writeErr == nil && eventErr == nil
	})
	if writeErr != nil {
		return writeErr
	}
	if eventErr != nil {
		return eventErr
	}
	if err != nil {
		return err
	}
	if !terminal {
		return proxy.UpstreamFailure(http.StatusBadGateway, "GHE Messages stream ended before message_stop")
	}
	capture, _ := json.Marshal(map[string]any{"type": "message", "usage": usage})
	if parsed := translator.ParseResponseUsage(capture); parsed != nil {
		translator.SetUsage(req.Ctx, parsed)
	}
	return nil
}

type gheMessageBlock struct {
	fields    map[string]any
	text      bytes.Buffer
	thinking  bytes.Buffer
	input     bytes.Buffer
	signature bytes.Buffer
}

// gheAggregateMessages preserves native thinking/signature/tool blocks and cache
// usage, instead of round-tripping a native client through Chat Completions.
func gheAggregateMessages(raw []byte) ([]byte, error) {
	message := map[string]any{}
	usage := map[string]any{}
	blocks := map[int]*gheMessageBlock{}
	terminal := false
	var eventErr error
	err := proxy.ScanStreamEvents(bytes.NewReader(raw), func(ev proxy.SSEEvent) bool {
		var event struct {
			Type    string         `json:"type"`
			Index   int            `json:"index"`
			Message map[string]any `json:"message"`
			Block   map[string]any `json:"content_block"`
			Delta   map[string]any `json:"delta"`
			Usage   map[string]any `json:"usage"`
		}
		if json.Unmarshal(ev.Data, &event) != nil {
			return true
		}
		switch event.Type {
		case "error":
			eventErr = &proxy.UpstreamError{StatusCode: http.StatusBadGateway, Body: append([]byte(nil), ev.Data...)}
			return false
		case "message_start":
			message = event.Message
			if existing, ok := message["usage"].(map[string]any); ok {
				usage = existing
			}
		case "content_block_start":
			block := &gheMessageBlock{fields: event.Block}
			if block.fields == nil {
				block.fields = map[string]any{}
			}
			if text, ok := block.fields["text"].(string); ok {
				block.text.WriteString(text)
			}
			if text, ok := block.fields["thinking"].(string); ok {
				block.thinking.WriteString(text)
			}
			if text, ok := block.fields["signature"].(string); ok {
				block.signature.WriteString(text)
			}
			blocks[event.Index] = block
		case "content_block_delta":
			block := blocks[event.Index]
			if block == nil {
				eventErr = proxy.UpstreamFailure(http.StatusBadGateway, "GHE Messages delta has no content block")
				return false
			}
			switch event.Delta["type"] {
			case "text_delta":
				text, _ := event.Delta["text"].(string)
				block.text.WriteString(text)
			case "thinking_delta":
				text, _ := event.Delta["thinking"].(string)
				block.thinking.WriteString(text)
			case "signature_delta":
				text, _ := event.Delta["signature"].(string)
				block.signature.WriteString(text)
			case "input_json_delta":
				text, _ := event.Delta["partial_json"].(string)
				block.input.WriteString(text)
			}
		case "message_delta":
			for key, value := range event.Delta {
				message[key] = value
			}
			for key, value := range event.Usage {
				usage[key] = value
			}
		case "message_stop":
			terminal = true
		}
		return true
	})
	if err != nil {
		return nil, err
	}
	if eventErr != nil {
		return nil, eventErr
	}
	if !terminal || message["id"] == nil || len(blocks) == 0 {
		return nil, proxy.UpstreamFailure(http.StatusBadGateway, proxy.NoCompletionInStream)
	}
	order := make([]int, 0, len(blocks))
	for index := range blocks {
		order = append(order, index)
	}
	sort.Ints(order)
	content := make([]any, 0, len(order))
	for _, index := range order {
		block := blocks[index]
		if block.fields["type"] == "text" {
			block.fields["text"] = block.text.String()
		}
		if block.fields["type"] == "thinking" {
			block.fields["thinking"] = block.thinking.String()
			block.fields["signature"] = block.signature.String()
		}
		if block.input.Len() > 0 {
			var input any
			if err := json.Unmarshal(block.input.Bytes(), &input); err != nil {
				return nil, fmt.Errorf("GHE tool input: %w", err)
			}
			block.fields["input"] = input
		}
		content = append(content, block.fields)
	}
	message["content"], message["usage"] = content, usage
	message["type"], message["role"] = "message", "assistant"
	return json.Marshal(message)
}
