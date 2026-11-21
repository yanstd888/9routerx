package chat

import (
	json "encoding/json/v2"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"9routerx/proxy/internal/db"
	"9routerx/proxy/internal/proxy/executor"
)

type gheHandlerTransport func(*http.Request) (*http.Response, error)

func (f gheHandlerTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

const gheHandlerChatSSE = "data: {\"id\":\"fixture\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"fixture-ok\"},\"finish_reason\":null}]}\n\ndata: {\"id\":\"fixture\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":2,\"completion_tokens\":3}}\n\ndata: [DONE]\n\n"
const gheHandlerMessagesSSE = "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg-fixture\",\"type\":\"message\",\"role\":\"assistant\",\"model\":\"claude-sonnet-4.6\",\"content\":[],\"usage\":{\"input_tokens\":2}}}\n\nevent: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"fixture-ok\"}}\n\nevent: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\nevent: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":3}}\n\nevent: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"
const gheHandlerResponsesSSE = "event: response.created\ndata: {\"type\":\"response.created\",\"response\":{\"id\":\"resp-fixture\",\"object\":\"response\",\"status\":\"in_progress\"}}\n\nevent: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"fixture-ok\"}\n\nevent: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp-fixture\",\"object\":\"response\",\"status\":\"completed\",\"model\":\"gpt-5-mini\",\"output\":[{\"type\":\"message\",\"role\":\"assistant\",\"content\":[{\"type\":\"output_text\",\"text\":\"fixture-ok\"}]}],\"usage\":{\"input_tokens\":2,\"output_tokens\":3}}}\n\n"

func TestGHERealHandlerNativeIngress(t *testing.T) {
	for _, endpoint := range []string{"chat", "messages", "responses"} {
		t.Run(endpoint, func(t *testing.T) {
			database, cleanup := setupChatTestDB(t)
			defer cleanup()
			repo := db.NewRepo(database)
			psd := map[string]any{"gheUrl": "https://enterprise.example", "copilotApiUrl": "https://api.enterprise.example", "copilotProxyUrl": "https://nes.enterprise.example", "copilotToken": "cached-derived", "copilotTokenExpiresAt": time.Now().Add(time.Hour).Unix()}
			raw, _ := json.Marshal(map[string]any{"accessToken": "oauth-fixture", "apiKey": "independent-fixture", "alias": "mine", "providerSpecificData": psd})
			if _, err := database.Exec(`INSERT INTO providerConnections(id,provider,authType,isActive,data,createdAt,updatedAt) VALUES('ghe-fixture','ghe-copilot','oauth',1,?,?,?)`, string(raw), time.Now().Format(time.RFC3339), time.Now().Format(time.RFC3339)); err != nil {
				t.Fatal(err)
			}
			h := NewChatHandler(repo)
			executor.RegisterAll()
			calls := 0
			h.Client = &http.Client{Transport: gheHandlerTransport(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.URL.Host != "api.enterprise.example" || r.Header.Get("Authorization") != "Bearer cached-derived" {
					t.Fatalf("wrong inference %s %v", r.URL, r.Header)
				}
				body, _ := io.ReadAll(r.Body)
				var payload map[string]any
				if err := json.Unmarshal(body, &payload); err != nil {
					t.Fatal(err)
				}
				if payload["stream"] != true {
					t.Errorf("not forced stream %s", body)
				}
				fixture := gheHandlerChatSSE
				switch endpoint {
				case "messages":
					if r.URL.Path != "/v1/messages" || payload["system"] == nil || payload["input"] != nil || !strings.Contains(string(body), "input_schema") {
						t.Errorf("native Messages body/URL %s %s", r.URL, body)
					}
					fixture = gheHandlerMessagesSSE
				case "responses":
					if r.URL.Path != "/responses" || payload["input"] == nil || payload["messages"] != nil || payload["previous_response_id"] != "resp-previous" {
						t.Errorf("native Responses body/URL %s %s", r.URL, body)
					}
					fixture = gheHandlerResponsesSSE
				default:
					if r.URL.Path != "/chat/completions" || payload["messages"] == nil {
						t.Errorf("chat URL/body %s %s", r.URL, body)
					}
				}
				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(fixture))}, nil
			})}
			body := `{"model":"ghe-copilot/gpt-4o-mini","messages":[{"role":"user","content":"hello"}],"stream":false}`
			path := "/v1/chat/completions"
			if endpoint == "messages" {
				body = `{"model":"ghe-copilot/claude-sonnet-4.6","system":[{"type":"text","text":"fixture-system","cache_control":{"type":"ephemeral"}}],"messages":[{"role":"user","content":"hello"}],"tools":[{"name":"lookup","input_schema":{"type":"object","properties":{}}}],"max_tokens":32,"stream":false}`
				path = "/v1/messages"
			}
			if endpoint == "responses" {
				body = `{"model":"ghe-copilot/gpt-5-mini","input":[{"role":"user","content":[{"type":"input_text","text":"hello"}]}],"previous_response_id":"resp-previous","max_output_tokens":32,"stream":false}`
				path = "/v1/responses"
			}
			r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
			rec := httptest.NewRecorder()
			switch endpoint {
			case "messages":
				h.HandleMessages(rec, r)
			case "responses":
				h.HandleResponses(rec, r)
			default:
				h.HandleChatCompletions(rec, r)
			}
			if rec.Code != 200 || calls != 1 || !strings.Contains(rec.Body.String(), "fixture-ok") || rec.Header().Get("Content-Type") != "application/json" {
				t.Fatalf("handler %d calls=%d body=%s", rec.Code, calls, rec.Body)
			}
		})
	}
}

