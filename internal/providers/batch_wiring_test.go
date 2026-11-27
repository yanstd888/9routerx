// 9RouterX — port and modifications by https://github.com/yanstd888/ — https://t.me/aiclub_id
package providers

import (
	"slices"
	"testing"
)

func TestProviderBatchAliasesRoundTrip(t *testing.T) {
	for _, id := range []string{"alibaba", "featherless-ai", "ovhcloud", "aihorde", "uc-direct", "synthetic", "river", "nace", "alibaba-cn", "agnes-cn", "volcengine-coding-plan", "volcengine-agent-plan", "leonardo", "opencode-server", "command-code", "commandcode-goat", "poe", "nlpcloud", "cerebrium", "clova-studio", "gigachat", "udio", "haiper", "huggingchat", "tencent-aistudio-web", "oneminai", "keenable", "notrack-web", "chatplayground"} {
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
	goatMeta := ProviderRegistry["commandcode-goat"]
	if goatMeta.Name != "Command Code (Provider API)" || goatMeta.Alias != "cmc-goat" || goatMeta.Website != "https://commandcode.ai/docs/provider" {
		t.Errorf("commandcode-goat metadata = %+v", goatMeta)
	}
	goatCfg := KnownProviders["commandcode-goat"]
	if goatCfg.BaseURL != "https://api.commandcode.ai/provider/v1/chat/completions" || goatCfg.SystemoneURL != "https://api.commandcode.ai/provider/v1/systemone" || goatCfg.AuthScheme != "bearer" {
		t.Errorf("commandcode-goat config = %+v", goatCfg)
	}
	if len(ProviderModels["commandcode-goat"]) != 97 || len(ProviderModels["cmc-goat"]) != 97 {
		t.Errorf("commandcode-goat catalog = %d/%d models", len(ProviderModels["commandcode-goat"]), len(ProviderModels["cmc-goat"]))
	}
	if got := ResolveAlias("cmc-goat"); got != "commandcode-goat" {
		t.Errorf("cmc-goat resolved to %q", got)
	}
	if got := CommandcodeGoatTarget("claude-opus-5-5"); got != FormatClaude {
		t.Errorf("claude target = %q", got)
	}
	poeMeta := ProviderRegistry["poe"]
	if poeMeta.Name != "Poe" || poeMeta.Alias != "poe" || poeMeta.Icon != "hub" || poeMeta.Color != "#F97316" || poeMeta.Website != "https://creator.poe.com/api-reference" {
		t.Errorf("poe metadata = %+v", poeMeta)
	}
	poeCfg := KnownProviders["poe"]
	if poeCfg.BaseURL != "https://api.poe.com/v1/chat/completions" || poeCfg.AuthScheme != "bearer" || poeCfg.Format != FormatOpenAI {
		t.Errorf("poe config = %+v", poeCfg)
	}
	if got := ResolveAlias("poe"); got != "poe" {
		t.Errorf("poe resolved to %q", got)
	}
	if len(ProviderModels["poe"]) != 3 {
		t.Errorf("poe catalog = %v", ProviderModels["poe"])
	}
	if got, ok := GetModelFormats("poe", "claude-opus-4.8"); !ok || got.TargetFormat != FormatClaude {
		t.Errorf("poe Claude formats = %+v, ok=%t", got, ok)
	}
	nlpMeta := ProviderRegistry["nlpcloud"]
	if nlpMeta.Name != "NLP Cloud" || nlpMeta.Alias != "nlpc" || nlpMeta.Icon != "psychology" || nlpMeta.Color != "#2196F3" || nlpMeta.Website != "https://docs.nlpcloud.com" {
		t.Errorf("nlpcloud metadata = %+v", nlpMeta)
	}
	nlpCfg := KnownProviders["nlpcloud"]
	if nlpCfg.BaseURL != "https://api.nlpcloud.io/v1/gpu" || nlpCfg.AuthScheme != "raw" || nlpCfg.Format != FormatOpenAI {
		t.Errorf("nlpcloud config = %+v", nlpCfg)
	}
	if len(ProviderModels["nlpcloud"]) != 5 || len(ProviderModels["nlpc"]) != 5 {
		t.Errorf("nlpcloud catalog = %d/%d models", len(ProviderModels["nlpcloud"]), len(ProviderModels["nlpc"]))
	}
	if got := ResolveAlias("nlpc"); got != "nlpcloud" {
		t.Errorf("nlpc resolved to %q", got)
	}
	cerebriumMeta := ProviderRegistry["cerebrium"]
	if cerebriumMeta.Name != "Cerebrium" || cerebriumMeta.Alias != "cerebrium" || cerebriumMeta.Icon != "memory" || cerebriumMeta.Color != "#EB3A6F" || cerebriumMeta.Website != "https://www.cerebrium.ai" {
		t.Errorf("cerebrium metadata = %+v", cerebriumMeta)
	}
	cerebriumCfg := KnownProviders["cerebrium"]
	if cerebriumCfg.BaseURL != "https://api.cerebrium.ai/v4" || cerebriumCfg.AuthScheme != "bearer" || cerebriumCfg.Format != FormatOpenAI {
		t.Errorf("cerebrium config = %+v", cerebriumCfg)
	}
	if len(ProviderModels["cerebrium"]) != 1 {
		t.Errorf("cerebrium catalog = %v", ProviderModels["cerebrium"])
	}
	if got := ResolveAlias("cerebrium"); got != "cerebrium" {
		t.Errorf("cerebrium resolved to %q", got)
	}
	clovaMeta := ProviderRegistry["clova-studio"]
	if clovaMeta.Name != "Naver CLOVA Studio" || clovaMeta.Alias != "clova" || clovaMeta.Icon != "auto_awesome" || clovaMeta.Color != "#03C75A" {
		t.Errorf("clova-studio metadata = %+v", clovaMeta)
	}
	clovaCfg := KnownProviders["clova-studio"]
	if clovaCfg.BaseURL != "https://clovastudio.stream.ntruss.com/v3/chat-completions" || clovaCfg.AuthScheme != "bearer" {
		t.Errorf("clova-studio config = %+v", clovaCfg)
	}
	if len(ProviderModels["clova-studio"]) != 3 || len(ProviderModels["clova"]) != 3 {
		t.Errorf("clova-studio catalog = %d/%d models", len(ProviderModels["clova-studio"]), len(ProviderModels["clova"]))
	}
	if got := ResolveAlias("clova"); got != "clova-studio" {
		t.Errorf("clova resolved to %q", got)
	}
	gigaMeta := ProviderRegistry["gigachat"]
	if gigaMeta.Name != "GigaChat (Sber)" || gigaMeta.Alias != "gigachat" || gigaMeta.Icon != "lock_person" || gigaMeta.Color != "#10B981" || gigaMeta.Website != "https://developers.sber.ru" {
		t.Errorf("gigachat metadata = %+v", gigaMeta)
	}
	gigaCfg := KnownProviders["gigachat"]
	if gigaCfg.BaseURL != "https://gigachat.devices.sberbank.ru/api/v1/chat/completions" || gigaCfg.AuthScheme != "bearer" || gigaCfg.Format != FormatOpenAI {
		t.Errorf("gigachat config = %+v", gigaCfg)
	}
	if len(ProviderModels["gigachat"]) != 3 {
		t.Errorf("gigachat catalog = %v", ProviderModels["gigachat"])
	}
	if got := ResolveAlias("gigachat"); got != "gigachat" {
		t.Errorf("gigachat resolved to %q", got)
	}
	udioMeta := ProviderRegistry["udio"]
	if udioMeta.Name != "Udio" || udioMeta.Alias != "udio" || udioMeta.Icon != "music_note" || udioMeta.Color != "#10B981" || udioMeta.Website != "https://udio.com" {
		t.Errorf("udio metadata = %+v", udioMeta)
	}
	udioCfg := KnownProviders["udio"]
	if udioCfg.BaseURL != "https://www.udio.com/api/generate-proxy" || udioCfg.AuthHeader != "Cookie" || udioCfg.AuthScheme != "raw" {
		t.Errorf("udio config = %+v", udioCfg)
	}
	if len(ProviderModels["udio"]) != 1 {
		t.Errorf("udio catalog = %v", ProviderModels["udio"])
	}
	if got := ResolveAlias("udio"); got != "udio" {
		t.Errorf("udio resolved to %q", got)
	}
	haiperMeta := ProviderRegistry["haiper"]
	if haiperMeta.Name != "Haiper" || haiperMeta.Alias != "hp" || haiperMeta.Icon != "videocam" || haiperMeta.Color != "#6366F1" || haiperMeta.Website != "https://haiper.ai" {
		t.Errorf("haiper metadata = %+v", haiperMeta)
	}
	haiperCfg := KnownProviders["haiper"]
	if haiperCfg.BaseURL != "https://api.haiper.ai/v1" || haiperCfg.AuthHeader != "HAIPER_KEY" || haiperCfg.AuthScheme != "raw" {
		t.Errorf("haiper config = %+v", haiperCfg)
	}
	if len(ProviderModels["haiper"]) != 2 || len(ProviderModels["hp"]) != 2 {
		t.Errorf("haiper catalog = %d/%d models", len(ProviderModels["haiper"]), len(ProviderModels["hp"]))
	}
	if got := ResolveAlias("hp"); got != "haiper" {
		t.Errorf("hp resolved to %q", got)
	}
	huggingMeta := ProviderRegistry["huggingchat"]
	if huggingMeta.Name != "HuggingChat (Free)" || huggingMeta.Alias != "huggingchat" || huggingMeta.Icon != "auto_awesome" || huggingMeta.Color != "#FFD21E" || huggingMeta.Website != "https://huggingface.co/chat" {
		t.Errorf("huggingchat metadata = %+v", huggingMeta)
	}
	huggingCfg := KnownProviders["huggingchat"]
	if huggingCfg.BaseURL != "https://huggingface.co/chat" || huggingCfg.AuthHeader != "Cookie" || huggingCfg.AuthScheme != "raw" {
		t.Errorf("huggingchat config = %+v", huggingCfg)
	}
	if len(ProviderModels["huggingchat"]) != 24 {
		t.Errorf("huggingchat catalog = %d models", len(ProviderModels["huggingchat"]))
	}
	if got := ResolveAlias("huggingchat"); got != "huggingchat" {
		t.Errorf("huggingchat resolved to %q", got)
	}
	taswMeta := ProviderRegistry["tencent-aistudio-web"]
	if taswMeta.Name != "Tencent AI Studio (Free)" || taswMeta.Alias != "tasw" || taswMeta.Icon != "auto_awesome" || taswMeta.Color != "#0052D9" || taswMeta.Website != "https://aistudio.tencent.ai" {
		t.Errorf("tencent-aistudio-web metadata = %+v", taswMeta)
	}
	taswCfg := KnownProviders["tencent-aistudio-web"]
	if taswCfg.BaseURL != "https://aistudio.tencent.ai" || taswCfg.AuthHeader != "Cookie" || taswCfg.AuthScheme != "raw" {
		t.Errorf("tencent-aistudio-web config = %+v", taswCfg)
	}
	if len(ProviderModels["tencent-aistudio-web"]) != 3 || len(ProviderModels["tasw"]) != 3 {
		t.Errorf("tencent catalog = %d/%d models", len(ProviderModels["tencent-aistudio-web"]), len(ProviderModels["tasw"]))
	}
	if got := ResolveAlias("tasw"); got != "tencent-aistudio-web" {
		t.Errorf("tasw resolved to %q", got)
	}
	minMeta := ProviderRegistry["oneminai"]
	if minMeta.Name != "1min.AI" || minMeta.Alias != "1min" || minMeta.Icon != "hub" || minMeta.Color != "#6366F1" || minMeta.Website != "https://1min.ai" {
		t.Errorf("oneminai metadata = %+v", minMeta)
	}
	minCfg := KnownProviders["oneminai"]
	if minCfg.BaseURL != "https://api.1min.ai/api/chat-with-ai" || minCfg.AuthHeader != "API-KEY" || minCfg.AuthScheme != "raw" {
		t.Errorf("oneminai config = %+v", minCfg)
	}
	if len(ProviderModels["oneminai"]) != 1 || len(ProviderModels["1min"]) != 1 {
		t.Errorf("oneminai catalog = %d/%d models", len(ProviderModels["oneminai"]), len(ProviderModels["1min"]))
	}
	if got := ResolveAlias("1min"); got != "oneminai" {
		t.Errorf("1min resolved to %q", got)
	}
	kenMeta := ProviderRegistry["keenable"]
	if kenMeta.Name != "Keenable" || kenMeta.Alias != "keenable" || kenMeta.Website != "https://docs.keenable.ai/api-reference" {
		t.Errorf("keenable metadata = %+v", kenMeta)
	}
	kenCfg := KnownProviders["keenable"]
	if kenCfg.FetchURL != "https://api.keenable.ai/v1/fetch/public" || kenCfg.SearchURL != "https://api.keenable.ai/v1/search/public" || kenCfg.StaticHeaders["X-Keenable-Title"] != "9RouterX" {
		t.Errorf("keenable config = %+v", kenCfg)
	}
	if got := ResolveAlias("keenable"); got != "keenable" {
		t.Errorf("keenable resolved to %q", got)
	}
	ntwMeta := ProviderRegistry["notrack-web"]
	if ntwMeta.Name != "NoTrack Web (Free)" || ntwMeta.Alias != "ntw" || ntwMeta.Icon != "auto_awesome" || ntwMeta.Color != "#0EA5E9" || ntwMeta.Website != "https://notrack.ai" {
		t.Errorf("notrack-web metadata = %+v", ntwMeta)
	}
	ntwCfg := KnownProviders["notrack-web"]
	if ntwCfg.BaseURL != "https://notrack.ai" || ntwCfg.AuthHeader != "Cookie" || ntwCfg.AuthScheme != "raw" {
		t.Errorf("notrack-web config = %+v", ntwCfg)
	}
	if len(ProviderModels["notrack-web"]) != 1 || len(ProviderModels["ntw"]) != 1 {
		t.Errorf("notrack catalog = %d/%d models", len(ProviderModels["notrack-web"]), len(ProviderModels["ntw"]))
	}
	if got := ResolveAlias("ntw"); got != "notrack-web" {
		t.Errorf("ntw resolved to %q", got)
	}
	cplMeta := ProviderRegistry["chatplayground"]
	if cplMeta.Name != "ChatPlayground" || cplMeta.Alias != "cpl" || cplMeta.Icon != "auto_awesome" || cplMeta.Color != "#7C3AED" || cplMeta.Website != "https://web.chatplayground.ai" {
		t.Errorf("chatplayground metadata = %+v", cplMeta)
	}
	cplCfg := KnownProviders["chatplayground"]
	if cplCfg.BaseURL != "https://app.chatplayground.ai/api/chat" || cplCfg.AuthHeader != "Authorization" || cplCfg.CatalogURL != "https://app.chatplayground.ai/api/models" {
		t.Errorf("chatplayground config = %+v", cplCfg)
	}
	if len(ProviderModels["chatplayground"]) != 33 || len(ProviderModels["cpl"]) != 33 {
		t.Errorf("chatplayground catalog = %d/%d models", len(ProviderModels["chatplayground"]), len(ProviderModels["cpl"]))
	}
	if got := ResolveAlias("cpl"); got != "chatplayground" {
		t.Errorf("cpl resolved to %q", got)
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
	for _, id := range []string{"dify"} {
		if _, ok := ProviderRegistry[id]; ok {
			t.Errorf("%s is selectable before its custom protocol is ported", id)
		}
		if _, ok := KnownProviders[id]; ok {
			t.Errorf("%s has an incorrect generic runtime config", id)
		}
	}
}
