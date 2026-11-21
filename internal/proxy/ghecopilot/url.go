package ghecopilot

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"9routerx/proxy/internal/proxy"
)

const Provider = "ghe-copilot"

// InstanceURL is the Enterprise origin, never a chat endpoint or a URL carrying credentials.
func InstanceURL(raw string) (string, error) {
	u, err := parseHTTPS(raw)
	if err != nil {
		return "", err
	}
	if strings.Trim(u.EscapedPath(), "/") != "" {
		return "", fmt.Errorf("GHE instance URL must not contain a path")
	}
	u.Path, u.RawPath = "", ""
	return u.String(), nil
}

func parseHTTPS(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u == nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.Contains(u.Hostname(), "%") {
		return nil, fmt.Errorf("GHE URL must be HTTPS without credentials, query, or fragment")
	}
	return u, nil
}

// APIBase prefers the inference API. The proxy endpoint is only a legacy fallback.
func APIBase(psd map[string]any) (string, error) {
	raw := String(psd, "copilotApiUrl")
	if raw == "" {
		raw = String(psd, "copilotProxyUrl")
	}
	if raw == "" {
		return InstanceURL(String(psd, "gheUrl"))
	}
	return EndpointBase(raw)
}

func EndpointBase(raw string) (string, error) {
	u, err := parseHTTPS(raw)
	if err != nil {
		return "", err
	}
	u.Path = strings.TrimRight(u.Path, "/")
	for _, suffix := range []string{"/v1/messages", "/chat/completions", "/responses", "/models"} {
		u.Path = strings.TrimSuffix(u.Path, suffix)
	}
	u.RawPath = ""
	return strings.TrimRight(u.String(), "/"), nil
}

func String(m map[string]any, key string) string {
	s, _ := m[key].(string)
	return strings.TrimSpace(s)
}

// Guard also marks permitted loopback dials; DNS rebinding remains the safe dialer's responsibility.
func Guard(ctx context.Context, raw string) (context.Context, error) {
	if _, err := parseHTTPS(raw); err != nil {
		return ctx, err
	}
	allow := proxy.AllowPrivateProviderURLs() || proxy.IsPrivateNetworkingAllowed() || proxy.IsLoopbackAllowed(ctx)
	if err := proxy.AssertDispatchURLAllowed(raw, allow); err != nil {
		return ctx, err
	}
	if allow {
		ctx = proxy.WithAllowLoopback(ctx)
	}
	return ctx, nil
}

// Client retains the connection's proxy transport, but never follows redirects or sends jar cookies.
var defaultClient = &http.Client{Transport: proxy.NewSafeTransport(), Timeout: 15 * time.Second}

func Client(client *http.Client) *http.Client {
	if client == nil {
		client = defaultClient
	}
	copy := *client
	if fallback, ok := copy.Transport.(*proxy.FallbackTransport); ok {
		copy.Transport = fallback.Base
	}
	if transport, ok := copy.Transport.(*http.Transport); ok && transport == http.DefaultTransport {
		copy.Transport = defaultClient.Transport
	}
	copy.Jar = nil
	copy.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &copy
}

func Do(ctx context.Context, client *http.Client, req *http.Request) (*http.Response, error) {
	guarded, err := Guard(ctx, req.URL.String())
	if err != nil {
		return nil, fmt.Errorf("GHE dispatch: %w", err)
	}
	req = req.Clone(guarded)
	req.Header.Del("Cookie")
	resp, err := Client(client).Do(req)
	if err != nil {
		return nil, fmt.Errorf("GHE request: %w", err)
	}
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		resp.Body.Close()
		return nil, fmt.Errorf("GHE endpoint returned a redirect (HTTP %d)", resp.StatusCode)
	}
	return resp, nil
}
