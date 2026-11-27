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

// Keenable serves web search (POST /v1/search, {"query","max_results"}) and
// page fetch (POST /v1/fetch, {"urls":[...]}) on one host. Each lane also has a
// keyless twin under /public, so the provider answers before a connection is
// added; a connection's key moves the call to the keyed path, which only lifts
// the rate limit. Neither the gateway's /v1/search body nor its /v1/web/fetch
// body matches Keenable's, so this adapter builds the request and maps the
// answer back to the gateway's search / fetch shape.
const (
	keenableHost = "api.keenable.ai"
	// keenableTitleHeader names the calling app; the keyless paths refuse a
	// request without it.
	keenableTitleHeader = "X-Keenable-Title"
	keenableAppTitle    = "9RouterX"
	// The search API accepts max_results from 1 to 50.
	keenableMaxSearchResults = 50
	// Keenable's snippet is the page text, often several thousand characters:
	// the full text goes under content.text, and `snippet` keeps a short lead.
	keenableSnippetRunes = 500
	keenableErrBodyLimit = 4 << 10
)

// keenableURLAllowed reports whether target points at the exact Keenable host
// over HTTPS. Nothing is sent otherwise, so a stray override can never carry a
// connection's key to another host.
func keenableURLAllowed(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	return u.Scheme == "https" && strings.EqualFold(u.Host, keenableHost) && u.User == nil
}

// keenablePublicURL turns a keyed lane URL into its keyless twin
// (/v1/search -> /v1/search/public).
func keenablePublicURL(keyed string) string {
	return strings.TrimRight(keyed, "/") + "/public"
}

// isKeenableProvider matches the Keenable provider id.
func isKeenableProvider(provider string) bool {
	return providers.ResolveAlias(provider) == "keenable"
}

// keenableEndpoint reports whether the Keenable adapter serves this endpoint.
func keenableEndpoint(endpoint string) bool {
	return endpoint == "/v1/search" || endpoint == "/v1/web/fetch"
}

// handleKeenable serves /v1/search and /v1/web/fetch for the keenable
// provider. With no active connection it calls the keyless path once. With
// connections it rotates over them the way the AI/ML API lane does: a pinned
// connection is tried once, a retryable failure locks the account and moves on.
func (h *MediaHandler) handleKeenable(w http.ResponseWriter, r *http.Request, body []byte, endpoint string, modelInfo *chat.ModelInfo) error {
	payload, query, err := buildKeenablePayload(endpoint, body)
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
			// No connection at all: the keyless path, through the provider's
			// proxy pool when one is assigned (same as other no-key lanes).
			conn, connData = nil, &chat.ConnectionData{ProxyPoolID: h.ChatH.ResolveProviderProxyPoolID(modelInfo.Provider)}
		}
		raw, attemptErr := h.runKeenable(r, endpoint, payload, modelInfo.Model, conn, connData)
		if attemptErr == nil {
			if endpoint == "/v1/search" {
				handlerutil.WriteJSON(w, http.StatusOK, keenableSearchResponse(query, raw))
				return nil
			}
			out, ferr := keenableFetchResponse(raw)
			if ferr != nil {
				return ferr
			}
			handlerutil.WriteJSON(w, http.StatusOK, out)
			return nil
		}
		lastErr = attemptErr
		if conn == nil || usePinned || !attemptErr.Retryable {
			return lastErr
		}
		excludeIDs = append(excludeIDs, conn.ID)
	}
}

// buildKeenablePayload maps a gateway search / fetch body to the Keenable
// request body. query is the normalised search query (empty for fetch).
func buildKeenablePayload(endpoint string, body []byte) (payload map[string]any, query string, err error) {
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
			return nil, "", searchError(http.StatusBadRequest, "missing query in search request")
		}
		limit := req.MaxResults
		if limit <= 0 {
			limit = defaultSearchMaxResults
		}
		if limit > keenableMaxSearchResults {
			limit = keenableMaxSearchResults
		}
		return map[string]any{"query": query, "max_results": limit}, query, nil
	case "/v1/web/fetch":
		var req struct {
			URL           string `json:"url"`
			MaxCharacters int    `json:"max_characters"`
		}
		if json.Unmarshal(body, &req) != nil || strings.TrimSpace(req.URL) == "" {
			return nil, "", searchError(http.StatusBadRequest, "missing or invalid url")
		}
		payload = map[string]any{"urls": []string{strings.TrimSpace(req.URL)}}
		if req.MaxCharacters > 0 {
			payload["content"] = map[string]any{"max_chars": req.MaxCharacters}
		}
		return payload, "", nil
	}
	return nil, "", searchError(http.StatusBadRequest, "Keenable does not serve "+endpoint)
}

