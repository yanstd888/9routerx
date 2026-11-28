// 9RouterX — derived from the work of https://github.com/yanstd888/ — port and modifications by the maintainers — https://t.me/aiclub_id
package dashboard

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestValidateGuardrailConfig_AcceptsCustomPatterns(t *testing.T) {
	tests := []struct {
		name    string
		config  string
		wantErr bool
	}{
		{
			name:    "valid custom pattern only",
			config:  `{"action":"block","customPatterns":[{"name":"proj","regex":"PROJ-[0-9]+"}]}`,
			wantErr: false,
		},
		{
			name:    "valid custom with custom detector in list",
			config:  `{"action":"mask","detectors":["custom"],"customPatterns":[{"name":"ssn","regex":"\\d{3}-\\d{2}-\\d{4}","replacement":"[SSN]"}]}`,
			wantErr: false,
		},
		{
			name:    "invalid regex in custom pattern",
			config:  `{"action":"block","customPatterns":[{"name":"broken","regex":"[invalid("}]}`,
			wantErr: true,
		},
		{
			name:    "empty regex in custom pattern",
			config:  `{"action":"block","customPatterns":[{"name":"empty","regex":"   "}]}`,
			wantErr: true,
		},
		{
			name:    "both detectors and custom patterns missing",
			config:  `{"action":"block"}`,
			wantErr: true,
		},
		{
			name:    "json_schema detector and pattern",
			config:  `{"action":"block","detectors":["json_schema"],"customPatterns":[{"name":"json_schema","regex":"{\"type\":\"object\",\"required\":[\"answer\"]}"}]}`,
			wantErr: false,
		},
		{
			name:    "malformed json_schema document",
			config:  `{"action":"block","customPatterns":[{"name":"json_schema","regex":"{not-json"}]}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateGuardrailConfig(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateGuardrailConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestHandleCreateGuardrailPolicy_ValidatesCustomPatterns(t *testing.T) {
	repo, cleanup := setupTestDB(t)
	t.Cleanup(cleanup)
	h := NewDashboardHandler(repo)

	// Valid policy with customPatterns
	body := `{"name":"Project Codes","scope":"global","config":"{\"action\":\"block\",\"customPatterns\":[{\"name\":\"proj\",\"regex\":\"PROJ-[0-9]+\"}]}"}`
	req := httptest.NewRequest("POST", "/api/guardrails/policies", bytes.NewReader([]byte(body)))
	rec := httptest.NewRecorder()

	h.HandleCreateGuardrailPolicy(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	// Invalid policy with broken regex
	brokenBody := `{"name":"Broken Regex","scope":"global","config":"{\"action\":\"block\",\"customPatterns\":[{\"name\":\"broken\",\"regex\":\"[unclosed\"}]}"}`
	req2 := httptest.NewRequest("POST", "/api/guardrails/policies", bytes.NewReader([]byte(brokenBody)))
	rec2 := httptest.NewRecorder()

	h.HandleCreateGuardrailPolicy(rec2, req2)
	if rec2.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", rec2.Code, rec2.Body.String())
	}
}

func TestHandleCreateGuardrailPolicy_MergesTopLevelSchemaPattern(t *testing.T) {
	repo, cleanup := setupTestDB(t)
	t.Cleanup(cleanup)
	h := NewDashboardHandler(repo)

	body := `{"name":"Schema Policy","scope":"global","config":"{\"action\":\"block\",\"detectors\":[\"json_schema\"]}",` +
		`"customPatterns":[{"name":"json_schema","regex":"{\"type\":\"object\",\"required\":[\"answer\"]}"}]}`
	rec := httptest.NewRecorder()
	h.HandleCreateGuardrailPolicy(rec, httptest.NewRequest(http.MethodPost, "/api/guardrails/policies", bytes.NewReader([]byte(body))))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var created struct {
		ID     string `json:"id"`
		Config string `json:"config"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode policy: %v", err)
	}
	if created.ID == "" {
		t.Fatal("created policy has no id")
	}
	if !strings.Contains(created.Config, "json_schema") {
		t.Fatalf("config = %q, want the top-level schema pattern persisted", created.Config)
	}

	stored, err := repo.GetGuardrailPolicy(created.ID)
	if err != nil {
		t.Fatalf("read back policy: %v", err)
	}
	if stored == nil || !strings.Contains(stored.Config, "json_schema") {
		t.Fatalf("stored config = %+v, want the schema pattern", stored)
	}
}

func TestHandleCreateGuardrailPolicy_RejectsMalformedSchema(t *testing.T) {
	repo, cleanup := setupTestDB(t)
	t.Cleanup(cleanup)
	h := NewDashboardHandler(repo)

	body := `{"name":"Broken Schema","scope":"global","config":"{\"action\":\"block\"}",` +
		`"customPatterns":[{"name":"json_schema","regex":"{not-json"}]}`
	rec := httptest.NewRecorder()
	h.HandleCreateGuardrailPolicy(rec, httptest.NewRequest(http.MethodPost, "/api/guardrails/policies", bytes.NewReader([]byte(body))))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "JSON Schema") {
		t.Errorf("error = %q, want the schema-specific reason", rec.Body.String())
	}
}
