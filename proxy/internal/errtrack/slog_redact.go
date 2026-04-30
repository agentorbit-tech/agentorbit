package errtrack

import (
	"log/slog"
	"regexp"
	"strings"
)

var sensitiveKeyRegex = regexp.MustCompile(`(?i)(authorization|api[_-]?key|provider[_-]?key|internal[_-]?token|secret|password|token)$`)

// SlogReplaceAttr redacts sensitive log attributes by key match and by
// substring scan of string values for API key / Bearer token patterns.
func SlogReplaceAttr(_ []string, a slog.Attr) slog.Attr {
	if sensitiveKeyRegex.MatchString(a.Key) {
		return slog.String(a.Key, "[redacted]")
	}
	if a.Value.Kind() == slog.KindString {
		s := a.Value.String()
		if strings.Contains(s, "sk-") || strings.Contains(s, "ao-") || strings.Contains(s, "Bearer ") {
			return slog.String(a.Key, redactSecrets(s))
		}
	}
	return a
}
