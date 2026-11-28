// 9RouterX — port and modifications by https://github.com/yanstd888/ — https://t.me/aiclub_id
package oauth

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"9routerx/proxy/internal/providers"
)

func init() { Register("gitlab", refreshGitlab) }

func refreshGitlab(ctx context.Context, p *Params) (*TokenResult, error) {
	base := strings.TrimRight(p.ProviderSpecificData["baseUrl"], "/")
	if base == "" {
		base = "https://gitlab.com"
	}
	parsed, err := url.Parse(base)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.Scheme != "https" {
		return nil, fmt.Errorf("gitlab: invalid OAuth instance URL")
	}
	clientID := p.ProviderSpecificData["clientId"]
	clientSecret := p.ProviderSpecificData["clientSecret"]
	if clientID == "" {
		cfg := providers.KnownOAuthConfigs["gitlab"]
		clientID, clientSecret = cfg.ClientID, cfg.ClientSecret
	}
	if clientID == "" {
		return nil, fmt.Errorf("gitlab: OAuth client ID is required for refresh")
	}
	return NewStandardRefresher(base+"/oauth/token", clientID, clientSecret)(ctx, p)
}
