// 9RouterX — port and modifications by https://github.com/yanstd888/ — https://t.me/aiclub_id
package providers

import (
	"slices"
	"testing"
)

func TestProviderBatchAliasesRoundTrip(t *testing.T) {
	for _, id := range []string{"alibaba", "featherless-ai", "ovhcloud", "aihorde", "uc-direct", "synthetic", "river", "nace", "alibaba-cn", "agnes-cn", "volcengine-coding-plan", "volcengine-agent-plan", "leonardo", "opencode-server", "command-code"} {
		meta := ProviderRegistry[id]
		if got := ResolveAlias(meta.Alias); got != id {
			t.Errorf("%s publishes alias %q resolving to %q", id, meta.Alias, got)
		}
	}
	if got := ResolveAlias("ali"); got != "alicode" {
		t.Errorf("existing ali alias was changed to %q", got)
	}
	cfg := KnownProviders["leonardo"]
	if cfg.BaseURL != "https://cloud.leonardo.ai/api/rest/v1" ||
		cfg.ImageURL != "https://cloud.leonardo.ai/api/rest/v1/generations" ||
		cfg.AuthHeader != "Authorization" || cfg.AuthScheme != "bearer" {
		t.Errorf("leonardo config = %+v", cfg)
	}
	for _, model := range []string{"phoenix", "sdxl"} {
		if got := GetProviderModelKind("leonardo", model); got != "image" {
			t.Errorf("leonardo/%s kind = %q, want image", model, got)
		}
	}
	meta := ProviderRegistry["opencode-server"]
	if meta.Name != "OpenCode CLI Server" || meta.Alias != "ocs" || meta.Icon != "terminal" ||
		meta.Color != "#F59E0B" || meta.Website != "https://opencode.ai/docs/server/" || meta.AuthType != "none" {
		t.Errorf("opencode-server metadata = %+v", meta)
	}
	serverCfg := KnownProviders["opencode-server"]
	if serverCfg.BaseURL != "http://127.0.0.1:4096" || serverCfg.CatalogURL != "http://127.0.0.1:4096/config/providers" ||
		!serverCfg.NoAuth || !serverCfg.AuthOptional || !serverCfg.LocalByDesign || serverCfg.Format != FormatOpenAI {
		t.Errorf("opencode-server config = %+v", serverCfg)
	}
	for _, model := range []string{"big-pickle", "space-bunny-free", "muse-spark-1.3-contributor-free", "nemotron-3-ultra-free", "mimo-v2.6-flash-free", "deepseek-v4.1-flash-free"} {
		if !slices.Contains(ProviderModels["opencode-server"], model) {
			t.Errorf("opencode-server model catalog lost %q", model)
		}
	}
	ccMeta := ProviderRegistry["command-code"]
	if ccMeta.Name != "Command Code" || ccMeta.Alias != "cmd" || ccMeta.Icon != "terminal" || ccMeta.Color != "#111827" || ccMeta.Website != "https://commandcode.ai/" {
		t.Errorf("command-code metadata = %+v", ccMeta)
	}
	ccCfg := KnownProviders["command-code"]
	if ccCfg.BaseURL != "https://api.commandcode.ai" || ccCfg.AuthHeader != "Authorization" || ccCfg.AuthScheme != "bearer" || ccCfg.Format != FormatOpenAI {
		t.Errorf("command-code config = %+v", ccCfg)
	}
	if len(ProviderModels["command-code"]) != 22 || len(ProviderModels["cmd"]) != 22 {
		t.Errorf("command-code catalog = %d/%d models", len(ProviderModels["command-code"]), len(ProviderModels["cmd"]))
	}
	if got := ResolveAlias("cmd"); got != "command-code" {
		t.Errorf("cmd resolved to %q", got)
	}
	if got := GetCapabilitiesDetailForModel("cmd", "claude-sonnet-5-5"); got.ContextWindow != 200000 || got.MaxOutput != 16384 {
		t.Errorf("cmd alias capabilities = %+v", got)
	}
	if got := ResolveAlias("ocs"); got != "opencode-server" {
		t.Errorf("ocs resolved to %q", got)
	}
	if got := GetProviderModelKind("drex", "drex-max"); got != "systemone" {
		t.Errorf("Drex kind = %q, want systemone", got)
	}
}

func TestProviderBatchOptionalAuthConfig(t *testing.T) {
	for _, id := range []string{"docker-model-runner", "lemonade", "llama-cpp", "llamafile", "lm-studio", "mlx-gemma", "mlx-qwen", "oobabooga", "triton", "xinference", "aihorde", "ovhcloud"} {
		cfg := KnownProviders[id]
		if !cfg.AuthOptional || cfg.NoAuth {
			t.Errorf("%s must permit anonymous access without suppressing supplied credentials", id)
		}
	}
	cfg := KnownProviders["helixmind"]
	if cfg.AuthHeader != "Authorization" || cfg.AuthScheme != "bearer" || cfg.Format == FormatClaude || cfg.UpstreamIsAnthropic {
		t.Errorf("helixmind default inherited its alternate protocol: %+v", cfg)
	}
}

func TestProviderBatchUnportedCustomLanesExcluded(t *testing.T) {
	for _, id := range []string{"haiper", "dify", "udio", "nlpcloud", "oneminai", "gigachat", "keenable"} {
		if _, ok := ProviderRegistry[id]; ok {
			t.Errorf("%s is selectable before its custom protocol is ported", id)
		}
		if _, ok := KnownProviders[id]; ok {
			t.Errorf("%s has an incorrect generic runtime config", id)
		}
	}
}
