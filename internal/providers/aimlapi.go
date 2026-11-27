// 9RouterX — source port and modifications by https://github.com/yanstd888/ — https://t.me/aiclub_id
package providers

// aimlapiChatType is the `type` AI/ML API's /v1/models gives a model served on
// the OpenAI chat-completions route. The same catalogue also lists image,
// video, speech, Responses and Anthropic Messages rows (one row per route, so
// a model id can repeat under several types).
const aimlapiChatType = "openai/chat-completions"

// FilterAimlapiChatModels keeps the /v1/models rows the chat lane can route:
// the openai/chat-completions ones, each id once. Without it "Check Latest
// Models" would offer video and image generators as chat seats.
func FilterAimlapiChatModels(items []any) []any {
	out := make([]any, 0, len(items))
	seen := map[string]bool{}
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if t, _ := m["type"].(string); t != aimlapiChatType {
			continue
		}
		id, _ := m["id"].(string)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, m)
	}
	return out
}
