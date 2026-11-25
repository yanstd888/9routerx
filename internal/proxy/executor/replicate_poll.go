// 9RouterX — ported from OmniRoute open-sse/executors/replicate.ts (pin 632d84f5).
package executor

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	json "encoding/json/v2"

	"9routerx/proxy/internal/proxy"
)

const replicateMaxBodyBytes = 2 << 20

var (
	replicatePollInterval = time.Second
	replicatePollDeadline = 120 * time.Second
	replicateUsageTimeout = 5 * time.Second
)

var replicateTerminalStatuses = map[string]bool{"succeeded": true, "failed": true, "canceled": true}

var replicateHostRE = regexp.MustCompile(`(?i)(^|\.)replicate\.com$`)

// replicateHTTPError keeps the upstream status and Retry-After so downstream
// retry classification sees a 429/401 rather than a flat 502.
type replicateHTTPError struct {
	status     int
	retryAfter string
	message    string
}

func (e *replicateHTTPError) Error() string { return e.message }

func replicateSameOrigin(candidate, requestURL string) bool {
	if candidate == "" {
		return false
	}
	a, err := url.Parse(candidate)
	if err != nil || a.Host == "" {
		return false
	}
	b, err := url.Parse(requestURL)
	if err != nil || b.Host == "" {
		return false
	}
	return a.Scheme == b.Scheme && a.Host == b.Host
}

// isTrustedStreamURL admits the request's own origin and https replicate.com
// hosts. A bare prefix check would accept stream.replicate.com.evil.test.
func isTrustedStreamURL(candidate, requestURL string) bool {
	if candidate == "" {
		return false
	}
	if replicateSameOrigin(candidate, requestURL) {
		return true
	}
	parsed, err := url.Parse(candidate)
	if err != nil || parsed.Host == "" || parsed.User != nil {
		return false
	}
	return parsed.Scheme == "https" && replicateHostRE.MatchString(parsed.Hostname())
}

func replicateDo(ctx context.Context, client *http.Client, method, rawURL string, headers map[string]string, body []byte) (*http.Response, error) {
	if err := proxy.AssertDispatchURLAllowed(rawURL, proxy.AllowPrivateProviderURLs()); err != nil {
		return nil, &replicateHTTPError{status: http.StatusForbidden, message: err.Error()}
	}
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	httpReq, err := http.NewRequestWithContext(ctx, method, rawURL, reader)
	if err != nil {
		return nil, fmt.Errorf("replicate build request: %w", err)
	}
	for name, value := range headers {
		httpReq.Header.Set(name, value)
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("replicate upstream: %w", err)
	}
	return resp, nil
}

func replicateDetail(body []byte) string {
	var probe struct {
		Detail any `json:"detail"`
	}
	if err := json.Unmarshal(body, &probe); err != nil {
		return ""
	}
	if detail, ok := probe.Detail.(string); ok {
		return detail
	}
	return ""
}

// replicateUpstreamFailure reads a bounded detail and keeps status/Retry-After.
func replicateUpstreamFailure(resp *http.Response) *replicateHTTPError {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	return &replicateHTTPError{
		status:     resp.StatusCode,
		retryAfter: resp.Header.Get("Retry-After"),
		message:    replicateUpstreamMessage(resp.StatusCode, replicateDetail(body)),
	}
}

func replicateReadPrediction(resp *http.Response) (map[string]any, error) {
	body, err := io.ReadAll(io.LimitReader(resp.Body, replicateMaxBodyBytes))
	if err != nil {
		return nil, fmt.Errorf("replicate read prediction: %w", err)
	}
	var prediction map[string]any
	if err := json.Unmarshal(body, &prediction); err != nil {
		return nil, fmt.Errorf("replicate decode prediction: %w", err)
	}
	return prediction, nil
}

func replicateWithoutPrefer(headers map[string]string) map[string]string {
	copyHeaders := make(map[string]string, len(headers))
	for name, value := range headers {
		if !strings.EqualFold(name, "Prefer") {
			copyHeaders[name] = value
		}
	}
	return copyHeaders
}

func replicateWait(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// pollUntilDone follows urls.get at a bounded interval. Only same-origin poll
// URLs are followed, so the Bearer header never reaches a foreign host.
func pollReplicateUntilDone(ctx context.Context, client *http.Client, initial map[string]any, requestURL string, headers map[string]string) (map[string]any, error) {
	status, _ := initial["status"].(string)
	if replicateTerminalStatuses[status] {
		return initial, nil
	}
	getURL, _ := replicateMap(initial["urls"])["get"].(string)
	if !replicateSameOrigin(getURL, requestURL) {
		return nil, &replicateHTTPError{status: http.StatusBadGateway, message: "Replicate returned an unexpected polling URL"}
	}
	pollHeaders := replicateWithoutPrefer(headers)
	deadline := time.Now().Add(replicatePollDeadline)
	current := initial
	for {
		if err := replicateWait(ctx, replicatePollInterval); err != nil {
			return nil, err
		}
		resp, err := replicateDo(ctx, client, http.MethodGet, getURL, pollHeaders, nil)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			failure := replicateUpstreamFailure(resp)
			resp.Body.Close()
			return nil, failure
		}
		current, err = replicateReadPrediction(resp)
		resp.Body.Close()
		if err != nil {
			return nil, err
		}
		status, _ = current["status"].(string)
		if replicateTerminalStatuses[status] || time.Now().After(deadline) {
			break
		}
	}
	if !replicateTerminalStatuses[status] {
		cancelReplicatePrediction(ctx, client, current, requestURL, pollHeaders)
	}
	return current, nil
}

// cancelReplicatePrediction is best effort: it stops a prediction we are
// abandoning so a combo fallback does not duplicate the work.
func cancelReplicatePrediction(ctx context.Context, client *http.Client, prediction map[string]any, requestURL string, headers map[string]string) {
	cancelURL, _ := replicateMap(prediction["urls"])["cancel"].(string)
	if !replicateSameOrigin(cancelURL, requestURL) {
		return
	}
	resp, err := replicateDo(ctx, client, http.MethodPost, cancelURL, replicateWithoutPrefer(headers), nil)
	if err == nil {
		resp.Body.Close()
	}
}

// fetchFinalReplicatePrediction picks up Replicate's own token counts when the
// prediction already finished. A failure leaves the original prediction alone.
func fetchFinalReplicatePrediction(ctx context.Context, client *http.Client, prediction map[string]any, requestURL string, headers map[string]string) map[string]any {
	getURL, _ := replicateMap(prediction["urls"])["get"].(string)
	if !replicateSameOrigin(getURL, requestURL) {
		return prediction
	}
	lookupCtx, cancel := context.WithTimeout(ctx, replicateUsageTimeout)
	defer cancel()
	resp, err := replicateDo(lookupCtx, client, http.MethodGet, getURL, replicateWithoutPrefer(headers), nil)
	if err != nil {
		return prediction
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return prediction
	}
	final, err := replicateReadPrediction(resp)
	if err != nil {
		return prediction
	}
	return final
}
