package executor

// Fallback Conol catalog from OmniRoute 632d84f5 services/conolModels.ts.
// Image upload and persisted session bindings are deliberately not implemented.
type conolModelData struct {
	ID      string
	Vision  bool
	Efforts []string
}

var conolModels = []conolModelData{
	{"claude-opus-5", true, []string{"low", "medium", "high", "xhigh"}},
	{"claude-opus-4-8", true, []string{"low", "medium", "high", "xhigh"}},
	{"claude-fable-5", true, []string{"low", "medium", "high", "xhigh"}},
	{"claude-sonnet-5", true, []string{"low", "medium", "high"}},
	{"claude-sonnet-4-6", true, []string{"low", "medium", "high"}},
	{"claude-haiku-4-5", true, []string{"minimal", "low", "medium", "high"}},
	{"gpt-5.5", true, []string{"low", "medium", "high", "xhigh"}},
	{"gpt-5.5-pro", true, []string{"medium", "high", "xhigh"}},
	{"gpt-5.6-sol", true, []string{"low", "medium", "high", "xhigh"}},
	{"gpt-5.6-terra", true, []string{"low", "medium", "high", "xhigh"}},
	{"gpt-5.6-luna", true, []string{"low", "medium", "high", "xhigh"}},
	{"deepseek/deepseek-v4-pro", false, []string{"high", "xhigh"}},
	{"z-ai/glm-5.2", false, []string{"minimal", "low", "medium", "high"}},
	{"tencent/hy3", false, []string{"minimal", "low", "medium", "high"}},
	{"moonshotai/kimi-k3", true, []string{"minimal", "low", "medium", "high"}},
	{"moonshotai/kimi-k2.7-code", true, []string{"minimal", "low", "medium", "high"}},
	{"qwen/qwen3.7-plus", true, []string{"minimal", "low", "medium", "high"}},
	{"qwen/qwen3.7-max", false, []string{"minimal", "low", "medium", "high"}},
	{"minimax/minimax-m3", true, []string{"minimal", "low", "medium", "high"}},
	{"stepfun/step-3.7-flash", true, []string{"minimal", "low", "medium", "high"}},
	{"google/gemini-3.7-flash", true, []string{"minimal", "low", "medium", "high"}},
	{"google/gemini-3.1-pro-preview", true, []string{"minimal", "low", "medium", "high"}},
	{"google/gemini-3.1-flash-lite", true, []string{"minimal", "low", "medium", "high"}},
	{"x-ai/grok-4.3", true, []string{"minimal", "low", "medium", "high"}},
	{"deepseek/deepseek-v4-flash", false, []string{"high", "xhigh"}},
	{"xiaomi/mimo-v2.5", true, []string{"minimal", "low", "medium", "high"}},
	{"xiaomi/mimo-v2.5-pro", false, []string{"minimal", "low", "medium", "high"}},
}
