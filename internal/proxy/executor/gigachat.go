// 9RouterX — native executor for GigaChat's Basic-to-Bearer token exchange.
// OmniRoute 632d84f5: open-sse/services/gigachatAuth.ts and
// config/providers/registry/gigachat/index.ts. The stored credential is an
// API key encoded as Basic for the token endpoint; inference uses a short-lived
// Bearer access token on the OpenAI-compatible chat lane.
package executor

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	json "encoding/json/v2"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"9routerx/proxy/internal/proxy"
)

const (
	gigachatDefaultAuthURL = "https://ngw.devices.sberbank.ru:9443/api/v2/oauth"
	gigachatDefaultScope   = "GIGACHAT_API_PERS"
	gigachatCacheSkew      = 60 * time.Second
	gigachatTokenBodyMax   = 1 << 20
)

type gigachatToken struct {
	access string
	expiry time.Time
}

var (
	gigachatCacheMu sync.Mutex
	gigachatCache   = map[string]gigachatToken{}
)

// ForwardGigaChat resolves the current access token, then hands the request to
// the shared OpenAI forwarder with the Bearer token rather than the Basic key.
func ForwardGigaChat(w http.ResponseWriter, req *Request) error {
	if req.Config == nil {
		return proxy.UpstreamFailure(http.StatusBadGateway, "GigaChat provider config missing")
	}
	basic := strings.TrimSpace(req.APIKey)
	if basic == "" {
		return proxy.UpstreamFailure(http.StatusUnauthorized, "GigaChat API key required")
	}
	token, err := gigachatAccess(reqCtx(req), req, basic)
	if err != nil {
		return proxy.UpstreamFailure(http.StatusBadGateway, err.Error())
	}
	cfg := *req.Config
	cfg.AuthHeader = "Authorization"
	cfg.AuthScheme = "bearer"
	req.Config = &cfg
	// The Basic credential is only for the token hop; restore the request after
	// the shared forwarder consumes the short-lived Bearer token so a retry or
	// the next request on the same object cannot exchange the token itself.
	defer func() { req.APIKey = basic }()
	req.APIKey = token
	return ForwardOpenAI(w, req)
}

func gigachatAccess(ctx context.Context, req *Request, basic string) (string, error) {
	authURL, scope := gigachatDefaultAuthURL, gigachatDefaultScope
	if req.ConnData != nil {
		if custom, ok := req.ConnData["authUrl"].(string); ok && strings.TrimSpace(custom) != "" {
			authURL = strings.TrimSpace(custom)
		}
		if custom, ok := req.ConnData["scope"].(string); ok && strings.TrimSpace(custom) != "" {
			scope = strings.TrimSpace(custom)
		}
		if access, ok := req.ConnData["accessToken"].(string); ok && access != "" {
			if expiry, ok := req.ConnData["expiresAt"].(string); ok {
				if when, err := time.Parse(time.RFC3339, expiry); err == nil && time.Until(when) > gigachatCacheSkew {
					return access, nil
				}
			}
		}
	}
	digest := sha256.Sum256([]byte(authURL + "::" + scope + "::" + basic))
	key := hex.EncodeToString(digest[:])
	gigachatCacheMu.Lock()
	defer gigachatCacheMu.Unlock()
	if cached, ok := gigachatCache[key]; ok && time.Until(cached.expiry) > gigachatCacheSkew {
		return cached.access, nil
	}
	access, expiry, err := gigachatExchange(ctx, req.Client, authURL, scope, basic)
	if err != nil {
		return "", err
	}
	gigachatCache[key] = gigachatToken{access: access, expiry: expiry}
	return access, nil
}

func gigachatExchange(ctx context.Context, client *http.Client, authURL, scope, basic string) (string, time.Time, error) {
	u, err := url.Parse(authURL)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return "", time.Time{}, fmt.Errorf("GigaChat auth URL must be an http(s) host")
	}
	requestCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(requestCtx, http.MethodPost, authURL, strings.NewReader(url.Values{"scope": {scope}}.Encode()))
	if err != nil {
		return "", time.Time{}, fmt.Errorf("GigaChat token request: %w", err)
	}
	request.Header.Set("Authorization", "Basic "+basic)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("RqUID", gigachatRequestID())
	if client == nil {
		client = http.DefaultClient
	}
	response, err := client.Do(request)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("GigaChat token request: %w", err)
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, gigachatTokenBodyMax))
	if err != nil {
		return "", time.Time{}, fmt.Errorf("read GigaChat token response: %w", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", time.Time{}, fmt.Errorf("GigaChat token request failed with status %d", response.StatusCode)
	}
	var payload struct {
		AccessToken string `json:"access_token"`
		Token       string `json:"tok"`
		ExpiresAt   any    `json:"expires_at"`
		Expires     any    `json:"exp"`
	}
	if json.Unmarshal(raw, &payload) != nil {
		return "", time.Time{}, fmt.Errorf("decode GigaChat token response")
	}
	access := payload.AccessToken
	if access == "" {
		access = payload.Token
	}
	if access == "" {
		return "", time.Time{}, fmt.Errorf("GigaChat token response missing access token")
	}
	expiry := gigachatExpiry(firstPresent(map[string]any{"expires_at": payload.ExpiresAt, "exp": payload.Expires}, "expires_at", "exp"))
	return access, expiry, nil
}

func gigachatExpiry(raw any) time.Time {
	switch value := raw.(type) {
	case float64:
		if value > 0 {
			return time.UnixMilli(int64(value))
		}
	case string:
		if seconds, err := time.ParseDuration(value); err == nil {
			return time.Now().Add(seconds)
		}
		if when, err := time.Parse(time.RFC3339, value); err == nil {
			return when
		}
	}
	return time.Now().Add(30 * time.Minute)
}

func gigachatRequestID() string {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return fmt.Sprintf("%x", time.Now().UnixNano())
	}
	return hex.EncodeToString(raw[:])
}
