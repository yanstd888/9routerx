// 9RouterX — this file is part of a port maintained by https://github.com/yanstd888/ — https://t.me/aiclub_id
package proxy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"9routerx/proxy/internal/providers"
)

var geminiDumpPaths = []string{
	"/tmp/9router-gemini-400.json",
	"/tmp/9router-ag-debug.json",
}

func callGeminiRejection(t *testing.T) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"thought signature missing"}}`))
	}))
	defer srv.Close()

	cfg := &providers.ProviderConfig{BaseURL: srv.URL}
	body := `{"model":"gemini-3.8-flash-medium","messages":[{"role":"user","content":"secret prompt"}]}`
	_, err := ForwardGemini(context.Background(), srv.Client(), cfg, "token", body, false, "", "gemini-3.8-flash-medium")
	if err == nil {
		t.Fatal("ForwardGemini() error = nil, want the 400 rejection")
	}
}

func clearGeminiDumps(t *testing.T) {
	t.Helper()
	if err := os.MkdirAll("/tmp", 0700); err != nil {
		t.Fatalf("create /tmp: %v", err)
	}
	for _, path := range geminiDumpPaths {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			t.Fatalf("remove %s: %v", path, err)
		}
	}
	t.Cleanup(func() {
		for _, path := range geminiDumpPaths {
			_ = os.Remove(path)
		}
	})
}

func TestForwardGemini_DebugDumpRequiresOptIn(t *testing.T) {
	t.Setenv("9RX_DEBUG_DUMP", "")
	clearGeminiDumps(t)
	callGeminiRejection(t)

	for _, path := range geminiDumpPaths {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("%s was created without 9RX_DEBUG_DUMP=1 (stat err %v)", path, err)
		}
	}
}

func TestForwardGemini_DebugDumpWithOptIn(t *testing.T) {
	t.Setenv("9RX_DEBUG_DUMP", "1")
	clearGeminiDumps(t)
	callGeminiRejection(t)

	for _, path := range geminiDumpPaths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if len(data) == 0 {
			t.Errorf("%s is empty", path)
		}
	}
}
