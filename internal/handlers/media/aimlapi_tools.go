// 9RouterX — source port and modifications by https://github.com/yanstd888/ — https://t.me/aiclub_id
package media

import (
	"bytes"
	json "encoding/json/v2"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"9routerx/proxy/internal/constants"
	"9routerx/proxy/internal/handlers/chat"
	"9routerx/proxy/internal/handlerutil"
	"9routerx/proxy/internal/log"
	"9routerx/proxy/internal/models"
	"9routerx/proxy/internal/providers"
)

// AI/ML API serves web search and page fetch from its Toolbox: one endpoint,
// POST /v1/tools/run, that takes {"tool": <id>, "input": {...}} and answers
// {"status", "output": {...}}. Neither the gateway's /v1/search body nor its
// /v1/web/fetch body matches that, so the generic forward (which relays the
// caller's body as-is) cannot be used; this adapter builds the Toolbox request
// and maps the answer back to the gateway's search / fetch shape.
const (
	aimlapiToolsHost = "tools.aimlapi.com"
	// aimlapiSearchTool is the curated web search tool (capability web_search).
	aimlapiSearchTool = "web/search"
	// aimlapiFetchTool turns one page into readable text; priced per call, so it
	// runs without a max_cost_usd cap.
	aimlapiFetchTool = "litescrape/web-fetch"
	// web/search refuses max_results above 20.
	aimlapiMaxSearchResults = 20
	aimlapiErrBodyLimit     = 4 << 10
)

// aimlapiToolsURLAllowed reports whether the Toolbox URL points at the exact
// AI/ML API host over HTTPS. The connection's key is attached only when this
// holds, so a stray override can never send it to another host.
func aimlapiToolsURLAllowed(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	return u.Scheme == "https" && strings.EqualFold(u.Host, aimlapiToolsHost) && u.User == nil
}

// isAimlapiProvider matches the AI/ML API provider id and its short alias.
func isAimlapiProvider(provider string) bool {
	return providers.ResolveAlias(provider) == "aimlapi"
}

// aimlapiToolsResponse is the envelope POST /v1/tools/run answers with.
type aimlapiToolsResponse struct {
	Status  string         `json:"status"`
	IsError bool           `json:"is_error"`
	Output  map[string]any `json:"output"`
	Cost    *struct {
		ActualUSD *float64 `json:"actual_usd"`
	} `json:"cost"`
	Error *struct {
		Message string `json:"message"`
		Code    string `json:"code"`
	} `json:"error"`
}

// handleAimlapiTool serves /v1/search and /v1/web/fetch for the aimlapi
// provider. It rotates over active connections the same way the Xquik search
// lane does: a pinned connection is tried once, a retryable failure locks the
// account and moves on.
func (h *MediaHandler) handleAimlapiTool(w http.ResponseWriter, r *http.Request, body []byte, endpoint string, modelInfo *chat.ModelInfo) error {
	tool, input, query, err := buildAimlapiToolInput(endpoint, body)
	if err != nil {
		return err
	}

	excludeIDs := []string{}
	usePinned := modelInfo.ConnectionID != ""
	var lastErr *searchUpstreamError
	for {
		conn, connData, cerr := h.ChatH.GetBestConnectionWithContext(r.Context(), modelInfo.Provider, modelInfo.ConnectionID, excludeIDs, modelInfo.Model)
		if cerr != nil || conn == nil {
			if lastErr != nil {
				return lastErr
			}
			return searchError(http.StatusBadRequest, fmt.Sprintf("no active connection for provider %s: %v", modelInfo.Provider, cerr))
		}
		out, attemptErr := h.runAimlapiTool(r, tool, input, modelInfo.Model, conn, connData)
		if attemptErr == nil {
			if endpoint == "/v1/search" {
				handlerutil.WriteJSON(w, http.StatusOK, aimlapiSearchResponse(query, out))
			} else {
				handlerutil.WriteJSON(w, http.StatusOK, aimlapiFetchResponse(out))
			}
			return nil
		}
		lastErr = attemptErr
		if usePinned || !attemptErr.Retryable {
			return lastErr
		}
		excludeIDs = append(excludeIDs, conn.ID)
	}
}

