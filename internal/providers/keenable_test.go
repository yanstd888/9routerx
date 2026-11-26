package providers

import "testing"

func TestKeenableRouteSplit(t *testing.T) {
	if got := KeenableFetchURL("https://api.keenable.ai/", false, "https://example.test/a b"); got != "https://api.keenable.ai/v1/fetch/public?url=https%3A%2F%2Fexample.test%2Fa+b" {
		t.Fatalf("public fetch = %q", got)
	}
	if got := KeenableFetchURL("https://api.keenable.ai", true, "https://example.test"); got != "https://api.keenable.ai/v1/fetch?url=https%3A%2F%2Fexample.test" {
		t.Fatalf("keyed fetch = %q", got)
	}
	if got := KeenableSearchURL("https://api.keenable.ai", false, "go router"); got != "https://api.keenable.ai/v1/search/public?query=go+router" {
		t.Fatalf("public search = %q", got)
	}
	if got := KeenableSearchURL("https://api.keenable.ai", true, "go"); got != "https://api.keenable.ai/v1/search?query=go" {
		t.Fatalf("keyed search = %q", got)
	}
}
