// 9RouterX — ported from OmniRoute open-sse/executors/replicate.ts (pin 632d84f5).
package executor

import (
	"regexp"
	"strings"

	json "encoding/json/v2"
)

const replicateDefaultBaseURL = "https://api.replicate.com/v1"
const replicateWaitSeconds = 60

var replicateModelRE = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+(?::[A-Za-z0-9]+)?$`)

type replicateParsedModel struct {
	path    string
	version string
}

// parseReplicateModel accepts owner/name and owner/name:version. The charset
// check happens before any URL is built, so a traversal id never reaches the
// network.
func parseReplicateModel(model string) (replicateParsedModel, bool) {
	id := strings.TrimSpace(model)
	if !replicateModelRE.MatchString(id) {
		return replicateParsedModel{}, false
	}
	path, version, _ := strings.Cut(id, ":")
	return replicateParsedModel{path: path, version: version}, true
}

type replicateMessage struct {
	Role      string              `json:"role"`
	Content   any                 `json:"content"`
	ToolCalls []replicateToolCall `json:"tool_calls"`
}

type replicateToolCall struct {
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type replicateRequest struct {
	Messages            []replicateMessage `json:"messages"`
	MaxCompletionTokens *float64           `json:"max_completion_tokens"`
	MaxTokens           *float64           `json:"max_tokens"`
	Temperature         *float64           `json:"temperature"`
	TopP                *float64           `json:"top_p"`
}

type replicateInput struct {
	Prompt       string   `json:"prompt"`
	SystemPrompt string   `json:"system_prompt,omitempty"`
	Image        string   `json:"image,omitempty"`
	MaxTokens    *float64 `json:"max_tokens,omitempty"`
	Temperature  *float64 `json:"temperature,omitempty"`
	TopP         *float64 `json:"top_p,omitempty"`
}

type replicatePayload struct {
	Version *string        `json:"version,omitempty"`
	Stream  *bool          `json:"stream,omitempty"`
	Input   replicateInput `json:"input"`
}

type replicateBuilt struct {
	payload    replicatePayload
	promptText string
	maxTokens  *float64
}

func replicateMap(v any) map[string]any {
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return nil
}

func replicateExtractText(content any) string {
	if s, ok := content.(string); ok {
		return strings.TrimSpace(s)
	}
	parts, ok := content.([]any)
	if !ok {
		return ""
	}
	var kept []string
	for _, part := range parts {
		item := replicateMap(part)
		if item == nil {
			continue
		}
		kind, _ := item["type"].(string)
		if kind != "text" && kind != "input_text" {
			continue
		}
		text, _ := item["text"].(string)
		if strings.TrimSpace(text) != "" {
			kept = append(kept, text)
		}
	}
	return strings.TrimSpace(strings.Join(kept, "\n"))
}

func replicateImageFromPart(part any) string {
	item := replicateMap(part)
	if item == nil {
		return ""
	}
	kind, _ := item["type"].(string)
	if kind != "image_url" && kind != "input_image" {
		return ""
	}
	switch ref := item["image_url"].(type) {
	case string:
		return ref
	default:
		url, _ := replicateMap(ref)["url"].(string)
		return url
	}
}

func replicateFirstImage(messages []replicateMessage) string {
	for _, message := range messages {
		parts, ok := message.Content.([]any)
		if !ok {
			continue
		}
		for _, part := range parts {
			if url := replicateImageFromPart(part); url != "" {
				return url
			}
		}
	}
	return ""
}

func replicateToolCallsText(calls []replicateToolCall) string {
	var lines []string
	for _, call := range calls {
		if call.Function.Name == "" {
			continue
		}
		lines = append(lines, "[tool call "+call.Function.Name+"("+call.Function.Arguments+")]")
	}
	return strings.Join(lines, "\n")
}

type replicateTurn struct {
	label string
	text  string
	user  bool
}

func replicateToTurn(role, text string) replicateTurn {
	switch strings.ToLower(role) {
	case "assistant":
		return replicateTurn{label: "Assistant", text: text}
	case "tool", "function":
		return replicateTurn{label: "Tool result", text: text, user: true}
	default:
		return replicateTurn{label: "User", text: text, user: true}
	}
}

// flattenTurns sends a lone user turn verbatim; history becomes a labelled
// transcript ending in an "Assistant:" prompt when the client's last turn was
// a user turn.
func replicateFlattenTurns(turns []replicateTurn) string {
	if len(turns) == 1 && turns[0].label == "User" {
		return turns[0].text
	}
	lines := make([]string, 0, len(turns)+1)
	for _, turn := range turns {
		lines = append(lines, turn.label+": "+turn.text)
	}
	if len(turns) > 0 && turns[len(turns)-1].user {
		lines = append(lines, "Assistant:")
	}
	return strings.Join(lines, "\n")
}

func replicateResolvePrompt(messages []replicateMessage) (string, string) {
	var systemParts []string
	var turns []replicateTurn
	for _, message := range messages {
		text := []string{replicateExtractText(message.Content), replicateToolCallsText(message.ToolCalls)}
		combined := strings.TrimSpace(strings.Join(textNonEmpty(text), "\n"))
		if combined == "" {
			continue
		}
		role := message.Role
		if role == "" {
			role = "user"
		}
		if strings.EqualFold(role, "system") || strings.EqualFold(role, "developer") {
			systemParts = append(systemParts, combined)
			continue
		}
		turns = append(turns, replicateToTurn(role, combined))
	}
	return replicateFlattenTurns(turns), strings.Join(systemParts, "\n\n")
}

func textNonEmpty(values []string) []string {
	var kept []string
	for _, value := range values {
		if value != "" {
			kept = append(kept, value)
		}
	}
	return kept
}

func replicateBuildPayload(body []byte, parsed replicateParsedModel, stream bool) (replicateBuilt, bool) {
	var source replicateRequest
	if err := json.Unmarshal(body, &source); err != nil {
		return replicateBuilt{}, false
	}
	prompt, systemPrompt := replicateResolvePrompt(source.Messages)
	if prompt == "" {
		return replicateBuilt{}, false
	}
	maxTokens := source.MaxTokens
	if source.MaxCompletionTokens != nil {
		maxTokens = source.MaxCompletionTokens
	}
	built := replicateBuilt{
		promptText: prompt,
		maxTokens:  maxTokens,
		payload: replicatePayload{
			Input: replicateInput{
				Prompt:       prompt,
				SystemPrompt: systemPrompt,
				Image:        replicateFirstImage(source.Messages),
				MaxTokens:    maxTokens,
				Temperature:  source.Temperature,
				TopP:         source.TopP,
			},
		},
	}
	if parsed.version != "" {
		version := parsed.version
		built.payload.Version = &version
	}
	if stream {
		yes := true
		built.payload.Stream = &yes
	}
	return built, true
}

func replicateModelFromBody(body []byte) string {
	var probe struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(body, &probe); err != nil {
		return ""
	}
	return probe.Model
}
