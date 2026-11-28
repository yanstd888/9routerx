### Fixed
- Probe response parsing now unwraps one generic `{"data":{...}}` envelope after the existing Cline unwrap, when the object has exactly one object-valued field carrying `choices` or `content`. Multi-field objects and deeper nesting are left alone.

### Verification
- `TestParseProbeBody_UnwrapsSingleObjectEnvelope` was red before the fix and green after; `TestParseProbeBody_LeavesMultiFieldObjectAlone` pins the boundary.
- `tools/p1_gate.sh sec3-ca4-rerun`: all steps green.
