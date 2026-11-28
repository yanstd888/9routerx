package dashboard

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"sort"
	"strings"

	"9routerx/proxy/internal/providers"
)

type gheCatalogEntry struct {
	ID           string           `json:"id"`
	Model        string           `json:"model"`
	Name         string           `json:"name"`
	Endpoints    []jsontext.Value `json:"supported_endpoints"`
	Capabilities struct {
		Type string `json:"type"`
	} `json:"capabilities"`
	Policy struct {
		State string `json:"state"`
	} `json:"policy"`
}

func parseGHECatalog(raw []byte) ([]CopilotModel, map[string]any, error) {
	var env struct {
		Data []gheCatalogEntry `json:"data"`
	}
	var items []gheCatalogEntry
	if err := json.Unmarshal(raw, &items); err != nil {
		if err := json.Unmarshal(raw, &env); err != nil {
			return nil, nil, err
		}
		items = env.Data
	}
	models := []CopilotModel{}
	formats := map[string]any{}
	for _, item := range items {
		id := strings.TrimSpace(item.ID)
		if id == "" {
			id = strings.TrimSpace(item.Model)
		}
		if id == "" || (item.Policy.State != "" && item.Policy.State != "enabled") || (item.Capabilities.Type != "" && item.Capabilities.Type != "chat") {
			continue
		}
		endpoints := []string{}
		for _, raw := range item.Endpoints {
			var endpoint string
			if json.Unmarshal(raw, &endpoint) != nil {
				var object struct {
					Endpoint string `json:"endpoint"`
				}
				_ = json.Unmarshal(raw, &object)
				endpoint = object.Endpoint
			}
			endpoints = append(endpoints, endpoint)
		}
		target := gheCatalogFormat(endpoints)
		if target == "" && item.Capabilities.Type != "chat" {
			continue
		}
		if target == "" {
			target = providers.GHECopilotTargetFormat(id, nil)
		}
		formats[id] = target
		name := item.Name
		if name == "" {
			name = id
		}
		models = append(models, CopilotModel{ID: id, Name: name})
	}
	sort.Slice(models, func(i, j int) bool { return models[i].ID < models[j].ID })
	return models, formats, nil
}

func gheCatalogFormat(endpoints []string) string {
	for _, endpoint := range endpoints {
		if endpoint == "/v1/messages" || endpoint == "/messages" {
			return providers.FormatClaude
		}
	}
	for _, endpoint := range endpoints {
		if endpoint == "/responses" {
			return providers.FormatOpenAIResponses
		}
	}
	for _, endpoint := range endpoints {
		if endpoint == "/chat/completions" {
			return providers.FormatOpenAI
		}
	}
	return ""
}
