package memoryd

import (
	"regexp"
	"strings"
)

var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)sk-[a-z0-9]{16,}`),
	regexp.MustCompile(`(?i)api[_-]?key\s*[:=]\s*['\"]?[a-z0-9_\-]{12,}`),
	regexp.MustCompile(`(?i)-----begin [a-z ]*private key-----`),
	regexp.MustCompile(`(?i)aws(.{0,20})?(secret|access).{0,20}[=:]\s*['\"]?[a-z0-9/+=]{16,}`),
}

func ContainsLikelySecret(text string) bool {
	for _, p := range secretPatterns {
		if p.MatchString(text) {
			return true
		}
	}
	return false
}

func RedactSecrets(text string) string {
	out := text
	for _, p := range secretPatterns {
		out = p.ReplaceAllString(out, "[REDACTED_SECRET]")
	}
	return strings.TrimSpace(out)
}
