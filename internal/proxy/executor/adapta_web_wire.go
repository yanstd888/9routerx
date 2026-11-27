package executor

import (
	"context"
	"encoding/base64"
	json "encoding/json/v2"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"9routerx/proxy/internal/proxy"
)

func adaptaGuard(ctx context.Context, raw string) (context.Context, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return ctx, proxy.UpstreamFailure(http.StatusBadRequest, "Adapta endpoint has an invalid URL")
	}
	allowLocal := proxy.AllowPrivateProviderURLs() || proxy.IsPrivateNetworkingAllowed() || proxy.IsLoopbackAllowed(ctx)
	if !allowLocal && parsed.Scheme != "https" {
		return ctx, proxy.UpstreamFailure(http.StatusBadRequest, "Adapta endpoint must use HTTPS")
	}
	if err := proxy.AssertDispatchURLAllowed(raw, allowLocal); err != nil {
		return ctx, proxy.UpstreamFailure(http.StatusBadRequest, "Adapta endpoint blocked by SSRF guard")
	}
	if allowLocal {
		ctx = proxy.WithAllowLoopback(ctx)
	}
	return ctx, nil
}

func adaptaBodyModel(body []byte) string {
	var parsed struct {
		Model string `json:"model"`
	}
	if json.Unmarshal(body, &parsed) != nil {
		return ""
	}
	return strings.TrimSpace(parsed.Model)
}

func adaptaText(content any) string {
	if text, ok := content.(string); ok {
		return text
	}
	parts, ok := content.([]any)
	if !ok {
		return ""
	}
	var out []string
	for _, raw := range parts {
		if part, ok := raw.(map[string]any); ok {
			if text, ok := part["text"].(string); ok {
				out = append(out, text)
			}
		}
	}
	return strings.Join(out, "\n")
}

func adaptaMessages(body []byte) ([]any, error) {
	var request struct {
		Messages []struct {
			Role    string `json:"role"`
			Content any    `json:"content"`
		} `json:"messages"`
	}
	if json.Unmarshal(body, &request) != nil {
		return nil, fmt.Errorf("decode Adapta request")
	}
	var system string
	type turn struct{ role, text string }
	var turns []turn
	for _, message := range request.Messages {
		text := strings.TrimSpace(adaptaText(message.Content))
		if text == "" {
			continue
		}
		role := message.Role
		if role == "developer" {
			role = "system"
		}
		if role == "system" {
			if system != "" {
				system += "\n"
			}
			system += text
			continue
		}
		if role == "user" || role == "assistant" {
			turns = append(turns, turn{role, text})
		}
	}
	if len(turns) == 0 {
		return nil, fmt.Errorf("no messages provided")
	}
	out := make([]any, 0, len(turns))
	injected := false
	for _, item := range turns {
		text := item.text
		if !injected && system != "" && item.role == "user" {
			text = system + "\n\n" + text
			injected = true
		}
		out = append(out, map[string]any{
			"role": item.role,
			"parts": []any{map[string]any{"type": "text", "text": text}},
		})
	}
	return out, nil
}

func adaptaClientJWT(raw string) string {
	value := strings.TrimSpace(raw)
	if idx := strings.IndexByte(value, '='); idx > 0 && !strings.HasPrefix(value, "eyJ") {
		return strings.TrimSpace(value[idx+1:])
	}
	return value
}

func adaptaSessionJWT(ctx context.Context, req *Request) (string, error) {
	clientJWT := adaptaClientJWT(req.APIKey)
	if clientJWT == "" {
		return "", fmt.Errorf("Adapta __client cookie value is required")
	}
	key := clientJWT
	if len(key) > 32 {
		key = key[:32]
	}
	adaptaJWTMu.Lock()
	if cached, ok := adaptaJWTs[key]; ok && time.Until(cached.expiry) > 30*time.Second {
		adaptaJWTMu.Unlock()
		return cached.token, nil
	}
	adaptaJWTMu.Unlock()

	clerkURL := adaptaDefaultClerkURL
	if req.ConnData != nil {
		if custom, ok := req.ConnData["clerkUrl"].(string); ok && strings.TrimSpace(custom) != "" {
			clerkURL = strings.TrimRight(strings.TrimSpace(custom), "/")
		}
	}
	client := req.Client
	if client == nil {
		client = http.DefaultClient
	}
	sessionID, err := adaptaSessionID(ctx, client, clerkURL, clientJWT)
	if err != nil {
		return "", err
	}
	guarded, err := adaptaGuard(ctx, clerkURL)
	if err != nil {
		return "", fmt.Errorf("Adapta auth endpoint blocked")
	}
	httpReq, err := http.NewRequestWithContext(guarded, http.MethodPost, clerkURL+"/v1/client/sessions/"+url.PathEscape(sessionID)+"/tokens", nil)
	if err != nil {
		return "", fmt.Errorf("create Adapta token request")
	}
	httpReq.Header.Set("Cookie", "__client="+clientJWT)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("User-Agent", adaptaUserAgent)
	httpReq.Header.Set("Origin", adaptaDefaultBaseURL)
	response, err := client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("Adapta token refresh failed")
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil || response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", fmt.Errorf("Adapta token refresh failed")
	}
	var parsed struct {
		JWT string `json:"jwt"`
	}
	if json.Unmarshal(raw, &parsed) != nil || !strings.HasPrefix(parsed.JWT, "eyJ") {
		return "", fmt.Errorf("Adapta token refresh returned no jwt")
	}
	expiry := adaptaExpiry(parsed.JWT)
	adaptaJWTMu.Lock()
	adaptaJWTs[key] = adaptaJWT{token: parsed.JWT, expiry: expiry}
	adaptaJWTMu.Unlock()
	return parsed.JWT, nil
}

func adaptaSessionID(ctx context.Context, client *http.Client, clerkURL, clientJWT string) (string, error) {
	guarded, err := adaptaGuard(ctx, clerkURL)
	if err != nil {
		return "", fmt.Errorf("Adapta auth endpoint blocked")
	}
	request, err := http.NewRequestWithContext(guarded, http.MethodGet, clerkURL+"/v1/client", nil)
	if err != nil {
		return "", fmt.Errorf("create Adapta client request")
	}
	request.Header.Set("Cookie", "__client="+clientJWT)
	request.Header.Set("User-Agent", adaptaUserAgent)
	request.Header.Set("Origin", adaptaDefaultBaseURL)
	response, err := client.Do(request)
	if err != nil {
		return "", fmt.Errorf("Adapta client lookup failed")
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil || response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", fmt.Errorf("Adapta client lookup failed")
	}
	var parsed struct {
		Response struct {
			Sessions []struct {
				ID     string `json:"id"`
				Status string `json:"status"`
			} `json:"sessions"`
		} `json:"response"`
	}
	if json.Unmarshal(raw, &parsed) != nil {
		return "", fmt.Errorf("Adapta client lookup invalid")
	}
	for _, session := range parsed.Response.Sessions {
		if session.Status == "active" && session.ID != "" {
			return session.ID, nil
		}
	}
	return "", fmt.Errorf("no active Adapta Clerk session")
}

func adaptaExpiry(jwt string) time.Time {
	parts := strings.Split(jwt, ".")
	if len(parts) < 2 {
		return time.Now().Add(time.Minute)
	}
	decoded, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return time.Now().Add(time.Minute)
	}
	var payload struct {
		Exp float64 `json:"exp"`
	}
	if json.Unmarshal(decoded, &payload) == nil && payload.Exp > 0 {
		return time.Unix(int64(payload.Exp), 0)
	}
	return time.Now().Add(time.Minute)
}
