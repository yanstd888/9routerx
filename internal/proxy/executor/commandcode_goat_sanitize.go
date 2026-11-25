package executor

import (
	json "encoding/json/v2"
	"fmt"
)

// sanitizeCommandcodeGoatChatBody cleans up OpenAI chat request bodies before sending to
// /provider/v1/chat/completions. The Command Code Provider API strictly validates message
// content: null or empty-array content on user/tool/system roles returns HTTP 400
// ("Invalid input: messages.N.content"), and foreign block types (like tool_result from
// mixed-combo translation) must be serialized to plain text.
func sanitizeCommandcodeGoatChatBody(body []byte) []byte {
	var reqMap map[string]any
	if err := json.Unmarshal(body, &reqMap); err != nil {
		return body
	}
	msgs, ok := reqMap["messages"].([]any)
	if !ok || len(msgs) == 0 {
		return body
	}
	changed := false
	for i, m := range msgs {
		msgMap, ok := m.(map[string]any)
		if !ok {
			continue
		}
		role, _ := msgMap["role"].(string)
		content := msgMap["content"]

		if content == nil {
			if role == "tool" || role == "user" || role == "system" {
				msgMap["content"] = ""
				msgs[i] = msgMap
				changed = true
			}
		} else if arr, ok := content.([]any); ok {
			if len(arr) == 0 {
				msgMap["content"] = ""
				msgs[i] = msgMap
				changed = true
			} else {
				cleanParts := make([]any, 0, len(arr))
				partChanged := false
				for _, part := range arr {
					switch p := part.(type) {
					case string:
						cleanParts = append(cleanParts, map[string]any{"type": "text", "text": p})
						partChanged = true
					case map[string]any:
						pType, _ := p["type"].(string)
						if pType == "text" || pType == "image_url" {
							cleanParts = append(cleanParts, p)
						} else if pType == "tool_result" {
							txt, _ := p["content"].(string)
							cleanParts = append(cleanParts, map[string]any{"type": "text", "text": txt})
							partChanged = true
						} else if txt, ok := p["text"].(string); ok {
							cleanParts = append(cleanParts, map[string]any{"type": "text", "text": txt})
							partChanged = true
						} else {
							b, _ := json.Marshal(p)
							cleanParts = append(cleanParts, map[string]any{"type": "text", "text": string(b)})
							partChanged = true
						}
					default:
						cleanParts = append(cleanParts, map[string]any{"type": "text", "text": fmt.Sprintf("%v", p)})
						partChanged = true
					}
				}
				if len(cleanParts) == 0 {
					msgMap["content"] = ""
					msgs[i] = msgMap
					changed = true
				} else if partChanged {
					msgMap["content"] = cleanParts
					msgs[i] = msgMap
					changed = true
				}
			}
		}
	}
	if !changed {
		return body
	}
	reqMap["messages"] = msgs
	out, err := json.Marshal(reqMap)
	if err != nil {
		return body
	}
	return out
}
