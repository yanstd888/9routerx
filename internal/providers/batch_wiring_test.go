// 9RouterX — port and modifications by https://github.com/yanstd888/ — https://t.me/aiclub_id
package providers

import (
	"slices"
	"testing"
)

func TestProviderBatchAliasesRoundTrip(t *testing.T) {
	for _, id := range []string{"alibaba", "featherless-ai", "ovhcloud", "aihorde", "uc-direct", "synthetic", "river", "nace", "alibaba-cn", "agnes-cn", "volcengine-coding-plan", "volcengine-agent-plan", "leonardo", "opencode-server", "command-code", "commandcode-goat", "poe", "nlpcloud", "cerebrium", "clova-studio", "gigachat", "udio", "haiper", "huggingchat", "tencent-aistudio-web", "oneminai", "keenable", "notrack-web", "chatplayground", "syntx", "adapta-web", "conol-web", "uncloseai", "friendliai", "databricks", "snowflake", "charm-hyper", "token-kiosk"} {
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
	syntxMeta := ProviderRegistry["syntx"]
	if syntxMeta.Name != "SYNTX.ai (Unofficial/Experimental)" || syntxMeta.Alias != "stx" || syntxMeta.Icon != "auto_awesome" || syntxMeta.Color != "#FF4D00" || syntxMeta.Website != "https://syntx.ai" {
		t.Errorf("syntx metadata = %+v", syntxMeta)
	}
	syntxCfg := KnownProviders["syntx"]
	if syntxCfg.BaseURL != "https://api.syntx.ai" || syntxCfg.AuthScheme != "bearer" || syntxCfg.Format != FormatOpenAI {
		t.Errorf("syntx config = %+v", syntxCfg)
	}
	if len(ProviderModels["syntx"]) != len(syntxModelsForTest()) || len(ProviderModels["stx"]) != len(syntxModelsForTest()) {
		t.Errorf("syntx catalog = %d/%d models", len(ProviderModels["syntx"]), len(ProviderModels["stx"]))
	}
	if got := ResolveAlias("stx"); got != "syntx" {
		t.Errorf("stx resolved to %q", got)
	}
	adpMeta := ProviderRegistry["adapta-web"]
	if adpMeta.Name != "Adapta.org (Adapta One Web)" || adpMeta.Alias != "adp-web" || adpMeta.Icon != "auto_awesome" || adpMeta.Color != "#6E3AD3" || adpMeta.Website != "https://agent.adapta.one" {
		t.Errorf("adapta-web metadata = %+v", adpMeta)
	}
	adpCfg := KnownProviders["adapta-web"]
	if adpCfg.BaseURL != "https://agent.adapta.one" || adpCfg.AuthScheme != "bearer" || adpCfg.Format != FormatOpenAI {
		t.Errorf("adapta-web config = %+v", adpCfg)
	}
	if len(ProviderModels["adapta-web"]) != 7 || len(ProviderModels["adp-web"]) != 7 {
		t.Errorf("adapta catalog = %d/%d models", len(ProviderModels["adapta-web"]), len(ProviderModels["adp-web"]))
	}
	if got := ResolveAlias("adp-web"); got != "adapta-web" {
		t.Errorf("adp-web resolved to %q", got)
	}
	conolMeta := ProviderRegistry["conol-web"]
	if conolMeta.Name != "Conol (Unofficial/Experimental)" || conolMeta.Alias != "cnl" || conolMeta.Icon != "auto_awesome" || conolMeta.Color != "#F6C945" || conolMeta.Website != "https://conol.ai" {
		t.Errorf("conol-web metadata = %+v", conolMeta)
	}
	conolCfg := KnownProviders["conol-web"]
	if conolCfg.BaseURL != "https://conol.ai/api/sessions" || conolCfg.AuthHeader != "Cookie" || conolCfg.AuthScheme != "raw" {
		t.Errorf("conol-web config = %+v", conolCfg)
	}
	if len(ProviderModels["conol-web"]) != len(conolModels) || len(ProviderModels["cnl"]) != len(conolModels) {
		t.Errorf("conol catalog = %d/%d models", len(ProviderModels["conol-web"]), len(ProviderModels["cnl"]))
	}
	if got := ResolveAlias("cnl"); got != "conol-web" {
		t.Errorf("cnl resolved to %q", got)
	}
	uncMeta := ProviderRegistry["uncloseai"]
	if uncMeta.Name != "UncloseAI" || uncMeta.Alias != "unc" || uncMeta.Icon != "smart_toy" || uncMeta.Color != "#43A047" || uncMeta.Website != "https://uncloseai.com" {
		t.Errorf("uncloseai metadata = %+v", uncMeta)
	}
	uncCfg := KnownProviders["uncloseai"]
	if uncCfg.BaseURL != "https://hermes.ai.unturf.com/v1/chat/completions" || uncCfg.AuthHeader != "Authorization" || uncCfg.AuthScheme != "bearer" || !uncCfg.AuthOptional {
		t.Errorf("uncloseai config = %+v", uncCfg)
	}
	if len(ProviderModels["uncloseai"]) != 1 || len(ProviderModels["unc"]) != 1 {
		t.Errorf("uncloseai catalog = %d/%d models, want 1/1", len(ProviderModels["uncloseai"]), len(ProviderModels["unc"]))
	}
	if got := ResolveAlias("unc"); got != "uncloseai" {
		t.Errorf("unc resolved to %q", got)
	}
	friendliMeta := ProviderRegistry["friendliai"]
	if friendliMeta.Name != "FriendliAI" || friendliMeta.Alias != "friendli" || friendliMeta.Icon != "handshake" || friendliMeta.Color != "#EC4899" || friendliMeta.Website != "https://friendli.ai" {
		t.Errorf("friendliai metadata = %+v", friendliMeta)
	}
	friendliCfg := KnownProviders["friendliai"]
	if friendliCfg.BaseURL != "https://api.friendli.ai/serverless/v1/chat/completions" || friendliCfg.AuthHeader != "Authorization" || friendliCfg.AuthScheme != "bearer" {
		t.Errorf("friendliai config = %+v", friendliCfg)
	}
	if len(ProviderModels["friendliai"]) != 2 || len(ProviderModels["friendli"]) != 2 {
		t.Errorf("friendliai catalog = %d/%d models, want 2/2", len(ProviderModels["friendliai"]), len(ProviderModels["friendli"]))
	}
	if got := ResolveAlias("friendli"); got != "friendliai" {
		t.Errorf("friendli resolved to %q", got)
	}
	dbxMeta := ProviderRegistry["databricks"]
	if dbxMeta.Name != "Databricks" || dbxMeta.Alias != "databricks" || dbxMeta.Icon != "table_chart" || dbxMeta.Color != "#F97316" || dbxMeta.Website != "https://www.databricks.com" {
		t.Errorf("databricks metadata = %+v", dbxMeta)
	}
	dbxCfg := KnownProviders["databricks"]
	if dbxCfg.BaseURL != "https://adb-0000000000000000.0.azuredatabricks.net/serving-endpoints/chat/completions" || dbxCfg.AuthHeader != "Authorization" || dbxCfg.AuthScheme != "bearer" {
		t.Errorf("databricks config = %+v", dbxCfg)
	}
	if len(ProviderModels["databricks"]) != 4 {
		t.Errorf("databricks catalog = %d models, want 4", len(ProviderModels["databricks"]))
	}
	snflMeta := ProviderRegistry["snowflake"]
	if snflMeta.Name != "Snowflake Cortex" || snflMeta.Alias != "snowflake" || snflMeta.Icon != "ac_unit" || snflMeta.Color != "#29B5E8" || snflMeta.Website != "https://www.snowflake.com" {
		t.Errorf("snowflake metadata = %+v", snflMeta)
	}
	snflCfg := KnownProviders["snowflake"]
	if snflCfg.BaseURL != "https://{account}.snowflakecomputing.com/api/v2/cortex/inference:complete" || snflCfg.AuthHeader != "Authorization" || snflCfg.AuthScheme != "bearer" {
		t.Errorf("snowflake config = %+v", snflCfg)
	}
	if len(ProviderModels["snowflake"]) != 4 {
		t.Errorf("snowflake catalog = %d models, want 4", len(ProviderModels["snowflake"]))
	}
	hyperMeta := ProviderRegistry["charm-hyper"]
	if hyperMeta.Name != "Charm Hyper" || hyperMeta.Alias != "charm-hyper" || hyperMeta.Icon != "router" || hyperMeta.Color != "#7C3AED" || hyperMeta.Website != "https://hyper.charm.land" {
		t.Errorf("charm-hyper metadata = %+v", hyperMeta)
	}
	hyperCfg := KnownProviders["charm-hyper"]
	if hyperCfg.BaseURL != "https://hyper.charm.land/v1/chat/completions" || hyperCfg.AuthHeader != "Authorization" || hyperCfg.AuthScheme != "bearer" {
		t.Errorf("charm-hyper config = %+v", hyperCfg)
	}
	if len(ProviderModels["charm-hyper"]) != 1 {
		t.Errorf("charm-hyper catalog = %d models, want 1", len(ProviderModels["charm-hyper"]))
	}
	tkiMeta := ProviderRegistry["token-kiosk"]
	if tkiMeta.Name != "Token Kiosk" || tkiMeta.Alias != "tk" || tkiMeta.Icon != "hub" || tkiMeta.Color != "#6366F1" || tkiMeta.Website != "https://agent-router.gaib.ai" {
		t.Errorf("token-kiosk metadata = %+v", tkiMeta)
	}
	tkiCfg := KnownProviders["token-kiosk"]
	if tkiCfg.BaseURL != "https://agent-router.gaib.ai/v1/chat/completions" || tkiCfg.AuthHeader != "Authorization" || tkiCfg.AuthScheme != "bearer" {
		t.Errorf("token-kiosk config = %+v", tkiCfg)
	}
	if len(ProviderModels["token-kiosk"]) != 5 || len(ProviderModels["tk"]) != 5 {
		t.Errorf("token-kiosk catalog = %d/%d models, want 5/5", len(ProviderModels["token-kiosk"]), len(ProviderModels["tk"]))
	}
	if got := ResolveAlias("tk"); got != "token-kiosk" {
		t.Errorf("tk resolved to %q", got)
	}
	if got := ResolveAlias("ocs"); got != "opencode-server" {
		t.Errorf("ocs resolved to %q", got)
	}
	if got := GetProviderModelKind("drex", "drex-max"); got != "systemone" {
		t.Errorf("Drex kind = %q, want systemone", got)
	}
}

func TestProviderBatchOptionalAuthConfig(t *testing.T) {
	for _, id := range []string{"docker-model-runner", "lemonade", "llama-cpp", "llamafile", "lm-studio", "mlx-gemma", "mlx-qwen", "oobabooga", "triton", "xinference", "aihorde", "ovhcloud", "uncloseai"} {
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