func TestGHERealPrepareRenewWithoutRefreshGrant(t *testing.T) {
	database, cleanup := setupChatTestDB(t)
	defer cleanup()
	repo := db.NewRepo(database)
	raw := `{"accessToken":"oauth-fixture","apiKey":"independent-fixture","alias":"keep","providerSpecificData":{"gheUrl":"https://enterprise.example","copilotTokenExpiresAt":0,"githubLogin":"fixture"}}`
	if _, err := database.Exec(`INSERT INTO providerConnections(id,provider,authType,isActive,data,createdAt,updatedAt) VALUES('ghe-renew','ghe-copilot','oauth',1,?,?,?)`, raw, time.Now().Format(time.RFC3339), time.Now().Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	h := NewChatHandler(repo)
	calls := 0
	h.Client = &http.Client{Transport: gheHandlerTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Path != "/api/v3/copilot_internal/v2/token" || r.Header.Get("Authorization") != "Bearer oauth-fixture" {
			t.Fatalf("wrong exchange")
		}
		body, _ := json.Marshal(map[string]any{"token": "renewed-derived", "expires_at": time.Now().Add(time.Hour).Format(time.RFC3339), "endpoints": map[string]any{"api": "https://dynamic.api.example", "proxy": "https://nes.example"}})
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(string(body)))}, nil
	})}
	for i := 0; i < 2; i++ {
		fresh, token, err := h.prepareGHE(t.Context(), "ghe-renew", &ConnectionData{AccessToken: "old"}, false, "")
		if err != nil || token != "renewed-derived" || fresh.BaseURL != "https://dynamic.api.example/chat/completions" {
			t.Fatalf("prepare token=%s data=%v err=%v", token, fresh, err)
		}
	}
	if calls != 1 {
		t.Fatalf("exchanges=%d", calls)
	}
	conn, _ := repo.GetProviderConnectionByID("ghe-renew")
	var saved map[string]any
	_ = json.Unmarshal([]byte(conn.Data), &saved)
	if saved["apiKey"] != "independent-fixture" || saved["accessToken"] != "oauth-fixture" || saved["alias"] != "keep" {
		t.Fatalf("credential clobber %v", saved)
	}
	psd, _ := saved["providerSpecificData"].(map[string]any)
	if psd["copilotToken"] != "renewed-derived" || psd["copilotApiUrl"] != "https://dynamic.api.example" || psd["githubLogin"] != "fixture" {
		t.Fatalf("persisted PSD=%v", psd)
	}
}
