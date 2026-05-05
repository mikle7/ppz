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

func TestIsAutoPipe_MessageSource(t *testing.T) {
	src := Source{Kind: SourceKindMessage}
	for _, pipe := range []string{"broadcast", "inbox"} {
		if !src.IsAutoPipe(pipe) {
			t.Errorf("IsAutoPipe(%q) = false for message source, want true", pipe)
		}
	}
	for _, pipe := range []string{"custom", "results", "logs"} {
		if src.IsAutoPipe(pipe) {
			t.Errorf("IsAutoPipe(%q) = true for message source, want false", pipe)
		}
	}
}

func TestIsAutoPipe_PTYSource(t *testing.T) {
	src := Source{Kind: SourceKindPTY}
	for _, pipe := range []string{"broadcast", "inbox", "stdin", "stdout", "stdctrl"} {
		if !src.IsAutoPipe(pipe) {
			t.Errorf("IsAutoPipe(%q) = false for pty source, want true", pipe)
		}
	}
	for _, pipe := range []string{"custom", "results", "logs"} {
		if src.IsAutoPipe(pipe) {
			t.Errorf("IsAutoPipe(%q) = true for pty source, want false", pipe)
		}
	}
}
