// 9RouterX — derived from the work of https://github.com/yanstd888/ — port and modifications by the maintainers — https://t.me/aiclub_id
package providers

import (
	"net/http"
	"strings"
	"time"
)

// ProviderConfig describes how to reach an upstream provider.
type ProviderConfig struct {
	BaseURL       string
	AuthHeader    string            // "Authorization" or "x-api-key"
	AuthScheme    string            // "bearer" or "raw"
	NoAuth        bool              // true = never send credentials
	AuthOptional  bool              // anonymous access is allowed; supplied credentials are preserved
	DefaultAPIKey string            // fallback API key when none provided
	StaticHeaders map[string]string // extra headers to set on every request
	Format        string            // "" (OpenAI standard), "gemini-native"
	CatalogURL    string            // optional model catalog root for a native executor
	ImageURL      string            // override /images/generations endpoint
	TTSURL        string            // override /audio/speech endpoint
	STTURL        string            // override /audio/transcriptions endpoint
	VideoURL      string            // override /videos/generations endpoint
	VoicesURL     string            // override /audio/voices listing endpoint
	SystemoneURL  string            // override /systemone endpoint (System One structured evaluation)
	SystemonePath string            // registry path a saved host override is joined with (empty: override is a full URL)
	FetchURL      string            // override /web/fetch endpoint (Jina, Firecrawl, etc.)
	FetchMethod   string            // HTTP method for fetch: GET or POST (default POST)
	SearchURL     string            // /v1/search endpoint. Empty means the provider has no search lane.
	SearchMethod  string            // HTTP method for search: GET or POST (default POST)
	UsageURL      string            // override /usage endpoint (quota tracker)
	// ThinkingFormat mirrors the upstream registry's transport.thinkingFormat:
	// the wire dialect this provider expects for reasoning fields ("" = the
	// model's capability decides). Consumed by internal/thinking.resolveFormat.
	ThinkingFormat string

	// PreserveCacheControl mirrors the upstream registry's
	// quirks.preserveCacheControl: a provider registered with it keeps the
	// cache_control markers on its content blocks, so the OpenAI filter must
	// not strip them.
	PreserveCacheControl bool

	// HeaderOverrides is the operator's per-provider Custom Headers, kept apart
	// from StaticHeaders so a forwarder can apply it last. StaticHeaders is
	// written by both the registry and an executor; an override is the operator
	// correcting that value, and an executor that rebuilds its own header map
	// would otherwise drop it. Upstream applies these after buildHeaders for
	// exactly that reason (open-sse/executors/base.js:132).
	HeaderOverrides map[string]string

	// UpstreamIsAnthropic marks an entry whose upstream speaks Anthropic Messages while
	// the client may speak OpenAI, on a host that is not api.anthropic.com. The existing
	// detection keys off that host, so a gateway like Bedrock needs its own flag for the
	// response to be translated rather than passed through.
	UpstreamIsAnthropic bool

	// LocalByDesign marks a provider whose upstream endpoint is intended to run on the
	// local host or loopback network (e.g. ollama-local on localhost:11434).
	LocalByDesign bool

	// Hidden keeps a provider out of the lists a person scans — the provider
	// pickers, the quota page — while leaving it fully reachable: connections
	// still resolve it and requests still route to it. The MMF entries use it
	// because they exist to serve traffic, not to be chosen.
	Hidden bool

	// RequireClaudeToolType marks providers whose Anthropic-compatible Messages
	// endpoint strictly rejects tool declarations that omit the `type` field
	// (e.g. MiniMax). When true, DefaultClaudeToolType will inject `type: "custom"`.
	RequireClaudeToolType bool

	// RelayURL is the edge relay (vercel/cloudflare/deno/netlify) an assigned
	// proxy pool puts in front of this provider, or "" when the request leaves
	// directly. It is a hint for the executors, not a rewrite: BaseURL stays the
	// real provider endpoint and the connection's client moves the dial to the
	// relay (internal/proxy.RelayTransport). Upstream expresses the same thing as
	// proxyOptions.vercelRelayUrl.
	//
	// An executor reads it only to opt OUT of something the relay cannot carry.
	// Opencode's stealth TLS client owns the socket, so it stands aside when a
	// relay fronts the request; everything else needs no provider-specific code,
	// which is the point.
	RelayURL string

	// StallTimeoutMs is the optional per-provider stall timeout in milliseconds.
	StallTimeoutMs int `json:"stallTimeoutMs,omitempty"`
}

// Bedrock protocol constants. Region is substituted per request by the executor, so the
// BaseURL below documents the shape rather than being dialed directly.
const (
	BedrockService          = "bedrock"
	BedrockAnthropicVersion = "bedrock-2023-05-31"
	BedrockStreamPath       = "invoke-with-response-stream"
	BedrockInvokePath       = "invoke"
	// BedrockChunkEvent is the only event type InvokeModelWithResponseStream defines
	// today; each chunk payload is {"bytes": "<base64 of one Anthropic event>"}.
	BedrockChunkEvent = "chunk"
	// BedrockTerminalEvent is how Anthropic closes a well-formed Bedrock stream.
	// Tracking it is what lets us tell a finished answer from an upstream that hung up
	// cleanly halfway through one.
	BedrockTerminalEvent = "message_stop"

	// BedrockProbePath is ListFoundationModels on the control-plane host, used by the
	// dashboard's validate and Test paths.
	BedrockProbePath         = "foundation-models"
	BedrockErrorTypeHeader   = "x-amzn-errortype"
	BedrockAccessDeniedError = "AccessDeniedException"
)

// BedrockModelFamily reports whether a Bedrock model id belongs to the family this
// provider entry serves, and what to tell the user when it does not.
//
// Bedrock model families use different request and response shapes, so each gets its own
// provider entry. Rejecting the wrong family here beats letting it sail through and fail
// mid-stream, after the call has already been billed.
func BedrockModelFamily(providerID, model string) (ok bool, hint string) {
	// Inference-profile ids look like "us.anthropic.claude-…" or "global.xai.grok-…";
	// the family is whatever follows the region prefix, or is the whole id when bare.
	family := model
	if idx := strings.IndexAny(model, "./"); idx >= 0 {
		family = model[idx+1:]
	}
	switch providerID {
	case "bedrock":
		return strings.HasPrefix(family, "anthropic."),
			`an Anthropic model, e.g. "us.anthropic.claude-sonnet-5"`
	case "bedrock-xai":
		return strings.HasPrefix(family, "xai."), `an xAI model, e.g. "us.xai.grok-4.6"`
	default:
		return true, ""
	}
}

// IsGeminiNative returns true if provider uses Gemini-native format.
func (p *ProviderConfig) IsGeminiNative() bool { return p.Format == "gemini-native" }

// IsGeminiOpenAICompat returns true if provider is Gemini behind an
// OpenAI-compatible endpoint whose tool schema validation is as strict as the
// native one (e.g. the "gemini" provider at /v1beta/openai/chat/completions).
func (p *ProviderConfig) IsGeminiOpenAICompat() bool { return p.Format == "gemini-openai" }

// WithStaticHeader returns cfg with one static header set, copying the config
// and the header map so a shared catalog entry is never mutated. A header the
// config already carries is replaced in the copy.
func WithStaticHeader(cfg *ProviderConfig, name, value string) *ProviderConfig {
	if cfg == nil {
		return nil
	}
	cloned := *cfg
	cloned.StaticHeaders = make(map[string]string, len(cfg.StaticHeaders)+1)
	for k, v := range cfg.StaticHeaders {
		cloned.StaticHeaders[k] = v
	}
	cloned.StaticHeaders[name] = value
	return &cloned
}

// WithoutStaticHeader returns cfg with one static header removed, copying both
// the config and the map. It answers cfg unchanged when the header is absent,
// so the common path allocates nothing.
func WithoutStaticHeader(cfg *ProviderConfig, name string) *ProviderConfig {
	if cfg == nil {
		return nil
	}
	if _, present := cfg.StaticHeaders[name]; !present {
		return cfg
	}
	cloned := *cfg
	cloned.StaticHeaders = make(map[string]string, len(cfg.StaticHeaders)-1)
	for k, v := range cfg.StaticHeaders {
		if k != name {
			cloned.StaticHeaders[k] = v
		}
	}
	return &cloned
}

