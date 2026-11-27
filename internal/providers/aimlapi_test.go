// 9RouterX — source port and modifications by https://github.com/yanstd888/ — https://t.me/aiclub_id
package providers

import (
	"slices"
	"testing"
)

func TestAimlapiProvider_Registered(t *testing.T) {
	cfg, ok := KnownProviders["aimlapi"]
	if !ok {
		t.Fatal("aimlapi missing from KnownProviders")
	}
	if cfg.BaseURL != "https://api.aimlapi.com/v1/chat/completions" {
		t.Errorf("BaseURL = %q", cfg.BaseURL)
	}
	if cfg.AuthHeader != "Authorization" || cfg.AuthScheme != "bearer" {
		t.Errorf("auth = %s/%s, want Authorization/bearer", cfg.AuthHeader, cfg.AuthScheme)
	}
	if got := ModelsListURL("aimlapi"); got != "https://api.aimlapi.com/v1/models" {
		t.Errorf("ModelsListURL = %q", got)
	}

	meta, ok := ProviderRegistry["aimlapi"]
	if !ok {
		t.Fatal("aimlapi missing from ProviderRegistry")
	}
	if meta.Name != "AI/ML API" || meta.Category != CategoryAPIKey || meta.AuthType != "apikey" {
		t.Errorf("meta = %+v", meta)
	}
	for _, kind := range []string{"llm", "webSearch", "webFetch"} {
		if !slices.Contains(meta.ServiceKinds, kind) {
			t.Errorf("ServiceKinds %v missing %q", meta.ServiceKinds, kind)
		}
	}
	if meta.Notice == nil || meta.Notice.APIKeyURL != "https://aimlapi.com/app/keys" {
		t.Errorf("Notice = %+v", meta.Notice)
	}
	if ResolveAlias("aiml") != "aimlapi" {
		t.Errorf("alias aiml -> %q", ResolveAlias("aiml"))
	}
	if len(ProviderModels["aimlapi"]) == 0 {
		t.Error("no built-in aimlapi models")
	}
	if !slices.Contains(ProviderModels["aimlapi"], "openai/gpt-5.4-mini") {
		t.Error("built-in list lacks openai/gpt-5.4-mini")
	}
}

func TestFilterAimlapiChatModels(t *testing.T) {
	in := []any{
		map[string]any{"id": "openai/gpt-5.4-mini", "type": "openai/chat-completions"},
		map[string]any{"id": "openai/gpt-5.4-mini", "type": "openai/responses/submit"},
		map[string]any{"id": "anthropic/claude-sonnet-4.6", "type": "anthropic/messages"},
		map[string]any{"id": "anthropic/claude-sonnet-4.6", "type": "openai/chat-completions"},
		map[string]any{"id": "google/veo-4", "type": "internal/video-generations/submit"},
		map[string]any{"id": "flux/pro", "type": "openai/image-generations"},
		map[string]any{"id": "openai/gpt-5.4-mini", "type": "openai/chat-completions"},
		"not-a-model",
	}
	out := FilterAimlapiChatModels(in)
	var ids []string
	for _, m := range out {
		ids = append(ids, m.(map[string]any)["id"].(string))
	}
	want := []string{"openai/gpt-5.4-mini", "anthropic/claude-sonnet-4.6"}
	if !slices.Equal(ids, want) {
		t.Errorf("ids = %v, want %v", ids, want)
	}
}
