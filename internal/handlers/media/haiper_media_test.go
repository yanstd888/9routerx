package media

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"9routerx/proxy/internal/proxy"
)

func TestHaiperEndpoints(t *testing.T) {
	image, status := haiperEndpoints("https://api.haiper.ai/v1")
	if image != "https://api.haiper.ai/v1/jobs/gen2/text2image" || status != "https://api.haiper.ai/v1/jobs" {
		t.Fatalf("image endpoints = %q %q", image, status)
	}
	video, status := haiperVideoEndpoints("http://127.0.0.1:1234/v1/")
	if video != "http://127.0.0.1:1234/v1/jobs/gen2/text2video" || status != "http://127.0.0.1:1234/v1/jobs" {
		t.Fatalf("video endpoints = %q %q", video, status)
	}
}

func TestHaiperStatusAndGuard(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("HAIPER_KEY"); got != "haiper-key" {
			t.Errorf("HAIPER_KEY = %q", got)
		}
		_, _ = w.Write([]byte(`{"status":"succeeded","creation_url":"http://127.0.0.1/result.mp3"}`))
	}))
	defer server.Close()
	ctx := proxy.WithAllowLoopback(context.Background())
	status, err := haiperStatusOf(ctx, server.Client(), "haiper-key", server.URL+"/v1/jobs/job-1")
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if status.Status != "succeeded" || status.CreationURL == "" {
		t.Fatalf("status = %+v", status)
	}
	_, err = haiperGuardURL(context.Background(), "http://169.254.169.254/latest/meta-data")
	if err == nil || !strings.Contains(err.Error(), "blocked") {
		t.Fatalf("metadata endpoint error = %v", err)
	}
}
