// 9RouterX — port and modifications by https://github.com/yanstd888/ — https://t.me/aiclub_id
package oauth

import (
	"context"
	json "encoding/json/v2"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"9routerx/proxy/internal/providers"
)

func init() { Register("workbuddy", refreshWorkbuddy) }

func refreshWorkbuddy(ctx context.Context, p *Params) (*TokenResult, error) {
	if p.RefreshToken == "" {
		return nil, fmt.Errorf("workbuddy: refresh token is required")
	}
	client := p.Client
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://www.workbuddy.ai/v2/plugin/auth/token/refresh", strings.NewReader(`{}`))
	if err != nil {
		return nil, fmt.Errorf("workbuddy refresh request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Refresh-Token", p.RefreshToken)
	req.Header.Set("X-Product", "SaaS")
	req.Header.Set("X-Domain", "www.workbuddy.ai")
	req.Header.Set("User-Agent", "WorkBuddy/5.5.6")
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("workbuddy refresh: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, &providers.OAuthRefreshError{Status: resp.StatusCode, Body: "WorkBuddy refresh rejected"}
	}
	var payload struct {
		Code int `json:"code"`
		Data struct {
			AccessToken  string `json:"accessToken"`
			RefreshToken string `json:"refreshToken"`
			ExpiresIn    int    `json:"expiresIn"`
		} `json:"data"`
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("workbuddy refresh response: %w", err)
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("workbuddy refresh response: %w", err)
	}
	if payload.Code != 0 || payload.Data.AccessToken == "" {
		return nil, fmt.Errorf("workbuddy refresh returned no token (code %d)", payload.Code)
	}
	if payload.Data.RefreshToken == "" {
		payload.Data.RefreshToken = p.RefreshToken
	}
	return &TokenResult{AccessToken: payload.Data.AccessToken, RefreshToken: payload.Data.RefreshToken, ExpiresIn: payload.Data.ExpiresIn}, nil
}
