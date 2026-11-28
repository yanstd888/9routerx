### Fixed
- Model-test responses now carry `connectionId`, the account the resolver actually chose for the probe. A context-carried recorder is installed by `probeModel` and written by `getBestConnectionWithContext`, so no route header or client pin is needed and media lanes work too.
- The dashboard appends `(via <id>)` to failed verdicts, so an operator can tell a dead model from one account in cooldown. Added backend resolver and frontend parser tests.

### Verification
- `go test ./internal/handlers/chat/` with `TestProbeConnectionRecorderGetsResolvedConnection`, `bun test src/components/connections/modelCheckAll.test.ts`.
- `tools/p1_gate.sh sec3-ca2`: all steps green.
