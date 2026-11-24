package middleware

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"9routerx/proxy/internal/models"
)

func TestRequireRateLimitBodyRegression6MiB(t *testing.T) {
	for _, tt := range []struct {
		name          string
		contentLength int64
	}{
		{"ContentLengthMinus1", -1},
		{"ContentLengthZero", 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			payload := bytes.Repeat([]byte("abcd"), (6<<20)/4)
			copy(payload, `{"system":"`)
			copy(payload[len(payload)-2:], `"}`)
			original := &rateLimitRegressionBody{reader: bytes.NewReader(payload)}
			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			req.Body = original
			req.ContentLength = tt.contentLength
			const tpm = 2 << 20
			const wantCharge = (4 << 20) / 4
			req = req.WithContext(withAPIKey(req, &models.APIKey{
				ID: "body-regression", RateLimitTPM: new(tpm),
			}))
			rl := NewRateLimiter(time.Minute)
			called := false
			handler := RequireRateLimit(rl, nil, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				if original.bytesRead != 4<<20 {
					t.Errorf("limiter read %d bytes, want 4 MiB", original.bytesRead)
				}
				if original.closes != 0 {
					t.Errorf("limiter closed original body %d times", original.closes)
				}
				if got, ok := ReservedTokensFromContext(r.Context()); !ok || got != wantCharge {
					t.Errorf("reservation = %d, present = %v, want %d", got, ok, wantCharge)
				}
				assertRateLimitRegressionBody(t, r.Body, payload, nil)
				if err := r.Body.Close(); err != nil {
					t.Errorf("downstream Close() = %v", err)
				}
				w.WriteHeader(http.StatusOK)
			}))
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if !called || rec.Code != http.StatusOK {
				t.Fatalf("downstream called = %v, status = %d, want true, 200", called, rec.Code)
			}
			if original.closes != 1 {
				t.Errorf("original Close calls = %d, want 1", original.closes)
			}
			if bucket := rl.tokens["body-regression"]; bucket == nil || bucket.tokens != tpm-wantCharge {
				t.Errorf("TPM bucket = %+v, want remaining tokens %d", bucket, tpm-wantCharge)
			}
		})
	}
}

func TestPromptTokensForRequestBodyRegressionSmallBody(t *testing.T) {
	for _, tt := range []struct {
		name          string
		payload       string
		contentLength int64
		wantTokens    int
	}{
		{"unknown length", `{"system":"hello"}`, -1, 2},
		{"zero length", `{"system":"hello"}`, 0, 2},
		{"declared length", `{"system":"hello"}`, 18, 2},
		{"empty reader", "", -1, 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			payload := []byte(tt.payload)
			original := &rateLimitRegressionBody{reader: bytes.NewReader(payload)}
			req := &http.Request{Body: original, ContentLength: tt.contentLength}
			if got := promptTokensForRequest(req); got != tt.wantTokens {
				t.Errorf("prompt tokens = %d, want %d", got, tt.wantTokens)
			}
			if original.bytesRead != len(payload) || original.closes != 0 {
				t.Errorf("scan read %d bytes and closed %d times, want %d and 0", original.bytesRead, original.closes, len(payload))
			}
			assertRateLimitRegressionBody(t, req.Body, payload, nil)
			if err := req.Body.Close(); err != nil {
				t.Errorf("Close() = %v", err)
			}
			if original.closes != 1 {
				t.Errorf("original Close calls = %d, want 1", original.closes)
			}
		})
	}
}

func TestPromptTokensForRequestBodyRegressionScanCap(t *testing.T) {
	const capBytes = 4 << 20
	if maxTokenScanBody != capBytes {
		t.Fatalf("scan cap = %d, want %d", maxTokenScanBody, capBytes)
	}
	for _, tt := range []struct {
		name      string
		size      int
		wantRead  int
		wantToken int
	}{
		{"below cap", capBytes - 1, capBytes - 1, capBytes / 4},
		{"at cap", capBytes, capBytes, capBytes / 4},
		{"above cap", capBytes + 1, capBytes, capBytes / 4},
	} {
		t.Run(tt.name, func(t *testing.T) {
			payload := bytes.Repeat([]byte("x"), tt.size)
			original := &rateLimitRegressionBody{reader: bytes.NewReader(payload)}
			req := &http.Request{Body: original, ContentLength: -1}
			if got := promptTokensForRequest(req); got != tt.wantToken {
				t.Errorf("prompt tokens = %d, want %d", got, tt.wantToken)
			}
			if original.bytesRead != tt.wantRead || original.closes != 0 {
				t.Errorf("scan read %d bytes and closed %d times, want %d and 0", original.bytesRead, original.closes, tt.wantRead)
			}
			assertRateLimitRegressionBody(t, req.Body, payload, nil)
			if err := req.Body.Close(); err != nil {
				t.Errorf("Close() = %v", err)
			}
		})
	}
}

