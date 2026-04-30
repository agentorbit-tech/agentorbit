package errtrack

import (
	"log/slog"
	"regexp"
	"strings"
)

// sensitiveKeyRegex matches log attribute keys that should always be redacted.
// Case-insensitive. Covers: authorization, api_key/api-key/apikey,
// provider_key, internal_token, secret, password, token (when used as a
// suffix or full key).
var sensitiveKeyRegex = regexp.MustCompile(`(?i)(authorization|api[_-]?key|provider[_-]?key|internal[_-]?token|secret|password|token)$`)

// SlogReplaceAttr is a slog.HandlerOptions.ReplaceAttr function that
// redacts sensitive values regardless of the value type. Values are
// considered sensitive if EITHER:
//   - the attribute key matches sensitiveKeyRegex, OR
//   - the string-form of the value contains an API key prefix or
//     a Bearer token.
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
