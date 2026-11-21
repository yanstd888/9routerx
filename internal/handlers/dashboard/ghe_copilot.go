package dashboard

import (
	"context"
	json "encoding/json/v2"
	"fmt"
	"io"
	"net/http"
	"time"

	"9routerx/proxy/internal/handlerutil"
	"9routerx/proxy/internal/providers"
	"9routerx/proxy/internal/proxy/ghecopilot"
	"9routerx/proxy/internal/proxy/oauth"
)

func (h *DashboardHandler) handleGHEModels(w http.ResponseWriter, r *http.Request, id, raw string) {
	var data connectionProbeData
	var fields map[string]any
	if json.Unmarshal([]byte(raw), &data) != nil || json.Unmarshal([]byte(raw), &fields) != nil {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "invalid GHE connection data")
		return
	}
	client, _ := h.probeHTTPClient(data, fields)
	if injected, ok := r.Context().Value(probeClientKey{}).(*http.Client); ok {
		client = injected
	}
	client = ghecopilot.Client(client)
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	fresh, err := oauth.PrepareGHEConnection(ctx, h.Repo, client, id, false, "")
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadGateway, err.Error())
		return
	}
	psd, _ := fresh["providerSpecificData"].(map[string]any)
	// Discovery must never query the NES proxy: only the token's native API host
	// publishes the entitled chat catalog, so a legacy row must renew first.
	api := ghecopilot.String(psd, "copilotApiUrl")
	if api == "" {
		fresh, err = oauth.PrepareGHEConnection(ctx, h.Repo, client, id, true, "")
		if err != nil {
			handlerutil.WriteJSONError(w, http.StatusBadGateway, err.Error())
			return
		}
		psd, _ = fresh["providerSpecificData"].(map[string]any)
		api = ghecopilot.String(psd, "copilotApiUrl")
	}
	base, err := ghecopilot.EndpointBase(api)
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadGateway, "GHE token did not supply a valid Copilot API endpoint")
		return
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/models", nil)
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadGateway, err.Error())
		return
	}
	for key, value := range providers.GitHubCopilotChatHeaders("application/json", "user") {
		req.Header.Set(key, value)
	}
	encoded, _ := json.Marshal(fresh)
	req.Header.Set("Authorization", "Bearer "+copilotTokenFromData(string(encoded)))
	resp, err := ghecopilot.Do(ctx, client, req)
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadGateway, err.Error())
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		handlerutil.WriteJSONError(w, resp.StatusCode, fmt.Sprintf("GHE model catalog returned HTTP %d", resp.StatusCode))
		return
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadGateway, err.Error())
		return
	}
	models, formats, err := parseGHECatalog(body)
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadGateway, "invalid GHE Copilot model catalog")
		return
	}
	if err := oauth.StoreGHEModelFormats(ctx, h.Repo, id, formats); err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	handlerutil.WriteJSON(w, http.StatusOK, map[string]any{"provider": ghecopilot.Provider, "connectionId": id, "models": models})
}
