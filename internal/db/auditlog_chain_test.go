// 9RouterX — this file is part of a port maintained by https://github.com/yanstd888/ — https://t.me/aiclub_id
package db

import (
	"context"
	"database/sql"
	"testing"
)

// TestAuditChainIntact pins the happy path: sequential entries verify clean.
func TestAuditChainIntact(t *testing.T) {
	repo := openEphemeralSchemaDB(t)
	if err := EnsureAdditiveColumns(repo.RawDB()); err != nil {
		t.Fatalf("ensure columns: %v", err)
	}
	ctx := context.Background()
	// Clear any bootstrap audit entries so the chain starts fresh.
	if _, err := repo.RawDB().Exec(`DELETE FROM auditLog`); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		entry := AuditLogEntry{
			Actor:  "test",
			Action: "write",
			Target: "key-" + string(rune('a'+i)),
		}
		if err := repo.RecordAuditChainEntry(ctx, entry); err != nil {
			t.Fatalf("entry %d: %v", i, err)
		}
	}
	status, err := repo.VerifyAuditChain(ctx)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if !status.Intact {
		t.Errorf("chain broken at %s (id %d)", status.BrokenAt, status.BrokenID)
	}
	if status.Total < 5 {
		t.Errorf("total = %d, want >= 5", status.Total)
	}
}

// TestAuditChainTamperDetection pins the security contract: a direct SQL
// update that changes an entry's action breaks the chain at that row.
func TestAuditChainTamperDetection(t *testing.T) {
	repo := openEphemeralSchemaDB(t)
	if err := EnsureAdditiveColumns(repo.RawDB()); err != nil {
		t.Fatalf("ensure columns: %v", err)
	}
	ctx := context.Background()
	if _, err := repo.RawDB().Exec(`DELETE FROM auditLog`); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		entry := AuditLogEntry{Actor: "test", Action: "write"}
		if err := repo.RecordAuditChainEntry(ctx, entry); err != nil {
			t.Fatalf("entry %d: %v", i, err)
		}
	}
	// Tamper with entry 2 (the middle one).
	if _, err := repo.RawDB().Exec(`UPDATE auditLog SET action = 'tampered' WHERE id = 2`); err != nil {
		t.Fatal(err)
	}
	status, err := repo.VerifyAuditChain(ctx)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if status.Intact {
		t.Error("tampered chain verified as intact")
	}
	if status.BrokenID == 0 {
		t.Error("broken location not reported")
	}
}

func TestEnsureAuditChainColumnsIsIdempotent(t *testing.T) {
	repo := openEphemeralSchemaDB(t)
	for i := 0; i < 2; i++ {
		if err := EnsureAuditChainColumns(repo.RawDB()); err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
	}

	rows, err := repo.RawDB().Query(`PRAGMA table_info(auditLog)`)
	if err != nil {
		t.Fatalf("table info: %v", err)
	}
	defer rows.Close()
	found := map[string]bool{}
	for rows.Next() {
		var cid, notNull, pk int
		var name, colType string
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dflt, &pk); err != nil {
			t.Fatalf("scan column: %v", err)
		}
		found[name] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read table info: %v", err)
	}
	if !found["prevHash"] || !found["entryHash"] {
		t.Fatalf("chain columns missing: %v", found)
	}

	var indexes int
	if err := repo.RawDB().QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = 'idx_audit_prev_hash'`,
	).Scan(&indexes); err != nil {
		t.Fatalf("count index: %v", err)
	}
	if indexes != 1 {
		t.Fatalf("prevHash index count = %d, want 1", indexes)
	}
}