func TestPromptTokensForRequestBodyRegressionDeclaredOversizeNoRead(t *testing.T) {
	for _, tt := range []struct {
		name string
		size int64
	}{
		{"above cap", (4 << 20) + 1},
		{"6 MiB", 6 << 20},
	} {
		t.Run(tt.name, func(t *testing.T) {
			payload := []byte("unread body")
			original := &rateLimitRegressionBody{reader: bytes.NewReader(payload)}
			req := &http.Request{Body: original, ContentLength: tt.size}
			if got, want := promptTokensForRequest(req), int(tt.size/4)+1; got != want {
				t.Errorf("prompt tokens = %d, want %d", got, want)
			}
			if original.reads != 0 || original.closes != 0 || req.Body != original {
				t.Errorf("oversize body changed: reads = %d, closes = %d, same body = %v", original.reads, original.closes, req.Body == original)
			}
			assertRateLimitRegressionBody(t, req.Body, payload, nil)
			if err := req.Body.Close(); err != nil {
				t.Errorf("Close() = %v", err)
			}
		})
	}
}

func TestPromptTokensForRequestBodyRegressionCloseAndReadErrors(t *testing.T) {
	readErr := errors.New("original body read failed")
	closeErr := errors.New("original body close failed")
	payload := bytes.Repeat([]byte("x"), 6<<20)
	original := &rateLimitRegressionBody{
		reader:   io.MultiReader(bytes.NewReader(payload), rateLimitRegressionErrorReader{err: readErr}),
		closeErr: closeErr,
	}
	req := &http.Request{Body: original, ContentLength: -1}
	if got := promptTokensForRequest(req); got != (4<<20)/4 {
		t.Errorf("prompt tokens = %d, want %d", got, (4<<20)/4)
	}
	if original.bytesRead != 4<<20 || original.closes != 0 {
		t.Errorf("scan read %d bytes and closed %d times, want 4 MiB and 0", original.bytesRead, original.closes)
	}
	assertRateLimitRegressionBody(t, req.Body, payload, readErr)
	if err := req.Body.Close(); err != closeErr {
		t.Errorf("Close() = %v, want original error %v", err, closeErr)
	}
	if original.closes != 1 {
		t.Errorf("original Close calls = %d, want 1", original.closes)
	}
}

func TestPromptTokensForRequestBodyRegressionEmptyBody(t *testing.T) {
	for _, tt := range []struct {
		name string
		body io.ReadCloser
	}{
		{"nil", nil},
		{"NoBody", http.NoBody},
	} {
		t.Run(tt.name, func(t *testing.T) {
			req := &http.Request{Body: tt.body}
			if got := promptTokensForRequest(req); got != 0 {
				t.Errorf("prompt tokens = %d, want 0", got)
			}
			if req.Body != tt.body {
				t.Error("empty body was replaced")
			}
		})
	}
}

func TestPromptTokensForRequestBodyRegressionScanError(t *testing.T) {
	readErr := errors.New("scan failed")
	original := &rateLimitRegressionBody{reader: rateLimitRegressionErrorReader{err: readErr}}
	req := &http.Request{Body: original, ContentLength: -1}
	if got := promptTokensForRequest(req); got != 0 {
		t.Errorf("prompt tokens = %d, want 0", got)
	}
	if original.reads != 1 || original.closes != 0 || req.Body != original {
		t.Errorf("scan error changed body: reads = %d, closes = %d, same body = %v", original.reads, original.closes, req.Body == original)
	}
	if err := req.Body.Close(); err != nil {
		t.Errorf("Close() = %v", err)
	}
}

func assertRateLimitRegressionBody(t *testing.T, body io.Reader, want []byte, wantErr error) {
	t.Helper()
	hash := sha256.New()
	n, err := io.Copy(hash, body)
	if err != wantErr {
		t.Errorf("downstream read error = %v, want %v", err, wantErr)
	}
	if n != int64(len(want)) {
		t.Errorf("downstream read %d bytes, want %d", n, len(want))
	}
	wantHash := sha256.Sum256(want)
	if !bytes.Equal(hash.Sum(nil), wantHash[:]) {
		t.Errorf("downstream SHA-256 = %x, want %x", hash.Sum(nil), wantHash)
	}
}

type rateLimitRegressionBody struct {
	reader    io.Reader
	bytesRead int
	reads     int
	closes    int
	closeErr  error
}

func (b *rateLimitRegressionBody) Read(p []byte) (int, error) {
	b.reads++
	n, err := b.reader.Read(p)
	b.bytesRead += n
	return n, err
}

func (b *rateLimitRegressionBody) Close() error {
	b.closes++
	return b.closeErr
}

type rateLimitRegressionErrorReader struct{ err error }

func (r rateLimitRegressionErrorReader) Read([]byte) (int, error) { return 0, r.err }
