// 9RouterX — derived from the work of https://github.com/yanstd888/ — port and modifications by the maintainers — https://t.me/aiclub_id
package providers

// ProviderAliasMap maps short aliases to canonical provider IDs.
//
// Three aliases resolve to weblogin lanes that are runtime-only and not
// listed in the static registry (so a registry-vs-alias diff shows them as
// dangling, by design): "cweb" → chatgpt-web, "uc" → uc-web, and
// "veo-free" → veoaifree-web. ResolveAlias returns unknown aliases unchanged,
// and the web executor for each of these targets ships in the weblogin layer.
var ProviderAliasMap = map[string]string{
	"veap": "volcengine-agent-plan",
	"vecp": "volcengine-coding-plan",
	"agnescn": "agnes-cn",
	"ali-cn": "alibaba-cn",
	"b-ai":           "bai",
	"agnes-ai":       "agnes",
	"atria-asi":      "atria",
	"dahl-inference": "dahl",
	"thh":            "tokenharbor",
	"th":             "tokenharbor",
	"aai":            "assemblyai",
	"ag":             "antigravity",
	"ali":            "alicode",
	"ali-tp":         "alitp-intl",
	"alii":           "alicode-intl",
	"alitp":          "alitp-intl",
	"ant":            "anthropic",
	"ark":            "volcengine-ark",
	"az":             "azure",
	"bb":             "blackbox",
	"bfl":            "black-forest-labs",
	"bpm":            "byteplus",
	"brave":          "brave-search",
	"br":             "bedrock",
	"aws-bedrock":    "bedrock",
	"brx":            "bedrock-xai",
	"bedrock-grok":   "bedrock-xai",
	"cb":             "cerebras",
	"cbai":           "codebuddy-intl",
	"cc":             "claude",
	"cd":             "codebuddy-cn",
	"cf":             "cloudflare-ai",
	"ch":             "chutes",
	"cl":             "cline",
	"cmd":            "command-code",
	"nlpc":           "nlpcloud",
	"cmc-goat":       "commandcode-goat",
	"cmc":            "commandcode",
	"cp":             "clinepass",
	"cu":             "cursor",
	"cx":             "codex",
	"dg":             "deepgram",
	"ds":             "deepseek",
	"ds-web":         "deepseek-web",
	"sp":             "sumopod",
	"el":             "elevenlabs",
	"fal":            "fal-ai",
	"fb":             "freebuff",
	"fish":           "fish-audio",
	"fl":             "featherless",
	"fw":             "fireworks",
	"gb":             "grok-cli",
	"gc":             "gemini-cli",
	"gweb":           "gemini-web",
	"gcli":           "grok-cli",
	"cweb":           "chatgpt-web",
	"gh":             "github",
	"gl":             "gitlab",
	"glmcn":          "glm-cn",
	"gpse":           "google-pse",
	"gq":             "groq",
	"grok-build":     "grok-cli",
		"gw":             "grok-web",
		"mweb":           "muse-web",
		"uc":             "uc-web",
	"veo-free":       "veoaifree-web",
	"hf":             "huggingface",
	"hyp":            "hyperbolic",
	"ideo":           "ideogram",
	"leo":            "leonardo",
	"if":             "iflow",
	"jina":           "jina-ai",
	"kc":             "kilocode",
	"km":             "kimi",
	"kr":             "kiro",
	"mimo":           "xiaomi-mimo",
	"mm":             "minimax",
	// MiniMax Code (mcode). Upstream publishes the China entry as "mm", which
	// this table already carries for the API-key `minimax` provider and which
	// therefore keeps resolving there; `mmc` is the unambiguous prefix for the
	// credits lane. `mmg` is upstream's own alias for the global site.
		"mmc":               "minimax-code",
		"mmg":               "minimax-code-global",
		"mmf":               "mimo-free",
		"mimocode":          "mimo-free",
		"mimocode-free":     "mimo-free",
		"nb":                "nanobanana",
	"muse-ai":           "muse",
	"meta-model-api":    "muse",
	"muse-code":         "muse",
	"muse-subscription": "muse",
	"systemone":         "v1m",
	// Upstream publishes `jev` as a v1m alias, but jev-1.13-free already
	// belongs to opencode and jev-1.13 to opencode-zen. Registering it here
	// would let `jev/<model>` cross into v1m, which AGENTS.md §3 forbids and
	// which would silently hijack a model id two other providers already own.
	// The canonical `v1m/` prefix still reaches every v1m model.
	"ne":             "nebius",
	"nous-portal":    "nous",
	"nous-research":  "nous",
	"ns":             "nous",
	"nv":             "nvidia",
	"oa":             "openai",
	"oc":             "opencode",
	"ocs":            "opencode-server",
	"ocg":            "opencode-go",
	"ocz":            "opencode-zen",
	"or":             "openrouter",
	"pa":             "perplexity-agent",
	"polly":          "aws-polly",
	"pplx":           "perplexity",
	"pplx-agent":     "perplexity-agent",
	"pplx-responses": "perplexity-agent",
	"pw":             "perplexity-web",
	"qd":             "qoder",
	"qdcn":           "qoder-cn",
	"runway":         "runwayml",
	"stability":      "stability-ai",
	"tg":             "together",
	"vali":           "volcengine-ark",
	"vercel":         "vercel-ai-gateway",
	"vn":             "venice",
	"xmtp":           "xiaomi-tokenplan",
	"af":             "api-airforce",
	"bzl":            "bazaarlink",
	"dx":             "daoxe",
	"oad":            "openadapter",
	"bm":             "bluesminds",
	"cbcn":           "codebuddy-cn",
	"dv":             "devin-cli",
	"hunyuan":        "tencent",
	"kgw":            "kilo-gateway",
	"ps":             "poolside",
	"qianfan":        "baidu",
	"samba":          "sambanova",
	// "tr" belongs to tokenrouter, not trae. Upstream publishes trae's own
	// alias as "tr" but keeps trae out of the registry array (a hidden entry:
	// its SOLO agent has no tool calling), and builds the alias table from that
	// array in order, so tokenrouter's aliases[] entry wins
	// (open-sse/services/model.js:16-19). Verified against upstream:
	// resolveProviderAlias("tr") === "tokenrouter".
	"tr":       "tokenrouter",
	"marscode": "trae",
	"voyage":   "voyage-ai",
	"vx":       "vertex",
	"vxp":      "vertex-partner",
	"ws":       "windsurf",
	"xq":       "xquik",
	"zd":       "zed",
	"atmo":     "atmorouter",
	"rc":       "runcrate",
	"hc":       "hcnsec",
	// Lane names other routers publish as separate providers that this
	// registry already covers through the parent provider (dedup pre-check,
	// TASK-9RX-PROVIDERS): the -web/-oauth/-apikey suffix only names the
	// access mode, and the parent already serves that mode. Wire-compat
	// aliases, NOT new providers.
	"kimi-web":           "kimi",
	"venice-web":         "venice",
	"blackbox-web":       "blackbox",
	"xai-oauth":          "xai",
	"kimi-coding-apikey": "kimi-coding",
	"kepler":             "ovh",
	"kepler-ai":          "ovh",
	"ovh-kepler":         "ovh",
	"cpw":                "copilot-web",
	"ucd":             "uc-direct",
	"horde":           "aihorde",
	"river-ai": "river",
	"drex": "nace",
	"ooba":            "oobabooga",
	"lmstudio":        "lm-studio",
	"llamacpp":        "llama-cpp",
	"dmr":             "docker-model-runner",
	"zylo":            "zylo-api",
	"zm":              "zenmux",
	"paralon":         "paraloncloud",
	"ofa":             "openference-api",
	"naga":            "naga-ac",
	"llmkiwi":         "llm-kiwi",
	"glb":             "gitlawb",
	"g4fpol":          "g4f-pollinations",
	"g4foll":          "g4f-ollama",
	"g4fnv":           "g4f-nvidia",
	"g4fgroq":         "g4f-groq",
	"g4fgem":          "g4f-gemini",
	"fta":             "freetheai",
	"fmd":             "freemodel-dev",
	"faik":            "freeaiapikey",
	"dai":             "dit",
	"aiml":            "aimlapi",
	"ska":             "seekai",
	"scw":             "scaleway",
	"mdl":             "modal",
	"inet":            "inference-net",
	"lambda":          "lambda-ai",
	"ms":              "modelscope",
	"ollamacloud":     "ollama-cloud",
	"mimotp":          "xiaomi-mimo-token-plan",
	"qct":             "qwen-cloud-token-plan",
	"qwc":             "qwen-cloud",
	"ling":            "ant-ling",
	"lc":              "longcat",
	"arcee":           "arcee-ai",
	"pn":              "pioneer",
	"ybw":                "yuanbao-web",
	"zw":                 "zai-web",
	"nw":                 "notion-web",
	"arena":              "arena-ai",
	"mb":                 "megabrain",
	"gld":                "gitlab",
	"of":                 "openference",
	"wb":                 "workbuddy",
	"aq":                 "amazon-q",
	"iflytek":            "sparkdesk",
	"gitlab-duo":         "gitlab",
	"lmarena":            "arena-ai",
}

