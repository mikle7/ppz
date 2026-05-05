package cliproto

import (
	"bytes"
	"testing"
)

func TestPrintSourceDestroy(t *testing.T) {
	var b bytes.Buffer
	PrintSourceDestroy(&b, SourceDestroyReply{Handle: "apple"})
	want := "destroyed source=apple\n"
	if got := b.String(); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
