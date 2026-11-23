package chat

import (
	json "encoding/json/v2"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"9routerx/proxy/internal/db"
)

func TestMoonshotReviewDroppedThinkingDoesNotContaminate(t *testing.T) {
	capture := &upstreamCapture{}
	h := moonshotHandlerFixture(t, false, capture)
	body := `{"model":"moonshot/kimi-k3.1-pro","messages":[{"role":"assistant","content":[{"type":"thinking","thinking":"first turn secret"}]},{"role":"user","content":"next"},{"role":"assistant","content":[{"type":"text","text":"unrelated answer"}]},{"role":"user","content":"continue"}]}`
	r := httptest.NewRecorder()
	h.HandleMessages(r, httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body)))
	var sent map[string]any
	if err := json.Unmarshal([]byte(capture.got()), &sent); err != nil {
		t.Fatal(err)
	}
	for _, raw := range sent["messages"].([]any) {
		m := raw.(map[string]any)
		if m["content"] == "unrelated answer" && m["reasoning_content"] != nil {
			t.Fatalf("reasoning attached to unrelated turn: %+v", m)
		}
	}
}

func TestMoonshotReviewNativeEffortPrecedence(t *testing.T) {
	for _, fields := range []string{
		`"reasoning":{"effort":"none"},"thinking":{"type":"enabled","effort":"high"}`,
		`"reasoning":{"effort":null},"thinking":{"type":"enabled","effort":"high"}`,
		`"reasoning":{"effort":123},"thinking":{"type":"enabled","effort":"high"}`,
		`"reasoning":{"effort":{}},"thinking":{"type":"enabled","effort":"high"}`,
	} {
		t.Run(fields, func(t *testing.T) {
			capture := &upstreamCapture{}
			h := moonshotHandlerFixture(t, false, capture)
			body := `{"model":"moonshot/kimi-k3.1-pro",` + fields + `,"messages":[{"role":"user","content":"hello"}]}`
			r := httptest.NewRecorder()
			h.HandleMessages(r, httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body)))
			var source, sent map[string]any
			if err := json.Unmarshal([]byte(body), &source); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal([]byte(capture.got()), &sent); err != nil {
				t.Fatal(err)
			}
			want, _ := json.Marshal(source["reasoning"].(map[string]any)["effort"])
			got, _ := json.Marshal(sent["reasoning_effort"])
			if string(got) != string(want) {
				t.Fatalf("effort precedence: got %s want %s", got, want)
			}
		})
	}
}

func TestMoonshotReviewOutputConfigRawValues(t *testing.T) {
	for _, effort := range []string{"null", "123", "{}"} {
		t.Run(effort, func(t *testing.T) {
			capture := &upstreamCapture{}
			h := moonshotHandlerFixture(t, false, capture)
			body := `{"model":"moonshot/kimi-k3.1-pro","output_config":{"effort":` + effort + `},"messages":[{"role":"user","content":"hello"}]}`
			r := httptest.NewRecorder()
			h.HandleMessages(r, httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body)))
			if r.Code != http.StatusOK || !strings.Contains(capture.got(), `"reasoning_effort":`+effort) {
				t.Fatalf("typed converter swallowed raw effort: %d %s", r.Code, capture.got())
			}
		})
	}
}

func TestMoonshotReviewComboRestoresActualDispatchOnly(t *testing.T) {
	for _, format := range []string{"claude", "responses"} {
		for _, effort := range []string{"null", `"mystery"`, "{}"} {
			t.Run(format+"/"+effort, func(t *testing.T) {
				first, second := &upstreamCapture{}, &upstreamCapture{}
				other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					raw, err := io.ReadAll(r.Body)
					if err != nil {
						t.Error(err)
						return
					}
					first.record(raw)
					w.WriteHeader(http.StatusNotFound)
					if _, err := io.WriteString(w, `{"error":{"message":"fixture model unavailable"}}`); err != nil {
						t.Error(err)
					}
				}))
				defer other.Close()
				moonshot := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					raw, err := io.ReadAll(r.Body)
					if err != nil {
						t.Error(err)
						return
					}
					second.record(raw)
					w.WriteHeader(http.StatusBadRequest)
					if _, err := io.WriteString(w, `{"error":{"message":"invalid Moonshot effort"}}`); err != nil {
						t.Error(err)
					}
				}))
				defer moonshot.Close()
				database, cleanup := setupChatTestDB(t)
				defer cleanup()
				if _, err := database.Exec(`DELETE FROM providerConnections`); err != nil {
					t.Fatal(err)
				}
				seedConnDB(t, database, "groq", "review-other", "fixture", other.URL)
				seedConnDB(t, database, "moonshot", "review-moonshot", "fixture", moonshot.URL)
				repo := db.NewRepo(database)
				if err := repo.CreateCombo("review-combo", "review-combo", "llm", `["groq/llama-3.3-70b-versatile","moonshot/kimi-k3.1-pro"]`, "fallback"); err != nil {
					t.Fatal(err)
				}
				h := NewChatHandler(repo)
				body := `{"model":"review-combo","thinking":{"type":"enabled","effort":` + effort + `},"messages":[{"role":"user","content":"hello"}]}`
				path := "/v1/messages"
				if format == "responses" {
					path = "/v1/responses"
					body = `{"model":"review-combo","reasoning":{"effort":` + effort + `},"input":"hello"}`
				}
				r := httptest.NewRecorder()
				request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
				if format == "claude" {
					h.HandleMessages(r, request)
				} else {
					h.HandleResponses(r, request)
				}
				if first.got() == "" {
					t.Fatal("first provider was not tried")
				}
				if !strings.Contains(second.got(), `"reasoning_effort":`+effort) {
					t.Fatalf("Moonshot actual dispatch lost effort: %s", second.got())
				}
				if strings.Contains(first.got(), `"reasoning_effort":`+effort) || strings.Contains(first.got(), "_moonshot") {
					t.Fatalf("raw Moonshot intent leaked into sibling: %s", first.got())
				}
			})
		}
	}
}
