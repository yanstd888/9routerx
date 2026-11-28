// 9RouterX — this file is part of a port maintained by https://github.com/yanstd888/ — https://t.me/aiclub_id
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"9routerx/proxy/internal/providers"
)

// counts is the machine-readable provider-count claim. The five categories are
// ClassifyProvider's dashboard categories over the upstream KnownProviders map.
type counts struct {
	Total     int `json:"total"`
	APIKey    int `json:"apikey"`
	OAuth     int `json:"oauth"`
	FreeTier  int `json:"freeTier"`
	Free      int `json:"free"`
	WebCookie int `json:"webCookie"`
}

// collect derives every number from the upstream provider configurations through
// the same ClassifyProvider the dashboard uses. Counting the map directly avoids
// a second list of provider ids drifting away from the code.
func collect() counts {
	var c counts
	for id := range providers.KnownProviders {
		category, _, _ := providers.ClassifyProvider(id)
		switch category {
		case providers.CategoryAPIKey:
			c.APIKey++
		case providers.CategoryOAuth:
			c.OAuth++
		case providers.CategoryFreeTier:
			c.FreeTier++
		case providers.CategoryFree:
			c.Free++
		case providers.CategoryWebCookie:
			c.WebCookie++
		}
		c.Total++
	}
	return c
}

var markerPattern = regexp.MustCompile(`<!-- GEN:PROVIDERCOUNT(?:\s+([A-Za-z]+))? -->(.*?)<!-- /GEN -->`)

func (c counts) value(key string) (string, bool) {
	switch strings.TrimSpace(key) {
	case "", "total":
		return strconv.Itoa(c.Total), true
	case "apikey":
		return strconv.Itoa(c.APIKey), true
	case "oauth":
		return strconv.Itoa(c.OAuth), true
	case "freeTier":
		return strconv.Itoa(c.FreeTier), true
	case "free":
		return strconv.Itoa(c.Free), true
	case "freeAll":
		// The docs' "free" line folds both free categories together, matching
		// the dashboard's single Free section.
		return strconv.Itoa(c.FreeTier + c.Free), true
	case "webCookie":
		return strconv.Itoa(c.WebCookie), true
	}
	return "", false
}

// replaceMarkers fills the generated spans in place. The opening comment names
// which field a span renders, so one regenerator can serve prose, tables and the
// hero line without a per-file template map.
func replaceMarkers(src string, c counts) (string, error) {
	var failure error
	out := markerPattern.ReplaceAllStringFunc(src, func(match string) string {
		parts := markerPattern.FindStringSubmatch(match)
		value, ok := c.value(parts[1])
		if !ok {
			failure = fmt.Errorf("unknown provider-count marker field %q", parts[1])
			return match
		}
		return "<!-- GEN:PROVIDERCOUNT" + optionalField(parts[1]) + " -->" +
			value + "<!-- /GEN -->"
	})
	return out, failure
}

func optionalField(field string) string {
	if field == "" {
		return ""
	}
	return " " + field
}

var generatedDocs = []string{
	"README.md",
	"wiki/index.md",
	"wiki/providers.md",
	"wiki/comparison.md",
}

func writeDocs(c counts) error {
	for _, path := range generatedDocs {
		raw, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		updated, err := replaceMarkers(string(raw), c)
		if err != nil {
			return fmt.Errorf("update %s: %w", path, err)
		}
		if updated == string(raw) {
			continue
		}
		if err := os.WriteFile(path, []byte(updated), 0644); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
	}
	return nil
}

func main() {
	write := flag.Bool("write", false, "regenerate the marked README and wiki numbers")
	flag.Parse()
	c := collect()
	if *write {
		if err := writeDocs(c); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if err := json.NewEncoder(os.Stdout).Encode(c); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
