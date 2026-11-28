package providers

import (
	json "encoding/json/v2"
	"os"
	"reflect"
	"testing"
)

func TestMoonshotPinnedCatalogAndBrand(t *testing.T) {
	raw, err := os.ReadFile("testdata/p1-moonshot.json")
	if err != nil {
		t.Fatal(err)
	}
	var pin struct {
		SHA      string   `json:"sha"`
		Provider string   `json:"provider"`
		Name     string   `json:"name"`
		Endpoint string   `json:"endpoint"`
		Models   []string `json:"models"`
	}
	if err := json.Unmarshal(raw, &pin); err != nil {
		t.Fatal(err)
	}
	if pin.SHA != "632d84f569f72893ed148e5b19dd05e2fff1a52a" {
		t.Fatalf("wrong reference pin: %s", pin.SHA)
	}
	meta, exists := ProviderRegistry[pin.Provider]
	if !exists || meta.ID != "moonshot" || meta.Alias != "moonshot" || meta.Name != pin.Name || meta.Category != CategoryAPIKey || meta.AuthType != "apikey" {
		t.Fatalf("Moonshot brand/auth metadata = %+v", meta)
	}
	cfg := KnownProviders["moonshot"]
	if cfg.BaseURL != pin.Endpoint || cfg.AuthHeader != "Authorization" || cfg.AuthScheme != "bearer" || cfg.Format != FormatOpenAI || cfg.NoAuth || cfg.AuthOptional {
		t.Fatalf("Moonshot transport = %+v", cfg)
	}
	if !reflect.DeepEqual(GetProviderModels("moonshot"), pin.Models) {
		t.Fatalf("catalog = %v", GetProviderModels("moonshot"))
	}
	for _, model := range pin.Models {
		t.Run(model, func(t *testing.T) {
			caps := GetCapabilitiesForModel("moonshot", model)
			limit := 262144
			if model == "kimi-k3" || model == "kimi-k3.1-pro" || model == "kimi-k3.1-pro-highspeed" {
				limit = 1048576
			}
			if !caps.Vision || !caps.Reasoning || !caps.Tools || caps.ContextWindow != limit || caps.MaxOutput != limit {
				t.Fatalf("caps = %+v", caps)
			}
			formats, ok := GetModelFormats("moonshot", model)
			if !ok || formats.TargetFormat != FormatOpenAI || !reflect.DeepEqual(formats.SupportedFormats, []string{FormatOpenAI}) {
				t.Fatalf("formats = %+v", formats)
			}
		})
	}
	if ResolveAlias("km") != "kimi" || ResolveAlias("ms") != "modelscope" || ResolveAlias("moonshot") != "moonshot" {
		t.Fatal("provider aliases were conflated")
	}
	if KnownProviders["kimi"].BaseURL != "https://api.kimi.com/coding/v1/chat/completions" || KnownProviders["kimi-coding"].BaseURL != KnownProviders["kimi"].BaseURL {
		t.Fatal("Kimi Code transport changed")
	}
	if _, exists := ProviderRegistry["dify"]; exists {
		t.Fatal("Dify guard was bypassed")
	}
	if !reflect.DeepEqual(GetThinkingLevels("moonshot", "kimi-k3.1-pro"), []string{"low", "high", "max"}) {
		t.Fatal("K3.1 native tiers missing")
	}
}
