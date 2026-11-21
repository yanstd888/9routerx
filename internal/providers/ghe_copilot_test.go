package providers

import "testing"

func TestGHEIndependentCatalogFormats(t *testing.T) {
	if ResolveAlias("ghe-copilot") != "ghe-copilot" || GetProviderAlias("ghe-copilot") != "ghe-copilot" {
		t.Fatal("GHE aliased to another provider")
	}
	meta, ok := ProviderRegistry["ghe-copilot"]
	if !ok || meta.AuthType != "oauth" || meta.Category != CategoryOAuth {
		t.Fatalf("registry=%v", meta)
	}
	if len(ProviderModels["ghe-copilot"]) != 32 {
		t.Fatalf("catalog count=%d", len(ProviderModels["ghe-copilot"]))
	}
	seen := map[string]bool{}
	for _, model := range ProviderModels["ghe-copilot"] {
		if seen[model] {
			t.Fatalf("duplicate %s", model)
		}
		seen[model] = true
		if _, ok := GetModelFormats("ghe-copilot", model); !ok {
			t.Errorf("no wire declaration %s", model)
		}
	}
	for model, want := range map[string]string{"claude-sonnet-4.6": FormatClaude, "gpt-5-mini": FormatOpenAIResponses, "gpt-4o-mini": FormatOpenAI, "gemini-3.7-flash": FormatOpenAI} {
		if got := GHECopilotTargetFormat(model, nil); got != want {
			t.Errorf("format %s=%s want=%s", model, got, want)
		}
	}
	psd := map[string]any{"gheModelFormats": map[string]any{"discovered": FormatClaude}}
	if GHECopilotTargetFormat("discovered", psd) != FormatClaude {
		t.Fatal("discovery target ignored")
	}
	if _, ok := GetModelFormats("github", "discovered"); ok {
		t.Fatal("GHE discovery leaked to github")
	}
}
