package executor

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"9routerx/proxy/internal/providers"
)

func TestClovaPayloadReasoningAndSampling(t *testing.T) {
	body := []byte(`{"model":"HCX-007","messages":[{"role":"system","content":"be concise"},{"role":"user","content":"hi"}],"max_tokens":40000,"temperature":0.3,"top_p":0.8,"reasoning_effort":"high"}`)
	encoded, err := clovaPayload("HCX-007", body)
	if err != nil {
		t.Fatalf("payload: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(encoded, &payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got := payload["maxCompletionTokens"]; got != float64(32768) {
		t.Errorf("maxCompletionTokens = %v", got)
	}
	if _, ok := payload["maxTokens"]; ok {
		t.Error("chat maxTokens leaked to reasoning lane")
	}
	thinking, _ := payload["thinking"].(map[string]any)
	if thinking["effort"] != "high" {
		t.Errorf("thinking = %v", thinking)
	}
	if payload["temperature"] != 0.3 || payload["topP"] != 0.8 {
		t.Errorf("sampling = %v/%v", payload["temperature"], payload["topP"])
	}
}

func TestClovaPayloadVisionAndTools(t *testing.T) {
	visionBody := []byte(`{"model":"HCX-005","messages":[{"role":"user","content":[{"type":"text","text":"what"},{"type":"image_url","image_url":{"url":"https://example.test/a.png"}}]}]}`)
	encoded, err := clovaPayload("HCX-005", visionBody)
	if err != nil {
		t.Fatalf("vision payload: %v", err)
	}
	if !strings.Contains(string(encoded), `"imageUrl"`) {
		t.Fatalf("vision content = %s", encoded)
	}
	toolBody := []byte(`{"model":"HCX-DASH-002","messages":[{"role":"user","content":"hi"}],"tools":[{"type":"function","function":{"name":"lookup","description":"find","parameters":{"type":"object"}}}],"tool_choice":"auto"}`)
	encoded, err = clovaPayload("HCX-DASH-002", toolBody)
	if err != nil {
		t.Fatalf("tool payload: %v", err)
	}
	if !strings.Contains(string(encoded), `"maxTokens":1024`) || !strings.Contains(string(encoded), `"toolChoice":"auto"`) {
		t.Fatalf("tool payload = %s", encoded)
	}
}

func TestForwardClovaStudioNonStream(t *testing.T) {
	reply := "event: token\ndata: {\"message\":{\"content\":\"clova\"},\"finishReason\":null}\n\nevent: token\ndata: {\"message\":{\"content\":\"-ok\"},\"finishReason\":null}\n\nevent: result\ndata: {\"message\":{\"content\":\"clova-ok\"},\"finishReason\":\"stop\",\"usage\":{\"promptTokens\":2,\"completionTokens\":2,\"totalTokens\":4}}\n\n"
	var path, auth string
	var posted map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path, auth = r.URL.Path, r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&posted); err != nil {
			t.Errorf("decode body: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(reply))
	}))
	defer server.Close()

	req := &Request{
		Ctx: context.Background(), APIKey: "clova-test-key", Client: server.Client(),
		Body: []byte(`{"model":"HCX-DASH-002","messages":[{"role":"user","content":"hi"}]}`),
		Config: &providers.ProviderConfig{BaseURL: server.URL + "/v3/chat-completions", AuthHeader: "Authorization", AuthScheme: "bearer"},
	}
	rec := httptest.NewRecorder()
	if err := ForwardClovaStudio(rec, req); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if path != "/v3/chat-completions/HCX-DASH-002" {
		t.Fatalf("path = %q", path)
	}
	if auth != "Bearer clova-test-key" {
		t.Fatalf("authorization = %q", auth)
	}
	if _, ok := posted["stream"]; ok {
		t.Error("client stream flag leaked to native lane")
	}
	body := rec.Body.String()
	if !strings.Contains(body, "clova-ok") || strings.Count(body, "clova-ok") != 1 {
		t.Fatalf("completion = %s", body)
	}
}