// buildAimlapiToolInput maps a gateway search / fetch body to a Toolbox tool id
// and its input. query is the normalised search query (empty for fetch).
func buildAimlapiToolInput(endpoint string, body []byte) (tool string, input map[string]any, query string, err error) {
	switch endpoint {
	case "/v1/search":
		var req struct {
			Query      string `json:"query"`
			Prompt     string `json:"prompt"`
			MaxResults int    `json:"max_results"`
		}
		_ = json.Unmarshal(body, &req)
		query = sanitizeSearchQuery(req.Query)
		if query == "" {
			query = sanitizeSearchQuery(req.Prompt)
		}
		if query == "" {
			return "", nil, "", searchError(http.StatusBadRequest, "missing query in search request")
		}
		limit := req.MaxResults
		if limit <= 0 {
			limit = defaultSearchMaxResults
		}
		if limit > aimlapiMaxSearchResults {
			limit = aimlapiMaxSearchResults
		}
		return aimlapiSearchTool, map[string]any{"query": query, "max_results": limit}, query, nil
	case "/v1/web/fetch":
		var req struct {
			URL string `json:"url"`
		}
		if json.Unmarshal(body, &req) != nil || strings.TrimSpace(req.URL) == "" {
			return "", nil, "", searchError(http.StatusBadRequest, "missing or invalid url")
		}
		return aimlapiFetchTool, map[string]any{"url": strings.TrimSpace(req.URL)}, "", nil
	}
	return "", nil, "", searchError(http.StatusBadRequest, "AI/ML API Toolbox does not serve "+endpoint)
}

func (h *MediaHandler) runAimlapiTool(r *http.Request, tool string, input map[string]any, model string, conn *models.ProviderConnection, connData *chat.ConnectionData) (map[string]any, *searchUpstreamError) {
	apiKey := chat.ExtractAPIKey(connData)
	if apiKey == "" {
		return nil, searchError(http.StatusUnauthorized, "no API key found for AI/ML API connection")
	}
	cfg, cfgErr := h.ChatH.GetProviderConfig("aimlapi", connData)
	if cfgErr != nil || cfg == nil {
		return nil, searchError(http.StatusBadGateway, "AI/ML API provider config unavailable")
	}
	target := cfg.SearchURL
	if tool == aimlapiFetchTool {
		target = cfg.FetchURL
	}
	if !aimlapiToolsURLAllowed(target) {
		return nil, searchError(http.StatusBadGateway, "AI/ML API Toolbox URL must be https://"+aimlapiToolsHost)
	}

	payload, err := json.Marshal(map[string]any{"tool": tool, "input": input})
	if err != nil {
		return nil, searchError(http.StatusInternalServerError, err.Error())
	}
	// Only the headers built here go upstream: the caller's own headers (its
	// gateway key among them) are not relayed to the Toolbox.
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, target, bytes.NewReader(payload))
	if err != nil {
		return nil, searchError(http.StatusBadGateway, fmt.Sprintf("create AI/ML API request: %v", err))
	}
	req.Header.Set(constants.HeaderContentType, constants.ContentTypeJSON)
	req.Header.Set(constants.HeaderAccept, constants.ContentTypeJSON)
	handlerutil.SetAuthHeader(req, apiKey, cfg.AuthHeader, cfg.AuthScheme)

	client, clientErr := h.ChatH.GetClientForConnection(connData)
	if clientErr != nil {
		return nil, &searchUpstreamError{Message: clientErr.Error()}
	}
	if client == nil {
		client = h.Client
	}
	if client == nil {
		client = http.DefaultClient
	}
	// The Toolbox answers in place; it never redirects. Refusing to follow one
	// keeps the bearer key from being replayed to whatever host a 3xx names.
	noRedirect := *client
	noRedirect.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }

	start := time.Now()
	resp, err := noRedirect.Do(req)
	if err != nil {
		if r.Context().Err() != nil {
			return nil, &searchUpstreamError{Status: 499, Message: "client closed request"}
		}
		return nil, &searchUpstreamError{Status: http.StatusBadGateway, Message: fmt.Sprintf("AI/ML API upstream request failed: %v", err), Retryable: true}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		return nil, searchError(http.StatusBadGateway, fmt.Sprintf("AI/ML API Toolbox answered an unexpected redirect (status %d)", resp.StatusCode))
	}
	if resp.StatusCode >= 400 {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, aimlapiErrBodyLimit))
		errText := aimlapiErrorText(errBody, resp.Status)
		backoff := 0
		if h.Repo != nil {
			backoff = h.Repo.GetConnectionBackoffLevel(conn.ID)
		}
		classification := providers.ClassifyError(resp.StatusCode, errText, backoff)
		if classification.ShouldFallback && h.Repo != nil && !handlerutil.IsProbeContext(r.Context()) {
			cooldownSec := max(classification.CooldownMs/1000, 1)
			_ = h.Repo.LockConnectionModel(conn.ID, model, cooldownSec, classification.NewBackoffLevel)
			log.Warn("search", "aimlapi account locked, trying next", "conn", conn.ID[:min(8, len(conn.ID))], "status", resp.StatusCode, "cooldown_s", cooldownSec)
		}
		return nil, &searchUpstreamError{Status: resp.StatusCode, Message: fmt.Sprintf("AI/ML API upstream error (status %d): %s", resp.StatusCode, errText), Retryable: classification.ShouldFallback}
	}

	var tr aimlapiToolsResponse
	if err := json.UnmarshalRead(resp.Body, &tr); err != nil {
		return nil, &searchUpstreamError{Status: http.StatusBadGateway, Message: fmt.Sprintf("decode AI/ML API response: %v", err), Retryable: true}
	}
	if tr.Error != nil || tr.IsError || (tr.Status != "" && tr.Status != "completed") {
		msg := "tool run did not complete"
		if tr.Error != nil && tr.Error.Message != "" {
			msg = tr.Error.Message
		} else if tr.Status != "" {
			msg = "tool run status " + tr.Status
		}
		return nil, searchError(http.StatusBadGateway, "AI/ML API "+tool+": "+msg)
	}
	if tr.Output == nil {
		tr.Output = map[string]any{}
	}
	tr.Output["_latency_ms"] = time.Since(start).Milliseconds()
	if tr.Cost != nil && tr.Cost.ActualUSD != nil {
		tr.Output["_cost_usd"] = *tr.Cost.ActualUSD
	}

	if h.Repo != nil {
		h.Repo.UpdateConnectionLastUsed(conn.ID)
		if !handlerutil.IsProbeContext(r.Context()) {
			_ = h.Repo.UnlockConnectionModel(conn.ID, model)
		}
	}
	return tr.Output, nil
}

