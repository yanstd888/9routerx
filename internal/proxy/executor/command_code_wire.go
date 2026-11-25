package executor

import (
	json "encoding/json/v2"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"

	"9routerx/proxy/internal/proxy"
)

const (
	commandCodeDefaultBaseURL = "https://api.commandcode.ai"
	commandCodeMaxTokens      = 200000
	commandCodeDefaultTokens  = 16384
)

var commandCodeClaudeTargets = map[string]bool{
	"claude-opus-4-7":            true,
	"claude-fable-5":             true,
	"claude-opus-4-6":            true,
	"claude-sonnet-5-5":          true,
	"claude-sonnet-4-6":          true,
	"claude-haiku-4-5-20251001": true,
}

func commandCodeBaseURL(req *Request) (string, error) {
	raw := ""
	if req.ConnData != nil {
		raw, _ = req.ConnData["baseUrl"].(string)
	}
	if strings.TrimSpace(raw) == "" && req.Config != nil {
		raw = req.Config.BaseURL
	}
	if strings.TrimSpace(raw) == "" {
		raw = os.Getenv("COMMAND_CODE_BASE_URL")
	}
	if strings.TrimSpace(raw) == "" {
		raw = commandCodeDefaultBaseURL
	}
	base := strings.TrimRight(strings.TrimSpace(raw), "/")
	for _, suffix := range []string{commandCodeChatPath, commandCodeMessagesPath, commandCodeResponsesPath, "/chat/completions"} {
		base = strings.TrimSuffix(base, suffix)
	}
	u, err := url.Parse(base)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return "", proxy.UpstreamFailure(http.StatusBadRequest, "Command Code base URL must be an http(s) host")
	}
	return base, nil
}

// commandCodeWireModel strips the provider prefix and maps the two bare ids
// the pin documents as vendor-prefixed upstream. Everything else is already
// an advertised upstream id and passes through unchanged.
func commandCodeWireModel(model string) string {
	trimmed := strings.TrimSpace(model)
	trimmed = strings.TrimPrefix(trimmed, "command-code/")
	trimmed = strings.TrimPrefix(trimmed, "cmd/")
	if trimmed == "" {
		return trimmed
	}
	if strings.Contains(trimmed, "/") {
		return trimmed
	}
	switch trimmed {
	case "mimo-v2.5":
		return "xiaomi/mimo-v2.5"
	case "mimo-v2.5-pro":
		return "xiaomi/mimo-v2.5-pro"
	}
	return trimmed
}

func commandCodeClaudeModel(model string) bool {
	return commandCodeClaudeTargets[commandCodeWireModel(model)]
}

func commandCodeBodyModel(body []byte) string {
	var parsed struct {
		Model string `json:"model"`
	}
	if json.Unmarshal(body, &parsed) != nil {
		return ""
	}
	return strings.TrimSpace(parsed.Model)
}

func commandCodeNormalizeBody(body []byte, model string, stream bool) ([]byte, int, error) {
	if len(strings.TrimSpace(string(body))) == 0 {
		return nil, 0, fmt.Errorf("empty body")
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil || payload == nil {
		return nil, 0, fmt.Errorf("decode JSON object")
	}
	payload["model"] = model
	payload["stream"] = stream
	tokens := commandCodeTokenCap(payload)
	delete(payload, "max_tokens")
	delete(payload, "max_completion_tokens")
	delete(payload, "max_output_tokens")
	payload["max_tokens"] = tokens
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, 0, fmt.Errorf("encode JSON object: %w", err)
	}
	return encoded, tokens, nil
}

// commandCodeTokenCap clamps only a client-supplied positive cap to the pinned
// 200000 ceiling. A missing or non-positive value gets the registry's
// reasoning-safe 16384 default instead of leaking a tiny caller budget into
// a model that spends it all on hidden thinking.
func commandCodeTokenCap(payload map[string]any) int {
	for _, key := range []string{"max_tokens", "max_completion_tokens", "max_output_tokens"} {
		value, ok := payload[key].(float64)
		if !ok || value <= 0 {
			continue
		}
		if value > commandCodeMaxTokens {
			return commandCodeMaxTokens
		}
		return int(value)
	}
	return commandCodeDefaultTokens
}
