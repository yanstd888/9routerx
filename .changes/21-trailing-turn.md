### Fixed

- A trailing user turn whose content the cleanup emptied is now replaced by the "Continue." placeholder instead of being shipped alongside it, so the fallback lane no longer answers 400 from Anthropic for a body the client never wrote that way.

### Validation

- `go vet ./internal/translator/`, targeted and full `go test -count=1 ./internal/translator/`, including a new regression test covering empty block array, blank string, and null content tails.
