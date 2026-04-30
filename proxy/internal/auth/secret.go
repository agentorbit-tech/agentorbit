// Package auth: Secret wraps a sensitive string so it never leaks to logs,
// fmt output, or default JSON marshaling. Only header-set sites in the
// proxy handler are allowed to call Reveal().
package auth

// Secret hides a sensitive string. Reveal() returns the raw value.
type Secret struct {
	v string
}

// NewSecret wraps a raw secret string.
func NewSecret(s string) Secret { return Secret{v: s} }

// Reveal returns the raw value. Use only at deliberate trust boundaries
// (provider header set in the proxy forward path).
func (s Secret) Reveal() string { return s.v }

// String redacts in any fmt %s / Stringer context.
func (s Secret) String() string { return "[redacted]" }

// GoString redacts in fmt %#v.
func (s Secret) GoString() string { return "[redacted]" }

// MarshalJSON redacts on default JSON encoding.
func (s Secret) MarshalJSON() ([]byte, error) {
	return []byte(`"[redacted]"`), nil
}

// UnmarshalJSON accepts the plain JSON string sent by the processing
// /internal/auth/verify endpoint and stores it as the underlying value.
func (s *Secret) UnmarshalJSON(data []byte) error {
	if len(data) == 0 || string(data) == "null" {
		s.v = ""
		return nil
	}
	if len(data) >= 2 && data[0] == '"' && data[len(data)-1] == '"' {
		s.v = string(data[1 : len(data)-1])
		return nil
	}
	s.v = string(data)
	return nil
}
