package chat

import (
	"context"
	json "encoding/json/v2"

	"9routerx/proxy/internal/proxy/executor"
	"9routerx/proxy/internal/thinking"
	"9routerx/proxy/internal/translator"
)

func sanitizeProviderTools(body []byte, provider string) ([]byte, error) {
	if provider == "moonshot" {
		return executor.SanitizeMoonshotTools(body)
	}
	return translator.SanitizeOpenAITools(body)
}

// Moonshot needs the original effort value, including null and invalid strings.
func (h *ChatHandler) prepareMoonshotThinking(ctx context.Context, model string, body []byte, suffix *thinking.Intent) ([]byte, string, *thinking.Intent) {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil || payload == nil {
		return body, model, suffix
	}
	changed := false
	if suffix != nil {
		effort := suffix.Level
		if suffix.Mode == thinking.ModeNone {
			effort = "none"
		} else if effort == "" {
			effort = thinking.ToKimiReasoningEffort(suffix)
		}
		payload["reasoning_effort"] = effort
		changed = true
	}
	if h.injectProviderThinkingMode(h.providerThinkingMode(ctx, "moonshot"), payload, false, "moonshot", model) {
		changed = true
	}
	if !changed {
		return body, model, nil
	}
	out, err := json.Marshal(payload, json.Deterministic(true))
	if err != nil {
		return body, model, nil
	}
	return out, model, nil
}

type moonshotOriginalKey struct{}
type moonshotOriginal struct {
	body   string
	claude bool
}

// Strings keep the original carrier immutable across account/combo attempts.
func withMoonshotOriginal(ctx context.Context, body []byte, claude bool) context.Context {
	return context.WithValue(ctx, moonshotOriginalKey{}, moonshotOriginal{body: string(body), claude: claude})
}

func restoreMoonshotOriginal(ctx context.Context, prepared []byte) []byte {
	original, ok := ctx.Value(moonshotOriginalKey{}).(moonshotOriginal)
	if !ok {
		return prepared
	}
	return preserveMoonshotIntent([]byte(original.body), prepared, original.claude)
}

func preserveMoonshotClaudeIntent(original, prepared []byte) []byte {
	return preserveMoonshotIntent(original, prepared, true)
}

func preserveMoonshotIntent(original, prepared []byte, claude bool) []byte {
	var source, target map[string]any
	if json.Unmarshal(original, &source) != nil || json.Unmarshal(prepared, &target) != nil {
		return prepared
	}
	changed := false
	if claude {
		changed = restoreMoonshotClaudeMessages(source, target)
	}
	for _, key := range []string{"reasoning_effort", "reasoning", "thinking", "enable_thinking"} {
		if value, exists := source[key]; exists {
			target[key] = value
			changed = true
		}
	}
	_, explicit := source["reasoning_effort"]
	reasoning, _ := source["reasoning"].(map[string]any)
	_, reasoningEffort := reasoning["effort"]
	thinking, _ := source["thinking"].(map[string]any)
	_, thinkingEffort := thinking["effort"]
	if !explicit && (reasoningEffort || thinkingEffort || source["thinking"] != nil) {
		delete(target, "reasoning_effort")
	}
	// output_config is a Claude-only carrier, lower priority than native fields.
	if !explicit && !reasoningEffort && !thinkingEffort {
		if config, ok := source["output_config"].(map[string]any); ok {
			if effort, exists := config["effort"]; exists {
				target["reasoning_effort"] = effort
				changed = true
			}
		}
	}
	if !changed {
		return prepared
	}
	out, err := json.Marshal(target, json.Deterministic(true))
	if err != nil {
		return prepared
	}
	return out
}
