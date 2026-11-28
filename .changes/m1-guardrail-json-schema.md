### Fixed
- JSON-schema guardrail policies can now be saved and enforced from the dashboard. `json_schema` is accepted as a detector name, schema patterns are validated as JSON Schema rather than compiled as regexes, and request-level `customPatterns` are merged into the config the resolver actually reads. A schema-only policy now enables the engine on its own.
- Inbound JSON bodies that violate the schema are refused with HTTP 400 naming the offending fields; buffered responses keep the existing block behaviour and streaming responses remain unblocked. `wiki/security.md` documents the supported shape and the coverage boundary.

### Verification
- `TestSchemaOnlyPolicyEnablesEngine`, `TestValidateJSONSchemaRejectsMalformedDocument`, `TestInboundJSONSchemaBlockNamesOffendingField`, the extended `validateGuardrailConfig` table, `TestHandleCreateGuardrailPolicy_MergesTopLevelSchemaPattern` and `TestHandleCreateGuardrailPolicy_RejectsMalformedSchema`.
- `go run ./tools/sitegen -out _site` regenerates `security.html` with the new text.
- `tools/p1_gate.sh sec1-m1`: all steps green.
