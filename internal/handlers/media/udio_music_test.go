package media

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"9routerx/proxy/internal/handlers/chat"
	"9routerx/proxy/internal/proxy"
)

func TestUdioHelpers(t *testing.T) {
	if got := udioGenerateURL(""); got != "https://www.udio.com/api/generate-proxy" {
		t.Fatalf("default URL = %q", got)
	}
	base := "http://127.0.0.1:1234/api/generate-proxy"
	if got := udioStatusURL(base, []string{"a", "b"}); got != "http://127.0.0.1:1234/api/songs?songIds=a%2Cb" {
		t.Fatalf("status URL = %q", got)
	}
	if got := udioResolveAudio(base, "/audio/song.mp3"); got != "http://127.0.0.1:1234/audio/song.mp3" {
		t.Fatalf("audio URL = %q", got)
	}
	if got := udioCookie(&chat.ConnectionData{ProviderSpecificData: map[string]any{"cookie": "session=abc"}}); got != "session=abc" {
		t.Fatalf("cookie = %q", got)
	}
}

func TestUdioDownloadBounded(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/audio/song.mp3" {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "audio/mpeg")
		_, _ = w.Write([]byte("udio-bytes"))
	}))
	defer server.Close()
	ctx := proxy.WithAllowLoopback(context.Background())
	data, err := udioDownload(ctx, server.Client(), server.URL+"/audio/song.mp3")
	if err != nil {
		t.Fatalf("download: %v", err)
	}
	if string(data) != "udio-bytes" {
		t.Fatalf("audio = %q", data)
	}
}

func TestUdioCookieAndBlockedURL(t *testing.T) {
	if got := udioCookie(nil); got != "" {
		t.Fatalf("nil cookie = %q", got)
	}
	_, err := udioGuardURL(context.Background(), "http://169.254.169.254/latest/meta-data")
	if err == nil || !strings.Contains(err.Error(), "blocked") {
		t.Fatalf("metadata endpoint error = %v", err)
	}
}