// aimlapiErrorText pulls error.message out of a Toolbox error body, falling back
// to the raw text and then to the status line.
func aimlapiErrorText(body []byte, status string) string {
	var env struct {
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
		Message string `json:"message"`
	}
	if json.Unmarshal(body, &env) == nil {
		if env.Error != nil && env.Error.Message != "" {
			return env.Error.Message
		}
		if env.Message != "" {
			return env.Message
		}
	}
	if text := strings.TrimSpace(string(body)); text != "" {
		return text
	}
	return status
}

// aimlapiSearchResponse maps web/search output ({results:[{url,title,snippet}]})
// onto the search shape the other search lanes answer with.
func aimlapiSearchResponse(query string, out map[string]any) map[string]any {
	rawResults, _ := out["results"].([]any)
	results := make([]map[string]any, 0, len(rawResults))
	for i, item := range rawResults {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		link, _ := m["url"].(string)
		title, _ := m["title"].(string)
		snippet, _ := m["snippet"].(string)
		display := link
		if u, err := url.Parse(link); err == nil && u.Host != "" {
			display = u.Host + u.EscapedPath()
		}
		results = append(results, map[string]any{
			"title":        title,
			"url":          link,
			"display_url":  display,
			"snippet":      snippet,
			"position":     i + 1,
			"published_at": nil,
			"favicon_url":  nil,
			"content": map[string]any{
				"format": "text",
				"text":   snippet,
				"length": len(snippet),
			},
			"metadata": map[string]any{
				"author":      nil,
				"source_type": "web",
				"image_url":   nil,
			},
			"citation": map[string]any{
				"provider": "aimlapi",
				"rank":     i + 1,
			},
		})
	}
	latency := out["_latency_ms"]
	return map[string]any{
		"provider": "aimlapi",
		"query":    query,
		"results":  results,
		"usage": map[string]any{
			"queries_used":          1,
			"search_cost_usd":       out["_cost_usd"],
			"provider_credits_used": nil,
		},
		"pagination": map[string]any{
			"has_more":    false,
			"next_cursor": nil,
		},
		"answer": nil,
		"metrics": map[string]any{
			"response_time_ms":        latency,
			"upstream_latency_ms":     latency,
			"total_results_available": nil,
		},
		"errors": []any{},
	}
}

// aimlapiFetchResponse maps litescrape/web-fetch output ({url,title,content})
// to a flat fetch answer with the page text under `content`.
func aimlapiFetchResponse(out map[string]any) map[string]any {
	content, _ := out["content"].(string)
	return map[string]any{
		"provider": "aimlapi",
		"url":      out["url"],
		"title":    out["title"],
		"content":  content,
		"format":   "markdown",
		"usage": map[string]any{
			"fetch_cost_usd": out["_cost_usd"],
		},
	}
}

// aimlapiToolEndpoint reports whether the Toolbox adapter serves this endpoint.
func aimlapiToolEndpoint(endpoint string) bool {
	return endpoint == "/v1/search" || endpoint == "/v1/web/fetch"
}
