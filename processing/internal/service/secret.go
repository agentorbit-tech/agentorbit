// Package service: Secret is a wrapper around sensitive string values that
// redacts the underlying value in any standard formatting context (Stringer,
// fmt %v/%+v/%#v, JSON marshal). Use Reveal() to obtain the raw value at
// a deliberate trust boundary (HTTP response writer to the proxy, or
// header-set sites in the proxy itself).
package service

// Secret hides a sensitive string from logs, error messages, and JSON
// marshaling unless the holder explicitly calls Reveal(). The zero value
// (empty string) is safe to use.
type Secret struct {
	v string
}

// NewSecret wraps a raw secret string.
func NewSecret(s string) Secret { return Secret{v: s} }

// Reveal returns the raw underlying value. Call only at trust boundaries.
func (s Secret) Reveal() string { return s.v }

// String returns the redacted form for fmt %s and Stringer contexts.
func (s Secret) String() string { return "[redacted]" }

// GoString returns the redacted form for fmt %#v.
func (s Secret) GoString() string { return "[redacted]" }

// MarshalJSON returns a JSON string containing the redacted form. The
// internal verify handler that legitimately needs to send the real key
// over the wire to the proxy must build its own JSON envelope (it cannot
// rely on default marshaling of AuthVerifyResult).
func (s Secret) MarshalJSON() ([]byte, error) {
	return []byte(`"[redacted]"`), nil
}

// UnmarshalJSON accepts a JSON string and stores it as the secret value.
// Round-trip with MarshalJSON is intentionally lossy (you get back the
// literal "[redacted]" rather than the original); use the wire envelope
// in the internal handler when fidelity matters.
func (s *Secret) UnmarshalJSON(data []byte) error {
	if len(data) == 0 || string(data) == "null" {
		s.v = ""
		return nil
	}
	// Trim surrounding quotes if present (typical JSON string).
	if len(data) >= 2 && data[0] == '"' && data[len(data)-1] == '"' {
		s.v = string(data[1 : len(data)-1])
		return nil
	}
	s.v = string(data)
	return nil
}
