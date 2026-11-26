// 9RouterX — native Udio music generation lane.
// OmniRoute 632d84f5: config/musicRegistry.ts and
// open-sse/handlers/musicGeneration.ts. Udio uses a session cookie, a
// generate-proxy POST and a bounded /api/songs poll, not an API key.
package media

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	json "encoding/json/v2"
	"9routerx/proxy/internal/handlers/chat"
	"9routerx/proxy/internal/handlerutil"
	"9routerx/proxy/internal/proxy"
)

const (
	udioDefaultBaseURL   = "https://www.udio.com/api/generate-proxy"
	udioStatusPath       = "/api/songs"
	udioPollInterval     = 5 * time.Second
	udioDeadline         = 300 * time.Second
	udioMaxAudioBytes    = 50 << 20
	udioMaxRedirects     = 3
	udioRequestTimeout   = 60 * time.Second
)

var (
	errUdioDownload  = errors.New("failed to download Udio audio")
	errUdioBlocked   = errors.New("Udio endpoint blocked by SSRF guard")
	errUdioTooLarge  = errors.New("Udio audio exceeds download limit")
	errUdioTimeout   = errors.New("Udio download timed out")
	errUdioNoTracks  = errors.New("no tracks returned from Udio")
	errUdioTimedOut  = errors.New("Udio music generation timed out")
)

type udioGenerateResponse struct {
	TrackIDs []string `json:"track_ids"`
}

type udioSong struct {
	Finished bool   `json:"finished"`
	SongPath string `json:"song_path"`
}

func udioGuardURL(ctx context.Context, raw string) (context.Context, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return ctx, errUdioBlocked
	}
	allowLocal := proxy.AllowPrivateProviderURLs() || proxy.IsPrivateNetworkingAllowed() || proxy.IsLoopbackAllowed(ctx)
	if !allowLocal && parsed.Scheme != "https" {
		return ctx, errUdioBlocked
	}
	if err := proxy.AssertDispatchURLAllowed(raw, allowLocal); err != nil {
		return ctx, errUdioBlocked
	}
	if allowLocal {
		ctx = proxy.WithAllowLoopback(ctx)
	}
	return ctx, nil
}

func udioCookie(connData *chat.ConnectionData) string {
	if connData == nil {
		return ""
	}
	if cookie := strings.TrimSpace(connData.APIKey); cookie != "" {
		return cookie
	}
	if cookie, ok := connData.ProviderSpecificData["cookie"].(string); ok {
		return strings.TrimSpace(cookie)
	}
	return ""
}

func udioGenerateURL(base string) string {
	if trimmed := strings.TrimSpace(base); trimmed != "" {
		return strings.TrimRight(trimmed, "/")
	}
	return udioDefaultBaseURL
}

func udioStatusURL(base string, ids []string) string {
	root := "https://www.udio.com"
	if parsed, err := url.Parse(udioGenerateURL(base)); err == nil && parsed.Host != "" && parsed.Host != "www.udio.com" {
		root = parsed.Scheme + "://" + parsed.Host
	}
	return root + udioStatusPath + "?songIds=" + url.QueryEscape(strings.Join(ids, ","))
}

func udioResolveAudio(base, path string) string {
	if parsed, err := url.Parse(path); err == nil && parsed.IsAbs() {
		return path
	}
	root, err := url.Parse(udioGenerateURL(base))
	if err != nil {
		return path
	}
	root.Path = ""
	root.RawQuery = ""
	if resolved, err := root.Parse(path); err == nil {
		return resolved.String()
	}
	return path
}

