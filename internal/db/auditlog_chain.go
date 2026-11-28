// 9RouterX — this file is part of a port maintained by https://github.com/yanstd888/ — https://t.me/aiclub_id
package db

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"
)

// Audit log hash-chain (N7): every entry carries a hash that binds it to the
// previous one. Tampering with any row (changing the action, deleting an
// entry, inserting a fake one) breaks the chain at that point — the verify
// endpoint reports the first broken link.

// AuditChainEntry is one entry with its chain hash.
type AuditChainEntry struct {
	AuditLogEntry
	EntryHash string `json:"entryHash"`
	PrevHash  string `json:"prevHash"`
}

// AuditChainEntryJSON is the canonical form hashed — a stable field order.
func auditChainCanonical(ts, actor, action, target, ip string) string {
	return ts + "|" + actor + "|" + action + "|" + target + "|" + ip
}

// auditChainHash computes sha256(prevHash + canonical) for one entry.
func auditChainHash(prevHash, ts, actor, action, target, ip string) string {
	h := sha256.Sum256([]byte(prevHash + "|" + auditChainCanonical(ts, actor, action, target, ip)))
	return hex.EncodeToString(h[:])
}

// EnsureAuditChainColumns adds prevHash and entryHash to the auditLog table.
// Additive-only; entries written before the migration have empty hashes and
// start the chain fresh. Safe to call on every boot: both the columns and the
// lookup index are created only when missing.
func EnsureAuditChainColumns(db *sql.DB) error {
	for _, col := range []struct{ name, definition string }{
		{"prevHash", "TEXT DEFAULT ''"},
		{"entryHash", "TEXT DEFAULT ''"},
	} {
		if err := addColumnIfMissing(db, "auditLog", col.name, col.definition); err != nil {
			return fmt.Errorf("audit chain: add %s: %w", col.name, err)
		}
	}
	if _, err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_audit_prev_hash ON auditLog(prevHash)`); err != nil {
		return fmt.Errorf("audit chain: create prevHash index: %w", err)
	}
	return nil
}

// RecordAuditChainEntry writes one audit entry with its chain hash, reading
// the previous entry's hash to maintain the chain.
func (r *Repo) RecordAuditChainEntry(ctx context.Context, entry AuditLogEntry) error {
	if entry.Timestamp == "" {
		entry.Timestamp = auditClock().Format(time.RFC3339Nano)
	}
	// Read the previous hash.
	var prevHash string
	row := r.db.QueryRowContext(ctx,
		`SELECT entryHash FROM auditLog ORDER BY id DESC LIMIT 1`)
	if err := row.Scan(&prevHash); err != nil && err.Error() != "sql: no rows in result set" {
		return fmt.Errorf("audit chain: read prev: %w", err)
	}
	entryHash := auditChainHash(prevHash, entry.Timestamp, entry.Actor, entry.Action, entry.Target, entry.IP)
	if _, err := r.db.ExecContext(ctx,
		`INSERT INTO auditLog (timestamp, actor, action, target, beforeJSON, afterJSON, ip, prevHash, entryHash)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		entry.Timestamp, entry.Actor, entry.Action, entry.Target,
		entry.BeforeJSON, entry.AfterJSON, entry.IP, prevHash, entryHash,
	); err != nil {
		return fmt.Errorf("audit chain: insert: %w", err)
	}
	return nil
}

// AuditChainStatus is the verify endpoint's response.
type AuditChainStatus struct {
	Total    int64  `json:"total"`
	Intact   bool   `json:"intact"`
	BrokenAt string `json:"brokenAt,omitempty"`
	BrokenID int64  `json:"brokenId,omitempty"`
	LastHash string `json:"lastHash,omitempty"`
}

// VerifyAuditChain walks the whole log and verifies every link.
func (r *Repo) VerifyAuditChain(ctx context.Context) (AuditChainStatus, error) {
	status := AuditChainStatus{}
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, timestamp, actor, action, target, ip, prevHash, entryHash
		 FROM auditLog
		 ORDER BY id ASC`)
	if err != nil {
		return status, fmt.Errorf("audit chain: query: %w", err)
	}
	defer rows.Close()

	var prevHash string
	first := true
	for rows.Next() {
		var id int64
		var ts, actor, action, target, ip, rowPrev, rowHash string
		if err := rows.Scan(&id, &ts, &actor, &action, &target, &ip, &rowPrev, &rowHash); err != nil {
			return status, fmt.Errorf("audit chain: scan: %w", err)
		}
		if rowPrev == "" && rowHash == "" {
			// Pre-migration entry: no chain data, skip but keep walking.
			continue
		}
		status.Total++
		if first {
			prevHash = rowPrev
			first = false
		}
		expected := auditChainHash(prevHash, ts, actor, action, target, ip)
		if rowHash != expected || rowPrev != prevHash {
			status.Intact = false
			status.BrokenID = id
			status.BrokenAt = ts
			return status, nil
		}
		prevHash = rowHash
		status.Intact = true
		status.LastHash = rowHash
	}
	status.Intact = true
	return status, nil
}
