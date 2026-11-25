// 9RouterX — ported & modified by https://github.com/yanstd888/ — https://t.me/aiclub_id
package media

import (
	"9routerx/proxy/internal/handlers/chat"
	"9routerx/proxy/internal/handlerutil"
	"9routerx/proxy/internal/proxy"
	"bytes"
	"context"
	"encoding/base64"
	json "encoding/json/v2"
	"errors"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Native Leonardo image lane (OmniRoute 632d84f5): UUID models and a poll.
const (
	leonardoModelDefault   = "phoenix"
	leonardoMaxImageBytes  = 25 << 20
	leonardoMaxRedirects   = 3
	leonardoMaxErrorBytes  = 1 << 20
	leonardoDownloadMaxAge = 60 * time.Second
)

// Poll values are injectable; only the two catalog ids are mapped to UUIDs.
var (
	leonardoPollInterval = 5 * time.Second
	leonardoPollDeadline = 300 * time.Second
	leonardoModelIDs     = map[string]string{
		"phoenix": "de7d3faf-762f-48e0-b3b7-9d0ac3a3fcf3",
		"sdxl":    "16e7060a-803e-4df3-97ee-edcfa5dc9cc8",
	}
	errLeonardoDownload = errors.New("Failed to download image: upstream download failed")
	errLeonardoTimeout  = errors.New("Failed to download image: download timed out")
	errLeonardoTooLarge = errors.New("Failed to download image: image exceeds 25MiB limit")
	errLeonardoBlocked  = errors.New("Failed to download image: result URL blocked by SSRF guard")
)
var leonardoSizePattern = regexp.MustCompile(`(?i)^(\d+)x(\d+)$`)

type leonardoImageRequest struct {
	Prompt string `json:"prompt"`
	Model  string `json:"model"`
	Size   string `json:"size"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

// leonardoModel returns the choice and its platform UUID; unknown ids pass.
func leonardoModel(info *chat.ModelInfo, requested string) (string, string) {
	choice := ""
	if info != nil {
		choice = strings.TrimSpace(info.Model)
	}
	if choice == "" || choice == "leonardo" || choice == "leo" {
		choice = strings.TrimSpace(requested)
	}
	for _, prefix := range []string{"leonardo/", "leo/"} {
		choice = strings.TrimPrefix(choice, prefix)
	}
	if choice == "" || choice == "leonardo" || choice == "leo" {
		choice = leonardoModelDefault
	}
	if id, ok := leonardoModelIDs[choice]; ok {
		return choice, id
	}
	return choice, choice
}

// Explicit width/height win per axis, then size, then 1024.
func leonardoImageSize(req leonardoImageRequest) (int, int) {
	width, height := 1024, 1024
	if match := leonardoSizePattern.FindStringSubmatch(strings.TrimSpace(req.Size)); match != nil {
		width, _ = strconv.Atoi(match[1])
		height, _ = strconv.Atoi(match[2])
	}
	if req.Width > 0 {
		width = req.Width
	}
	if req.Height > 0 {
		height = req.Height
	}
	return width, height
}
func leonardoGuardURL(ctx context.Context, raw string) (context.Context, error) {
	u, err := url.Parse(raw)
	allowLocal := proxy.AllowPrivateProviderURLs() || proxy.IsPrivateNetworkingAllowed() || proxy.IsLoopbackAllowed(ctx)
	if err != nil || (!allowLocal && u.Scheme != "https") || proxy.AssertDispatchURLAllowed(raw, allowLocal) != nil {
		return ctx, errors.New("Leonardo endpoint blocked by SSRF guard")
	}
	if allowLocal {
		ctx = proxy.WithAllowLoopback(ctx)
	}
	return ctx, nil
}
func leonardoStatusURL(base, id string) string {
	u, _ := url.Parse(base)
	u.Path = strings.TrimRight(u.Path, "/") + "/" + url.PathEscape(id)
	u.RawPath, u.RawQuery, u.Fragment = "", "", ""
	return u.String()
}
func (h *MediaHandler) handleLeonardoImage(w http.ResponseWriter, r *http.Request, body []byte, modelInfo *chat.ModelInfo) error {
	var req leonardoImageRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return errors.New("invalid leonardo image request")
	}
	if strings.TrimSpace(req.Prompt) == "" {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, "missing required field: prompt")
		return nil
	}
	choice, modelID := leonardoModel(modelInfo, req.Model)
	_, connData, err := h.ChatH.GetBestConnectionWithContext(r.Context(), "leonardo", modelInfo.ConnectionID, nil, choice)
	if err != nil || connData == nil {
		return errors.New("no active connection for leonardo")
	}
	cfg, err := h.ChatH.GetProviderConfig("leonardo", connData)
	if err != nil {
		return errors.New("get leonardo config failed")
	}
	target := cfg.ImageURL
	if target == "" {
		target = cfg.BaseURL
	}
	if target == "" {
		return errors.New("Leonardo image endpoint is not configured")
	}
	guardedCtx, err := leonardoGuardURL(r.Context(), target)
	if err != nil {
		return err
	}
	client, err := h.ChatH.GetClientForConnection(connData)
	if err != nil {
		return err
	}
	noRedirect := *client
	noRedirect.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	client = &noRedirect
	width, height := leonardoImageSize(req)
	payload, _ := json.Marshal(struct {
		ModelID   string `json:"modelId"`
		Prompt    string `json:"prompt"`
		Width     int    `json:"width"`
		Height    int    `json:"height"`
		NumImages int    `json:"num_images"`
	}{modelID, req.Prompt, width, height, 1})
	httpReq, _ := http.NewRequestWithContext(guardedCtx, http.MethodPost, target, bytes.NewReader(payload))
	apiKey := chat.ExtractAPIKey(connData)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	resp, err := client.Do(httpReq)
	if err != nil {
		return errors.New("leonardo image request failed: " + sanitizeLeonardoTransportError(err))
	}
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, leonardoMaxErrorBytes))
	_ = resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(resp.StatusCode)
		_, _ = w.Write(raw)
		return nil
	}
	var submitted struct {
		SDGenerationJob *struct {
			GenerationID string `json:"generationId"`
		} `json:"sdGenerationJob"`
	}
	if err := json.Unmarshal(raw, &submitted); err != nil {
		return errors.New("parse leonardo image response failed")
	}
	if submitted.SDGenerationJob == nil || submitted.SDGenerationJob.GenerationID == "" {
		return errors.New("No generation ID returned")
	}
	return h.leonardoPoll(w, r, client, target, submitted.SDGenerationJob.GenerationID, apiKey)
}

type leonardoGeneration struct {
	Status          string `json:"status"`
	GeneratedImages []struct {
		URL string `json:"url"`
	} `json:"generated_images"`
}
type leonardoPollResult struct {
	leonardoGeneration
	GenerationsByPK *leonardoGeneration `json:"generations_by_pk"`
}

func (h *MediaHandler) leonardoPoll(w http.ResponseWriter, r *http.Request, client *http.Client, target, genID, apiKey string) error {
	deadline := time.Now().Add(leonardoPollDeadline)
	for time.Now().Before(deadline) {
		time.Sleep(leonardoPollInterval)
		statusURL := leonardoStatusURL(target, genID)
		guardedCtx, err := leonardoGuardURL(r.Context(), statusURL)
		if err != nil {
			return err
		}
		req, _ := http.NewRequestWithContext(guardedCtx, http.MethodGet, statusURL, nil)
		req.Header.Set("Authorization", "Bearer "+apiKey)
		resp, err := client.Do(req)
		if err != nil {
			return errors.New("leonardo poll failed: " + sanitizeLeonardoTransportError(err))
		}
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, leonardoMaxErrorBytes))
		_ = resp.Body.Close()
		var polled leonardoPollResult
		if err := json.Unmarshal(raw, &polled); err != nil {
			return errors.New("parse leonardo poll response failed")
		}
		gen := polled.leonardoGeneration
		if polled.GenerationsByPK != nil {
			gen = *polled.GenerationsByPK
		}
		if gen.Status == "FAILED" {
			handlerutil.WriteJSONError(w, http.StatusBadGateway, "Leonardo image generation failed")
			return nil
		}
		if gen.Status != "COMPLETE" || len(gen.GeneratedImages) == 0 || gen.GeneratedImages[0].URL == "" {
			continue
		}
		downloadCtx, cancel := context.WithTimeout(r.Context(), leonardoDownloadMaxAge)
		defer cancel()
		buf, err := h.leonardoDownloadImage(downloadCtx, client, gen.GeneratedImages[0].URL)
		if err != nil {
			return err
		}
		out, _ := json.Marshal(map[string]any{
			"created": time.Now().Unix(),
			"data":    []map[string]string{{"b64_json": base64.StdEncoding.EncodeToString(buf)}},
		})
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(out)
		return nil
	}
	handlerutil.WriteJSONError(w, http.StatusGatewayTimeout, "Leonardo image generation timed out")
	return nil
}
func (h *MediaHandler) leonardoDownloadImage(ctx context.Context, client *http.Client, rawURL string) ([]byte, error) {
	current := rawURL
	for hop := 0; hop <= leonardoMaxRedirects; hop++ {
		parsed, err := url.Parse(current)
		if err != nil {
			return nil, errLeonardoDownload
		}
		guardedCtx, err := leonardoGuardURL(ctx, current)
		if err != nil {
			return nil, errLeonardoBlocked
		}
		req, _ := http.NewRequestWithContext(guardedCtx, http.MethodGet, current, nil)
		resp, err := client.Do(req)
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				return nil, errLeonardoTimeout
			}
			return nil, errLeonardoDownload
		}
		if resp.StatusCode >= 300 && resp.StatusCode < 400 {
			location := resp.Header.Get("Location")
			_ = resp.Body.Close()
			if hop == leonardoMaxRedirects || location == "" {
				return nil, errLeonardoDownload
			}
			next, err := parsed.Parse(location)
			if err != nil {
				return nil, errLeonardoDownload
			}
			current = next.String()
			continue
		}
		buf, err := io.ReadAll(io.LimitReader(resp.Body, leonardoMaxImageBytes+1))
		_ = resp.Body.Close()
		if err != nil || resp.StatusCode != http.StatusOK {
			return nil, errLeonardoDownload
		}
		if len(buf) > leonardoMaxImageBytes {
			return nil, errLeonardoTooLarge
		}
		return buf, nil
	}
	return nil, errLeonardoDownload
}
func sanitizeLeonardoTransportError(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return "request timed out"
	}
	return "upstream request failed"
}
