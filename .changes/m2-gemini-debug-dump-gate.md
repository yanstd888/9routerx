### Fixed
- Gemini 400 and thought-signature errors no longer write the full request body to fixed, world-readable `/tmp` files on every production rejection. Both dumps now require `9RX_DEBUG_DUMP=1`, use mode 0600, and keep request previews out of logs unless the same opt-in is set. Upstream error details are still logged.

### Verification
- `TestForwardGemini_DebugDumpRequiresOptIn` and `TestForwardGemini_DebugDumpWithOptIn` exercise a real 400 response with a thought-signature error and assert the two paths are absent or present accordingly.
- `tools/p1_gate.sh sec1-m2`: all steps green.
