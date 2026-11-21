package oauth

import (
	"context"
	json "encoding/json/v2"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"9routerx/proxy/internal/providers"
	"9routerx/proxy/internal/proxy/ghecopilot"
)

// githubDeviceFlow parameterizes the same GitHub device grant without changing
// the public github provider's endpoints, response shaping, or lookups.
type githubDeviceFlow struct {
	clientID  string
	deviceURL string
	tokenURL  string
	post      func(context.Context, string, url.Values, map[string]string) (map[string]any, int, error)
}

func (flow githubDeviceFlow) start(ctx context.Context) (map[string]any, error) {
	data, status, err := flow.post(ctx, flow.deviceURL, url.Values{"client_id": {flow.clientID}, "scope": {"read:user"}}, nil)
	if err != nil {
		return nil, fmt.Errorf("device code request: %w", err)
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("device code request failed: status=%d", status)
	}
	return data, nil
}

func (flow githubDeviceFlow) poll(ctx context.Context, code string) (map[string]any, int, error) {
	return flow.post(ctx, flow.tokenURL, url.Values{
		"client_id": {flow.clientID}, "device_code": {code},
		"grant_type": {"urn:ietf:params:oauth:grant-type:device_code"},
	}, nil)
}

func gheFlow(instance string) (githubDeviceFlow, string, error) {
	base, err := ghecopilot.InstanceURL(instance)
	if err != nil {
		return githubDeviceFlow{}, "", err
	}
	return githubDeviceFlow{ghecopilot.ClientID(), base + "/login/device/code", base + "/login/oauth/access_token", ghePostForm}, base, nil
}

func ghePostForm(ctx context.Context, endpoint string, form url.Values, _ map[string]string) (map[string]any, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := ghecopilot.Do(ctx, deviceClient, req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	var data map[string]any
	if err := json.UnmarshalRead(io.LimitReader(resp.Body, 1<<20), &data); err != nil {
		return nil, resp.StatusCode, fmt.Errorf("invalid response from GHE host")
	}
	return data, resp.StatusCode, nil
}

func gheStart(ctx context.Context, instance string) (map[string]any, error) {
	flow, base, err := gheFlow(instance)
	if err != nil {
		return nil, err
	}
	data, err := flow.start(ctx)
	if err != nil {
		return nil, err
	}
	out := map[string]any{"session": map[string]any{"gheUrl": base}}
	for _, field := range []string{"device_code", "user_code", "verification_uri", "verification_uri_complete"} {
		if value := strVal(data, field); value != "" {
			out[field] = value
		}
	}
	if strVal(out, "device_code") == "" || strVal(out, "verification_uri") == "" {
		return nil, fmt.Errorf("GHE device response is missing code or verification URL")
	}
	for _, field := range []string{"verification_uri", "verification_uri_complete"} {
		if value := strVal(out, field); value != "" {
			u, err := url.Parse(value)
			origin, _ := url.Parse(base)
			if err != nil || u.Scheme != "https" || u.User != nil || !strings.EqualFold(u.Host, origin.Host) {
				return nil, fmt.Errorf("GHE verification URL must use the instance origin")
			}
		}
	}
	for _, field := range []string{"expires_in", "interval"} {
		if number, ok := data[field].(float64); ok && number > 0 {
			out[field] = number
		}
	}
	return out, nil
}

func ghePoll(ctx context.Context, code string, session map[string]any) (deviceTokens, error) {
	var tokens deviceTokens
	flow, base, err := gheFlow(strVal(session, "gheUrl"))
	if err != nil {
		return tokens, err
	}
	data, status, err := flow.poll(ctx, code)
	if err != nil {
		return tokens, err
	}
	if value := strVal(data, "error"); value != "" {
		if number, ok := data["interval"].(float64); ok && number > 0 {
			tokens.interval = int(number)
		}
		switch value {
		case "authorization_pending", "slow_down", "expired_token", "access_denied", "incorrect_device_code", "incorrect_client_credentials", "device_flow_disabled", "unsupported_grant_type":
			return tokens, fmt.Errorf("%s", value)
		default:
			return tokens, fmt.Errorf("invalid response from GHE host")
		}
	}
	if status != http.StatusOK {
		return tokens, fmt.Errorf("GHE device poll failed: HTTP %d", status)
	}
	tokens.access, tokens.refresh = strVal(data, "access_token"), strVal(data, "refresh_token")
	if tokens.access == "" {
		return tokens, fmt.Errorf("GHE device poll returned no access token")
	}
	if number, ok := data["expires_in"].(float64); ok && number > 0 {
		tokens.expiresIn = int(number)
	}
	tokens.extra = map[string]any{"gheUrl": base, "autoSync": true}
	copilot, exchangeErr := ghecopilot.Exchange(ctx, deviceClient, base, tokens.access)
	if exchangeErr == nil {
		ghecopilot.ApplyToken(tokens.extra, copilot)
	}
	if err := ctx.Err(); err != nil {
		return deviceTokens{}, err
	}
	user := gheUser(ctx, base, tokens.access)
	if err := ctx.Err(); err != nil {
		return deviceTokens{}, err
	}
	tokens.name, tokens.email = strVal(user, "name", "login"), strVal(user, "email")
	for field, value := range map[string]any{"githubUserId": user["id"], "githubLogin": strVal(user, "login"), "githubName": strVal(user, "name"), "githubEmail": tokens.email} {
		tokens.extra[field] = value
	}
	return tokens, nil
}

func gheUser(ctx context.Context, base, access string) map[string]any {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/api/v3/user", nil)
	if err != nil {
		return nil
	}
	req.Header.Set("Authorization", "Bearer "+access)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", providers.GitHubCopilotChatUserAgent())
	resp, err := ghecopilot.Do(ctx, deviceClient, req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	var user map[string]any
	_ = json.UnmarshalRead(io.LimitReader(resp.Body, 1<<20), &user)
	return user
}