// runKeenable sends one request. conn is nil for the keyless path; a connection
// whose key is empty also goes keyless.
func (h *MediaHandler) runKeenable(r *http.Request, endpoint string, payload map[string]any, model string, conn *models.ProviderConnection, connData *chat.ConnectionData) (map[string]any, *searchUpstreamError) {
	cfg, cfgErr := h.ChatH.GetProviderConfig("keenable", connData)
	if cfgErr != nil || cfg == nil {
		return nil, searchError(http.StatusBadGateway, "Keenable provider config unavailable")
	}
	target := cfg.SearchURL
	if endpoint == "/v1/web/fetch" {
		target = cfg.FetchURL
	}
	apiKey := ""
	if conn != nil {
		apiKey = chat.ExtractAPIKey(connData)
	}
	if apiKey == "" {
		target = keenablePublicURL(target)
	}
	if !keenableURLAllowed(target) {
		return nil, searchError(http.StatusBadGateway, "Keenable URL must be https://"+keenableHost)
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, searchError(http.StatusInternalServerError, err.Error())
	}
	// Only the headers built here go upstream: the caller's own headers (its
	// gateway key among them) are not relayed to Keenable.
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, target, bytes.NewReader(raw))
	if err != nil {
		return nil, searchError(http.StatusBadGateway, fmt.Sprintf("create Keenable request: %v", err))
	}
	req.Header.Set(constants.HeaderContentType, constants.ContentTypeJSON)
	req.Header.Set(constants.HeaderAccept, constants.ContentTypeJSON)
	req.Header.Set(keenableTitleHeader, keenableAppTitle)
	if apiKey != "" {
		handlerutil.SetAuthHeader(req, apiKey, cfg.AuthHeader, cfg.AuthScheme)
	}

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
	// Keenable answers in place; refusing a redirect keeps the key from being
	// replayed to whatever host a 3xx names.
	noRedirect := *client
	noRedirect.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }

	start := time.Now()
	resp, err := noRedirect.Do(req)
	if err != nil {
		if r.Context().Err() != nil {
			return nil, &searchUpstreamError{Status: 499, Message: "client closed request"}
		}
		return nil, &searchUpstreamError{Status: http.StatusBadGateway, Message: fmt.Sprintf("Keenable upstream request failed: %v", err), Retryable: true}
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		return nil, searchError(http.StatusBadGateway, fmt.Sprintf("Keenable answered an unexpected redirect (status %d)", resp.StatusCode))
	}
	if resp.StatusCode >= 400 {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, keenableErrBodyLimit))
		errText := keenableErrorText(errBody, resp.Status)
		if conn == nil {
			// Keyless: there is no account to lock or rotate to.
			return nil, searchError(resp.StatusCode, fmt.Sprintf("Keenable upstream error (status %d): %s", resp.StatusCode, errText))
		}
		backoff := 0
		if h.Repo != nil {
			backoff = h.Repo.GetConnectionBackoffLevel(conn.ID)
		}
		classification := providers.ClassifyError(resp.StatusCode, errText, backoff)
		if classification.ShouldFallback && h.Repo != nil && !handlerutil.IsProbeContext(r.Context()) {
			cooldownSec := max(classification.CooldownMs/1000, 1)
			_ = h.Repo.LockConnectionModel(conn.ID, model, cooldownSec, classification.NewBackoffLevel)
			log.Warn("search", "keenable account locked, trying next", "conn", conn.ID[:min(8, len(conn.ID))], "status", resp.StatusCode, "cooldown_s", cooldownSec)
		}
		return nil, &searchUpstreamError{Status: resp.StatusCode, Message: fmt.Sprintf("Keenable upstream error (status %d): %s", resp.StatusCode, errText), Retryable: classification.ShouldFallback}
	}

	var out map[string]any
	if err := json.UnmarshalRead(resp.Body, &out); err != nil {
		return nil, &searchUpstreamError{Status: http.StatusBadGateway, Message: fmt.Sprintf("decode Keenable response: %v", err), Retryable: true}
	}
	if out == nil {
		out = map[string]any{}
	}
	out["_latency_ms"] = time.Since(start).Milliseconds()

	if conn != nil && h.Repo != nil {
		_ = h.Repo.UpdateConnectionLastUsed(conn.ID)
		if !handlerutil.IsProbeContext(r.Context()) {
			_ = h.Repo.UnlockConnectionModel(conn.ID, model)
		}
	}
	return out, nil
}

