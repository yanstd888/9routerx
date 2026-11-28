package executor

// Fallback SYNTX catalog from OmniRoute 632d84f5 services/syntxModels.ts.
// Image upload and session rollover are deliberately not implemented.
type syntxModelData struct {
	ID            string
	Name          string
	AIName        string
	Vision        bool
	Thinking      bool
	ContextWindow int
	MaxOutput     int
}

var syntxModels = []syntxModelData{
	{"claude-fable-5-1", "Claude Fable 5.1", "claude", true, true, 2000000, 20000},
	{"claude-haiku-4-5-20251001", "Claude Haiku 4.5", "claude", true, true, 256000, 16000},
	{"claude-opus-5", "Claude Opus 5", "claude", true, true, 356000, 128000},
	{"claude-fable-5", "Claude Fable 5", "claude", true, true, 256000, 32000},
	{"claude-opus-4-6", "Claude Opus 4.6", "claude", true, true, 256000, 32000},
	{"claude-opus-4-7", "Claude Opus 4.7", "claude", true, true, 256000, 32000},
	{"claude-opus-4-8", "Claude Opus 4.8", "claude", true, true, 256000, 32000},
	{"claude-sonnet-4-6", "Claude Sonnet 4.6", "claude", true, true, 256000, 32000},
	{"claude-sonnet-5", "Claude Sonnet 5", "claude", true, true, 256000, 32000},
	{"qwen3-max-2026-01-23", "Qwen 3 Max", "qwen", false, true, 356000, 65536},
	{"qwen3.7-max", "Qwen 3.7 Max", "qwen", false, true, 356000, 65536},
	{"qwen3.7-plus", "Qwen 3.7 Plus", "qwen", false, true, 356000, 65536},
	{"deepseek-r1", "DeepSeek R1", "deepseek", false, true, 356000, 384000},
	{"deepseek-v3", "DeepSeek V3", "deepseek", false, false, 356000, 384000},
	{"gemini-3.1-pro-preview", "Gemini 3.1 Pro", "gemini", true, true, 356000, 65536},
	{"gemini-3.5-flash", "Gemini 3.5 Flash", "gemini", true, true, 356000, 65536},
	{"gemini-3.5-flash-lite", "Gemini 3.5 Flash Lite", "gemini", true, true, 356000, 65536},
	{"gemini-3.6-flash", "Gemini 3.6 Flash", "gemini", true, true, 356000, 65536},
	{"gemini-3.7-flash", "Gemini 3.7 Flash", "gemini", true, true, 200000, 12000},
	{"gemini-3.8-flash", "Gemini 3.8 Flash", "gemini", true, true, 200000, 12000},
	{"gpt-5-mini-2025-08-07", "GPT-5 Mini", "chatgpt", true, true, 356000, 128000},
	{"gpt-5-nano-2025-08-07", "GPT-5 Nano", "chatgpt", true, true, 356000, 128000},
	{"gpt-5.6-luna", "GPT 5.6 Luna", "chatgpt", true, true, 356000, 128000},
	{"gpt-6-astra", "GPT 6 Astra", "chatgpt", true, true, 356000, 128000},
	{"gpt-5.6-terra", "GPT 5.6 Terra", "chatgpt", true, true, 356000, 128000},
	{"gpt-5.6-sol", "GPT 5.6 Sol", "chatgpt", true, true, 356000, 128000},
	{"gpt-5.2", "GPT 5.2", "chatgpt", true, true, 356000, 128000},
	{"gpt-5.4", "GPT 5.4", "chatgpt", true, true, 356000, 128000},
	{"gpt-5.5", "GPT 5.5", "chatgpt", true, true, 356000, 128000},
	{"sonar", "Perplexity Sonar + Internet", "perplexity", false, false, 128000, 8192},
	{"grok-4.6", "Grok 4.6", "grok", false, true, 356000, 32800},
	{"grok-4.3", "Grok 4.3", "grok", true, true, 356000, 131072},
	{"grok-4.5", "Grok 4.5", "grok", true, true, 356000, 32768},
	{"glm-5.2", "GLM 5.2", "zai", false, true, 356000, 128000},
}
