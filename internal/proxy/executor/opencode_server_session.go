package executor

import (
	"bytes"
	"context"
	json "encoding/json/v2"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"9routerx/proxy/internal/proxy"
)

const (
	opencodeServerDefaultBase     = "http://127.0.0.1:4096"
	opencodeServerCatalogPath     = "/config/providers"
	opencodeServerHealthPath      = "/global/health"
	opencodeServerV2ProbePath     = "/api/provider"
	opencodeServerCatalogTTL      = 60 * time.Second
	opencodeServerUpstreamBodyMax = 8 << 20
)

type opencodeServerModel struct {
	ID         string
	ProviderID string
	Free       bool
}

type opencodeServerCatalogEntry struct {
	at     time.Time
	models map[string]opencodeServerModel
}

var (
	opencodeServerCatalogMu    sync.Mutex
	opencodeServerCatalogs     = map[string]*opencodeServerCatalogEntry{}
	opencodeServerSeedModelIDs = []string{"big-pickle", "space-bunny-free", "muse-spark-1.3-contributor-free", "nemotron-3-ultra-free", "mimo-v2.6-flash-free", "deepseek-v4.1-flash-free"}
)

func opencodeServerBaseURL(req *Request) (string, error) {
	raw := strings.TrimSpace(os.Getenv("OPENCODE_SERVER_URL"))
	if raw == "" && req.ConnData != nil {
		raw, _ = req.ConnData["opencode_server_url"].(string)
	}
	if raw == "" && req.Config != nil {
		raw = req.Config.BaseURL
	}
	if raw == "" {
		raw = opencodeServerDefaultBase
	}
	raw = strings.TrimRight(strings.TrimSpace(raw), "/")
	raw = strings.TrimSuffix(raw, "/chat/completions")
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", proxy.UpstreamFailure(http.StatusBadRequest, "OpenCode CLI Server URL must be an http(s) host")
	}
	return raw, nil
}
func opencodeServerClient(req *Request) *http.Client {
	if req.Client != nil {
		return req.Client
	}
	return http.DefaultClient
}
func opencodeServerDo(ctx context.Context, client *http.Client, method, target string, payload any) ([]byte, int, error) {
	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return nil, 0, fmt.Errorf("encode OpenCode request: %w", err)
		}
		body = bytes.NewReader(encoded)
	}
	r, err := http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		return nil, 0, fmt.Errorf("OpenCode request: %w", err)
	}
	// The local server is keyless. Never carry the gateway credential or any
	// operator headers onto this hop, even when a connection stores one.
	r.Header.Set("Accept", "application/json")
	if payload != nil {
		r.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(r)
	if err != nil {
		return nil, 0, fmt.Errorf("OpenCode request: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, opencodeServerUpstreamBodyMax))
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("read OpenCode response: %w", err)
	}
	return data, resp.StatusCode, nil
}

// opencodeServerCheckHealth rejects v2 servers: the CLI Server protocol lives
// on the v1 /config/providers and /session surfaces.
func opencodeServerCheckHealth(ctx context.Context, client *http.Client, base string) error {
	body, status, err := opencodeServerDo(ctx, client, http.MethodGet, base+opencodeServerHealthPath, nil)
	if err != nil {
		return proxy.UpstreamFailure(http.StatusBadGateway, err.Error())
	}
	if status != http.StatusOK {
		return proxy.UpstreamFailure(status, "OpenCode CLI Server health check failed")
	}
	var health struct {
		Healthy bool   `json:"healthy"`
		Version string `json:"version"`
	}
	if json.Unmarshal(body, &health) != nil {
		return proxy.UpstreamFailure(http.StatusBadGateway, "OpenCode CLI Server health payload is invalid")
	}
	if !health.Healthy {
		return proxy.UpstreamFailure(http.StatusBadGateway, "OpenCode CLI Server is unhealthy")
	}
	major := opencodeServerMajor(health.Version)
	if major >= 2 {
		return proxy.UpstreamFailure(http.StatusNotImplemented, "OpenCode CLI Server API v2 is not supported")
	}
	if major > 0 {
		return nil
	}
	// A versionless v1 health reply is ambiguous: a v2 server still answers
	// /api/provider where v1 exposes /config/providers.
	_, status, err = opencodeServerDo(ctx, client, http.MethodGet, base+opencodeServerV2ProbePath, nil)
	if err != nil {
		return proxy.UpstreamFailure(http.StatusBadGateway, err.Error())
	}
	if status >= 200 && status < 300 {
		return proxy.UpstreamFailure(http.StatusNotImplemented, "OpenCode CLI Server API v2 is not supported")
	}
	return nil
}
func opencodeServerMajor(version string) int {
	var digits strings.Builder
	for _, r := range version {
		if r < '0' || r > '9' {
			break
		}
		digits.WriteRune(r)
	}
	major, err := strconv.Atoi(digits.String())
	if err != nil {
		return 0
	}
	return major
}

// opencodeServerResolveModel reads the server catalog with a 60s TTL. Bare
// model ids are keyed to the first provider that declares them; a model the
// catalog does not know is a client 404.
func opencodeServerResolveModel(ctx context.Context, client *http.Client, base, model string) (opencodeServerModel, error) {
	models, err := opencodeServerCatalog(ctx, client, base)
	if err != nil {
		return opencodeServerModel{}, err
	}
	if found, ok := models[model]; ok {
		return found, nil
	}
	for _, seed := range opencodeServerSeedModelIDs {
		if seed == model {
			return opencodeServerModel{ID: model, ProviderID: "opencode", Free: true}, nil
		}
	}
	return opencodeServerModel{}, proxy.UpstreamFailure(http.StatusNotFound, "unknown OpenCode CLI Server model: "+model)
}
func opencodeServerCatalog(ctx context.Context, client *http.Client, base string) (map[string]opencodeServerModel, error) {
	opencodeServerCatalogMu.Lock()
	defer opencodeServerCatalogMu.Unlock()
	if entry := opencodeServerCatalogs[base]; entry != nil && time.Since(entry.at) < opencodeServerCatalogTTL {
		return entry.models, nil
	}
	var models map[string]opencodeServerModel
	body, status, err := opencodeServerDo(ctx, client, http.MethodGet, base+opencodeServerCatalogPath, nil)
	if err == nil && status != http.StatusOK {
		err = proxy.UpstreamFailure(status, "OpenCode CLI Server catalog failed")
	}
	if err == nil {
		models = opencodeServerParseCatalog(body)
	}
	if err != nil {
		// A catalog outage must not break a model already known from the pin.
		models = opencodeServerSeedCatalog()
	}
	opencodeServerCatalogs[base] = &opencodeServerCatalogEntry{at: time.Now(), models: models}
	return models, nil
}
func opencodeServerSeedCatalog() map[string]opencodeServerModel {
	models := make(map[string]opencodeServerModel, len(opencodeServerSeedModelIDs))
	for _, id := range opencodeServerSeedModelIDs {
		models[id] = opencodeServerModel{ID: id, ProviderID: "opencode", Free: true}
	}
	return models
}
func opencodeServerParseCatalog(body []byte) map[string]opencodeServerModel {
	var root any
	if json.Unmarshal(body, &root) != nil {
		return opencodeServerSeedCatalog()
	}
	models := map[string]opencodeServerModel{}
	opencodeServerWalkProviders(root, models)
	if len(models) == 0 {
		return opencodeServerSeedCatalog()
	}
	return models
}
func opencodeServerWalkProviders(value any, models map[string]opencodeServerModel) {
	switch v := value.(type) {
	case []any:
		for _, item := range v {
			opencodeServerWalkProviders(item, models)
		}
	case map[string]any:
		id := opencodeServerString(v, "id")
		if id == "" {
			id = opencodeServerString(v, "providerID")
		}
		if id == "" {
			id = opencodeServerString(v, "providerId")
		}
		if raw, ok := v["models"]; ok {
			opencodeServerWalkModels(raw, id, models)
		}
		for _, key := range []string{"providers", "provider"} {
			if raw, ok := v[key]; ok {
				opencodeServerWalkProviders(raw, models)
			}
		}
	}
}
func opencodeServerWalkModels(value any, providerID string, models map[string]opencodeServerModel) {
	switch v := value.(type) {
	case map[string]any:
		for id, raw := range v {
			opencodeServerAddModel(models, id, providerID, raw)
		}
	case []any:
		for _, raw := range v {
			if id, ok := raw.(string); ok {
				opencodeServerAddModel(models, id, providerID, nil)
				continue
			}
			if entry, ok := raw.(map[string]any); ok {
				id := opencodeServerString(entry, "id")
				if id == "" {
					id = opencodeServerString(entry, "modelID")
				}
				if id != "" {
					opencodeServerAddModel(models, id, providerID, entry)
				}
			}
		}
	}
}
func opencodeServerAddModel(models map[string]opencodeServerModel, id, providerID string, raw any) {
	if id == "" {
		return
	}
	if _, exists := models[id]; exists {
		return // first provider wins
	}
	free := false
	if entry, ok := raw.(map[string]any); ok {
		if cost, ok := entry["cost"].(map[string]any); ok {
			free = opencodeServerZero(cost["input"]) && opencodeServerZero(cost["output"])
		}
	}
	models[id] = opencodeServerModel{ID: id, ProviderID: providerID, Free: free}
}
func opencodeServerZero(value any) bool {
	switch v := value.(type) {
	case float64:
		return v == 0
	case string:
		return v == "" || v == "0"
	case nil:
		return true
	}
	return false
}
func opencodeServerString(entry map[string]any, key string) string {
	value, _ := entry[key].(string)
	return value
}
