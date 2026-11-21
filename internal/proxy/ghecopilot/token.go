package ghecopilot

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"9routerx/proxy/internal/providers"
)

func ClientID() string {
	for _, key := range []string{"GHE_COPILOT_OAUTH_CLIENT_ID", "GITHUB_OAUTH_CLIENT_ID"} {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return "Iv1.b507a08c87ecfe98"
}

// Expiry accepts Unix seconds, milliseconds, numeric strings, and RFC3339 dates.
func Expiry(value any) (time.Time, bool) {
	var number float64
	switch v := value.(type) {
	case float64:
		number = v
	case int:
		number = float64(v)
	case int64:
		number = float64(v)
	case string:
		if stamp, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(v)); err == nil {
			return stamp, true
		}
		var err error
		number, err = strconv.ParseFloat(strings.TrimSpace(v), 64)
		if err != nil {
			return time.Time{}, false
		}
	default:
		return time.Time{}, false
	}
	if number <= 0 || number > 1e15 {
		return time.Time{}, false
	}
	if number > 1e12 {
		number /= 1000
	}
	return time.Unix(int64(number), 0), true
}

func Cached(psd map[string]any, now time.Time) bool {
	expiry, known := Expiry(psd["copilotTokenExpiresAt"])
	return String(psd, "copilotToken") != "" && known && expiry.After(now.Add(time.Minute))
}

type Token struct {
	Token     string `json:"token"`
	ExpiresAt any    `json:"expires_at"`
	Endpoints struct {
		API   string `json:"api"`
		Proxy string `json:"proxy"`
	} `json:"endpoints"`
}

type HTTPError struct{ Status int }

func (e *HTTPError) Error() string {
	return fmt.Sprintf("GHE token endpoint returned HTTP %d", e.Status)
}

func lookupHeaders(access string) http.Header {
	headers := http.Header{}
	headers.Set("Authorization", "Bearer "+access)
	headers.Set("Accept", "application/json")
	headers.Set("X-GitHub-Api-Version", "2022-11-28")
	headers.Set("User-Agent", providers.GitHubCopilotChatUserAgent())
	return headers
}

func Exchange(ctx context.Context, client *http.Client, instance, access string) (*Token, error) {
	base, err := InstanceURL(instance)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(access) == "" {
		return nil, fmt.Errorf("GHE GitHub OAuth access token is missing")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/api/v3/copilot_internal/v2/token", nil)
	if err != nil {
		return nil, err
	}
	req.Header = lookupHeaders(access)
	resp, err := Do(ctx, client, req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, &HTTPError{resp.StatusCode}
	}
	var token Token
	if err := json.UnmarshalRead(io.LimitReader(resp.Body, 1<<20), &token); err != nil {
		return nil, fmt.Errorf("decode GHE Copilot token: %w", err)
	}
	if token.Token == "" {
		return nil, fmt.Errorf("GHE Copilot exchange returned no token")
	}
	if stamp, ok := Expiry(token.ExpiresAt); !ok || !stamp.After(time.Now()) {
		return nil, fmt.Errorf("GHE Copilot exchange returned missing or expired expiry")
	}
	for _, endpoint := range []string{token.Endpoints.API, token.Endpoints.Proxy} {
		if endpoint == "" {
			continue
		}
		if _, err := EndpointBase(endpoint); err != nil {
			return nil, err
		}
		if _, err := Guard(ctx, endpoint); err != nil {
			return nil, err
		}
	}
	return &token, nil
}

func ApplyToken(psd map[string]any, token *Token) {
	psd["copilotToken"] = token.Token
	psd["copilotTokenExpiresAt"] = token.ExpiresAt
	if token.Endpoints.API != "" {
		psd["copilotApiUrl"] = token.Endpoints.API
	}
	if token.Endpoints.Proxy != "" {
		psd["copilotProxyUrl"] = token.Endpoints.Proxy
	}
}

type OAuthToken struct {
	Access    string `json:"access_token"`
	Refresh   string `json:"refresh_token"`
	ExpiresIn int    `json:"expires_in"`
	Error     string `json:"error"`
}

func RefreshOAuth(ctx context.Context, client *http.Client, instance, refresh string) (*OAuthToken, error) {
	base, err := InstanceURL(instance)
	if err != nil {
		return nil, err
	}
	form := url.Values{"grant_type": {"refresh_token"}, "refresh_token": {refresh}, "client_id": {ClientID()}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/login/oauth/access_token", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := Do(ctx, client, req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, &HTTPError{resp.StatusCode}
	}
	var token OAuthToken
	if err := json.UnmarshalRead(io.LimitReader(resp.Body, 1<<20), &token); err != nil {
		return nil, fmt.Errorf("decode GHE OAuth refresh: %w", err)
	}
	if token.Access == "" || token.Error != "" {
		return nil, fmt.Errorf("GHE OAuth refresh was rejected")
	}
	if token.Refresh == "" {
		token.Refresh = refresh
	}
	return &token, nil
}

// Renew keeps OAuth grants distinct from the derived inference credential. A rotated
// OAuth pair is returned even if its subsequent internal exchange fails, so it is not lost.
func Renew(ctx context.Context, client *http.Client, data map[string]any, force bool) (bool, error) {
	psd, _ := data["providerSpecificData"].(map[string]any)
	if psd == nil {
		return false, fmt.Errorf("GHE providerSpecificData is missing")
	}
	instance, err := InstanceURL(String(psd, "gheUrl"))
	if err != nil {
		return false, err
	}
	if _, err := Guard(ctx, instance); err != nil {
		return false, err
	}
	if !force && Cached(psd, time.Now()) {
		return false, validateAPI(ctx, psd)
	}
	access := String(data, "accessToken")
	token, exchangeErr := Exchange(ctx, client, instance, access)
	if exchangeErr == nil {
		ApplyToken(psd, token)
		return true, validateAPI(ctx, psd)
	}
	if ctx.Err() != nil {
		return false, ctx.Err()
	}
	var status *HTTPError
	if !errors.As(exchangeErr, &status) || String(data, "refreshToken") == "" {
		return false, exchangeErr
	}
	grant, err := RefreshOAuth(ctx, client, instance, String(data, "refreshToken"))
	if err != nil {
		return false, fmt.Errorf("GHE OAuth fallback: %w", err)
	}
	if String(data, "apiKey") == access && access != "" {
		data["apiKey"] = grant.Access
	}
	data["accessToken"], data["refreshToken"] = grant.Access, grant.Refresh
	data["expiresAt"] = nil
	if grant.ExpiresIn > 0 {
		data["expiresAt"] = time.Now().Add(time.Duration(grant.ExpiresIn) * time.Second).UTC().Format(time.RFC3339)
	}
	token, err = Exchange(ctx, client, instance, grant.Access)
	if err != nil {
		return true, fmt.Errorf("GHE refreshed OAuth internal exchange: %w", err)
	}
	ApplyToken(psd, token)
	return true, validateAPI(ctx, psd)
}

func validateAPI(ctx context.Context, psd map[string]any) error {
	base, err := APIBase(psd)
	if err != nil {
		return err
	}
	_, err = Guard(ctx, base)
	return err
}
