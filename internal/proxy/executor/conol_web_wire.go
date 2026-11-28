package executor

import (
	"context"
	json "encoding/json/v2"
	"net/http"
	"net/url"
	"strings"

	"9routerx/proxy/internal/proxy"
)

const conolUserAgentValue = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 Chrome/120.0.0.0 Safari/537.36"

func conolGuard(ctx context.Context, raw string) (context.Context, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return ctx, proxy.UpstreamFailure(http.StatusBadRequest, "Conol endpoint has an invalid URL")
	}
	allowLocal := proxy.AllowPrivateProviderURLs() || proxy.IsPrivateNetworkingAllowed() || proxy.IsLoopbackAllowed(ctx)
	if !allowLocal && parsed.Scheme != "https" {
		return ctx, proxy.UpstreamFailure(http.StatusBadRequest, "Conol endpoint must use HTTPS")
	}
	if err := proxy.AssertDispatchURLAllowed(raw, allowLocal); err != nil {
		return ctx, proxy.UpstreamFailure(http.StatusBadRequest, "Conol endpoint blocked by SSRF guard")
	}
	if allowLocal {
		ctx = proxy.WithAllowLoopback(ctx)
	}
	return ctx, nil
}

func conolCookie(raw string) string {
	cookie := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(raw), "Cookie:"))
	if strings.HasPrefix(cookie, "{") {
		var parsed map[string]any
		if json.Unmarshal([]byte(cookie), &parsed) == nil {
			for _, key := range []string{"cookie", "__Secure-better-auth.session_token", "sessionToken"} {
				if value, ok := parsed[key].(string); ok && strings.TrimSpace(value) != "" {
					cookie = strings.TrimSpace(value)
					break
				}
			}
		}
	}
	if cookie == "" {
		return ""
	}
	if strings.Contains(cookie, "=") {
		return cookie
	}
	return "__Secure-better-auth.session_token=" + cookie
}

func conolHeaders(cookie, sessionID, contentType string) map[string]string {
	referer := conolWebOrigin + "/home"
	if sessionID != "" {
		referer += "?chat_session=" + url.QueryEscape(sessionID)
	}
	headers := map[string]string{
		"accept": "application/json", "accept-language": "en-US,en;q=0.9",
		"cookie": cookie, "origin": conolWebOrigin, "referer": referer,
		"user-agent": conolUserAgentValue,
	}
	if contentType != "" {
		headers["content-type"] = contentType
	}
	return headers
}

func conolText(content any) string {
	switch value := content.(type) {
	case string:
		return value
	case map[string]any:
		for _, key := range []string{"text", "content", "message"} {
			if nested, ok := value[key]; ok {
				if text := conolText(nested); text != "" {
					return text
				}
			}
		}
	case []any:
		var parts []string
		for _, raw := range value {
			if text := conolText(raw); text != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, "\n")
	}
	return ""
}

func conolPrompt(body []byte) string {
	var request struct {
		Messages []struct {
			Role    string `json:"role"`
			Content any    `json:"content"`
		} `json:"messages"`
	}
	if json.Unmarshal(body, &request) != nil {
		return ""
	}
	for i := len(request.Messages) - 1; i >= 0; i-- {
		if strings.EqualFold(request.Messages[i].Role, "user") {
			return strings.TrimSpace(conolText(request.Messages[i].Content))
		}
	}
	return ""
}

var conolEffortOrder = []string{"minimal", "low", "medium", "high", "xhigh"}

func conolModel(requested string, body []byte) (string, string) {
	raw := strings.TrimSpace(requested)
	if raw == "" {
		var parsed struct {
			Model string `json:"model"`
		}
		if json.Unmarshal(body, &parsed) == nil {
			raw = strings.TrimSpace(parsed.Model)
		}
	}
	for _, prefix := range []string{"conol-web/", "conol/", "cnl/"} {
		raw = strings.TrimPrefix(raw, prefix)
	}
	effort := "xhigh"
	for _, suffix := range conolEffortOrder {
		marker := "-" + suffix
		if strings.HasSuffix(raw, marker) {
			raw = strings.TrimSuffix(raw, marker)
			effort = suffix
			break
		}
	}
	if raw == "" {
		raw = "claude-sonnet-5"
	}
	for _, model := range conolModels {
		if model.ID != raw {
			continue
		}
		if !conolHasEffort(model.Efforts, effort) {
			effort = conolClampEffort(model.Efforts, effort)
		}
		return raw, effort
	}
	return raw, ""
}

func conolHasEffort(supported []string, effort string) bool {
	for _, value := range supported {
		if value == effort {
			return true
		}
	}
	return false
}

func conolClampEffort(supported []string, requested string) string {
	var rank int
	for i, value := range conolEffortOrder {
		if value == requested {
			rank = i
			break
		}
	}
	best := ""
	bestRank := -1
	for _, value := range supported {
		for i, ordered := range conolEffortOrder {
			if value == ordered && i <= rank && i > bestRank {
				best, bestRank = value, i
			}
		}
	}
	if best == "" && len(supported) > 0 {
		best = supported[0]
	}
	return best
}
