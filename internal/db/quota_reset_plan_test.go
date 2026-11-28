// 9RouterX — ported & modified by https://github.com/yanstd888/ — https://t.me/aiclub_id
package db

import (
	"strings"
	"testing"

	"9routerx/proxy/internal/handlerutil"
)

// TestTokenUsageSinceQueryPlan pins the plan of the single-key lifetime spend
// read the limiter runs on every request with an armed token quota. The
// ledger carries idx_uh_clientkey_tok and idx_uh_apikey_tok for exactly this
// shape, and SQLite can serve the OR across the two columns through both
// indexes. A full "SCAN usageHistory" here is one whole-table pass per
// request on the hot path — the shape the batch reader was fixed for, and the
// reason this read is cached at the limiter layer.
func TestTokenUsageSinceQueryPlan(t *testing.T) {
	r := openEphemeralSchemaDB(t)

	if err := r.CreateApiKey("kp", "sk-plan-key-0000000000000002", "plan key", ""); err != nil {
		t.Fatalf("CreateApiKey: %v", err)
	}
	secret := "sk-plan-key-0000000000000002"
	masked := handlerutil.MaskAPIKey(secret)

	rows, err := r.db.Query(`EXPLAIN QUERY PLAN SELECT COALESCE(SUM(COALESCE(promptTokens,0) + COALESCE(completionTokens,0)), 0)
	   FROM usageHistory WHERE (clientKey = ? OR apiKey = ? OR apiKey = ?)`,
		masked, secret, masked)
	if err != nil {
		t.Fatalf("explain query plan: %v", err)
	}
	defer func() { _ = rows.Close() }()

	var plan strings.Builder
	for rows.Next() {
		var id, parent, notused int
		var detail string
		if err := rows.Scan(&id, &parent, &notused, &detail); err != nil {
			t.Fatalf("scan plan: %v", err)
		}
		plan.WriteString(detail)
		plan.WriteString("\n")
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate plan: %v", err)
	}

	if strings.Contains(plan.String(), "SCAN usageHistory") {
		t.Errorf("the lifetime spend read full-scans usageHistory; it must be answered through the token indexes. plan:\n%s", plan.String())
	}
	if !strings.Contains(plan.String(), "idx_uh_") {
		t.Errorf("the lifetime spend read does not use a ledger token index. plan:\n%s", plan.String())
	}
}