// modelsListURL is the OpenAI-compatible /v1/models endpoint of providers whose
// catalogue is fetched live for the dashboard's "Suggested free models" import.
// It is the same set upstream wires into PROVIDER_MODELS_CONFIG
// (src/app/api/providers/[id]/models/route.js, v0.5.91).
var modelsListURL = map[string]string{
	"tokenharbor": "https://tokenharbor.ai/v1/models",
	"dahl":        "https://inference.dahl.global/v1/models",
	"atria":       "https://api.atria-asi.ai/v1/models",
	"agnes":       "https://apihub.agnes-ai.com/v1/models",
	"bai":         "https://api.b.ai/v1/models",
	// Meta's Model API refuses /v1/models without the protocol version header,
	// so the live-catalog fetch carries it on top of the bearer token
	// (upstream PROVIDER_MODELS_CONFIG.muse).
	"muse": "https://api.meta.ai/v1/models",
	// SumoPod exposes the standard OpenAI /v1/models catalogue (auth required,
	// same contract as tokenharbor/dahl).
	"sumopod": "https://ai.sumopod.com/v1/models",
	"nous":    "https://inference-api.nousresearch.com/v1/models",
	// Command Code publishes its whole Provider API catalogue on the standard
	// OpenAI /v1/models shape, keyless: every id it sells, the free tiers
	// included (reflection/beam:free, stealth/glyph-cluster:free,
	// poolside/laguna-s-2.1-free) and the paid ones added since the registry
	// snapshot (minimax/minimax-m3.1-flash, z-ai/glm-5.3-flash, …). The
	// registry catalog is a fixed 22-model list, so without this entry the
	// dashboard cannot discover anything the snapshot predates.
	"command-code": "https://api.commandcode.ai/provider/v1/models",
	"commandcode-goat": "https://api.commandcode.ai/provider/v1/models",
	"cmc-goat":         "https://api.commandcode.ai/provider/v1/models",
	"commandcode":      "https://api.commandcode.ai/provider/v1/models",
	// Five more providers whose catalogue is reachable without a valid
	// credential, so the discovery path that fixed commandcode covers them
	// unchanged: "Check Latest Models", "Import from /models" and "Sync Models"
	// all read this one map.
	//
	// They split on whether the endpoint *validates* the bearer the handler
	// always sends, and the difference is only in how a revoked key reports:
	//
	//   - nvidia, ollama and openrouter answer 200 to any Authorization header,
	//     bogus token included, so discovery survives a revoked credential and
	//     still lists what the provider serves.
	//   - vercel-ai-gateway and chutes answer 200 with no header at all but 401
	//     as soon as one is sent, so a revoked key reports a failed fetch
	//     exactly like the keyed aggregators do.
	//
	// Either way a connection must exist first — writeConnectionModels refuses
	// a provider with no stored key before it dials anything.
	//
	// vercel-ai-gateway and chutes ship no static catalog at all (absent from
	// ProviderModels and from web/src/lib/models.ts), so before this entry the
	// dashboard could not list a single model for either. nvidia and ollama
	// carry 11 and 8 static rows against 84 and 23 live ones. openrouter
	// already had a web modelsFetcher for its free tier, but no backend entry,
	// so its "Check Latest Models" button never rendered.
	"vercel-ai-gateway": "https://ai-gateway.vercel.sh/v1/models",
	"chutes":            "https://llm.chutes.ai/v1/models",
	"nvidia":            "https://integrate.api.nvidia.com/v1/models",
	// Ollama's cloud library is not an OpenAI /v1/models endpoint: it answers
	// {models:[{name, model, …}]}. writeConnectionModels accepts either `data`
	// or `models`, and the dashboard falls back to the `model` field when `id`
	// is absent, so the shape needs no special case here.
	"ollama":     "https://ollama.com/api/tags",
	"openrouter": "https://openrouter.ai/api/v1/models",
	"hcnsec":     "https://api.hcnsec.cn/v1/models",
}

// modelsListHeaders carries the extra headers one live-catalogue endpoint
// needs beyond the connection's bearer token. Upstream keys the same values
// off PROVIDER_MODELS_CONFIG per provider (src/app/api/providers/[id]/models/
// route.js), which sets headers per entry — the flat Go map only has room for
// the ones that actually differ.
var modelsListHeaders = map[string]map[string]string{
	"muse": {"x-api-version": "1.0.0", "Content-Type": "application/json"},
}

// ModelsListHeaders returns the extra headers the live catalogue endpoint of a
// provider requires, or nil when it needs none. The map is shared by every
// request, so a caller must not mutate it.
func ModelsListHeaders(provider string) map[string]string {
	return modelsListHeaders[strings.ToLower(provider)]
}

// ModelsListURL returns the live catalogue endpoint for a provider, or "" when
// the provider has none.
func ModelsListURL(provider string) string {
	return modelsListURL[strings.ToLower(provider)]
}

// AnthropicBetaRedactThinking asks Anthropic to return thinking blocks as a
// signature only. That is right for clients that never render thinking, but it
// blanks the very summaries a client requested with
// `thinking.display: "summarized"`, so it is dropped per request when the body
// asks for them. Upstream: ANTHROPIC_BETA_REDACT_THINKING in
// open-sse/providers/shared.js.
const AnthropicBetaRedactThinking = "redact-thinking-2026-02-12"

// WithoutBetaFlag returns a copy of headers with one Anthropic-Beta flag
// removed. The registry's header map is shared by every request, so a
// request-scoped edit has to copy rather than mutate.
func WithoutBetaFlag(headers map[string]string, flag string) map[string]string {
	current, ok := headers["Anthropic-Beta"]
	if !ok {
		return headers
	}
	kept := make([]string, 0, 8)
	for _, existing := range strings.Split(current, ",") {
		if trimmed := strings.TrimSpace(existing); trimmed != "" && trimmed != flag {
			kept = append(kept, trimmed)
		}
	}
	res := make(map[string]string, len(headers))
	for k, v := range headers {
		res[k] = v
	}
	res["Anthropic-Beta"] = strings.Join(kept, ",")
	return res
}

// MergeAnthropicBeta unions any number of comma-separated beta flag lists into
// one de-duplicated header value, keeping the order they were seen in. The
// caller's own flags are merged in rather than dropped: a client asking for a
// beta the gateway does not list would otherwise be refused without ever being
// told why. Port of mergeAnthropicBeta in open-sse/providers/shared.js.
func MergeAnthropicBeta(values ...string) string {
	seen := make(map[string]bool, 8)
	merged := make([]string, 0, 8)
	for _, value := range values {
		for _, flag := range strings.Split(value, ",") {
			flag = strings.TrimSpace(flag)
			if flag == "" || seen[flag] {
				continue
			}
			seen[flag] = true
			merged = append(merged, flag)
		}
	}
	return strings.Join(merged, ",")
}

// WithHeader returns a copy of headers with one entry set. The registry's
// header map is shared by every request, so a request-scoped change has to
// copy rather than mutate — the same reason WithoutBetaFlag copies.
func WithHeader(headers map[string]string, key, value string) map[string]string {
	res := make(map[string]string, len(headers)+1)
	for k, v := range headers {
		res[k] = v
	}
	res[key] = value
	return res
}

// MergeHeaderOverrides returns a copy of headers with the operator's
// per-provider overrides applied on top. It is the Go shape of upstream's
// `Object.assign(headers, providerOverrides.headers)`
// (decolua/9router v0.5.95, open-sse/executors/base.js:132) — an override
// wins over what the registry set, which is the whole point: the operator is
// correcting a header the gateway sends, not adding a second opinion.
//
// The registry's header map is shared by every request, so this copies rather
// than mutating, for the same reason WithHeader copies.
func MergeHeaderOverrides(headers, overrides map[string]string) map[string]string {
	if len(overrides) == 0 {
		return headers
	}
	res := make(map[string]string, len(headers)+len(overrides))
	for k, v := range headers {
		res[k] = v
	}
	for k, v := range overrides {
		res[k] = v
	}
	return res
}

// ApplyHeaderOverrides returns a copy of headers with the operator's overrides
// applied last, matching an existing name case-insensitively.
//
// MergeHeaderOverrides replaces by exact key, which is enough for the generic
// forward path because both sides spell the header the same way. An executor
// that assembles its own map is the different case: CodeBuddy writes
// `X-IDE-Type` while an operator's entry may be `x-ide-type`, and a
// case-sensitive set would leave both keys in the map, where which one wins
// depends on map iteration order. Matching the key already present keeps
// exactly one entry per name, which is what the operator's correction means.
func ApplyHeaderOverrides(headers, overrides map[string]string) map[string]string {
	if len(overrides) == 0 {
		return headers
	}
	res := make(map[string]string, len(headers)+len(overrides))
	for k, v := range headers {
		res[k] = v
	}
	for name, value := range overrides {
		applied := false
		for existing := range res {
			if strings.EqualFold(existing, name) {
				res[existing] = value
				applied = true
				break
			}
		}
		if !applied {
			res[name] = value
		}
	}
	return res
}

