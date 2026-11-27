package executor

import (
	"bytes"
	"context"
	"errors"
	"io"
	"math/rand"
	"net/http"
	"sync"
	"time"

	"9routerx/proxy/internal/constants"
	"9routerx/proxy/internal/log"
	"9routerx/proxy/internal/proxy"
)

const (
	openCodeRetryDelay          = 200 * time.Millisecond
	openCodeRefusalToken        = "FreeTierError"
	openCodeUsageLimitToken     = "FreeUsageLimitError"
	openCodeMaxOverloadAttempts = 8
	openCodeOverloadBase        = 800 * time.Millisecond
	openCodeOverloadMax         = 4 * time.Second
	openCodeOverloadJitter      = 250 * time.Millisecond
)

var openCodeRetryWaitMu sync.Mutex
var openCodeRetryWait = waitOpenCodeRetry

func setOpenCodeRetryWaitForTest(wait func(context.Context, time.Duration) bool) func() {
	openCodeRetryWaitMu.Lock()
	previous := openCodeRetryWait
	openCodeRetryWait = wait
	openCodeRetryWaitMu.Unlock()
	return func() {
		openCodeRetryWaitMu.Lock()
		openCodeRetryWait = previous
		openCodeRetryWaitMu.Unlock()
	}
}

func waitForOpenCodeRetry(ctx context.Context, delay time.Duration) bool {
	openCodeRetryWaitMu.Lock()
	wait := openCodeRetryWait
	openCodeRetryWaitMu.Unlock()
	return wait(ctx, delay)
}

type openCodeRotation struct {
	rotate func()
	expire func()
}

func waitOpenCodeRetry(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return ctx.Err() == nil
	}
}

func openCodeOverloadBackoff(retry int) time.Duration {
	var delay time.Duration
	if retry <= 3 {
		delay = openCodeOverloadBase * time.Duration(retry)
	} else {
		delay = openCodeOverloadMax
	}
	jitter := time.Duration(rand.Int63n(int64(openCodeOverloadJitter) + 1))
	return delay + jitter
}

func isOpenCodeRefusal(ue *proxy.UpstreamError) bool {
	return ue != nil && ue.StatusCode == http.StatusForbidden && bytes.Contains(ue.Body, []byte(openCodeRefusalToken))
}

func isOpencodeUsageLimit(ue *proxy.UpstreamError) bool {
	return ue != nil && ue.StatusCode == http.StatusTooManyRequests && bytes.Contains(ue.Body, []byte(openCodeUsageLimitToken))
}

func isOpenCodeOverload(ue *proxy.UpstreamError) bool {
	if ue == nil {
		return false
	}
	if ue.StatusCode >= 500 && ue.StatusCode <= 599 {
		return true
	}
	if ue.StatusCode != http.StatusTooManyRequests || isOpencodeUsageLimit(ue) {
		return false
	}
	body := bytes.ToLower(ue.Body)
	for _, token := range [][]byte{[]byte("overload"), []byte("server_busy"), []byte("capacity"), []byte("temporarily_unavailable")} {
		if bytes.Contains(body, token) {
			return true
		}
	}
	return false
}

func refusedResponseToError(resp *http.Response) *proxy.UpstreamError {
	if resp == nil || (resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusTooManyRequests) {
		return nil
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, constants.UpstreamErrLimit))
	_ = resp.Body.Close()
	return &proxy.UpstreamError{StatusCode: resp.StatusCode, Body: body}
}

func overloadResponseToError(resp *http.Response) *proxy.UpstreamError {
	if resp == nil || !((resp.StatusCode >= 500 && resp.StatusCode <= 599) || resp.StatusCode == http.StatusTooManyRequests) {
		return nil
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, constants.UpstreamErrLimit))
	_ = resp.Body.Close()
	return &proxy.UpstreamError{StatusCode: resp.StatusCode, Body: body}
}

func forwardOpenCode(ctx context.Context, send, sendDirect func() (*http.Response, error)) (*http.Response, error) {
	resp, err := send()
	if err == nil {
		if ue := refusedResponseToError(resp); ue != nil {
			err = ue
		} else if ue := overloadResponseToError(resp); ue != nil {
			err = ue
		} else {
			return resp, nil
		}
	}
	var upstream *proxy.UpstreamError
	if errors.As(err, &upstream) && isOpenCodeRefusal(upstream) {
		if proxy.StrictProxy(ctx) || !waitForOpenCodeRetry(ctx, openCodeRetryDelay) {
			return nil, upstream
		}
		directResp, directErr := sendDirect()
		if directErr != nil {
			return nil, upstream
		}
		if refused := refusedResponseToError(directResp); refused != nil {
			return nil, upstream
		}
		return directResp, nil
	}
	if !errors.As(err, &upstream) || !isOpenCodeOverload(upstream) {
		return nil, err
	}
	last := upstream
	for retry := 1; retry < openCodeMaxOverloadAttempts; retry++ {
		if !waitForOpenCodeRetry(ctx, openCodeOverloadBackoff(retry)) {
			return nil, last
		}
		resp, err = send()
		if err == nil {
			if refusal := refusedResponseToError(resp); refusal != nil && (isOpenCodeRefusal(refusal) || isOpencodeUsageLimit(refusal)) {
				return nil, refusal
			}
			if overloaded := overloadResponseToError(resp); overloaded != nil {
				if !isOpenCodeOverload(overloaded) {
					return nil, overloaded
				}
				last = overloaded
				continue
			}
			return resp, nil
		}
		if !errors.As(err, &upstream) || !isOpenCodeOverload(upstream) {
			return nil, err
		}
		last = upstream
	}
	return nil, last
}

func forwardOpenCodeRotating(ctx context.Context, send, sendDirect func() (*http.Response, error), rotate func()) (*http.Response, error) {
	return forwardOpenCodeRotatingWithExpiry(ctx, send, sendDirect, &openCodeRotation{rotate: rotate})
}

func forwardOpenCodeRotatingWithExpiry(ctx context.Context, send, sendDirect func() (*http.Response, error), rotation *openCodeRotation) (*http.Response, error) {
	resp, err := forwardOpenCode(ctx, send, sendDirect)
	if err == nil || rotation == nil || rotation.rotate == nil {
		return resp, err
	}
	var upstream *proxy.UpstreamError
	if !errors.As(err, &upstream) || !isOpencodeUsageLimit(upstream) {
		return resp, err
	}
	log.Warn("opencode", "free session budget spent, rotating session and replaying once")
	rotation.rotate()
	resp, err = forwardOpenCode(ctx, send, sendDirect)
	if err != nil && errors.As(err, &upstream) && isOpencodeUsageLimit(upstream) {
		// The freshly installed stable session was refused too. Retire it so the
		// next request mints a clean session instead of replaying a rejected id.
		if rotation.expire != nil {
			rotation.expire()
		}
		return nil, err
	}
	return resp, err
}