func (h *MediaHandler) handleUdioMusic(w http.ResponseWriter, r *http.Request, req MusicGenerationRequest, cookie, base string, connData *chat.ConnectionData) {
	if cookie == "" {
		handlerutil.WriteJSONError(w, http.StatusUnauthorized, "Udio session cookie is required")
		return
	}
	ctx := r.Context()
	generateURL := udioGenerateURL(base)
	guarded, err := udioGuardURL(ctx, generateURL)
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	payload, err := json.Marshal(map[string]any{
		"prompt": req.Prompt,
		"samplerOptions": map[string]any{"seed": -1},
	})
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, "build Udio request: "+err.Error())
		return
	}
	client, clientErr := h.ChatH.GetClientForConnection(connData)
	if clientErr != nil || client == nil {
		handlerutil.WriteJSONError(w, http.StatusBadGateway, "create Udio client")
		return
	}
	httpReq, err := http.NewRequestWithContext(guarded, http.MethodPost, generateURL, strings.NewReader(string(payload)))
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadGateway, "create Udio request: "+err.Error())
		return
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Cookie", cookie)
	resp, err := client.Do(httpReq)
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadGateway, "Udio request failed")
		return
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	_ = resp.Body.Close()
	if err != nil || resp.StatusCode < 200 || resp.StatusCode >= 300 {
		handlerutil.WriteJSONError(w, http.StatusBadGateway, "Udio request failed")
		return
	}
	var generated udioGenerateResponse
	if json.Unmarshal(raw, &generated) != nil || len(generated.TrackIDs) == 0 {
		handlerutil.WriteJSONError(w, http.StatusBadGateway, errUdioNoTracks.Error())
		return
	}

	deadline := time.Now().Add(udioDeadline)
	var audio []byte
	for time.Now().Before(deadline) {
		songs, err := udioFetchSongs(ctx, client, cookie, udioStatusURL(base, generated.TrackIDs))
		if err == nil {
			for _, song := range songs {
				if !song.Finished || song.SongPath == "" {
					continue
				}
				audio, err = udioDownload(ctx, client, udioResolveAudio(base, song.SongPath))
				if err != nil {
					handlerutil.WriteJSONError(w, http.StatusBadGateway, sanitizeUdioError(err))
					return
				}
				break
			}
		}
		if len(audio) > 0 {
			break
		}
		select {
		case <-ctx.Done():
			handlerutil.WriteJSONError(w, http.StatusRequestTimeout, errUdioTimedOut.Error())
			return
		case <-time.After(udioPollInterval):
		}
	}
	if len(audio) == 0 {
		handlerutil.WriteJSONError(w, http.StatusGatewayTimeout, errUdioTimedOut.Error())
		return
	}
	encoded, err := json.Marshal(map[string]any{
		"created": time.Now().Unix(),
		"data":    []any{map[string]any{"b64_json": base64.StdEncoding.EncodeToString(audio), "format": "mp3"}},
	})
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, "encode Udio response")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(encoded)
}

func udioFetchSongs(ctx context.Context, client *http.Client, cookie, target string) ([]udioSong, error) {
	guarded, err := udioGuardURL(ctx, target)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(guarded, http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Cookie", cookie)
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("Udio status %d", response.StatusCode)
	}
	var songs []udioSong
	statusBody, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(statusBody, &songs); err != nil {
		return nil, err
	}
	return songs, nil
}

func udioDownload(ctx context.Context, client *http.Client, raw string) ([]byte, error) {
	requestCtx, cancel := context.WithTimeout(ctx, udioRequestTimeout)
	defer cancel()
	current := raw
	for hop := 0; hop <= udioMaxRedirects; hop++ {
		guarded, err := udioGuardURL(requestCtx, current)
		if err != nil {
			return nil, err
		}
		request, err := http.NewRequestWithContext(guarded, http.MethodGet, current, nil)
		if err != nil {
			return nil, err
		}
		response, err := client.Do(request)
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				return nil, errUdioTimeout
			}
			return nil, err
		}
		if response.StatusCode >= 300 && response.StatusCode < 400 {
			location := response.Header.Get("Location")
			_ = response.Body.Close()
			if hop == udioMaxRedirects || location == "" {
				return nil, errUdioDownload
			}
			base, err := url.Parse(current)
			if err != nil {
				return nil, errUdioDownload
			}
			next, err := base.Parse(location)
			if err != nil {
				return nil, errUdioDownload
			}
			current = next.String()
			continue
		}
		buf, err := io.ReadAll(io.LimitReader(response.Body, udioMaxAudioBytes+1))
		_ = response.Body.Close()
		if err != nil {
			return nil, err
		}
		if response.StatusCode != http.StatusOK {
			return nil, errUdioDownload
		}
		if len(buf) > udioMaxAudioBytes {
			return nil, errUdioTooLarge
		}
		return buf, nil
	}
	return nil, errUdioDownload
}

func sanitizeUdioError(err error) string {
	switch {
	case errors.Is(err, errUdioTimeout):
		return errUdioTimeout.Error()
	case errors.Is(err, errUdioTooLarge):
		return errUdioTooLarge.Error()
	case errors.Is(err, errUdioBlocked):
		return errUdioBlocked.Error()
	default:
		return errUdioDownload.Error()
	}
}