// ResolveAlias returns the canonical provider ID for an alias, or the alias itself if not found.
func ResolveAlias(alias string) string {
	if canonical, ok := ProviderAliasMap[alias]; ok {
		return canonical
	}
	return alias
}

// IsNoAuthProvider reports whether a provider is marked noAuth in the registry,
// i.e. it is usable without any configured credential. Aliases resolve to their
// canonical id first, so callers may pass either form.
func IsNoAuthProvider(providerID string) bool {
	canon := ResolveAlias(providerID)
	if cfg, ok := KnownProviders[canon]; ok {
		return cfg.NoAuth || cfg.AuthOptional
	}
	return false
}

// GetProviderAlias returns the alias upstream publishes a provider under, ported
// from the registry's uiAlias/alias. Providers without a registry alias are
// published under their id (clinepass, nvidia, openrouter, openai) — exactly
// like upstream getProviderAlias: AI_PROVIDERS[id]?.alias || id.
//
// ProviderAliasMap stays the alias -> canonical id table used when RESOLVING
// incoming model ids (cp/* still routes to clinepass), so removing the old
// first-wins inverse here only changes the published prefix.
func GetProviderAlias(providerID string) string {
	if alias, ok := RegistryAliases[providerID]; ok && alias != "" {
		return alias
	}
	return providerID
}
