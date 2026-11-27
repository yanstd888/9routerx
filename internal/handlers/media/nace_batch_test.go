// 9RouterX — port and modifications by https://github.com/yanstd888/ — https://t.me/aiclub_id
package media

import (
	json "encoding/json/v2"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"9routerx/proxy/internal/db"
)

func TestNaceSystemoneWireContract(t *testing.T) {
	const response = `{"answers":{"urgency":{"type":"score","score":0.4,"confidence":0.9,"legend":["low","high"]}}}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/systemone" {
			t.Errorf("request = %s %s, want POST /v1/systemone", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer fixture-key" {
			t.Errorf("Authorization = %q", got)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		var payload struct {
			Model     string         `json:"model"`
			State     string         `json:"state"`
			Images    []string       `json:"images"`
			Questions map[string]any `json:"questions"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatal(err)
		}
		if payload.Model != "drex-max" || payload.State != "fixture state" || len(payload.Images) != 1 || payload.Questions["urgency"] == nil {
			t.Errorf("payload changed: %s", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, response)
	}))
	defer server.Close()
	database, cleanup := setupEmbeddingsTestDB(t)
	defer cleanup()
	repo := db.NewRepo(database)
	priority := 1
	if err := repo.CreateProviderConnectionFull("nace-fixture", "nace", "apikey", "Nace fixture", &priority, `{"apiKey":"fixture-key","baseUrl":"`+server.URL+`"}`); err != nil {
		t.Fatal(err)
	}
	handler := newTestMediaHandler(repo)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/systemone", strings.NewReader(`{"model":"drex/drex-max","state":"fixture state","images":["data:image/png;base64,fixture"],"questions":{"urgency":{"type":"score"}}}`))
	handler.HandleSystemone(rec, req)
	if rec.Code != http.StatusOK || rec.Body.String() != response {
		t.Fatalf("response = %d %s, want unchanged upstream JSON", rec.Code, rec.Body.String())
	}
}
