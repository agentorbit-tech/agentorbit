package migrations

import (
	"strings"
	"testing"
)

// TestMaxEmbeddedVersion ensures the helper correctly walks the embedded FS
// and surfaces the highest *.up.sql migration version.
func TestMaxEmbeddedVersion(t *testing.T) {
	v, err := MaxEmbeddedVersion()
	if err != nil {
		t.Fatalf("MaxEmbeddedVersion: %v", err)
	}
	if v == 0 {
		t.Fatalf("expected a positive version when migrations are embedded, got 0")
	}

	// Sanity: the same version should appear in at least one filename.
	entries, err := FS.ReadDir(".")
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	found := false
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".up.sql") {
			continue
		}
		// Filenames look like 0027_add_session_sort_indexes.up.sql.
		if strings.HasPrefix(e.Name(), pad4(int(v))+"_") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("MaxEmbeddedVersion returned %d but no matching .up.sql file", v)
	}
}

func pad4(n int) string {
	const zeros = "0000"
	s := ""
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	if len(s) >= 4 {
		return s
	}
	return zeros[:4-len(s)] + s
}
