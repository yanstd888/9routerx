// 9RouterX — this file is part of a port maintained by https://github.com/yanstd888/ — https://t.me/aiclub_id
package main

import "testing"

func TestCollectMatchesRegistry(t *testing.T) {
	c := collect()
	if c.Total != c.APIKey+c.OAuth+c.FreeTier+c.Free+c.WebCookie {
		t.Fatalf("category totals %+v sum to %d, not total %d", c,
			c.APIKey+c.OAuth+c.FreeTier+c.Free+c.WebCookie, c.Total)
	}
	if c.Total == 0 {
		t.Fatal("collect returned no connectable providers")
	}
}

func TestReplaceMarkers(t *testing.T) {
	c := counts{Total: 344, APIKey: 265, OAuth: 34, FreeTier: 23, Free: 11, WebCookie: 11}
	src := "a <!-- GEN:PROVIDERCOUNT total -->154<!-- /GEN --> " +
		"b <!-- GEN:PROVIDERCOUNT freeAll -->29<!-- /GEN --> " +
		"c <!-- GEN:PROVIDERCOUNT apikey -->91<!-- /GEN --> " +
		"d <!-- GEN:PROVIDERCOUNT webCookie -->6<!-- /GEN -->"
	want := "a <!-- GEN:PROVIDERCOUNT total -->344<!-- /GEN --> " +
		"b <!-- GEN:PROVIDERCOUNT freeAll -->34<!-- /GEN --> " +
		"c <!-- GEN:PROVIDERCOUNT apikey -->265<!-- /GEN --> " +
		"d <!-- GEN:PROVIDERCOUNT webCookie -->11<!-- /GEN -->"
	got, err := replaceMarkers(src, c)
	if err != nil {
		t.Fatalf("replaceMarkers: %v", err)
	}
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestReplaceMarkersRejectsUnknownField(t *testing.T) {
	src := "<!-- GEN:PROVIDERCOUNT bogus -->0<!-- /GEN -->"
	if _, err := replaceMarkers(src, counts{}); err == nil {
		t.Fatal("unknown marker field was accepted")
	}
}
