package cli

// filterControlResponses scans `input` for terminal control responses
// (DA1/DA2 attributes, focus in/out, cursor position report) and returns
// (a) `out`: the input minus any matched responses, and (b) `pending`: a
// trailing slice that begins with ESC and is too short to classify yet.
// The caller carries `pending` over to the next read so cross-buffer
// sequences are handled.
//
// Why filter at the stdin bridge: the wrapped child (or its outer shell)
// often sends queries the moment it starts — DA1 (`\x1b[c`), focus
// reporting, cursor position — and the local terminal emits the
// responses into our stdin. Naïve readline implementations (and any
// program that doesn't recognise these specific shapes) treat the
// response as typed input and self-insert it, which writes the bytes
// to the program's stdout and pollutes the .raw stream visible to
// `ppz terminal view`. Stripping the responses at the source matches
// the dance tmux/screen do in their own VT parsers.
//
// Sequences recognised (CSI = `\x1b[`):
//   CSI ?<params>c    — DA1 response  (e.g. \x1b[?1;2c)
//   CSI ><params>c    — DA2 response  (e.g. \x1b[>1;95;0c)
//   CSI <row>;<col>R  — cursor position report
//   CSI I             — focus in
//   CSI O             — focus out
// All other ESC sequences pass through unchanged (we don't try to be
// a full VT emulator).
func filterControlResponses(input []byte) (out, pending []byte) {
	out = make([]byte, 0, len(input))
	i := 0
	for i < len(input) {
		b := input[i]
		if b != 0x1b {
			out = append(out, b)
			i++
			continue
		}
		// ESC. Need at least one more byte to see the introducer.
		if i+1 >= len(input) {
			return out, append([]byte(nil), input[i:]...)
		}
		if input[i+1] != '[' {
			// Not a CSI sequence — pass ESC and the next byte through.
			out = append(out, input[i], input[i+1])
			i += 2
			continue
		}
		// CSI: \x1b [ ... <final byte 0x40-0x7E>
		end := i + 2
		for end < len(input) {
			c := input[end]
			if c >= 0x40 && c <= 0x7E {
				break
			}
			end++
		}
		if end >= len(input) {
			// Incomplete CSI — carry over.
			return out, append([]byte(nil), input[i:]...)
		}
		seq := input[i : end+1]
		if !isControlResponse(seq) {
			out = append(out, seq...)
		}
		i = end + 1
	}
	return out, nil
}

func isControlResponse(seq []byte) bool {
	if len(seq) < 3 || seq[0] != 0x1b || seq[1] != '[' {
		return false
	}
	final := seq[len(seq)-1]
	body := seq[2 : len(seq)-1]
	switch final {
	case 'c':
		// DA1 / DA2 responses always carry private (?) or secondary (>)
		// markers. A bare \x1b[c is a query the user might legitimately
		// send and we should pass through.
		return len(body) > 0 && (body[0] == '?' || body[0] == '>')
	case 'R':
		// Cursor position report: \x1b[<row>;<col>R. The user is highly
		// unlikely to type this; safe to drop.
		return true
	case 'I', 'O':
		// Focus events are exactly \x1b[I / \x1b[O.
		return len(body) == 0
	}
	return false
}
