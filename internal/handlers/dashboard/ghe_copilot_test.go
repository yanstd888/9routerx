package dashboard

import (
	json "encoding/json/v2"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
)

type gheCatalogTransport func(*http.Request) (*http.Response, error)

func (f gheCatalogTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestGHEModelCatalogUsesNativeAPIAndPersistsFormats(t *testing.T) {
	repo, cleanup := setupTestDB(t)
	defer cleanup()
	data, _ := json.Marshal(map[string]any{"accessToken": "oauth-fixture", "apiKey": "independent", "alias": "mine", "providerSpecificData": map[string]any{"gheUrl": "https://enterprise.example", "copilotToken": "cached-derived", "copilotTokenExpiresAt": time.Now().Add(time.Hour).Unix(), "copilotApiUrl": "https://api.enterprise.example/responses", "copilotProxyUrl": "https://nes.enterprise.example"}})
	if _, err := repo.RawDB().Exec(`INSERT INTO providerConnections (id,provider,authType,isActive,data,createdAt,updatedAt) VALUES('ghe-catalog','ghe-copilot','oauth',1,?,'2026-01-01','2026-01-01')`, string(data)); err != nil {
		t.Fatal(err)
	}
	calls := 0
	client := &http.Client{Transport: gheCatalogTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.String() != "https://api.enterprise.example/models" || r.Header.Get("Authorization") != "Bearer cached-derived" {
			t.Fatalf("wrong catalog request %s %v", r.URL, r.Header)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"data":[{"id":"claude-discovered","capabilities":{"type":"chat"},"supported_endpoints":["/v1/messages","/chat/completions"],"policy":{"state":"enabled"}},{"id":"gpt-discovered","capabilities":{"type":"chat"},"supported_endpoints":[{"endpoint":"/responses"}]},{"id":"nes","capabilities":{"type":"completion"},"policy":{"state":"enabled"}},{"id":"blocked","capabilities":{"type":"chat"},"policy":{"state":"disabled"}}]}`))}, nil
	})}
	h := NewDashboardHandler(repo)
	router := chi.NewRouter()
	router.Get("/api/providers/{id}/models", h.HandleGetConnectionModels)
	req := httptest.NewRequest(http.MethodGet, "/api/providers/ghe-catalog/models", nil)
	req = req.WithContext(withProbeClient(req.Context(), client))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != 200 || calls != 1 {
		t.Fatalf("catalog=%d calls=%d %s", rec.Code, calls, rec.Body)
	}
	var out struct {
		Models []CopilotModel `json:"models"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if len(out.Models) != 2 {
		t.Fatalf("models=%v", out.Models)
	}
	conn, _ := repo.GetProviderConnectionByID("ghe-catalog")
	var saved map[string]any
	_ = json.Unmarshal([]byte(conn.Data), &saved)
	psd, _ := saved["providerSpecificData"].(map[string]any)
	formats, _ := psd["gheModelFormats"].(map[string]any)
	if formats["claude-discovered"] != "claude" || formats["gpt-discovered"] != "openai-responses" || saved["apiKey"] != "independent" || psd["copilotToken"] != "cached-derived" {
		t.Fatalf("saved=%v", saved)
	}
	if strings.Contains(rec.Body.String(), "oauth-fixture") || strings.Contains(rec.Body.String(), "cached-derived") {
		t.Fatal("catalog leaked credentials")
	}
}

func TestGHECatalogRejectInvalidJSON(t *testing.T) {
	if _, _, err := parseGHECatalog([]byte("not JSON")); err == nil {
		t.Fatal("accepted invalid catalog")
	}
}