// keenableErrorText pulls a message out of a Keenable error body
// ({"detail"}, {"error":{"message"}} or {"message"}), falling back to the raw
// text and then to the status line.
func keenableErrorText(body []byte, status string) string {
	var env struct {
		Detail any `json:"detail"`
		Error  *struct {
			Message string `json:"message"`
		} `json:"error"`
		Message string `json:"message"`
	}
	if json.Unmarshal(body, &env) == nil {
		if s, ok := env.Detail.(string); ok && s != "" {
			return s
		}
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

// keenableLead collapses whitespace and keeps the first keenableSnippetRunes
// runes of a page text.
func keenableLead(text string) string {
	lead := strings.Join(strings.Fields(text), " ")
	if runes := []rune(lead); len(runes) > keenableSnippetRunes {
		return string(runes[:keenableSnippetRunes]) + "…"
	}
	return lead
}

// keenableSearchResponse maps a Keenable search answer
// ({results:[{title,url,snippet}]}) onto the search shape the other search
// lanes answer with. Keenable puts the page text in `snippet`; `description`
// is a fallback only.
func keenableSearchResponse(query string, out map[string]any) map[string]any {
	rawResults, _ := out["results"].([]any)
	results := make([]map[string]any, 0, len(rawResults))
	for _, item := range rawResults {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		link, _ := m["url"].(string)
		title, _ := m["title"].(string)
		text, _ := m["snippet"].(string)
		if strings.TrimSpace(text) == "" {
			text, _ = m["description"].(string)
		}
		display := link
		if u, err := url.Parse(link); err == nil && u.Host != "" {
			display = u.Host + u.EscapedPath()
		}
		rank := len(results) + 1
		results = append(results, map[string]any{
			"title":        title,
			"url":          link,
			"display_url":  display,
			"snippet":      keenableLead(text),
			"position":     rank,
			"published_at": nil,
			"favicon_url":  nil,
			"content": map[string]any{
				"format": "text",
				"text":   text,
				"length": len(text),
			},
			"metadata": map[string]any{
				"author":      nil,
				"source_type": "web",
				"image_url":   nil,
			},
			"citation": map[string]any{
				"provider": "keenable",
				"rank":     rank,
			},
		})
	}
	latency := out["_latency_ms"]
	return map[string]any{
		"provider": "keenable",
		"query":    query,
		"results":  results,
		"usage": map[string]any{
			"queries_used":          1,
			"search_cost_usd":       nil,
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

// keenableFetchResponse maps the first entry of a Keenable fetch answer
// ({results:[{url,title,content}]}, or {url,error:{status,message}} for a page
// that could not be read) to a flat fetch answer with the page under `content`.
func keenableFetchResponse(out map[string]any) (map[string]any, error) {
	rawResults, _ := out["results"].([]any)
	if len(rawResults) == 0 {
		return nil, searchError(http.StatusBadGateway, "Keenable fetch returned no result")
	}
	page, _ := rawResults[0].(map[string]any)
	if page == nil {
		return nil, searchError(http.StatusBadGateway, "Keenable fetch returned an unreadable result")
	}
	if e, ok := page["error"].(map[string]any); ok {
		status := http.StatusBadGateway
		if s, ok := e["status"].(float64); ok && s >= 400 && s < 600 {
			status = int(s)
		}
		msg, _ := e["message"].(string)
		if msg == "" {
			msg = "page could not be fetched"
		}
		return nil, searchError(status, "Keenable fetch: "+msg)
	}
	content, _ := page["content"].(string)
	return map[string]any{
		"provider": "keenable",
		"url":      page["url"],
		"title":    page["title"],
		"content":  content,
		"format":   "markdown",
		"usage": map[string]any{
			"fetch_cost_usd": nil,
		},
	}, nil
}
