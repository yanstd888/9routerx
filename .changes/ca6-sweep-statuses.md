### Fixed
- A Check All sweep now ends with a terminal status for every id it started. Ids still `testing` after `Promise.allSettled` become `error`, the progress counter is set to the full total, and the summary is recomputed from the finalized statuses, so the total matches the visible rows.

### Verification
- `finalizeSweepStatuses` tests cover the stuck case and confirm statuses outside the sweep stay untouched; `formatCheckAllSummary` then reports total 3 for three visible rows.
- `bun test src/components/connections/modelCheckAll.test.ts`: 16 pass; `tools/p1_gate.sh sec3-ca6`: all steps green.
