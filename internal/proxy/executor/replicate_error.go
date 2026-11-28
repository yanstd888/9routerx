// 9RouterX — ported from OmniRoute open-sse/executors/replicate.ts (pin 632d84f5).
package executor

import (
	"fmt"
	"regexp"
	"strings"
)

// Provider-local credential scrub for upstream error detail. Hosts stay so the
// message still says where the call failed; only credential material goes.
// Kept here instead of importing a shared scanner for one error field.
const replicateRedacted = "[REDACTED]"

var (
	replicateUserInfoRE   = regexp.MustCompile(`(?i)\b([a-z][a-z0-9+.-]*://)[^/\s@]+@`)
	replicateAuthRE       = regexp.MustCompile(`(?i)\b(Bearer|Basic)(\s+)[A-Za-z0-9._~+/=-]{8,}`)
	replicateCredentialRE = regexp.MustCompile(`(?i)\b(api[_-]?key|access[_-]?token|refresh[_-]?token|authorization|cookie|secret|key)(\s*[:=]\s*)(?:"[^"]*"|'[^']*'|[^\s,;&]+)`)
	replicateTokenRE      = regexp.MustCompile(`\b(?:r8_|sk-|tok_|gh[pousr]_|xox[baprs]-)[A-Za-z0-9_-]{6,}\b`)
	replicateLongTokenRE  = regexp.MustCompile(`\b[A-Za-z0-9_-]{24,}\b`)
)

func replicateLooksLikeToken(s string) bool {
	var letter, digit bool
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= '0' && c <= '9':
			digit = true
		case (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z'):
			letter = true
		}
		if letter && digit {
			return true
		}
	}
	return false
}

func replicateRedactCredentials(s string) string {
	s = replicateUserInfoRE.ReplaceAllString(s, "$1")
	s = replicateAuthRE.ReplaceAllString(s, "${1}${2}"+replicateRedacted)
	s = replicateCredentialRE.ReplaceAllString(s, "${1}${2}"+replicateRedacted)
	s = replicateTokenRE.ReplaceAllString(s, replicateRedacted)
	return replicateLongTokenRE.ReplaceAllStringFunc(s, func(match string) string {
		if replicateLooksLikeToken(match) {
			return replicateRedacted
		}
		return match
	})
}

func replicateSanitizeDetail(detail string) string {
	s := strings.TrimSpace(detail)
	s = replicateRedactCredentials(s)
	if i := strings.Index(s, " at /"); i >= 0 {
		s = s[:i]
	}
	for _, marker := range []string{"/srv/", "/home/", "/usr/", "/var/", "/root/", "/app/"} {
		if i := strings.Index(s, marker); i >= 0 {
			start := i
			for start > 0 && s[start-1] != ' ' {
				start--
			}
			s = s[:start]
		}
	}
	s = strings.TrimSpace(s)
	if len(s) > 200 {
		s = strings.ToValidUTF8(s[:200], "")
	}
	return strings.TrimSpace(s)
}

func replicateUpstreamMessage(status int, detail string) string {
	safe := replicateSanitizeDetail(detail)
	if safe == "" {
		return fmt.Sprintf("Replicate API failed with status %d", status)
	}
	return fmt.Sprintf("Replicate API failed with status %d: %s", status, safe)
}
