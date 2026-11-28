package providers

// GHECopilotCatalogSource pins the independent Enterprise model catalog. GitHub's
// evolving public catalog is deliberately not reused or aliased here.
const GHECopilotCatalogSource = "OmniRoute/632d84f569f72893ed148e5b19dd05e2fff1a52a/open-sse/config/providers/registry/ghe-copilot/index.ts"

var gheCopilotModels = []string{
	"claude-fable-5", "claude-opus-5", "claude-opus-4.8-fast", "claude-opus-4.8", "claude-opus-4.7", "claude-sonnet-4.6", "claude-opus-4.5", "claude-sonnet-5", "claude-sonnet-5-5", "claude-sonnet-4.5", "claude-haiku-4.5",
	"gemini-3.1-pro-preview", "gemini-3.7-flash",
	"gpt-6.1-terra", "gpt-6.1-sol", "gpt-6-luna", "gpt-6-sol", "gpt-6-astra", "gpt-5.6-sol", "gpt-5.6-terra", "gpt-5.6-luna", "gpt-5.5", "gpt-5.4", "gpt-5.4-mini", "gpt-5.3-codex", "gpt-5-mini",
	"gpt-4o-2024-11-20", "gpt-4o-mini", "gpt-4-0125-preview", "kimi-k2.7-code", "mai-code-1-flash", "oswe-vscode-prime",
}

var gheCopilotFormats = map[string]ModelFormats{
	"claude-fable-5":         gheSingleFormat(FormatClaude),
	"claude-opus-5":          gheSingleFormat(FormatClaude),
	"claude-opus-4.8-fast":   gheSingleFormat(FormatClaude),
	"claude-opus-4.8":        gheSingleFormat(FormatClaude),
	"claude-opus-4.7":        gheSingleFormat(FormatClaude),
	"claude-sonnet-4.6":      gheSingleFormat(FormatClaude),
	"claude-opus-4.5":        gheSingleFormat(FormatClaude),
	"claude-sonnet-5":        gheSingleFormat(FormatClaude),
	"claude-sonnet-5-5":      gheSingleFormat(FormatClaude),
	"claude-sonnet-4.5":      gheSingleFormat(FormatClaude),
	"claude-haiku-4.5":       gheSingleFormat(FormatClaude),
	"gemini-3.1-pro-preview": gheSingleFormat(FormatOpenAI),
	"gemini-3.7-flash":       gheSingleFormat(FormatOpenAI),
	"gpt-6.1-terra":          gheSingleFormat(FormatOpenAIResponses),
	"gpt-6.1-sol":            gheSingleFormat(FormatOpenAIResponses),
	"gpt-6-luna":             gheSingleFormat(FormatOpenAIResponses),
	"gpt-6-sol":              gheSingleFormat(FormatOpenAIResponses),
	"gpt-6-astra":            gheSingleFormat(FormatOpenAIResponses),
	"gpt-5.6-sol":            gheSingleFormat(FormatOpenAIResponses),
	"gpt-5.6-terra":          gheSingleFormat(FormatOpenAIResponses),
	"gpt-5.6-luna":           gheSingleFormat(FormatOpenAIResponses),
	"gpt-5.5":                gheSingleFormat(FormatOpenAIResponses),
	"gpt-5.4":                gheSingleFormat(FormatOpenAIResponses),
	"gpt-5.4-mini":           gheSingleFormat(FormatOpenAIResponses),
	"gpt-5.3-codex":          gheSingleFormat(FormatOpenAIResponses),
	"gpt-5-mini":             gheSingleFormat(FormatOpenAIResponses),
	"gpt-4o-2024-11-20":      gheSingleFormat(FormatOpenAI),
	"gpt-4o-mini":            gheSingleFormat(FormatOpenAI),
	"gpt-4-0125-preview":     gheSingleFormat(FormatOpenAI),
	"kimi-k2.7-code":         gheSingleFormat(FormatOpenAI),
	"mai-code-1-flash":       gheSingleFormat(FormatOpenAIResponses),
	"oswe-vscode-prime":      gheSingleFormat(FormatOpenAIResponses),
}

func GHECopilotTargetFormat(model string, psd map[string]any) string {
	if formats, ok := psd["gheModelFormats"].(map[string]any); ok {
		if format, ok := formats[model].(string); ok && (format == FormatClaude || format == FormatOpenAI || format == FormatOpenAIResponses) {
			return format
		}
	}
	if format, ok := gheCopilotFormats[baseModelID(model)]; ok {
		return format.TargetFormat
	}
	return FormatOpenAI
}

func gheSingleFormat(format string) ModelFormats {
	return ModelFormats{TargetFormat: format, SupportedFormats: []string{format}}
}
