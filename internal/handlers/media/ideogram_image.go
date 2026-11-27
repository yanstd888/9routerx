// 9RouterX — ported & modified by https://github.com/yanstd888/ — https://t.me/aiclub_id
package media

import (
	"bytes"
	"context"
	"encoding/base64"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"9routerx/proxy/internal/handlers/chat"
	"9routerx/proxy/internal/handlerutil"
	"9routerx/proxy/internal/proxy"
)

// Ideogram's native image lane: port of the pinned
// handlers/imageGeneration/providers/ideogram.ts (OmniRoute 632d84f5). The
// generic /v1/images/generations forward would append another path to the
// bare service root and forward caller-only fields the vendor ignores.
const (
	ideogramModelDefault   = "V_3"
	ideogramAspectRatio    = "ASPECT_16_9"
	ideogramMaxImageBytes  = 25 << 20 // UPSTREAM_MEDIA_LIMITS.image
	ideogramMaxRedirects   = 3
	ideogramMaxErrorBytes  = 1 << 20
	ideogramDownloadMaxAge = 60 * time.Second
)

var (
	errIdeogramDownload = errors.New("failed to download image: upstream download failed")
	errIdeogramTimeout  = errors.New("failed to download image: download timed out")
	errIdeogramTooLarge = errors.New("failed to download image: image exceeds 25MiB limit")
	errIdeogramBlocked  = errors.New("failed to download image: result URL blocked by SSRF guard")
)

type ideogramImageRequest struct {
	Prompt string `json:"prompt"`
	Model  string `json:"model"`
}

// ideogramGuardURL applies the dispatch guard to both the generate endpoint
// and every result-download hop. Local endpoints are permitted only through
// the same operator/context switches the generic dispatch path honours; cloud
// metadata literals stay refused either way.
func ideogramGuardURL(ctx context.Context, raw string) (context.Context, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return ctx, errors.New("ideogram endpoint has an invalid URL")
	}
	allowLocal := proxy.AllowPrivateProviderURLs() || proxy.IsPrivateNetworkingAllowed() || proxy.IsLoopbackAllowed(ctx)
	if !allowLocal && parsed.Scheme != "https" {
		return ctx, errors.New("ideogram endpoint must use HTTPS")
	}
	if err := proxy.AssertDispatchURLAllowed(raw, allowLocal); err != nil {
		return ctx, errors.New("ideogram endpoint blocked by SSRF guard")
	}
	if allowLocal {
		ctx = proxy.WithAllowLoopback(ctx)
	}
	return ctx, nil
}

// ideogramModel resolves the vendor id from the router's already-resolved
// model. The caller's raw field is only a fallback for a direct invocation.
func ideogramModel(info *chat.ModelInfo, requested string) string {
	model := strings.TrimSpace(info.Model)
	if model == "" || model == "ideogram" || model == "ideo" {
		model = strings.TrimSpace(requested)
		for _, prefix := range []string{"ideogram/", "ideo/"} {
			model = strings.TrimPrefix(model, prefix)
		}
	}
	if model == "" || model == "ideogram" || model == "ideo" {
		return ideogramModelDefault
	}
	return model
}

