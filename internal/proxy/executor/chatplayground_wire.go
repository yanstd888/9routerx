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

const chatplaygroundUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"

func chatplaygroundGuard(ctx context.Context, raw string) (context.Context, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return ctx, proxy.UpstreamFailure(http.StatusBadRequest, "ChatPlayground endpoint has an invalid URL")
	}
	allowLocal := proxy.AllowPrivateProviderURLs() || proxy.IsPrivateNetworkingAllowed() || proxy.IsLoopbackAllowed(ctx)
	if !allowLocal && parsed.Scheme != "https" {
		return ctx, proxy.UpstreamFailure(http.StatusBadRequest, "ChatPlayground endpoint must use HTTPS")
	}
	if err := proxy.AssertDispatchURLAllowed(raw, allowLocal); err != nil {
		return ctx, proxy.UpstreamFailure(http.StatusBadRequest, "ChatPlayground endpoint blocked by SSRF guard")
	}
	if allowLocal {
		ctx = proxy.WithAllowLoopback(ctx)
	}
	return ctx, nil
}

func chatplaygroundToken(ctx context.Context, req *Request) (string, error) {
	raw := strings.TrimSpace(req.APIKey)
	if raw == "" {
		return "", fmt.Errorf("ChatPlayground Clerk credential is required")
	}
	if strings.HasPrefix(raw, "Bearer ") {
		raw = strings.TrimSpace(strings.TrimPrefix(raw, "Bearer "))
	}
	if strings.Count(raw, ".") == 2 && strings.HasPrefix(raw, "eyJ") {
		return raw, nil
	}
	cookies := chatplaygroundCookies(raw)
	clientCookie := cookies["__client"]
	session := cookies["__session"]
	if session == "" || !strings.HasPrefix(session, "eyJ") {
		return "", fmt.Errorf("provide a Clerk JWT or cookie export with __client and __session")
	}
	sid := chatplaygroundSID(session)
	if sid == "" || clientCookie == "" {
		return "", fmt.Errorf("Clerk cookie export missing __client or session id")
	}
	chatplaygroundJWTMu.Lock()
	if cached, ok := chatplaygroundJWTs[sid]; ok && time.Until(cached.expiry) > 15*time.Second {
		chatplaygroundJWTMu.Unlock()
		return cached.token, nil
	}
	chatplaygroundJWTMu.Unlock()

	authURL := chatplaygroundClerkHost + "/v1/client/sessions/" + url.PathEscape(sid) + "/tokens?__clerk_api_version=2025-11-10&_clerk_js_version=5.127.2"
	if req.ConnData != nil {
		if custom, ok := req.ConnData["authUrl"].(string); ok && strings.TrimSpace(custom) != "" {
			authURL = strings.TrimSpace(custom)
		}
	}
	guarded, err := chatplaygroundGuard(ctx, authURL)
	if err != nil {
		return "", fmt.Errorf("ChatPlayground auth endpoint blocked")
	}
	httpReq, err := http.NewRequestWithContext(guarded, http.MethodPost, authURL, strings.NewReader("organization_id=&token="))
	if err != nil {
		return "", fmt.Errorf("create Clerk auth request")
	}
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	httpReq.Header.Set("Accept", "*/*")
	httpReq.Header.Set("Origin", chatplaygroundWebOrigin)
	httpReq.Header.Set("Referer", chatplaygroundWebOrigin+"/")
	httpReq.Header.Set("User-Agent", chatplaygroundUserAgent)
	httpReq.Header.Set("Cookie", "__client="+clientCookie)
	client := req.Client
	if client == nil {
		client = http.DefaultClient
	}
	response, err := client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("Clerk token mint failed")
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil || response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", fmt.Errorf("Clerk token mint failed")
	}
	var parsed struct {
		JWT string `json:"jwt"`
	}
	if json.Unmarshal(body, &parsed) != nil || parsed.JWT == "" {
		return "", fmt.Errorf("Clerk token mint returned no jwt")
	}
	expiry := chatplaygroundJWTExpiry(parsed.JWT)
	chatplaygroundJWTMu.Lock()
	chatplaygroundJWTs[sid] = chatplaygroundJWT{token: parsed.JWT, expiry: expiry}
	chatplaygroundJWTMu.Unlock()
	return parsed.JWT, nil
}

func chatplaygroundCookies(raw string) map[string]string {
	out := map[string]string{}
	for _, pair := range strings.Split(strings.TrimPrefix(raw, "Cookie:"), ";") {
		idx := strings.IndexByte(pair, '=')
		if idx <= 0 {
			continue
		}
		out[strings.TrimSpace(pair[:idx])] = strings.TrimSpace(pair[idx+1:])
	}
	return out
}

func chatplaygroundSID(jwt string) string {
	parts := strings.Split(jwt, ".")
	if len(parts) < 2 {
		return ""
	}
	decoded, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return ""
	}
	var payload struct {
		SID string `json:"sid"`
		Sub string `json:"sub"`
	}
	if json.Unmarshal(decoded, &payload) != nil {
		return ""
	}
	if payload.SID != "" {
		return payload.SID
	}
	return payload.Sub
}

func chatplaygroundJWTExpiry(jwt string) time.Time {
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

func chatplaygroundError(status int, detail string) string {
	detail = strings.TrimSpace(detail)
	if len(detail) > 200 {
		detail = detail[:200]
	}
	if detail == "" {
		return fmt.Sprintf("ChatPlayground API error (%d)", status)
	}
	return detail
}
