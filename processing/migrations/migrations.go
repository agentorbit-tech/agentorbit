// Package migrations bundles the SQL migration files into the binary via
// embed.FS. The same FS is consumed by the `agentorbit-migrate` executable
// (which actually applies migrations) and by the processing service at boot
// (which only reads it to determine the highest expected schema version).
package migrations

import (
	"embed"
	"fmt"
	"strconv"
	"strings"
)

//go:embed *.sql
var FS embed.FS

// MaxEmbeddedVersion returns the highest migration version embedded in this
// binary by scanning *.up.sql filenames. Filenames follow the convention
// `<NNNN>_<slug>.up.sql`. Returns 0 when no migrations are embedded.
func MaxEmbeddedVersion() (uint, error) {
	entries, err := FS.ReadDir(".")
	if err != nil {
		return 0, fmt.Errorf("read embedded migrations dir: %w", err)
	}
	var max uint
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".up.sql") {
			continue
		}
		// Extract the version prefix: digits up to the first '_' or '.'.
		i := 0
		for i < len(name) && name[i] >= '0' && name[i] <= '9' {
			i++
		}
		if i == 0 {
			continue
		}
		n, err := strconv.ParseUint(name[:i], 10, 64)
		if err != nil {
			return 0, fmt.Errorf("parse version in %q: %w", name, err)
		}
		if uint(n) > max {
			max = uint(n)
		}
	}
	return max, nil
}
