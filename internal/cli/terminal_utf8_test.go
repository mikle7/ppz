package cli

// Reproduces the user-observed bug: PTY master reads land at arbitrary
// byte boundaries — sometimes mid-multi-byte-UTF-8-character — and our
// publisher converts each chunk to a Go string + ships it via JSON.
// JSON marshalling silently rewrites the partial bytes as U+FFFD, so the
// viewer sees `???` where it should see `─`.
//
// The fix: split each incoming chunk on the last *complete* UTF-8 rune,
// carry the trailing partial bytes to the next read, and rejoin them
// with the next chunk before publishing.

import (
	"bytes"
	"testing"
)

func TestSplitOnUTF8Boundary(t *testing.T) {
	cases := []struct {
		name         string
		in           []byte
		wantComplete []byte
		wantPending  []byte
	}{
		{
			name: "empty",
		},
		{
			name:         "all ascii",
			in:           []byte("hello"),
			wantComplete: []byte("hello"),
		},
		{
			name:         "complete 3-byte at end",
			in:           []byte("foo─bar"),
			wantComplete: []byte("foo─bar"),
		},
		{
			name:         "truncated 3-byte: only the lead byte",
			in:           append([]byte("foo"), 0xe2),
			wantComplete: []byte("foo"),
			wantPending:  []byte{0xe2},
		},
		{
			name:         "truncated 3-byte: lead + first continuation",
			in:           append([]byte("foo"), 0xe2, 0x94),
			wantComplete: []byte("foo"),
			wantPending:  []byte{0xe2, 0x94},
		},
		{
			name:         "truncated 2-byte at start",
			in:           []byte{0xc3},
			wantComplete: nil,
			wantPending:  []byte{0xc3},
		},
		{
			name:         "truncated 4-byte (emoji): 2 of 4 bytes",
			in:           []byte{0xf0, 0x9f},
			wantComplete: nil,
			wantPending:  []byte{0xf0, 0x9f},
		},
		{
			name:         "complete 4-byte char",
			in:           []byte("👋"), // U+1F44B, 4 bytes f0 9f 91 8b
			wantComplete: []byte("👋"),
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gotComplete, gotPending := splitOnUTF8Boundary(c.in)
			if !bytes.Equal(gotComplete, c.wantComplete) {
				t.Errorf("complete: got %q (% x), want %q (% x)",
					gotComplete, gotComplete, c.wantComplete, c.wantComplete)
			}
			if !bytes.Equal(gotPending, c.wantPending) {
				t.Errorf("pending: got % x, want % x", gotPending, c.wantPending)
			}
		})
	}
}
