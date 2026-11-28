// 9RouterX — port and modifications by https://github.com/yanstd888/ — https://t.me/aiclub_id
package dashboard

import (
	json "encoding/json/v2"
	"net/http"

	"9routerx/proxy/internal/handlerutil"
	"9routerx/proxy/internal/models"
	"9routerx/proxy/internal/proxy/executor"
)

func (h *DashboardHandler) writeDevinConnectionModels(w http.ResponseWriter, r *http.Request, conn *models.ProviderConnection) {
	var data struct {
		APIKey               string `json:"apiKey"`
		AccessToken          string `json:"accessToken"`
		BaseURL              string `json:"baseUrl"`
		ProviderSpecificData struct {
			BaseURL string `json:"baseUrl"`
		} `json:"providerSpecificData"`
	}
	if json.Unmarshal([]byte(conn.Data), &data) != nil {
		handlerutil.WriteJSONError(w, 400, "invalid Devin connection data")
		return
	}
	key := data.AccessToken
	if key == "" {
		key = data.APIKey
	}
	base := data.BaseURL
	if base == "" {
		base = data.ProviderSpecificData.BaseURL
	}
	client := http.DefaultClient
	if configured, ok := r.Context().Value(probeClientKey{}).(*http.Client); ok && configured != nil {
		client = configured
	}
	items, err := executor.FetchDevinDesktopModels(r.Context(), client, key, base)
	if err != nil {
		handlerutil.WriteJSONError(w, 502, "Devin native model discovery failed")
		return
	}
	handlerutil.WriteJSON(w, 200, map[string]any{"provider": "devin-desktop", "connectionId": conn.ID, "models": items})
}
