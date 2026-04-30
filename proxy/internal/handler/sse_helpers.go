package handler

import (
	"bytes"
	"io"

	"github.com/agentorbit-tech/agentorbit/proxy/internal/masking"
)

// appendRing keeps the last capLen bytes of (existing ++ next).
// Used to maintain a bounded tail buffer of the SSE stream so that
// parseSSELastChunk can still locate the final "data: {...}" JSON line
// without buffering the entire response.
//
// Three cases:
//  1. len(next) >= capLen → the trailing capLen bytes of next dominate;
//     existing is discarded.
//  2. len(existing)+len(next) > capLen → keep the last capLen bytes of
//     the concatenation.
//  3. len(existing)+len(next) <= capLen → return existing ++ next.
func appendRing(existing, next []byte, capLen int) []byte {
	if capLen <= 0 {
		return nil
	}
	if len(next) >= capLen {
		out := make([]byte, capLen)
		copy(out, next[len(next)-capLen:])
		return out
	}
	combined := append(existing, next...)
	if len(combined) > capLen {
		combined = combined[len(combined)-capLen:]
	}
	return combined
}

// unmaskStreaming reads from src in fixed-size chunks and writes incrementally
// unmasked output to dst, holding back enough of the tail to absorb any masked
// placeholder that straddles a chunk boundary.
//
// Algorithm: maintain pending = all unread input. On each read, compute a
// pre-unmask cutoff `safeUpTo` such that pending[:safeUpTo] can be unmasked
// independently of any future read. Two safety conditions must hold:
//   (a) no entry.Masked occurrence straddles position safeUpTo (would mean a
//       partial match in pending[:safeUpTo] would later become a full match);
//   (b) the last (maxLen-1) bytes of pending[:safeUpTo] are not a strict
//       prefix of any entry.Masked (those bytes might join a future read to
//       form a complete placeholder).
//
// Both conditions reduce to: shrink safeUpTo until pending[safeUpTo-k:safeUpTo]
// is not a prefix of any entry.Masked for any k in [1, maxLen-1]. We never
// emit those potentially-partial bytes until either more bytes resolve the
// ambiguity or we hit EOF.
//
// At EOF we emit everything (no future bytes can extend a partial match).
//
// Returns (bytesEmitted, err). EOF terminates normally; write errors propagate.
func unmaskStreaming(dst io.Writer, src io.Reader, entries []masking.MaskEntry, chunkSize int) (int, error) {
	if chunkSize <= 0 {
		chunkSize = 32 * 1024
	}
	maxLen := 0
	for _, e := range entries {
		if l := len(e.Masked); l > maxLen {
			maxLen = l
		}
	}
	// safeCutoff returns the largest s ≤ len(pending) such that
	// pending[s-k:s] is NOT a strict prefix of any e.Masked for k in [1, maxLen-1].
	// On EOF it returns len(pending).
	safeCutoff := func(pending []byte, eof bool) int {
		if eof {
			return len(pending)
		}
		if maxLen <= 1 {
			return len(pending)
		}
		// Start at len(pending) and walk backwards until the trailing window
		// is not a strict prefix of any placeholder.
		for s := len(pending); s >= 0; s-- {
			conflict := false
			for _, e := range entries {
				m := []byte(e.Masked)
				if len(m) <= 1 {
					continue
				}
				maxK := len(m) - 1
				if maxK > s {
					maxK = s
				}
				for k := 1; k <= maxK; k++ {
					if bytes.Equal(pending[s-k:s], m[:k]) {
						conflict = true
						break
					}
				}
				if conflict {
					break
				}
			}
			if !conflict {
				return s
			}
		}
		return 0
	}
	var (
		pending        []byte
		emittedPostLen int
		emitted        int
	)
	flush := func(eof bool) error {
		safeUpTo := safeCutoff(pending, eof)
		full := masking.UnmaskContent(pending[:safeUpTo], entries)
		if len(full) <= emittedPostLen {
			return nil
		}
		toWrite := full[emittedPostLen:]
		n, werr := dst.Write(toWrite)
		emitted += n
		if werr != nil {
			return werr
		}
		emittedPostLen = len(full)
		return nil
	}
	buf := make([]byte, chunkSize)
	for {
		n, rerr := src.Read(buf)
		if n > 0 {
			pending = append(pending, buf[:n]...)
			if werr := flush(false); werr != nil {
				return emitted, werr
			}
		}
		if rerr != nil {
			if rerr == io.EOF {
				if err := flush(true); err != nil {
					return emitted, err
				}
				return emitted, nil
			}
			return emitted, rerr
		}
	}
}

// lastJSONDataLineFromTail walks the tail ring buffer (split on '\n') and
// returns the JSON payload of the last "data: {...}" line.
// Returns "" if no such line exists.
//
// Skips "data: [DONE]" (which begins with "[" not "{") and any non-data
// lines (event:, id:, retry:, comments).
func lastJSONDataLineFromTail(tail []byte) string {
	lines := bytes.Split(tail, []byte("\n"))
	for i := len(lines) - 1; i >= 0; i-- {
		line := lines[i]
		if bytes.HasPrefix(line, []byte("data: {")) {
			return string(bytes.TrimPrefix(line, []byte("data: ")))
		}
	}
	return ""
}
