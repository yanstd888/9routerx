package providers

import (
	"net/url"
	"strings"
)

// KeenableFetchURL and KeenableSearchURL pin the public/keyed route split.
// The public routes work without a key; an API key only lifts rate limits.
func KeenableFetchURL(base string, keyed bool, target string) string {
	base = strings.TrimRight(base, "/")
	path := "/v1/fetch/public"
	if keyed {
		path = "/v1/fetch"
	}
	return base + path + "?url=" + url.QueryEscape(target)
}

func KeenableSearchURL(base string, keyed bool, query string) string {
	base = strings.TrimRight(base, "/")
	path := "/v1/search/public"
	if keyed {
		path = "/v1/search"
	}
	return base + path + "?query=" + url.QueryEscape(query)
}
