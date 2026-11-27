package oauth

import (
	"context"
	json "encoding/json/v2"
	"fmt"
	"io"
	"net/http"
	"strings"

	"9routerx/proxy/internal/log"
	"9routerx/proxy/internal/providers"
)

type codebuddySite struct {
	url, domain, userAgent string
}

var codebuddySites = map[string]codebuddySite{
	"codebuddy-cn":   {"https://copilot.tencent.com/v2/plugin/auth/token/refresh", "copilot.tencent.com", "CLI/2.63.2 CodeBuddy/2.63.2"},
	"codebuddy-intl": {"https://www.codebuddy.ai/v2/plugin/auth/token/refresh", "www.codebuddy.ai", "IDE/2.63.2 CodeBuddy/2.63.2"},
}

func init() {
	Register("codebuddy-cn", refreshCodebuddyCN)
	Register("codebuddy-intl", refreshCodebuddyIntl)
}

func refreshCodebuddyCN(ctx context.Context, p *Params) (*TokenResult, error) {
	return refreshCodebuddy(ctx, p, "codebuddy-cn")
}

func refreshCodebuddyIntl(ctx context.Context, p *Params) (*TokenResult, error) {
	return refreshCodebuddy(ctx, p, "codebuddy-intl")
}

func refreshCodebuddy(ctx context.Context, p *Params, provider string) (*TokenResult, error) {
	site, ok := codebuddySites[provider]
	if !ok || strings.TrimSpace(p.RefreshToken) == "" {
		return nil, fmt.Errorf("%s: unsupported provider or missing refresh token", provider)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, site.url, strings.NewReader("{}"))
	if err != nil {
		return nil, fmt.Errorf("%s: create refresh request: %w", provider, err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", site.userAgent)
	request.Header.Set("X-Requested-With", "XMLHttpRequest")
	request.Header.Set("X-Domain", site.domain)
	request.Header.Set("X-Refresh-Token", p.RefreshToken)
	request.Header.Set("X-Auth-Refresh-Source", "plugin")
	request.Header.Set("X-Product", "SaaS")
	request.Header.Set("Content-Type", "application/json")
	client := p.Client
	if client == nil {
		client = http.DefaultClient
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("%s: token refresh request failed: %w", provider, err)
	}
	defer func() {
		if err := response.Body.Close(); err != nil {
			log.Warn("oauth", "close CodeBuddy token response body", "provider", provider, "error", err)
		}
	}()
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("%s: read token refresh response: %w", provider, err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, &providers.OAuthRefreshError{Status: response.StatusCode}
	}
	var envelope struct {
		Code *float64 `json:"code"`
		Msg  string   `json:"msg"`
		Data struct {
			AccessToken  string `json:"accessToken"`
			RefreshToken string `json:"refreshToken"`
			ExpiresIn    int    `json:"expiresIn"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("%s: decode refresh response: %w", provider, err)
	}
	if envelope.Code != nil && *envelope.Code != 0 {
		return nil, fmt.Errorf("%s: token refresh rejected with code %d: %s", provider, int(*envelope.Code), envelope.Msg)
	}
	if envelope.Data.AccessToken == "" {
		return nil, fmt.Errorf("%s: token refresh response missing access token", provider)
	}
	refresh := envelope.Data.RefreshToken
	if refresh == "" {
		refresh = p.RefreshToken
	}
	return &TokenResult{AccessToken: envelope.Data.AccessToken, RefreshToken: refresh, ExpiresIn: envelope.Data.ExpiresIn}, nil
}
