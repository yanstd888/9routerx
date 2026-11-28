// 9RouterX — this file is part of a port maintained by https://github.com/yanstd888/ — https://t.me/aiclub_id
package guardrails

import (
	json "encoding/json/v2"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSchemaOnlyPolicyEnablesEngine(t *testing.T) {
	engine := NewEngineWithCustom(nil, ActionBlock, []CustomPattern{
		{Name: "json_schema", Regex: widgetSchema},
	})
	if !engine.Enabled() {
		t.Fatal("schema-only engine is disabled, so it never runs")
	}
	_, decision := engine.ScanJSON(map[string]any{"wrong": "shape"})
	if !decision.Blocked() {
		t.Fatalf("decision = %+v, want block from the schema detector", decision)
	}
}

func TestValidateJSONSchemaRejectsMalformedDocument(t *testing.T) {
	if err := ValidateJSONSchema(widgetSchema); err != nil {
		t.Fatalf("valid schema rejected: %v", err)
	}
	if err := ValidateJSONSchema("{not-json"); err == nil {
		t.Fatal("malformed schema accepted")
	}
}

func TestInboundJSONSchemaBlockNamesOffendingField(t *testing.T) {
	cfg, err := json.Marshal(map[string]any{
		"action": "block",
		"customPatterns": []map[string]string{
			{"name": "json_schema", "regex": widgetSchema},
		},
	})
	if err != nil {
		t.Fatalf("marshal policy: %v", err)
	}

	var seenBody string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		seenBody = string(b)
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions",
		strings.NewReader(`{"answer":42}`))
	rec := httptest.NewRecorder()
	Inbound(jsonStore(string(cfg)), nil, nil)(next).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "answer") {
		t.Errorf("error = %q, want the offending field name", rec.Body.String())
	}
	if seenBody != "" {
		t.Errorf("blocked body reached the downstream handler: %q", seenBody)
	}
}
