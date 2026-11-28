package oauth

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"9routerx/proxy/internal/models"
)

type gheMemoryStore struct {
	mu       sync.Mutex
	provider string
	data     string
	writes   int
}

func (s *gheMemoryStore) GetProviderConnectionByID(id string) (*models.ProviderConnection, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return &models.ProviderConnection{ID: id, Provider: s.provider, Data: s.data}, nil
}
func (s *gheMemoryStore) UpdateConnectionData(id, raw string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data = raw
	s.writes++
	return nil
}

type gheRenewTransport func(*http.Request) (*http.Response, error)

func (f gheRenewTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestGHERefreshLockReloadAndIsolation(t *testing.T) {
	store := &gheMemoryStore{provider: "ghe-copilot", data: `{"accessToken":"oauth-fixture","apiKey":"independent","alias":"mine","providerSpecificData":{"gheUrl":"https://enterprise.example"}}`}
	var calls atomic.Int32
	client := &http.Client{Transport: gheRenewTransport(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		if r.Header.Get("Authorization") != "Bearer oauth-fixture" {
			t.Error("wrong OAuth token")
		}
		raw, _ := json.Marshal(map[string]any{"token": "fresh-derived", "expires_at": time.Now().Add(time.Hour).Unix(), "endpoints": map[string]any{"api": "https://api.enterprise.example", "proxy": "https://nes.enterprise.example"}})
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(string(raw)))}, nil
	})}
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Go(func() {
			data, err := PrepareGHEConnection(t.Context(), store, client, "ghe-lock", false, "")
			if err != nil {
				t.Error(err)
				return
			}
			psd, _ := data["providerSpecificData"].(map[string]any)
			if psd["copilotToken"] != "fresh-derived" || data["apiKey"] != "independent" || data["alias"] != "mine" {
				t.Errorf("wrong refreshed data %v", data)
			}
		})
	}
	wg.Wait()
	if calls.Load() != 1 || store.writes != 1 {
		t.Fatalf("calls=%d writes=%d", calls.Load(), store.writes)
	}
	store.provider = "github"
	if _, err := PrepareGHEConnection(t.Context(), store, client, "wrong-provider", false, ""); err == nil {
		t.Fatal("cross-provider credential accepted")
	}
	if calls.Load() != 1 {
		t.Fatal("cross-provider dispatch")
	}
}

func TestGHEWaitCancellation(t *testing.T) {
	slot := SlotFor("ghe-canceled-lock")
	slot.Mu.Lock()
	defer slot.Mu.Unlock()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	store := &gheMemoryStore{provider: "ghe-copilot", data: `{}`}
	if _, err := PrepareGHEConnection(ctx, store, nil, "ghe-canceled-lock", false, ""); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel=%v", err)
	}
	if store.writes != 0 {
		t.Fatal("canceled persisted")
	}
}

func TestGHEFallbackRotatedOAuthPersistedOnExchangeFailure(t *testing.T) {
	store := &gheMemoryStore{provider: "ghe-copilot", data: `{"accessToken":"old-oauth","apiKey":"old-oauth","refreshToken":"old-refresh","providerSpecificData":{"gheUrl":"https://enterprise.example"}}`}
	client := &http.Client{Transport: gheRenewTransport(func(r *http.Request) (*http.Response, error) {
		status, raw := 401, `{}`
		if r.URL.Path == "/login/oauth/access_token" {
			status, raw = 200, `{"access_token":"rotated-oauth","refresh_token":"rotated-refresh","expires_in":3600}`
		}
		return &http.Response{StatusCode: status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(raw))}, nil
	})}
	if _, err := PrepareGHEConnection(t.Context(), store, client, "ghe-fallback-persist", false, ""); err == nil {
		t.Fatal("expected second exchange failure")
	}
	if store.writes != 1 || !strings.Contains(store.data, "rotated-refresh") || !strings.Contains(store.data, "rotated-oauth") {
		t.Fatalf("rotated grant lost: %s writes=%d", store.data, store.writes)
	}
}