func (h *MediaHandler) handleIdeogramImage(w http.ResponseWriter, r *http.Request, body []byte, modelInfo *chat.ModelInfo) error {
	var req ideogramImageRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return fmt.Errorf("parse ideogram image request: %w", err)
	}
	if strings.TrimSpace(req.Prompt) == "" {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "missing required field: prompt")
		return nil
	}
	model := ideogramModel(modelInfo, req.Model)
	conn, connData, err := h.ChatH.GetBestConnectionWithContext(r.Context(), "ideogram", modelInfo.ConnectionID, nil, model)
	if err != nil || connData == nil {
		return fmt.Errorf("no active connection for ideogram: %w", err)
	}
	cfg, err := h.ChatH.GetProviderConfig("ideogram", connData)
	if err != nil {
		return fmt.Errorf("get ideogram config: %w", err)
	}
	// The native image endpoint is the config's ImageURL, not the bare catalog
	// root. Never append /generate here: the operator's config owns the path.
	target := cfg.ImageURL
	if target == "" {
		target = cfg.BaseURL
	}
	if target == "" {
		return errors.New("ideogram image endpoint is not configured")
	}
	// Both configured endpoints pass the dispatch guard; only one is dialed.
	if cfg.BaseURL != "" {
		if _, err := ideogramGuardURL(r.Context(), cfg.BaseURL); err != nil {
			return err
		}
	}
	guardedCtx, err := ideogramGuardURL(r.Context(), target)
	if err != nil {
		return err
	}
	client, err := h.ChatH.GetClientForConnection(connData)
	if err != nil {
		return err
	}
	// A redirect would replay the Api-Key header to a host the guard has not
	// judged, so neither the POST nor the result GET is allowed to follow one.
	noRedirect := *client
	noRedirect.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	client = &noRedirect
	payload, err := json.Marshal(map[string]string{
		"prompt":       req.Prompt,
		"aspect_ratio": ideogramAspectRatio,
		"model":        model,
	})
	if err != nil {
		return fmt.Errorf("build ideogram image request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(guardedCtx, http.MethodPost, target, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("create ideogram image request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	handlerutil.SetAuthHeader(httpReq, chat.ExtractAPIKey(connData), cfg.AuthHeader, cfg.AuthScheme)
	resp, err := client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("ideogram image request failed: %s", sanitizeIdeogramTransportError(err))
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, ideogramMaxErrorBytes))
	if err != nil {
		return fmt.Errorf("read ideogram image response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// The vendor's error body is the answer the caller must see; rewriting
		// it would hide the reason and the status it arrived with.
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(resp.StatusCode)
		_, _ = w.Write(raw)
		return nil
	}
	var result struct {
		Data []struct {
			URL string `json:"url"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return fmt.Errorf("parse ideogram image response: %w", err)
	}
	if len(result.Data) == 0 || result.Data[0].URL == "" {
		return errors.New("no images returned from Ideogram")
	}
	downloadCtx, cancel := context.WithTimeout(r.Context(), ideogramDownloadMaxAge)
	defer cancel()
	buf, err := h.ideogramDownloadImage(downloadCtx, client, result.Data[0].URL)
	if err != nil {
		return err
	}
	out, err := json.Marshal(map[string]any{
		"created": time.Now().Unix(),
		"data":    []map[string]string{{"b64_json": base64.StdEncoding.EncodeToString(buf)}},
	})
	if err != nil {
		return fmt.Errorf("format ideogram image response: %w", err)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(out)
	if h.Repo != nil {
		_ = h.Repo.UpdateConnectionLastUsed(conn.ID)
	}
	return nil
}

// ideogramDownloadImage fetches the vendor's result URL with the image byte
// and time caps. Every redirect hop is re-guarded, and the GET carries no
// headers at all — the Api-Key belongs to the generate POST only.
func (h *MediaHandler) ideogramDownloadImage(ctx context.Context, client *http.Client, rawURL string) ([]byte, error) {
	current := rawURL
	for hop := 0; hop <= ideogramMaxRedirects; hop++ {
		parsed, err := url.Parse(current)
		if err != nil {
			return nil, errIdeogramDownload
		}
		guardedCtx, err := ideogramGuardURL(ctx, current)
		if err != nil {
			return nil, errIdeogramBlocked
		}
		req, err := http.NewRequestWithContext(guardedCtx, http.MethodGet, current, nil)
		if err != nil {
			return nil, errIdeogramDownload
		}
		resp, err := client.Do(req)
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				return nil, errIdeogramTimeout
			}
			return nil, errIdeogramDownload
		}
		if resp.StatusCode >= 300 && resp.StatusCode < 400 {
			location := resp.Header.Get("Location")
			_ = resp.Body.Close()
			if hop == ideogramMaxRedirects || location == "" {
				return nil, errIdeogramDownload
			}
			next, err := parsed.Parse(location)
			if err != nil {
				return nil, errIdeogramDownload
			}
			current = next.String()
			continue
		}
		buf, err := io.ReadAll(io.LimitReader(resp.Body, ideogramMaxImageBytes+1))
		_ = resp.Body.Close()
		if err != nil {
			return nil, errIdeogramDownload
		}
		if resp.StatusCode != http.StatusOK {
			return nil, errIdeogramDownload
		}
		if len(buf) > ideogramMaxImageBytes {
			return nil, errIdeogramTooLarge
		}
		return buf, nil
	}
	return nil, errIdeogramDownload
}

// Transport errors can echo the full request URL; the caller only needs the
// failure class, never an upstream address or credential.
func sanitizeIdeogramTransportError(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return "request timed out"
	}
	return "upstream request failed"
}