// KnownProviders maps provider IDs to their upstream configuration.
var KnownProviders = map[string]ProviderConfig{
	"ghe-copilot": {
		BaseURL: "https://api.githubcopilot.com/chat/completions",
		AuthHeader: "Authorization", AuthScheme: "bearer", Format: FormatOpenAI,
		StaticHeaders: GitHubCopilotChatHeaders("text/event-stream", "user"),
		PreserveCacheControl: true,
	},
	"devin-desktop": {
		BaseURL: "https://server.codeium.com",
	},
	"volcengine-agent-plan": {
		BaseURL: "https://ark.cn-beijing.volces.com/api/plan/v3/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	"volcengine-coding-plan": {
		BaseURL: "https://ark.cn-beijing.volces.com/api/coding/v3/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	"agnes-cn": {
		BaseURL: "https://api.agnes-ai.cn/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	"alibaba-cn": {
		BaseURL: "https://dashscope.aliyuncs.com/compatible-mode/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	"openai": {
		BaseURL:    "https://api.openai.com/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	"agnes": {
		BaseURL:    "https://apihub.agnes-ai.com/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// Muse — Meta's Model API. Dual auth upstream (Muse Code subscription
	// device-code key and a dev.meta.ai pay-as-you-go key); both ride the same
	// bearer transport, and only an account-issued (OAuth) key needs the
	// protocol version header. Every Muse Spark model declares
	// targetFormat "openai-responses" (see museModelFormats), so /v1/responses
	// is the transport the executor dials for this provider.
	"muse": {
		BaseURL:    "https://api.meta.ai/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
		Format:     "openai-responses",
		StaticHeaders: map[string]string{
			"x-api-version": "1.0.0",
		},
	},
	// muse-web: Baidu Muse Spark via web session (port(serenhope) TASK-5.3).
	// Distinct from Meta Model API above. Disabled by default via ENABLE_WEB_EXECUTORS.
	"muse-web": {
		BaseURL:    "https://yiyan.baidu.com/eb/chat/conversation/v2",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// v1m System One — a calibrated decision engine, not a chat model. Its
	// two registry models carry kind "systemone", so the only endpoint it
	// answers is /v1/systemone. BaseURL is empty because the port has no
	// chat lane for this provider; an empty base never resolves a connection.
	"v1m": {
		AuthHeader:   "Authorization",
		AuthScheme:   "bearer",
		SystemoneURL: "https://v1m.ir/v1/systemone",
	},
	"anthropic": {
		BaseURL:    "https://api.anthropic.com/v1/messages",
		AuthHeader: "x-api-key",
		AuthScheme: "raw",
	},
	"dahl": {
		BaseURL:    "https://inference.dahl.global/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	"bai": {
		BaseURL:    "https://api.b.ai/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	"atria": {
		BaseURL:    "https://api.atria-asi.ai/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	"deepseek": {
		BaseURL:    "https://api.deepseek.com/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// DeepSeek Web: the consumer chat site reached with the credential a browser
	// login leaves behind, mirrored from OmniRoute
	// open-sse/config/providers/registry/deepseek/web/index.ts. It is a distinct
	// deployment with its own credential and quota, so it is never aliased onto
	// the API-key `deepseek` above (AGENTS.md section 3.A). Unlike grok-web and
	// perplexity-web it does not send the pasted value as a raw cookie: the
	// userToken is exchanged for a short-lived Bearer access token first, which
	// is why it keeps the Authorization/bearer shape.
	"deepseek-web": {
		BaseURL:    "https://chat.deepseek.com/api/v0/chat/completion",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// SumoPod — Indonesian AI gateway (ai.sumopod.com), OpenAI-compatible
	// chat completions with 60+ models across Claude, Gemini, GPT, DeepSeek,
	// Qwen, GLM, Kimi, MiniMax, Seed, Grok, Hy and Mimo.
	"hcnsec": {
		BaseURL:        "https://api.hcnsec.cn/v1/chat/completions",
		AuthHeader:     "Authorization",
		AuthScheme:     "bearer",
		ThinkingFormat: "openai",
		TTSURL:         "https://api.hcnsec.cn/v1/audio/speech",
	},
	"sumopod": {
		BaseURL:    "https://ai.sumopod.com/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	"groq": {
		BaseURL:    "https://api.groq.com/openai/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	"nvidia": {
		BaseURL:    "https://integrate.api.nvidia.com/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	"cerebrium": {
		BaseURL:    "https://api.cerebrium.ai/v4",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
		Format:     FormatOpenAI,
	},
	"nlpcloud": {
		BaseURL:    "https://api.nlpcloud.io/v1/gpu",
		AuthHeader: "Authorization",
		AuthScheme: "raw",
		Format:     FormatOpenAI,
	},
	"poe": {
		BaseURL:    "https://api.poe.com/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
		Format:     FormatOpenAI,
	},
	"openrouter": {
		BaseURL:        "https://openrouter.ai/api/v1/chat/completions",
		AuthHeader:     "Authorization",
		AuthScheme:     "bearer",
		ThinkingFormat: "openai",
		ImageURL:       "https://openrouter.ai/api/v1/images/generations",
		VideoURL:       "https://openrouter.ai/api/v1/videos",
		SystemoneURL:   "https://openrouter.ai/api/v1/systemone",
		StaticHeaders: map[string]string{
			"HTTP-Referer": "https://endpoint-proxy.local",
			"X-Title":      "Endpoint Proxy",
		},
	},
	"cerebras": {
		BaseURL:    "https://api.cerebras.ai/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	"together": {
		BaseURL:    "https://api.together.xyz/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	"fireworks": {
		BaseURL:    "https://api.fireworks.ai/inference/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	"opencode": {
		BaseURL:       "https://opencode.ai/inference/openai/v1/chat/completions",
		AuthHeader:    "Authorization",
		AuthScheme:    "bearer",
		DefaultAPIKey: "public",
		NoAuth:        true,
		SystemoneURL:  "https://opencode.ai/zen/v1/systemone",
		StaticHeaders: map[string]string{"x-opencode-client": "cli", "User-Agent": "opencode/latest/2.1.4/cli"},
	},
	// OpenCode CLI Server: the local `opencode serve` v1 API. Keyless by
	// contract, so no Authorization header is ever built for this lane.
	// AuthOptional also keeps a custom dashboard URL usable without a key.
	"opencode-server": {
		BaseURL:       "http://127.0.0.1:4096",
		CatalogURL:    "http://127.0.0.1:4096/config/providers",
		NoAuth:        true,
		AuthOptional:  true,
		LocalByDesign: true,
		Format:        FormatOpenAI,
	},
	// Pay-as-you-go lane (upstream open-sse/providers/registry/opencode-zen.js):
	// a key is required, and the same key answers /chat/completions, /messages
	// and /responses. NoAuth stays false so a connection with no key is refused
	// instead of silently falling back to the free tier's "public" placeholder.
	"opencode-zen": {
		BaseURL:      "https://opencode.ai/zen/v1/chat/completions",
		AuthHeader:   "Authorization",
		AuthScheme:   "bearer",
		UsageURL:     "https://opencode.ai/zen/v1/usage",
		SystemoneURL: "https://opencode.ai/zen/v1/systemone",
		StaticHeaders: map[string]string{
			"x-opencode-client": "desktop",
			"User-Agent":        "opencode/1.19.4",
		},
	},
	"gemini": {
		BaseURL:    "https://generativelanguage.googleapis.com/v1beta/openai/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
		Format:     "gemini-openai",
	},
	// Gemini Web: gemini.google.com reached directly over HTTP, the way the
	// page's own JavaScript reaches it. This was never an API — but it is an
	// HTTP surface, and the executor drives it without a browser.
	//
	// NoAuth because the site answers an anonymous visitor: one model is served
	// with no credential at all. A pasted __Secure-1PSID cookie is honored when
	// one is configured, and the only thing it buys is the choice of model — an
	// anonymous session is served one, a signed-in one several. Requiring a
	// cookie would turn a plug-and-play provider into one that is not, for no
	// gain.
	"gemini-web": {
		BaseURL:       "https://gemini.google.com/app",
		AuthHeader:    "Authorization",
		AuthScheme:    "bearer",
		DefaultAPIKey: "anonymous",
		NoAuth:        true,
	},
	"antigravity": {
		BaseURL:    "https://daily-cloudcode-pa.googleapis.com",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
		Format:     "gemini-native",
	},
	"github": {
		BaseURL:    "https://api.githubcopilot.com/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
		// CLI identity, not the VS Code extension identity: Copilot exposes a
		// wider entitled catalogue to `copilot-developer-cli` than to
		// `vscode-chat`, so the old headers imported fewer models than the
		// account owns and advertised ones it is not entitled to. See
		// GitHubCopilotChatHeaders for why the header set is exactly the CLI's.
		StaticHeaders: GitHubCopilotChatHeaders("", ""),
	},
	"mistral": {
		BaseURL:    "https://api.mistral.ai/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	"perplexity": {
		BaseURL:    "https://api.perplexity.ai/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	"xai": {
		BaseURL:    "https://api.x.ai/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
		ImageURL:   "https://api.x.ai/v1/images/generations",
		VideoURL:   "https://api.x.ai/v1/videos",
	},
	"cohere": {
		BaseURL:    "https://api.cohere.ai/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// Ollama Cloud, not a self-hosted daemon. This entry used to carry
	// http://localhost:11434, which sent every ollama.com API-key connection to
	// a loopback port that only exists on a machine running `ollama serve`
	// (#192). Upstream keeps the two apart as well: registry ollama.js dials
	// ollama.com and only ollama-local.js points at 11434.
	//
	// ollama.com serves the OpenAI-compatible lane at /v1, verified live: an
	// unknown path 404s, /v1/models answers 200, and a bad key draws a 401 with
	// an OpenAI-shaped error body. That is the lane ForwardOpenAI already speaks,
	// so the fix needs no native-translator port.
	"ollama": {
		BaseURL:      "https://ollama.com/v1/chat/completions",
		AuthHeader:   "Authorization",
		AuthScheme:   "bearer",
		FetchURL:     "https://ollama.com/api/web_fetch",
		FetchMethod:  "POST",
		SystemoneURL: "https://ollama.com/v1/systemone",
	},
	"siliconflow": {
		BaseURL:        "https://api.siliconflow.com/v1/chat/completions",
		AuthHeader:     "Authorization",
		AuthScheme:     "bearer",
		ThinkingFormat: "openai",
	},
	"sensenova": {
		BaseURL:    "https://token.sensenova.cn/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
		ImageURL:   "https://token.sensenova.cn/v1/images/generations",
	},
	"cloudflare-ai": {
		BaseURL:        "https://api.cloudflare.com/client/v4/accounts/{accountId}/ai/v1/chat/completions",
		AuthHeader:     "Authorization",
		AuthScheme:     "bearer",
		ThinkingFormat: "openai",
	},
		"mimo-free": {
			BaseURL:       "https://api.xiaomimimimo.com/api/free-ai/openai/chat",
			AuthHeader:    "Authorization",
			AuthScheme:    "bearer",
			DefaultAPIKey: "mimo-dynamic",
			NoAuth:        true,
			Hidden:        true,
		},
		"mimocode": {
			BaseURL:       "https://api.xiaomimimimo.com/api/free-ai/openai/chat",
			AuthHeader:    "Authorization",
			AuthScheme:    "bearer",
			DefaultAPIKey: "mimo-dynamic",
			NoAuth:        true,
			Hidden:        true,
		},
	"blackbox": {
		BaseURL:        "https://api.blackbox.ai/v1/chat/completions",
		AuthHeader:     "Authorization",
		AuthScheme:     "bearer",
		ThinkingFormat: "openai",
	},
	"featherless": {
		BaseURL:    "https://api.featherless.ai/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	"hyperbolic": {
		BaseURL:    "https://api.hyperbolic.xyz/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	"kilocode": {
		BaseURL:    "https://api.kilo.ai/api/openrouter/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// MegaBrain: OpenRouter-compatible gateway on its own host (OmniRoute
	// validation/megabrain.ts, pinned 632d84f5).
	"megabrain": {
		BaseURL:    "https://api.getmegabrain.com/api/openrouter/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// Maritalk uses Authorization: Key <token> (OmniRoute's authHeader
	// "key" names the scheme, not an HTTP header; pinned 632d84f5).
	"maritalk": {
		BaseURL:    "https://chat.maritaca.ai/api/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "key",
	},
	// Openference: OpenAI-compatible gateway (OmniRoute open-sse registry,
	// pinned 632d84f5).
	"openference": {
		BaseURL:    "https://api.openference.com/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// WorkBuddy: streaming-only gateway on www.workbuddy.ai (OmniRoute
	// open-sse registry n/workbuddy family, pinned 632d84f5).
	"workbuddy": {
		BaseURL:    "https://www.workbuddy.ai/v2/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
		StaticHeaders: map[string]string{"X-Product": "SaaS"},
	},
	"nanobanana": {
		BaseURL:    "https://api.nanobananaapi.ai/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	"opencode-go": {
		BaseURL:    "https://opencode.ai/zen/go/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	"venice": {
		BaseURL:        "https://api.venice.ai/api/v1/chat/completions",
		AuthHeader:     "Authorization",
		AuthScheme:     "bearer",
		ThinkingFormat: "openai",
	},
	"vercel-ai-gateway": {
		BaseURL:        "https://ai-gateway.vercel.sh/v1/chat/completions",
		AuthHeader:     "Authorization",
		AuthScheme:     "bearer",
		ThinkingFormat: "openai",
	},
	"volcengine-ark": {
		BaseURL:    "https://ark.cn-beijing.volces.com/api/coding/v3/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	"xiaomi-mimo": {
		BaseURL:    "https://api.xiaomimimo.com/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	"xiaomi-tokenplan": {
		BaseURL:    "https://token-plan-sgp.xiaomimimo.com/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	"chutes": {
		BaseURL:    "https://llm.chutes.ai/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	"cline": {
		BaseURL:    "https://api.cline.bot/api/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
		// No StaticHeaders: the 9Router client identity (upstream
		// buildClineHeaders) carries the linked version, which a registry
		// literal cannot see (import cycle). getProviderConfig injects it
		// per request via ClineIdentityHeaders.
	},
	"alicode": {
		BaseURL:    "https://coding.dashscope.aliyuncs.com/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
		// Upstream registers the preserveCacheControl quirk for the alicode
		// family: these gateways answer 400 when the cache_control markers are
		// stripped from the content blocks.
		PreserveCacheControl: true,
	},
	"alicode-intl": {
		// Coding Plan keys live on the coding-intl host; dashscope-intl is the
		// Model Studio (alims-intl) host and rejects them
		// (upstream registry/alicode-intl.js:17).
		BaseURL:              "https://coding-intl.dashscope.aliyuncs.com/v1/chat/completions",
		AuthHeader:           "Authorization",
		AuthScheme:           "bearer",
		PreserveCacheControl: true,
	},
	"byteplus": {
		BaseURL:    "https://ark.ap-southeast.bytepluses.com/api/coding/v3/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	"codebuddy-cn": {
		BaseURL:        "https://copilot.tencent.com/v2/chat/completions",
		AuthHeader:     "Authorization",
		AuthScheme:     "bearer",
		ThinkingFormat: "openai",
		StaticHeaders: map[string]string{
			"User-Agent":          "CLI/2.133.1 CodeBuddy/2.133.1",
			"X-Product":           "SaaS",
			"X-IDE-Type":          "CLI",
			"X-IDE-Name":          "CLI",
			"X-IDE-Version":       "2.133.1",
			"X-Domain":            "www.codebuddy.cn",
			"x-requested-with":    "XMLHttpRequest",
			"x-codebuddy-request": "1",
		},
	},
	"codebuddy-intl": {
		BaseURL:        "https://www.codebuddy.ai/v2/chat/completions",
		AuthHeader:     "Authorization",
		AuthScheme:     "bearer",
		ThinkingFormat: "openai",
		StaticHeaders: map[string]string{
			"User-Agent":          "CLI/2.108.1 CodeBuddy/2.108.1",
			"X-Product":           "SaaS",
			"X-IDE-Type":          "CLI",
			"X-IDE-Name":          "CLI",
			"x-requested-with":    "XMLHttpRequest",
			"x-codebuddy-request": "1",
		},
	},
	"gitlab": {
		BaseURL:    "https://gitlab.com/api/v4/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
		Hidden:     true,
	},
	"glm-cn": {
		BaseURL:    "https://open.bigmodel.cn/api/coding/paas/v4/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	"glm": {
		BaseURL:    "https://api.z.ai/api/coding/paas/v4/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// zcode: ZCode start-plan lane. Auth is the plan JWT stored on each
	// connection (device-flow login), never a shared key — the same shape as
	// kimi and muse. Format=claude: the plan endpoint speaks Anthropic Messages,
	// so the dispatcher converts OpenAI bodies and flags the reply lane.
	// AI21 Labs: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"ai21": {
		BaseURL:    "https://api.ai21.com/studio/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// Codestral: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"codestral": {
		BaseURL:    "https://codestral.mistral.ai/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// Writer: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"writer": {
		BaseURL:    "https://api.writer.com/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// Upstage: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"upstage": {
		BaseURL:    "https://api.upstage.ai/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// Reka: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"reka": {
		BaseURL:    "https://api.reka.ai/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// Liquid AI: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"liquid": {
		BaseURL:    "https://inference.liquid.ai/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// Inception: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"inception": {
		BaseURL:    "https://api.inceptionlabs.ai/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// Galadriel: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"galadriel": {
		BaseURL:    "https://api.galadriel.ai/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// Pioneer AI: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"pioneer": {
		BaseURL:    "https://api.pioneer.ai/v1/chat/completions",
		AuthHeader: "x-api-key",
		AuthScheme: "raw",
	},
	// Yi (01.AI): mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"yi": {
		BaseURL:    "https://api.lingyiwanwu.com/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// Arcee AI: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"arcee-ai": {
		BaseURL:    "https://api.arcee.ai/api/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// BigModel.cn (Zhipu): mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"bigmodel": {
		BaseURL:    "https://open.bigmodel.cn/api/paas/v4/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// StepFun: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"stepfun": {
		BaseURL:    "https://api.stepfun.com/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// SparkDesk: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"sparkdesk": {
		BaseURL:    "https://spark-api-open.xf-yun.com/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// InternLM (Intern-S1): mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"internlm": {
		BaseURL:    "https://chat.intern-ai.org.cn/api/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// LongCat AI: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"longcat": {
		BaseURL:    "https://api.longcat.chat/openai/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// Ant Ling / Ring (inclusionAI): mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"ant-ling": {
		BaseURL:    "https://api.ant-ling.com/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// Coze: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"coze": {
		BaseURL:    "https://api.coze.com/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// PLaMo: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"plamo": {
		BaseURL:    "https://api.platform.preferredai.jp/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// Sarvam AI: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"sarvam": {
		BaseURL:    "https://api.sarvam.ai/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// Typhoon: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"typhoon": {
		BaseURL:    "https://api.opentyphoon.ai/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// Qwen Cloud: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"qwen-cloud": {
		BaseURL:    "https://dashscope-intl.aliyuncs.com/compatible-mode/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// Alibaba Cloud Model Studio: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"alibaba": {
		BaseURL:    "https://dashscope-intl.aliyuncs.com/compatible-mode/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// Qwen Cloud Token Plan: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"qwen-cloud-token-plan": {
		BaseURL:    "https://token-plan.ap-southeast-1.maas.aliyuncs.com/compatible-mode/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// Xiaomi MiMo Token Plan: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"xiaomi-mimo-token-plan": {
		BaseURL:    "https://token-plan-sgp.xiaomimimo.com/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// Baichuan: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"baichuan": {
		BaseURL:    "https://api.baichuan-ai.com/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// Volcengine: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"volcengine": {
		BaseURL:    "https://ark.cn-beijing.volces.com/api/v3/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// Doubao: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"doubao": {
		BaseURL:    "https://ark.cn-beijing.volces.com/api/v3/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// Z.AI: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"zai": {
		BaseURL:    "https://api.z.ai/api/anthropic/v1/messages?beta=true",
		AuthHeader: "x-api-key",
		AuthScheme: "raw",
		Format: FormatClaude,
		UpstreamIsAnthropic: true,
		StaticHeaders: map[string]string{"Anthropic-Version": "2023-06-01"},
	},
	// SEA-LION: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"sealion": {
		BaseURL:    "https://api.sea-lion.ai/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// DeepInfra: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"deepinfra": {
		BaseURL:    "https://api.deepinfra.com/v1/openai/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// Baseten: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"baseten": {
		BaseURL:    "https://inference.baseten.co/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// Heroku AI: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"heroku": {
		BaseURL:    "https://us.inference.heroku.com/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// Novita AI: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"novita": {
		BaseURL:    "https://api.novita.ai/openai/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// nScale: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"nscale": {
		BaseURL:    "https://inference.api.nscale.com/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// PublicAI: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"publicai": {
		BaseURL:    "https://api.publicai.co/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// Weights & Biases Inference: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"wandb": {
		BaseURL:    "https://api.inference.wandb.ai/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// Ollama Cloud: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"ollama-cloud": {
		BaseURL:    "https://ollama.com/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// ModelScope: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"modelscope": {
		BaseURL:    "https://api-inference.modelscope.cn/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// DigitalOcean: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"digitalocean": {
		BaseURL:    "https://inference.do-ai.run/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// Lambda AI: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"lambda-ai": {
		BaseURL:    "https://api.lambda.ai/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// Bytez: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"bytez": {
		BaseURL:    "https://api.bytez.com/models/v2/openai/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// Inference.net: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"inference-net": {
		BaseURL:    "https://api.inference.net/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// Hetzner Inference API: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"hetzner": {
		BaseURL:    "https://inference.hetzner.com/api/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// Modal: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"modal": {
		BaseURL:    "https://api.modal.ai/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// Predibase: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"predibase": {
		BaseURL:    "https://serving.app.predibase.com/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// Grokified: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"grokified": {
		BaseURL:    "https://api.grokified.com/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// Featherless AI: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"featherless-ai": {
		BaseURL:    "https://api.featherless.ai/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// OVHcloud AI: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"ovhcloud": {
		BaseURL:    "https://oai.endpoints.kepler.ai.cloud.ovh.net/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
		AuthOptional: true,
	},
	// KIE.AI: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"kie": {
		BaseURL:    "https://api.kie.ai/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// OpenVecta: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"openvecta": {
		BaseURL:    "https://api.openvecta.com/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// Scaleway AI: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"scaleway": {
		BaseURL:    "https://api.scaleway.ai/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// Nube.sh: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"nube": {
		BaseURL:    "https://ai.nube.sh/api/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// ainetcafe: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"ainetcafe": {
		BaseURL:    "https://microquickjs.com/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// SeekAi: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"seekai": {
		BaseURL:    "https://seekai.cc/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// TabiToken: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"tabitoken": {
		BaseURL:    "https://tabitoken.com/v1/messages",
		AuthHeader: "x-api-key",
		AuthScheme: "raw",
		Format: FormatClaude,
		UpstreamIsAnthropic: true,
		StaticHeaders: map[string]string{"Anthropic-Version": "2023-06-01"},
	},
	// Speka AI: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"speka": {
		BaseURL:    "https://speka.me/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// Naga AI: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"naga-ai": {
		BaseURL:    "https://api.naga.ac/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// AgentRouter: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"agentrouter": {
		BaseURL:    "https://agentrouter.org/v1/messages",
		AuthHeader: "x-api-key",
		AuthScheme: "raw",
		Format: FormatClaude,
		UpstreamIsAnthropic: true,
		StaticHeaders: map[string]string{"Anthropic-Version": "2023-06-01"},
	},
	// AI/ML API: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"aimlapi": {
		BaseURL:    "https://api.aimlapi.com/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// AnyAPI AI: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"anyapi": {
		BaseURL:    "https://api.anyapi.ai/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// AINative Studio: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"ainative": {
		BaseURL:    "https://api.ainative.studio/api/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// Aion Labs: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"aion": {
		BaseURL:    "https://api.aionlabs.ai/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// Apmix: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"apmix": {
		BaseURL:    "https://api.apmix.ai/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// Auriko: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"auriko": {
		BaseURL:    "https://api.auriko.ai/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// BeatAPI: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"beatapi": {
		BaseURL:    "https://api.beatapi.io/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// ChatAnywhere: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"chatanywhere": {
		BaseURL:    "https://api.chatanywhere.org/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// Chenzk API: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"chenzk": {
		BaseURL:    "https://chenzk.top/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// CloudCode.ONE: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"cloudcode-one": {
		BaseURL:    "https://api.cloudcode.one/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// CrofAI: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"crof": {
		BaseURL:    "https://crof.ai/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// Demon Route: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"demonroute": {
		BaseURL:    "https://api.demonroute.com/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// DGrid: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"dgrid": {
		BaseURL:    "https://api.dgrid.ai/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// DIT.ai: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"dit": {
		BaseURL:    "https://api.dit.ai/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// DXNT / DX Token: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"dxnt": {
		BaseURL:    "https://www.dxnt.com/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// Electron Hub: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"electronhub": {
		BaseURL:    "https://api.electronhub.ai/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// EmpirioLabs AI: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"empiriolabs": {
		BaseURL:    "https://api.empiriolabs.ai/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// EURouter: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"eurouter": {
		BaseURL:    "https://api.eurouter.ai/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// Factory: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"factory": {
		BaseURL:    "https://api.factory.ai/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// FastRouter: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"fastrouter": {
		BaseURL:    "https://api.fastrouter.ai/api/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// Free.ai: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"free-ai": {
		BaseURL:    "https://api.free.ai/v1/chat/",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// FreeAIAPIKey: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"freeaiapikey": {
		BaseURL:    "https://api.freeaiapikey.com/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// FreeInference: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"freeinference": {
		BaseURL:    "https://freeinference.org/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// FreeModel.dev: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"freemodel-dev": {
		BaseURL:    "https://api.freemodel.dev/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// FreeTheAi: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"freetheai": {
		BaseURL:    "https://api.freetheai.org/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// g4f.space — Gemini: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"g4f-gemini": {
		BaseURL:    "https://g4f.space/api/gemini/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// g4f.space — Groq: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"g4f-groq": {
		BaseURL:    "https://g4f.space/api/groq/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// g4f.space — NVIDIA: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"g4f-nvidia": {
		BaseURL:    "https://g4f.space/api/nvidia/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// g4f.space — Ollama: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"g4f-ollama": {
		BaseURL:    "https://g4f.space/api/ollama/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// g4f.space — Pollinations: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"g4f-pollinations": {
		BaseURL:    "https://g4f.space/api/pollinations/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// Gisoo: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"gisoo": {
		BaseURL:    "https://gisoo.pro/api/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// Gitlawb Opengateway (MiMo): mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"gitlawb": {
		BaseURL:    "https://opengateway.gitlawb.com/v1/xiaomi-mimo",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
		StaticHeaders: map[string]string{"User-Agent": "OpenClaude/1.0 (linux; x86_64)", "X-Title": "OpenClaude CLI", "HTTP-Referer": "https://github.com/Gitlawb/openclaude"},
	},
	// GreenPT: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"greenpt": {
		BaseURL:    "https://api.greenpt.ai/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// HelixMind: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"helixmind": {
		BaseURL:    "https://helixmind.online/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// Helyx AI: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"helyxai": {
		BaseURL:    "https://helyxai.space/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// LiteRouter: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"literouter": {
		BaseURL:    "https://api.literouter.com/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// LLM.Kiwi: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"llm-kiwi": {
		BaseURL:    "https://api.llm.kiwi/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// LLM Gateway: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"llmgateway": {
		BaseURL:    "https://api.llmgateway.io/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// Logfare: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"logfare": {
		BaseURL:    "https://logfare.ai/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// Lyceum: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"lyceum": {
		BaseURL:    "https://api.lyceum.technology/openai/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// Mixlayer: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"mixlayer": {
		BaseURL:    "https://models.mixlayer.ai/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// MNN AI: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"mnn-ai": {
		BaseURL:    "https://api.mnnai.ru/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// Naga.ac: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"naga-ac": {
		BaseURL:    "https://api.naga.ac/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// NanoGPT: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"nanogpt": {
		BaseURL:    "https://nano-gpt.com/api/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// NaraRouter: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"nara": {
		BaseURL:    "https://router.bynara.id/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// NavyAI: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"navy": {
		BaseURL:    "https://api.navy/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// OfoxAI: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"ofoxai": {
		BaseURL:    "https://api.ofox.ai/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// onomeo: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"onomeo": {
		BaseURL:    "https://onomeo.com/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// Openference API: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"openference-api": {
		BaseURL:    "https://api.openference.com/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// Opper: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"opper": {
		BaseURL:    "https://api.opper.ai/v3/compat/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// ParalonCloud: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"paraloncloud": {
		BaseURL:    "https://paraloncloud.com/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// Poixe AI: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"poixe-ai": {
		BaseURL:    "https://api.poixe.com/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// Qiniu: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"qiniu": {
		BaseURL:    "https://api.qnaigc.com/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// Regolo AI: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"regolo": {
		BaseURL:    "https://api.regolo.ai/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// Requesty: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"requesty": {
		BaseURL:    "https://router.requesty.ai/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// Routeway: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"routeway": {
		BaseURL:    "https://api.routeway.ai/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// UnoRouter: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"unorouter": {
		BaseURL:    "https://api.unorouter.com/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// Void AI: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"void-ai": {
		BaseURL:    "https://api.voidai.app/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// Wafer AI: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"wafer": {
		BaseURL:    "https://pass.wafer.ai/v1/messages",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
		Format: FormatClaude,
		UpstreamIsAnthropic: true,
		StaticHeaders: map[string]string{"Anthropic-Version": "2023-06-01"},
	},
	// xKiro: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"xkiro": {
		BaseURL:    "https://api.xkiro.com/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// Y-API: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"y-api": {
		BaseURL:    "https://api.y-api.bestvirtualgoods.com/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// Yolo-Auto: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"yolo-auto": {
		BaseURL:    "https://yolo-auto.com/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// ZenMux: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"zenmux": {
		BaseURL:    "https://zenmux.ai/api/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// ZeroLimitAI: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"zerolimitai": {
		BaseURL:    "https://www.zerolimitai.com/api/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// Zylo API: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"zylo-api": {
		BaseURL:    "https://api.zyloai.net/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// OrcaRouter: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"orcarouter": {
		BaseURL:    "https://api.orcarouter.ai/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
		StaticHeaders: map[string]string{"HTTP-Referer": "https://endpoint-proxy.local", "X-Title": "Endpoint Proxy"},
	},
	// MegaNova AI: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"meganova-ai": {
		BaseURL:    "https://api.meganova.ai/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// Docker Model Runner: local self-host lane from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"docker-model-runner": {
		BaseURL:    "http://localhost:12434/engines/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
		LocalByDesign: true,
		AuthOptional: true,
	},
	// Lemonade Server: local self-host lane from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"lemonade": {
		BaseURL:    "http://localhost:13305/api/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
		LocalByDesign: true,
		AuthOptional: true,
	},
	// llama.cpp: local self-host lane from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"llama-cpp": {
		BaseURL:    "http://127.0.0.1:8080/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
		LocalByDesign: true,
		AuthOptional: true,
	},
	// Llamafile: local self-host lane from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"llamafile": {
		BaseURL:    "http://127.0.0.1:8080/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
		LocalByDesign: true,
		AuthOptional: true,
	},
	// LM Studio: local self-host lane from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"lm-studio": {
		BaseURL:    "http://localhost:1234/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
		LocalByDesign: true,
		AuthOptional: true,
	},
	// MLX Gemma 26B: local self-host lane from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"mlx-gemma": {
		BaseURL:    "http://localhost:11435/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
		LocalByDesign: true,
		AuthOptional: true,
	},
	// MLX Qwen 3.8 27B: local self-host lane from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"mlx-qwen": {
		BaseURL:    "http://localhost:11436/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
		LocalByDesign: true,
		AuthOptional: true,
	},
	// oobabooga: local self-host lane from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"oobabooga": {
		BaseURL:    "http://localhost:5000/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
		LocalByDesign: true,
		AuthOptional: true,
	},
	// NVIDIA Triton: local self-host lane from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"triton": {
		BaseURL:    "http://localhost:8000/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
		LocalByDesign: true,
		AuthOptional: true,
	},
	// XInference: local self-host lane from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"xinference": {
		BaseURL:    "http://localhost:9997/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
		LocalByDesign: true,
		AuthOptional: true,
	},
	"river": {
		BaseURL:    "https://api.river.ai/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	"nace": {
		BaseURL:       "https://console.nace.ai",
		AuthHeader:    "Authorization",
		AuthScheme:    "bearer",
		SystemoneURL:  "https://console.nace.ai/v1/systemone",
		SystemonePath: "/v1/systemone",
	},
	"uc-direct": {
		BaseURL:    "https://api.uncensored.com/api/v1/chat/completions",
		AuthHeader: "x-api-key",
		AuthScheme: "raw",
	},
	// Synthetic: mechanical passthrough from the OmniRoute open-sse registry
	// (pinned 632d84f5).
	"synthetic": {
		BaseURL:    "https://api.synthetic.new/openai/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	"zcode": {
		BaseURL: "https://zcode.z.ai/api/v1/zcode-plan/anthropic/v1/messages",
		Format:  FormatClaude,
	},
	"kimchi": {
		BaseURL:    "https://llm.kimchi.dev/openai/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
		StaticHeaders: map[string]string{
			"User-Agent": "kimchi/0.1.50",
		},
	},
	"ideogram": {
		// The image lane POSTs to BaseURL verbatim (no appended path); ImageURL
		// only advertises the endpoint in the catalog.
		BaseURL:    "https://api.ideogram.ai",
		AuthHeader: "Api-Key",
		AuthScheme: "raw",
		ImageURL:   "https://api.ideogram.ai/generate",
	},
	"leonardo": {
		// Pinned OmniRoute 632d84f5: API root plus the native generations
		// endpoint. The media handler POSTs to ImageURL and polls under it.
		BaseURL:    "https://cloud.leonardo.ai/api/rest/v1",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
		ImageURL:   "https://cloud.leonardo.ai/api/rest/v1/generations",
	},
	"iflow": {
		BaseURL:        "https://apis.iflow.cn/v1/chat/completions",
		AuthHeader:     "Authorization",
		AuthScheme:     "bearer",
		ThinkingFormat: "openai",
		Hidden:         true,
	},
	"alitp-intl": {
		// Token Plan is served from the .maas host under the compatible-mode
		// path, not the bare /v1 path (upstream registry/alitp-intl.js:23).
		BaseURL:              "https://token-plan.ap-southeast-1.maas.aliyuncs.com/compatible-mode/v1/chat/completions",
		AuthHeader:           "Authorization",
		AuthScheme:           "bearer",
		PreserveCacheControl: true,
	},
	"fish-audio": {
		BaseURL:    "https://api.fish.audio/v1/tts",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},

	"nebius": {
		BaseURL:    "https://api.studio.nebius.ai/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	"nous": {
		BaseURL:    "https://inference-api.nousresearch.com/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	"minimax": {
		BaseURL:               "https://api.minimax.io/v1/chat/completions",
		AuthHeader:            "Authorization",
		AuthScheme:            "bearer",
		RequireClaudeToolType: true,
	},
	"moonshot": {
		BaseURL:    "https://api.moonshot.ai/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
		Format:     FormatOpenAI,
	},
	"replicate": {
		// The native executor rebuilds this base into /models/{owner}/{name}/
		// predictions (or /predictions for a pinned version).
		BaseURL:    "https://api.replicate.com/v1",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
		Format:     FormatOpenAI,
	},
	"kimi": {
		BaseURL:    "https://api.kimi.com/coding/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	"clinepass": {
		BaseURL:    "https://api.cline.bot/api/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
		// No StaticHeaders: see "cline" above.
	},
	"perplexity-agent": {
		BaseURL:    "https://api.perplexity.ai/v1/responses",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
		Format:     "openai-responses",
	},
	"command-code": {
		BaseURL:    "https://api.commandcode.ai",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
		Format:     FormatOpenAI,
	},
	"commandcode-goat": {
		BaseURL:       "https://api.commandcode.ai/provider/v1/chat/completions",
		AuthHeader:    "Authorization",
		AuthScheme:    "bearer",
		SystemoneURL:  "https://api.commandcode.ai/provider/v1/systemone",
		SystemonePath: "/systemone",
	},
	"commandcode": {
		BaseURL:    "https://api.commandcode.ai/alpha/generate",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
		StaticHeaders: map[string]string{
			// Static identity headers of command-code@1.88.0
			// buildCommandAuthHeaders() (upstream decolua/9router b1effa03).
			"User-Agent":             "cli",
			"x-command-code-version": "1.88.0",
			"x-cli-environment":      "production",
			"x-cli-surface":          "cli",
			"x-taste-learning":       "false",
		},
	},
	"ollama-local": {
		BaseURL:       "http://localhost:11434/v1/chat/completions",
		AuthHeader:    "Authorization",
		AuthScheme:    "bearer",
		LocalByDesign: true,
		SystemoneURL:  "http://localhost:11434/v1/systemone",
		// Saved host (providerSpecificData.baseUrl) is joined with this path.
		SystemonePath: "/v1/systemone",
	},
	"minimax-cn": {
		BaseURL:               "https://api.minimaxi.com/v1/chat/completions",
		AuthHeader:            "Authorization",
		AuthScheme:            "bearer",
		RequireClaudeToolType: true,
	},
	// MiniMax Code (mcode) — the coding-subscription credits lane, served as a
	// provider. Anthropic Messages on MiniMax's agent gateway, signed in with
	// MiniMax Code's own OAuth device flow (PKCE S256, client mcode-public) but
	// with tokens of our own: ~/.minimax is never touched, so a sign-in here
	// neither signs the CLI out nor collides with another account.
	//
	// Global is the same protocol on different hosts, and sign-ins are per
	// site — an account on agent.minimax.cn says nothing about
	// agent.minimax.io. The two are never aliased onto each other
	// (AGENTS.md §3.A). Port of upstream decolua/9router registry
	// minimax-code.js / minimax-code-global.js.
	"minimax-code": {
		BaseURL:               "https://agent.minimax.cn/mavis/api/v1/llm/v1/messages",
		AuthHeader:            "Authorization",
		AuthScheme:            "bearer",
		ThinkingFormat:        "claude-adaptive",
		Format:                "claude",
		RequireClaudeToolType: true,
		StaticHeaders: map[string]string{
			"User-Agent":       "MiniMaxAgent",
			"X-Mavis-Agent-Id": "main",
			// MiniMax requires the anthropic-version header on its Messages
			// endpoint; the bearer carries the real credential.
			"anthropic-version": "2023-06-01",
		},
	},
	"minimax-code-global": {
		BaseURL:               "https://agent.minimax.io/mavis/api/v1/llm/v1/messages",
		AuthHeader:            "Authorization",
		AuthScheme:            "bearer",
		ThinkingFormat:        "claude-adaptive",
		Format:                "claude",
		RequireClaudeToolType: true,
		StaticHeaders: map[string]string{
			"User-Agent":        "MiniMaxAgent",
			"X-Mavis-Agent-Id":  "main",
			"anthropic-version": "2023-06-01",
		},
	},
	"kimi-coding": {
		BaseURL:    "https://api.kimi.com/coding/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	"claude": {
		BaseURL:    "https://api.anthropic.com/v1/messages",
		AuthHeader: "x-api-key",
		AuthScheme: "raw",
		StaticHeaders: map[string]string{
			"anthropic-version":                         "2023-06-01",
			"Anthropic-Beta":                            "claude-code-20250219,oauth-2025-04-20,interleaved-thinking-2025-05-14,context-management-2025-06-27,prompt-caching-scope-2026-01-05,advanced-tool-use-2025-11-20,effort-2025-11-24,structured-outputs-2025-12-15,fast-mode-2026-02-01,redact-thinking-2026-02-12,token-efficient-tools-2026-03-28",
			"Anthropic-Dangerous-Direct-Browser-Access": "true",
			"User-Agent":                                "claude-cli/2.1.280 (external, sdk-cli)",
			"X-App":                                     "cli",
			"X-Stainless-Helper-Method":                 "stream",
			"X-Stainless-Retry-Count":                   "0",
		},
	},
	"codex": {
		BaseURL:    "https://chatgpt.com/backend-api/codex/responses",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
		StaticHeaders: map[string]string{
			"originator": "codex_cli_rs",
			"User-Agent": CodexCLIUserAgent,
			// Upstream sends `version` next to the User-Agent; the codex
			// backend gates newer models on the pair.
			"version": CodexCLIVersionHeader,
		},
	},
	"grok-cli": {
		BaseURL:    "https://cli-chat-proxy.grok.com/v1/responses",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	"kiro": {
		BaseURL:    "https://q.us-east-1.amazonaws.com/generateAssistantResponse",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
		// The Kiro envelope carries the effort in its own schema, not in a body
		// field. Declaring the format keeps the pass from forwarding a
		// reasoning dialect the envelope does not carry.
		ThinkingFormat: "kiro",
	},
	// Amazon Q rides the same Kiro AWS endpoint (OmniRoute KiroExecutor("amazon-q")
	// pattern, pinned 632d84f5).
	"amazon-q": {
		BaseURL:    "https://q.us-east-1.amazonaws.com/generateAssistantResponse",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	"bedrock": {
		// The executor substitutes the region per request and signs each request with
		// SigV4, so AuthHeader/AuthScheme are absent by design: there is no static
		// bearer token to configure.
		BaseURL: "https://bedrock-runtime.{region}.amazonaws.com",
		// Anthropic models on Bedrock take the Anthropic Messages body verbatim, so the
		// existing claude translators are reused rather than a Bedrock-specific pair.
		Format:              "claude",
		UpstreamIsAnthropic: true,
	},
	"bedrock-xai": {
		// Same credential story, but no Format: Grok on Bedrock speaks OpenAI Chat
		// Completions, so the standard openai wire applies and the response needs no
		// translation.
		BaseURL: "https://bedrock-runtime.{region}.amazonaws.com",
	},
	"elevenlabs": {
		BaseURL:    "https://api.elevenlabs.io",
		AuthHeader: "xi-api-key",
		AuthScheme: "raw",
		TTSURL:     "https://api.elevenlabs.io/v1/text-to-speech",
		// Scribe STT is a multipart endpoint with its own auth header and its own
		// render formats, so it cannot ride the OpenAI-compatible transcriptions
		// lane the rest of the media handlers use.
		STTURL:    "https://api.elevenlabs.io/v1/speech-to-text",
		VoicesURL: "https://api.elevenlabs.io/v1/voices",
	},
	"deepgram": {
		BaseURL:    "https://api.deepgram.com",
		AuthHeader: "token",
		AuthScheme: "raw",
		STTURL:     "https://api.deepgram.com/v1/listen",
	},
	"assemblyai": {
		BaseURL:    "https://api.assemblyai.com",
		AuthHeader: "Authorization",
		AuthScheme: "raw",
		STTURL:     "https://api.assemblyai.com/v2/transcript",
	},
	"stability-ai": {
		BaseURL:    "https://api.stability.ai",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
		ImageURL:   "https://api.stability.ai/v2beta/stable-image/generate",
	},
	"black-forest-labs": {
		BaseURL:    "https://api.bfl.ai",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
		ImageURL:   "https://api.bfl.ai/v1",
	},
	"fal-ai": {
		BaseURL:    "https://queue.fal.run",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
		ImageURL:   "https://queue.fal.run",
	},
	"recraft": {
		BaseURL:       "https://external.api.recraft.ai",
		AuthHeader:    "Authorization",
		AuthScheme:    "bearer",
		DefaultAPIKey: "public",
		ImageURL:      "https://external.api.recraft.ai/v1/images/generations",
	},
	"azure": {
		BaseURL:    "",
		AuthHeader: "api-key",
		AuthScheme: "raw",
	},
	"jina-reader": {
		BaseURL:     "https://r.jina.ai",
		AuthHeader:  "Authorization",
		AuthScheme:  "bearer",
		FetchURL:    "https://r.jina.ai",
		FetchMethod: "GET",
	},
	"firecrawl": {
		// Firecrawl serves .dev, not .com; upstream hits
		// https://api.firecrawl.dev/v1/scrape (registry/firecrawl.js:20).
		BaseURL:    "https://api.firecrawl.dev/v1",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
		FetchURL:   "https://api.firecrawl.dev/v1/scrape",
	},
	"freebuff": {
		BaseURL:    "https://www.codebuff.com/api/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
		StaticHeaders: map[string]string{
			"User-Agent": "ai-sdk/openai-compatible/1.0/codebuff",
		},
	},

	"aws-polly": {
		BaseURL:    "https://polly.us-east-1.amazonaws.com/v1/speech",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	"brave-search": {
		BaseURL:      "https://api.search.brave.com/res/v1/web/search",
		AuthHeader:   "X-Subscription-Token",
		AuthScheme:   "raw",
		SearchURL:    "https://api.search.brave.com/res/v1/web/search",
		SearchMethod: "GET",
	},
	"cartesia": {
		BaseURL:    "https://api.cartesia.ai/tts/bytes",
		AuthHeader: "x-api-key",
		AuthScheme: "raw",
		TTSURL:     "https://api.cartesia.ai/tts/bytes",
		Hidden:     true,
	},
	"exa": {
		BaseURL:      "https://api.exa.ai/search",
		AuthHeader:   "x-api-key",
		AuthScheme:   "raw",
		SearchURL:    "https://api.exa.ai/search",
		SearchMethod: "POST",
		FetchURL:     "https://api.exa.ai/contents",
		FetchMethod:  "POST",
	},
	"huggingface": {
		// The legacy api-inference.huggingface.co host is gone; upstream
		// open-sse/providers/registry/huggingface.js:64-99 routes everything
		// through the Inference Providers router.
		BaseURL:    "https://router.huggingface.co",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
		ImageURL:   "https://router.huggingface.co",
		STTURL:     "https://router.huggingface.co/hf-inference/models",
	},
	"inworld": {
		BaseURL:    "https://api.inworld.ai/tts/v1/voice",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
		TTSURL:     "https://api.inworld.ai/tts/v1/voice",
	},
	"jina-ai": {
		BaseURL:    "https://api.jina.ai/v1/embeddings",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	"linkup": {
		BaseURL:      "https://api.linkup.so/v1/search",
		AuthHeader:   "Authorization",
		AuthScheme:   "bearer",
		SearchURL:    "https://api.linkup.so/v1/search",
		SearchMethod: "POST",
	},
	// Perplexity Web (Pro/Max): cookie site with no executor behind it yet —
	// upstream parks it the same way (serviceKinds: [], "the code and saved
	// connections remain intact"), so the cookie probe and any stored
	// connection keep working while it stays out of the lists a person scans.
	"perplexity-web": {
		BaseURL:    "https://www.perplexity.ai/rest/sse/perplexity_ask",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
		Hidden:     true,
	},
	"playht": {
		BaseURL:    "https://api.play.ht/api/v2/tts/stream",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
		TTSURL:     "https://api.play.ht/api/v2/tts/stream",
		Hidden:     true,
	},
	// TinyFish — one x-api-key credential behind two separate hosts (search
	// and fetch), which is why BaseURL is a non-endpoint root here: appending
	// /v1/search to it would hit a host that does not serve search.
	"tinyfish": {
		BaseURL:      "https://api.tinyfish.ai",
		AuthHeader:   "x-api-key",
		AuthScheme:   "raw",
		FetchURL:     "https://api.fetch.tinyfish.ai",
		FetchMethod:  "POST",
		SearchURL:    "https://api.search.tinyfish.ai",
		SearchMethod: "GET",
	},
	"runwayml": {
		BaseURL:    "https://api.dev.runwayml.com/v1",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
		ImageURL:   "https://api.dev.runwayml.com/v1",
	},
	"searchapi": {
		BaseURL:      "https://www.searchapi.io/api/v1/search",
		AuthHeader:   "Authorization",
		AuthScheme:   "bearer",
		SearchURL:    "https://www.searchapi.io/api/v1/search",
		SearchMethod: "GET",
	},
	"serper": {
		BaseURL:      "https://google.serper.dev",
		AuthHeader:   "x-api-key",
		AuthScheme:   "raw",
		SearchURL:    "https://google.serper.dev/search",
		SearchMethod: "POST",
	},
	"tavily": {
		BaseURL:      "https://api.tavily.com/search",
		AuthHeader:   "Authorization",
		AuthScheme:   "bearer",
		SearchURL:    "https://api.tavily.com/search",
		SearchMethod: "POST",
		FetchURL:     "https://api.tavily.com/extract",
		FetchMethod:  "POST",
	},
	"xquik": {
		BaseURL:      "https://xquik.com/api/v1/x/tweets/search",
		AuthHeader:   "x-api-key",
		AuthScheme:   "raw",
		SearchURL:    "https://xquik.com/api/v1/x/tweets/search",
		SearchMethod: "GET",
	},
	"ollama-search": {
		BaseURL:      "https://ollama.com/api/web_search",
		AuthHeader:   "Authorization",
		AuthScheme:   "bearer",
		SearchURL:    "https://ollama.com/api/web_search",
		SearchMethod: "POST",
	},
	"zai-search": {
		BaseURL:      "https://api.z.ai/api/mcp/web_search",
		AuthHeader:   "Authorization",
		AuthScheme:   "bearer",
		SearchURL:    "https://api.z.ai/api/mcp/web_search",
		SearchMethod: "POST",
	},
	"vertex": {
		BaseURL:    "https://aiplatform.googleapis.com/v1",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	"vertex-partner": {
		BaseURL:    "https://aiplatform.googleapis.com/v1",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	"voyage-ai": {
		BaseURL:    "https://api.voyageai.com/v1/embeddings",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	"youcom": {
		BaseURL:      "https://ydc-index.io/v1/search",
		AuthHeader:   "x-api-key",
		AuthScheme:   "raw",
		SearchURL:    "https://ydc-index.io/v1/search",
		SearchMethod: "GET",
	},
	"qoder": {
		BaseURL:    "https://api3.qoder.sh/algo/api/v2/service/pro/sse/agent_chat_generation",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// Qoder CN is a distinct deployment with its own gateway and credentials.
	// It is never aliased onto qoder (AGENTS.md section 3.A).
	"qoder-cn": {
		BaseURL:    "https://gateway.qoder.com.cn/algo/api/v2/service/pro/sse/agent_chat_generation",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	"grok-web": {
		BaseURL:    "https://grok.com/rest/app-chat/conversations/new",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// Microsoft Copilot web: OpenAI-format passthrough over the Copilot web
	// /c/api (xrouter registry transport, pinned 7fd5698). The stored
	// credential is the browser Cookie header, sent verbatim (scheme raw).
	"copilot-web": {
		BaseURL:    "https://copilot.microsoft.com/c/api/chat/completions",
		AuthHeader: "Cookie",
		AuthScheme: "raw",
	},
	// Tencent Yuanbao web: same passthrough shape (xrouter registry transport,
	// pinned 7fd5698) over yuanbao.tencent.com's /api.
	"yuanbao-web": {
		BaseURL:    "https://yuanbao.tencent.com/api/chat/completions",
		AuthHeader: "Cookie",
		AuthScheme: "raw",
	},
	// Z.ai web (chat.z.ai): the credential is the Local Storage "token" JWT,
	// sent as Bearer (xrouter registry transport, pinned 7fd5698).
	"zai-web": {
		BaseURL:    "https://chat.z.ai/openai/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// Notion AI web: OpenAI-format passthrough over notion.so's internal
	// /api/v3 (xrouter registry transport, pinned 7fd5698); the credential is
	// the browser Cookie header carrying token_v2, sent verbatim.
	"notion-web": {
		BaseURL:    "https://www.notion.so/api/v3/chat/completions",
		AuthHeader: "Cookie",
		AuthScheme: "raw",
	},
	// OVH Kepler AI keyless endpoint (xrouter registry transport, pinned
	// 7fd5698): anonymous OpenAI chat; the transport sends an empty
	// Authorization header, which the NoAuth config expresses by sending none.
	"ovh": {
		BaseURL:    "https://oai.endpoints.kepler.ai.cloud.ovh.net/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
		NoAuth:     true,
	},
	// AI Horde crowdsourced inference: OpenAI-compatible chat over
	// oai.aihorde.net; the documented anonymous key rides when a connection
	// carries one, and NoAuth lets keyless requests through.
	"aihorde": {
		BaseURL:    "https://oai.aihorde.net/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
		AuthOptional: true,
	},
	// arena.ai evaluation lane: the executor owns the wire protocol; the
	// BaseURL only lets an operator repoint the endpoint.
	"arena-ai": {
		BaseURL:    "https://arena.ai/nextjs-api/stream/create-evaluation",
		AuthHeader: "Cookie",
		AuthScheme: "raw",
	},
	// UC web (uncensored.com persona): image + video over the un-metered
	// internal API; the credential is the full uncensored.com Cookie header
	// and every request mints a short-lived Clerk session from __client.
	"uc-web": {
		BaseURL:    "https://internal.chatuncensored.ai",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	// veoaifree.com free tier: WordPress AJAX, no credentials (~6 requests
	// per hour per IP); image and video lanes are executor-driven.
	"veoaifree-web": {
		BaseURL:    "https://veoaifree.com",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
		NoAuth:     true,
	},
	// chatgpt-web is a browser-session executor: the credential is the
	// __Secure-next-auth.session-token cookie (or a browser accessToken),
	// never an API key. Disabled by default — see ENABLE_WEB_EXECUTORS and
	// executor.CheckWebExecutorAllowed. Grey-zone ToS: use your own account.
	"chatgpt-web": {
		BaseURL:    "https://chatgpt.com/backend-api/conversation",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},

	"sdwebui": {
		BaseURL:  "http://localhost:7860/sdapi/v1/txt2img",
		NoAuth:   true,
		ImageURL: "http://localhost:7860/sdapi/v1/txt2img",
	},
	"searxng": {
		BaseURL:      "http://localhost:4000/search",
		NoAuth:       true,
		SearchURL:    "http://localhost:4000/search",
		SearchMethod: "GET",
	},
	"comfyui": {
		BaseURL:  "http://localhost:8188",
		NoAuth:   true,
		ImageURL: "http://localhost:8188",
	},
	"tortoise": {
		BaseURL: "http://localhost:5000/api/tts",
		NoAuth:  true,
		TTSURL:  "http://localhost:5000/api/tts",
		Hidden:  true,
	},
	"coqui": {
		BaseURL: "http://localhost:5002/api/tts",
		NoAuth:  true,
		TTSURL:  "http://localhost:5002/api/tts",
		Hidden:  true,
	},
	"edge-tts": {
		BaseURL: "",
		NoAuth:  true,
	},
	"google-tts": {
		BaseURL: "",
		NoAuth:  true,
	},
	"local-device": {
		BaseURL: "",
		NoAuth:  true,
	},
	"topaz": {
		BaseURL:    "https://api.topaz.sh",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	"cursor": {
		BaseURL:    "https://api2.cursor.sh",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	"gemini-cli": {
		BaseURL:    "https://daily-cloudcode-pa.googleapis.com/v1internal",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	"google-pse": {
		BaseURL:      "https://www.googleapis.com/customsearch/v1",
		AuthHeader:   "Authorization",
		AuthScheme:   "bearer",
		SearchURL:    "https://www.googleapis.com/customsearch/v1",
		SearchMethod: "GET",
	},
	"alims-intl": {
		BaseURL:              "https://dashscope-intl.aliyuncs.com/compatible-mode/v1/chat/completions",
		AuthHeader:           "Authorization",
		AuthScheme:           "bearer",
		PreserveCacheControl: true,
	},
	"api-airforce": {
		BaseURL:    "https://api.airforce/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
		StaticHeaders: map[string]string{
			"HTTP-Referer": "https://endpoint-proxy.local",
			"X-Title":      "Endpoint Proxy",
		},
	},
	"baidu": {
		BaseURL:    "https://qianfan.baidubce.com/v2/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	"daoxe": {
		BaseURL:    "https://api.daoxe.com/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	"bazaarlink": {
		BaseURL:    "https://bazaarlink.ai/api/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	"bluesminds": {
		BaseURL:    "https://api.bluesminds.com/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
		Hidden:     true,
	},
	"devin-cli": {
		BaseURL:    "devin://acp/stdio",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	"kilo-gateway": {
		BaseURL:    "https://api.kilo.ai/api/gateway/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	"llm7": {
		BaseURL:    "https://api.llm7.io/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	"morph": {
		BaseURL:    "https://api.morphllm.com/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	"nan": {
		BaseURL:    "https://api.nan.builders/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	"openadapter": {
		BaseURL:    "https://api.openadapter.in/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	"poolside": {
		BaseURL:    "https://inference.poolside.ai/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	"sambanova": {
		BaseURL:    "https://api.sambanova.ai/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
		Hidden:     true,
	},
	"tencent": {
		BaseURL:    "https://api.hunyuan.cloud.tencent.com/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	"tokenharbor": {
		BaseURL:    "https://tokenharbor.ai/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	"tokenrouter": {
		BaseURL:        "https://api.tokenrouter.com/v1/chat/completions",
		AuthHeader:     "Authorization",
		AuthScheme:     "bearer",
		ThinkingFormat: "tokenrouter",
	},
	"trae": {
		BaseURL:    "https://core-normal.trae.ai/api/remote/v1",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
		StaticHeaders: map[string]string{
			"X-Trae-Client-Type":     "web",
			"X-Preferenced-Language": "en",
			"Referer":                "https://solo.trae.ai/",
		},
	},
	"windsurf": {
		BaseURL:    "https://server.codeium.com/exa.language_server_pb.LanguageServerService/GetChatMessage",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
		StaticHeaders: map[string]string{
			"Content-Type": "application/grpc-web+proto",
			"Accept":       "application/grpc-web+proto",
			"X-Grpc-Web":   "1",
		},
	},
	"zed": {
		BaseURL:    "https://cloud.zed.dev/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
		StaticHeaders: map[string]string{
			"content-type": "application/json",
		},
	},
	"atmorouter": {
		BaseURL:    "https://atmorouter.dev/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
	"runcrate": {
		BaseURL:    "https://api.runcrate.ai/v1/chat/completions",
		AuthHeader: "Authorization",
		AuthScheme: "bearer",
	},
}

// RetryableStatusCodes are HTTP status codes that trigger account fallback.
var RetryableStatusCodes = map[int]bool{
	http.StatusUnauthorized:       true, // 401
	http.StatusPaymentRequired:    true, // 402 (insufficient funds on paid models)
	http.StatusForbidden:          true, // 403 (Gemini/antigravity daily-quota errors can come as 403)
	http.StatusTooManyRequests:    true, // 429
	http.StatusBadGateway:         true, // 502
	http.StatusServiceUnavailable: true, // 503
	http.StatusGatewayTimeout:     true, // 504
}

// PreservesCacheControl reports whether a provider keeps the cache_control
// markers on its content blocks (upstream's quirks.preserveCacheControl).
func PreservesCacheControl(provider string) bool {
	if cfg, ok := KnownProviders[provider]; ok {
		return cfg.PreserveCacheControl
	}
	return false
}

// IsLocalByDesign reports whether a provider's official upstream is intended
// to run locally on loopback (e.g. ollama-local on localhost:11434).
func IsLocalByDesign(provider string) bool {
	if cfg, ok := KnownProviders[provider]; ok {
		return cfg.LocalByDesign
	}
	return false
}

// ResolveStallTimeout returns the stall timeout for this provider config.
// If c != nil && c.StallTimeoutMs > 0, returns time.Duration(c.StallTimeoutMs) * time.Millisecond.
// Otherwise returns defaultTimeout.
func (c *ProviderConfig) ResolveStallTimeout(defaultTimeout time.Duration) time.Duration {
	if c != nil && c.StallTimeoutMs > 0 {
		return time.Duration(c.StallTimeoutMs) * time.Millisecond
	}
	return defaultTimeout
}
