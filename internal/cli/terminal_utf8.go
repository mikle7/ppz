package cli

// splitOnUTF8Boundary returns (complete, pending) where `complete` is the
// longest prefix of `b` that ends at a valid UTF-8 character boundary, and
// `pending` is whatever trailing bytes form an incomplete multi-byte
// sequence at the end of `b`. Callers carry `pending` over to the next
// read and prepend before invoking again.
//
// Used by the PTY master reader so we don't ship a chunk through JSON
// (which would silently rewrite truncated bytes as U+FFFD) when the read
// boundary lands mid-character.
//
// Assumes the input is valid UTF-8 except possibly for an incomplete
// sequence at the very end (always true for PTY output read in chunks).
// Walks back at most 4 bytes — the maximum size of a UTF-8 rune — to
// find the start of the last sequence and decides whether it's complete.
func splitOnUTF8Boundary(b []byte) (complete, pending []byte) {
	if len(b) == 0 {
		return nil, nil
	}
	// Look for a multi-byte lead byte in the last 4 positions. A lead
	// byte has its top two bits set (0b11xxxxxx, i.e. >= 0xc0).
	start := len(b) - 1
	if start > len(b)-1 {
		start = len(b) - 1
	}
	minScan := len(b) - 4
	if minScan < 0 {
		minScan = 0
	}
	for i := start; i >= minScan; i-- {
		c := b[i]
		switch {
		case c < 0x80:
			// ASCII byte: it's a complete rune by itself, and bytes
			// after it (if any) are continuation bytes for an
			// incomplete sequence we haven't found a lead for —
			// shouldn't happen on valid input. Treat the whole input
			// as complete up to here + 1.
			return b, nil
		case c >= 0xc0:
			// Lead byte at position i. Expected total length:
			//   0xc0–0xdf (110xxxxx): 2 bytes
			//   0xe0–0xef (1110xxxx): 3 bytes
			//   0xf0–0xf7 (11110xxx): 4 bytes
			var expected int
			switch {
			case c < 0xe0:
				expected = 2
			case c < 0xf0:
				expected = 3
			default:
				expected = 4
			}
			actual := len(b) - i
			if actual < expected {
				return cloneBytes(b[:i]), cloneBytes(b[i:])
			}
			return b, nil
		}
		// 0x80 <= c < 0xc0: continuation byte, keep walking back.
	}
	// All scanned bytes were continuation bytes — input is malformed.
	// Pass through unchanged; downstream JSON encoding will mangle it,
	// which matches the legacy behaviour for truly invalid input.
	return b, nil
}

// cloneBytes — slice the input safely into independent backing arrays so
// callers can hold onto `pending` across reads without aliasing the next
// scratch buffer.
func cloneBytes(b []byte) []byte {
	if len(b) == 0 {
		return nil
	}
	out := make([]byte, len(b))
	copy(out, b)
	return out
}
