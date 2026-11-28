### Fixed
- Dashboard mutations now write their audit entries through `RecordAuditChainEntry`, so each row carries the hash of its predecessor instead of an unchained insert. `EnsureAuditChainColumns` is a real idempotent migration (chain columns plus `idx_audit_prev_hash`) and runs at boot alongside the other additive columns.
- Added the admin-only `GET /api/audit/verify` endpoint. It walks the chain, reports the first tampered row, and ignores pre-migration entries with no hashes.

### Verification
- `TestEnsureAuditChainColumnsIsIdempotent`, `TestRecordAuditWritesChainedEntry`, `TestHandleVerifyAuditChain`, plus the existing intact and tamper-detection tests.
- `tools/p1_gate.sh sec1-h2`: all steps green.
