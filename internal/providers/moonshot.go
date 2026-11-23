package providers

import "strings"

// The Moonshot API catalog is independent of the api.kimi.com coding transport.
// Source: OmniRoute 632d84f5 registry/moonshot/index.ts.
var moonshotCapabilities = map[string]Capabilities{
	"kimi-k3.1-pro":            moonshotCaps(1048576, true, false),
	"kimi-k3.1-pro-highspeed":  moonshotCaps(1048576, true, false),
	"kimi-k3":                  moonshotCaps(1048576, false, false),
	"k3":                       moonshotCaps(1048576, false, false),
	"k3-256k":                  moonshotCaps(1048576, false, false),
	"kimi-k2.7-code":           moonshotCaps(262144, false, false),
	"kimi-k2.7-code-highspeed": moonshotCaps(262144, false, false),
	"kimi-k2.6":                moonshotCaps(262144, false, true),
}

func moonshotCaps(limit int, video, canDisable bool) Capabilities {
	return Capabilities{
		Vision: true, VideoInput: video, Reasoning: true, Tools: true,
		ThinkingFormat: "kimi", ThinkingCanDisable: &canDisable,
		ContextWindow: limit, MaxOutput: limit,
	}
}

var moonshotModelFormats = map[string]ModelFormats{
	"kimi-k3.1-pro":            moonshotChatFormat(),
	"kimi-k3.1-pro-highspeed":  moonshotChatFormat(),
	"kimi-k3":                  moonshotChatFormat(),
	"k3":                       moonshotChatFormat(),
	"k3-256k":                  moonshotChatFormat(),
	"kimi-k2.7-code":           moonshotChatFormat(),
	"kimi-k2.7-code-highspeed": moonshotChatFormat(),
	"kimi-k2.6":                moonshotChatFormat(),
}

func moonshotChatFormat() ModelFormats {
	return ModelFormats{TargetFormat: FormatOpenAI, SupportedFormats: []string{FormatOpenAI}}
}

func moonshotThinkingLevels(model string) []string {
	model = strings.TrimPrefix(strings.ToLower(model), "moonshot/")
	model, _ = splitThinkingSuffix(model)
	switch model {
	case "kimi-k3.1-pro", "kimi-k3.1-pro-highspeed":
		return []string{"low", "high", "max"}
	case "kimi-k3", "k3", "k3-256k":
		return []string{"max"}
	case "kimi-k2.7-code", "kimi-k2.7-code-highspeed":
		return []string{"thinking"}
	case "kimi-k2.6":
		return []string{"none", "thinking"}
	default:
		return nil
	}
}
