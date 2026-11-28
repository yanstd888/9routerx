package executor

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"9routerx/proxy/internal/proxy"
)

const (
	devinDesktopBaseURL    = "https://server.codeium.com"
	devinDesktopAuthPath   = "/exa.auth_pb.AuthService/GetUserJwt"
	devinDesktopChatPath   = "/exa.api_server_pb.ApiServerService/GetChatMessage"
	devinDesktopModelsPath = "/exa.api_server_pb.ApiServerService/GetCliModelConfigs"
	devinDesktopFrameLimit = 16 << 20
	devinDesktopAuthLimit  = 1 << 20
)

func devinDesktopURL(ctx context.Context, base, path string) (context.Context, string, error) {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if base == "" {
		base = devinDesktopBaseURL
	}
	for _, suffix := range []string{devinDesktopChatPath, devinDesktopAuthPath, devinDesktopModelsPath} {
		base = strings.TrimSuffix(base, suffix)
	}
	u, err := url.Parse(base)
	if err != nil || u.RawQuery != "" || u.Fragment != "" || u.User != nil || u.Host == "" {
		return ctx, "", proxy.UpstreamFailure(http.StatusForbidden, "invalid Devin Desktop service URL")
	}
	endpoint := base + path
	allowLocal := proxy.AllowPrivateProviderURLs() || proxy.IsPrivateNetworkingAllowed() || proxy.IsLoopbackAllowed(ctx)
	if err := proxy.AssertDispatchURLAllowed(endpoint, allowLocal); err != nil {
		return ctx, "", proxy.UpstreamFailure(http.StatusForbidden, "Devin Desktop service URL blocked by SSRF guard")
	}
	if allowLocal {
		ctx = proxy.WithAllowLoopback(ctx)
	}
	return ctx, endpoint, nil
}

func devinDesktopClient(client *http.Client) *http.Client {
	if client == nil {
		client = http.DefaultClient
	}
	copy := *client
	// A 307/308 replays protobuf credentials even across origins. No redirects
	// are part of the native contract; preserve the transport, not this policy.
	copy.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &copy
}

func devinDesktopPost(ctx context.Context, client *http.Client, endpoint string, payload []byte, stream bool) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, devinDesktopFailure(ctx, "request", err)
	}
	req.Header.Set("Connect-Protocol-Version", "1")
	req.Header.Set("Content-Type", "application/proto")
	req.Header.Set("Accept", "*/*")
	if stream {
		req.Header.Set("Content-Type", "application/connect+proto")
		req.Header.Set("Accept", "application/connect+proto")
		req.Header.Set("Connect-Accept-Encoding", "gzip")
		req.Header.Set("User-Agent", "windsurf/3.6.27")
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, devinDesktopFailure(ctx, "connection", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_ = resp.Body.Close()
		return nil, proxy.UpstreamFailure(resp.StatusCode, fmt.Sprintf("Devin Desktop upstream returned HTTP %d", resp.StatusCode))
	}
	return resp, nil
}

func devinDesktopBounded(ctx context.Context, resp *http.Response, limit int) ([]byte, error) {
	defer func() { _ = resp.Body.Close() }()
	stop := context.AfterFunc(ctx, func() { _ = resp.Body.Close() })
	defer stop()
	if resp.ContentLength > int64(limit) {
		return nil, errors.New("payload exceeds safety limit")
	}
	payload, err := io.ReadAll(io.LimitReader(resp.Body, int64(limit)+1))
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err != nil {
		return nil, fmt.Errorf("read payload: %w", err)
	}
	if len(payload) > limit {
		return nil, errors.New("payload exceeds safety limit")
	}
	return payload, nil
}

func devinDesktopJWT(ctx context.Context, resp *http.Response) (string, error) {
	payload, err := devinDesktopBounded(ctx, resp, devinDesktopAuthLimit)
	if err != nil {
		return "", devinDesktopFailure(ctx, "authentication", err)
	}
	var jwt string
	err = devinDesktopFields(payload, func(field devinDesktopField) error {
		if field.number == 1 {
			if err := field.require(2); err != nil {
				return err
			}
			jwt = string(field.bytes)
		}
		// Field 2 (custom_api_server_url) is intentionally never followed.
		return nil
	})
	if err != nil || strings.TrimSpace(jwt) == "" {
		return "", proxy.UpstreamFailure(http.StatusBadGateway, "Devin Desktop authentication returned an invalid JWT response")
	}
	return jwt, nil
}

func devinDesktopFailure(ctx context.Context, operation string, cause error) error {
	if ctx.Err() != nil {
		return fmt.Errorf("devin desktop %s: %w", operation, ctx.Err())
	}
	if errors.Is(cause, context.Canceled) || errors.Is(cause, context.DeadlineExceeded) {
		return fmt.Errorf("devin desktop %s: %w", operation, cause)
	}
	// Neither upstream bodies nor transport/trailer messages may echo keys.
	return proxy.UpstreamFailure(http.StatusBadGateway, "Devin Desktop "+operation+" failed")
}
