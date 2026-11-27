// 9RouterX — native Haiper image/video generation lane.
// OmniRoute 632d84f5: open-sse/handlers/imageGeneration/providers/haiper.ts and
// open-sse/handlers/videoGeneration.ts. Haiper uses HAIPER_KEY, a job POST and
// a bounded /v1/jobs/{job_id} poll for both image and video models.
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
	"9routerx/proxy/internal/proxy"
)

const (
	haiperDefaultBaseURL = "https://api.haiper.ai/v1"
	haiperPollInterval   = 5 * time.Second
	haiperDeadline       = 300 * time.Second
	haiperMaxBytes       = 50 << 20
)

var (
	errHaiperNoJob    = errors.New("haiper response has no job id")
	errHaiperFailed   = errors.New("haiper generation failed")
	errHaiperTimedOut = errors.New("haiper generation timed out")
)

type haiperJob struct {
	JobID string `json:"job_id"`
}

type haiperStatus struct {
	Status      string `json:"status"`
	CreationURL string `json:"creation_url"`
	Output      struct {
		ImageURL string `json:"image_url"`
		VideoURL string `json:"video_url"`
	} `json:"output"`
}

func haiperGuardURL(ctx context.Context, raw string) (context.Context, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return ctx, errors.New("haiper endpoint blocked by SSRF guard")
	}
	allowLocal := proxy.AllowPrivateProviderURLs() || proxy.IsPrivateNetworkingAllowed() || proxy.IsLoopbackAllowed(ctx)
	if !allowLocal && parsed.Scheme != "https" {
		return ctx, errors.New("haiper endpoint must use HTTPS")
	}
	if err := proxy.AssertDispatchURLAllowed(raw, allowLocal); err != nil {
		return ctx, errors.New("haiper endpoint blocked by SSRF guard")
	}
	if allowLocal {
		ctx = proxy.WithAllowLoopback(ctx)
	}
	return ctx, nil
}

func haiperRoot(base string) string {
	if trimmed := strings.TrimSpace(base); trimmed != "" {
		return strings.TrimRight(trimmed, "/")
	}
	return haiperDefaultBaseURL
}

func haiperEndpoints(base string) (string, string) {
	root := haiperRoot(base)
	return root + "/jobs/gen2/text2image", root + "/jobs"
}

func haiperVideoEndpoints(base string) (string, string) {
	root := haiperRoot(base)
	return root + "/jobs/gen2/text2video", root + "/jobs"
}

func (h *MediaHandler) handleHaiperMedia(w http.ResponseWriter, r *http.Request, body []byte, modelInfo *chat.ModelInfo, kind string) error {
	var request struct {
		Prompt      string `json:"prompt"`
		AspectRatio string `json:"aspect_ratio"`
	}
	if json.Unmarshal(body, &request) != nil {
		return errors.New("invalid Haiper request")
	}
	if strings.TrimSpace(request.Prompt) == "" {
		return errors.New("missing required field: prompt")
	}
	conn, connData, err := h.ChatH.GetBestConnectionWithContext(r.Context(), "haiper", "", nil, modelInfo.Model)
	if err != nil || conn == nil {
		return errors.New("no active connection for haiper")
	}
	key := chat.ExtractAPIKey(connData)
	if key == "" {
		return errors.New("no API key found for haiper connection")
	}
	base := haiperRoot(connData.BaseURL)
	submitURL, statusRoot := haiperEndpoints(base)
	format := "png"
	payload := map[string]any{"prompt": request.Prompt, "aspect_ratio": request.AspectRatio}
	if request.AspectRatio == "" {
		payload["aspect_ratio"] = "16:9"
	}
	if kind == "video" {
		submitURL, statusRoot = haiperVideoEndpoints(base)
		format = "mp4"
		payload["duration"] = 4
		payload["aspect_ratio"] = "16:9"
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return errors.New("build Haiper request")
	}
	client, clientErr := h.ChatH.GetClientForConnection(connData)
	if clientErr != nil || client == nil {
		return errors.New("create Haiper client")
	}
	ctx := r.Context()
	guarded, err := haiperGuardURL(ctx, submitURL)
	if err != nil {
		return err
	}
	httpReq, err := http.NewRequestWithContext(guarded, http.MethodPost, submitURL, strings.NewReader(string(encoded)))
	if err != nil {
		return errors.New("create Haiper request")
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("HAIPER_KEY", key)
	resp, err := client.Do(httpReq)
	if err != nil {
		return errors.New("haiper request failed")
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	_ = resp.Body.Close()
	if err != nil || resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return errors.New("haiper request failed")
	}
	var job haiperJob
	if json.Unmarshal(raw, &job) != nil || job.JobID == "" {
		return errHaiperNoJob
	}

	deadline := time.Now().Add(haiperDeadline)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return errHaiperTimedOut
		case <-time.After(haiperPollInterval):
		}
		status, err := haiperStatusOf(ctx, client, key, statusRoot+"/"+job.JobID)
		if err != nil {
			continue
		}
		if status.Status == "failed" {
			return errHaiperFailed
		}
		resultURL := status.CreationURL
		if kind == "video" {
			if resultURL == "" {
				resultURL = status.Output.VideoURL
			}
		} else if resultURL == "" {
			resultURL = status.Output.ImageURL
		}
		if status.Status != "completed" && status.Status != "succeeded" || resultURL == "" {
			continue
		}
		media, err := udioDownload(ctx, client, resultURL)
		if err != nil {
			return errors.New(sanitizeUdioError(err))
		}
		reply, err := json.Marshal(map[string]any{
			"created": time.Now().Unix(),
			"data":    []any{map[string]any{"b64_json": base64.StdEncoding.EncodeToString(media), "format": format}},
		})
		if err != nil {
			return errors.New("encode Haiper response")
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(reply)
		return nil
	}
	return errHaiperTimedOut
}

func haiperStatusOf(ctx context.Context, client *http.Client, key, target string) (haiperStatus, error) {
	var status haiperStatus
	guarded, err := haiperGuardURL(ctx, target)
	if err != nil {
		return status, err
	}
	request, err := http.NewRequestWithContext(guarded, http.MethodGet, target, nil)
	if err != nil {
		return status, err
	}
	request.Header.Set("HAIPER_KEY", key)
	response, err := client.Do(request)
	if err != nil {
		return status, err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return status, fmt.Errorf("haiper status %d", response.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return status, err
	}
	if err := json.Unmarshal(raw, &status); err != nil {
		return status, err
	}
	return status, nil
}
