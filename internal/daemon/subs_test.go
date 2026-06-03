package daemon

import (
	"reflect"
	"testing"
)

func TestSubscriptionsAddIsIdempotentAndSorted(t *testing.T) {
	s := newSubscriptions(t.TempDir())
	if err := s.Add("mysh", "room"); err != nil {
		t.Fatal(err)
	}
	// Adding the same subject again is a no-op.
	if err := s.Add("mysh", "room"); err != nil {
		t.Fatal(err)
	}
	if err := s.Add("mysh", "alice.inbox", "  ", ""); err != nil {
		t.Fatal(err)
	}
	got := s.List("mysh")
	want := []string{"alice.inbox", "room"} // sorted, blanks dropped, no dupes
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("List = %v, want %v", got, want)
	}
}

func TestSubscriptionsRemoveIsIdempotent(t *testing.T) {
	s := newSubscriptions(t.TempDir())
	_ = s.Add("mysh", "room", "alice.inbox")
	if err := s.Remove("mysh", "room"); err != nil {
		t.Fatal(err)
	}
	// Removing an absent subject is a no-op (no error).
	if err := s.Remove("mysh", "room"); err != nil {
		t.Fatal(err)
	}
	if got, want := s.List("mysh"), []string{"alice.inbox"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("List = %v, want %v", got, want)
	}
}

func TestSubscriptionsIsolatedPerSession(t *testing.T) {
	s := newSubscriptions(t.TempDir())
	_ = s.Add("foo", "foo.inbox")
	_ = s.Add("desk", "room")
	if got := s.List("foo"); !reflect.DeepEqual(got, []string{"foo.inbox"}) {
		t.Fatalf("foo session = %v", got)
	}
	if got := s.List("desk"); !reflect.DeepEqual(got, []string{"room"}) {
		t.Fatalf("desk session = %v", got)
	}
}

func TestSubscriptionsSweepHandleAcrossSessions(t *testing.T) {
	s := newSubscriptions(t.TempDir())
	// alice's own auto-sub, plus a personal shell that subscribed to alice's
	// pipes and an unrelated room.
	_ = s.Add("alice", "alice.inbox")
	_ = s.Add("desk", "alice.inbox", "alice.broadcast", "room")

	if err := s.SweepHandle("alice"); err != nil {
		t.Fatal(err)
	}
	if got := s.List("alice"); len(got) != 0 {
		t.Fatalf("alice session after sweep = %v, want empty", got)
	}
	// The desk shell keeps the unrelated room; alice.* are swept.
	if got, want := s.List("desk"), []string{"room"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("desk session after sweep = %v, want %v", got, want)
	}
}

func TestSubscriptionsSweepHandleNoFilesIsNoError(t *testing.T) {
	s := newSubscriptions(t.TempDir())
	if err := s.SweepHandle("ghost"); err != nil {
		t.Fatalf("SweepHandle on empty dir: %v", err)
	}
}
