package executor

import (
	"context"
	"encoding/json/v2"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"9routerx/proxy/internal/proxy"
)

func opencodeServerStatus(t *testing.T, err error) int {
	t.Helper()
	var upstream *proxy.UpstreamError
	if !errors.As(err, &upstream) {
		t.Fatalf("error %v is not an UpstreamError", err)
	}
	return upstream.StatusCode
}

func TestOpenCodeServerHealthGate(t *testing.T) {
	t.Run("v1 health is accepted and no credential is sent", func(t *testing.T) {
		f := newOpenCodeServerFixture(t)
		rec := httptest.NewRecorder()
		if err := ForwardOpenCodeServer(rec, opencodeServerRequest(f, "big-pickle", false, nil)); err != nil {
			t.Fatalf("execute: %v", err)
		}
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d", rec.Code)
		}
		for _, call := range f.calls("/global/health") {
			if call.header.Get("Authorization") != "" || call.header.Get("x-api-key") != "" {
				t.Fatalf("health request leaked a credential: %v", call.header)
			}
		}
	})
	t.Run("v2 health is rejected with 501", func(t *testing.T) {
		f := newOpenCodeServerFixture(t)
		f.version = "2.1.0"
		err := ForwardOpenCodeServer(httptest.NewRecorder(), opencodeServerRequest(f, "big-pickle", false, nil))
		if got := opencodeServerStatus(t, err); got != http.StatusNotImplemented {
			t.Fatalf("status = %d, want 501", got)
		}
	})
	t.Run("versionless server with v2 provider probe is rejected", func(t *testing.T) {
		f := newOpenCodeServerFixture(t)
		f.version = ""
		f.probe = http.StatusOK
		err := ForwardOpenCodeServer(httptest.NewRecorder(), opencodeServerRequest(f, "big-pickle", false, nil))
		if got := opencodeServerStatus(t, err); got != http.StatusNotImplemented {
			t.Fatalf("status = %d, want 501", got)
		}
		if len(f.calls("/api/provider")) != 1 {
			t.Fatalf("v2 probe calls = %d, want 1", len(f.calls("/api/provider")))
		}
	})
	t.Run("unhealthy server is rejected", func(t *testing.T) {
		f := newOpenCodeServerFixture(t)
		f.healthy = false
		err := ForwardOpenCodeServer(httptest.NewRecorder(), opencodeServerRequest(f, "big-pickle", false, nil))
		if got := opencodeServerStatus(t, err); got != http.StatusBadGateway {
			t.Fatalf("status = %d, want 502", got)
		}
	})
}

func TestOpenCodeServerCatalogAndUnknownModel(t *testing.T) {
	t.Run("bare ids resolve first provider and free cost", func(t *testing.T) {
		models := opencodeServerParseCatalog([]byte(opencodeServerFixtureCatalog))
		if models["big-pickle"].ProviderID != "opencode" {
			t.Fatalf("big-pickle provider = %q, want first provider opencode", models["big-pickle"].ProviderID)
		}
		if !models["big-pickle"].Free || !models["space-bunny-free"].Free {
			t.Fatalf("free cost was not recorded: %+v", models)
		}
	})
	t.Run("catalog is cached for 60s", func(t *testing.T) {
		f := newOpenCodeServerFixture(t)
		client := f.server.Client()
		if _, err := opencodeServerCatalog(context.Background(), client, f.server.URL); err != nil {
			t.Fatalf("catalog: %v", err)
		}
		f.catalog = []byte(`{"providers":[]}`)
		if _, err := opencodeServerCatalog(context.Background(), client, f.server.URL); err != nil {
			t.Fatalf("cached catalog: %v", err)
		}
		if got := len(f.calls("/config/providers")); got != 1 {
			t.Fatalf("catalog fetches = %d, want 1", got)
		}
	})
	t.Run("unknown model is 404", func(t *testing.T) {
		f := newOpenCodeServerFixture(t)
		err := ForwardOpenCodeServer(httptest.NewRecorder(), opencodeServerRequest(f, "not-a-model", false, nil))
		if got := opencodeServerStatus(t, err); got != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", got)
		}
	})
}

func TestOpenCodeServerEndpointPrecedence(t *testing.T) {
	f := newOpenCodeServerFixture(t)
	req := opencodeServerRequest(f, "big-pickle", false, nil)
	req.ConnData = map[string]any{"opencode_server_url": f.server.URL + "/dashboard"}
	// Dashboard value with a trailing slash must normalize to the bare root.
	if got, err := opencodeServerBaseURL(req); err != nil || got != f.server.URL+"/dashboard" {
		t.Fatalf("dashboard endpoint = %q, %v", got, err)
	}
	t.Setenv("OPENCODE_SERVER_URL", "http://example.invalid:4096")
	if got, err := opencodeServerBaseURL(req); err != nil || got != "http://example.invalid:4096" {
		t.Fatalf("env endpoint = %q, %v", got, err)
	}
	t.Setenv("OPENCODE_SERVER_URL", "")
	req.ConnData = nil
	req.Config.BaseURL = ""
	if got, err := opencodeServerBaseURL(req); err != nil || got != opencodeServerDefaultBase {
		t.Fatalf("default endpoint = %q, %v", got, err)
	}
}

func TestOpenCodeServerBlockingMessage(t *testing.T) {
	f := newOpenCodeServerFixture(t)
	var captured opencodeServerMessageBody
	f.onPrompt = func() {}
	rec := httptest.NewRecorder()
	req := opencodeServerRequest(f, "big-pickle", false, nil)
	if err := ForwardOpenCodeServer(rec, req); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	messageCalls := f.calls("/session/ses_fixture/message")
	if len(messageCalls) != 1 {
		t.Fatalf("message calls = %d, want 1", len(messageCalls))
	}
	if err := json.Unmarshal(messageCalls[0].body, &captured); err != nil {
		t.Fatalf("decode message body: %v", err)
	}
	if captured.Model["providerID"] != "opencode" || captured.Model["modelID"] != "big-pickle" {
		t.Fatalf("model = %+v", captured.Model)
	}
	if captured.System != "Be brief" || len(captured.Tools) != 0 || len(captured.Parts) != 1 {
		t.Fatalf("body = %+v", captured)
	}
	prompt, _ := captured.Parts[0]["text"].(string)
	if !strings.Contains(prompt, "[system]") || !strings.Contains(prompt, "[user]") {
		t.Fatalf("flattened prompt = %q", prompt)
	}
	for _, call := range messageCalls {
		if call.header.Get("Authorization") != "" {
			t.Fatalf("message leaked an Authorization header")
		}
	}
	if len(f.calls("/session/ses_fixture/abort")) != 1 || len(f.calls("/session/ses_fixture")) != 1 {
		t.Fatalf("cleanup was not best-effort completed")
	}
	var answer map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &answer); err != nil {
		t.Fatalf("decode answer: %v", err)
	}
	choices, _ := answer["choices"].([]any)
	if len(choices) != 1 {
		t.Fatalf("choices = %d", len(choices))
	}
	message, _ := choices[0].(map[string]any)["message"].(map[string]any)
	if message["content"] != "Hello" || message["reasoning_content"] != "Thinking" {
		t.Fatalf("message = %+v", message)
	}
	usage, _ := answer["usage"].(map[string]any)
	if usage["prompt_tokens"] != float64(16) || usage["completion_tokens"] != float64(10) {
		t.Fatalf("usage = %+v", usage)
	}
}
