package db

import (
	"reflect"
	"testing"
)

func TestSourcePipes_MessageSourceIncludesInbox(t *testing.T) {
	got := (Source{Kind: SourceKindMessage}).Pipes()
	want := []string{"broadcast", "inbox"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("message source pipes = %#v, want %#v", got, want)
	}
}

func TestSourcePipes_PTYSourceIncludesInbox(t *testing.T) {
	got := (Source{Kind: SourceKindPTY}).Pipes()
	want := []string{"broadcast", "stdin", "stdout", "stdctrl", "inbox"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("pty source pipes = %#v, want %#v", got, want)
	}
}
