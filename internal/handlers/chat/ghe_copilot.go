package chat

import (
	"context"
	json "encoding/json/v2"
	"fmt"

	"9routerx/proxy/internal/providers"
	"9routerx/proxy/internal/proxy/ghecopilot"
	"9routerx/proxy/internal/proxy/oauth"
)

func (h *ChatHandler) gheTargetFormat(model, connectionID string) string {
	if h.Repo != nil {
		connections, err := h.Repo.GetProviderConnections(ghecopilot.Provider, true)
		if err == nil {
			for _, conn := range connections {
				if connectionID != "" && conn.ID != connectionID {
					continue
				}
				var data ConnectionData
				if json.Unmarshal([]byte(conn.Data), &data) == nil {
					if formats, ok := data.ProviderSpecificData["gheModelFormats"].(map[string]any); ok {
						if _, exists := formats[model]; exists {
							return providers.GHECopilotTargetFormat(model, data.ProviderSpecificData)
						}
					}
				}
			}
		}
	}
	return providers.GHECopilotTargetFormat(model, nil)
}

func (h *ChatHandler) prepareGHE(ctx context.Context, id string, incoming *ConnectionData, force bool, rejected string) (*ConnectionData, string, error) {
	if incoming == nil {
		return nil, "", fmt.Errorf("GHE connection data is missing")
	}
	client, err := h.getClientForConnection(incoming)
	if err != nil {
		return nil, "", err
	}
	var data map[string]any
	if h.Repo != nil && id != "" {
		data, err = oauth.PrepareGHEConnection(ctx, h.Repo, client, id, force, rejected)
	} else {
		// The no-repository path exists for fixture dispatch only, not account lookup.
		raw, marshalErr := json.Marshal(incoming)
		if marshalErr != nil {
			return nil, "", marshalErr
		}
		if err = json.Unmarshal(raw, &data); err == nil {
			_, err = ghecopilot.Renew(ctx, client, data, force)
		}
	}
	if err != nil {
		return nil, "", fmt.Errorf("prepare GHE Copilot credentials: %w", err)
	}
	var fresh ConnectionData
	raw, err := json.Marshal(data)
	if err != nil {
		return nil, "", err
	}
	if err := json.Unmarshal(raw, &fresh); err != nil {
		return nil, "", err
	}
	// Request egress resolution is not a credential; keep the selected pool and strictness.
	fresh.ProxyPoolID = incoming.ProxyPoolID
	fresh.ConnectionProxyEnabled = incoming.ConnectionProxyEnabled
	fresh.ConnectionProxyURL = incoming.ConnectionProxyURL
	fresh.ConnectionNoProxy = incoming.ConnectionNoProxy
	fresh.StrictProxy = incoming.StrictProxy
	fresh.ResolvedProxyPool = incoming.ResolvedProxyPool
	base, err := ghecopilot.APIBase(fresh.ProviderSpecificData)
	if err != nil {
		return nil, "", err
	}
	fresh.BaseURL = base + "/chat/completions"
	token := ghecopilot.String(fresh.ProviderSpecificData, "copilotToken")
	if token == "" {
		return nil, "", fmt.Errorf("GHE derived Copilot token is missing")
	}
	return &fresh, token, nil
}
