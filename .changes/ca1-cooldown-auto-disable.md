### Fixed
- "Disable dead models" now disables only verdicts that are hard `error`. A `blocked` verdict is an account cooldown that resolves on its own; disabling it used to remove the model from `/v1/models`, every picker and every combo permanently.
- The button tooltip now says cooldown models are left alone. Added `isDeadProbeStatus` with a regression test covering error, blocked, testing, ok and unprobed states.

### Verification
- `bun test src/components/connections/modelCheckAll.test.ts`: 13 pass.
- `tools/p1_gate.sh sec3-ca1`: all steps green.
