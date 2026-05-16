package cli

import (
	"reflect"
	"testing"
	"time"

	"github.com/pipescloud/ppz/internal/cliproto"
)

// pickAwaitTarget chooses the single pipe to drain. Spec:
//   1. Candidates = pipes with Unread > 0 that match the patterns.
//   2. Winner = oldest LastAt among candidates.
//   3. Tie-break = lexicographic on <handle>.<pipe>.
//
// All sub-tests below should pass once pickAwaitTarget is implemented.
// RED today: the stub returns ok=false unconditionally.

func t(ms int64) *time.Time {
	v := time.UnixMilli(ms).UTC()
	return &v
}

func TestPickAwaitTarget_OldestUnreadWins(t_ *testing.T) {
	reply := cliproto.ListReply{
		Sources: []cliproto.Source{
			{
				Handle: "alice",
				PipeInfos: []cliproto.PipeInfo{
					{Pipe: "inbox", Unread: 1, LastAt: t(3000)},
					{Pipe: "notes", Unread: 1, LastAt: t(1000)}, // oldest
				},
			},
			{
				Handle: "bob",
				PipeInfos: []cliproto.PipeInfo{
					{Pipe: "inbox", Unread: 2, LastAt: t(2000)},
				},
			},
		},
	}
	h, p, ok := pickAwaitTarget(reply, nil)
	if !ok {
		t_.Fatalf("pickAwaitTarget: ok=false, want a candidate")
	}
	if h != "alice" || p != "notes" {
		t_.Fatalf("pickAwaitTarget = (%q, %q), want (alice, notes)", h, p)
	}
}

func TestPickAwaitTarget_OnlyConsidersUnread(t_ *testing.T) {
	reply := cliproto.ListReply{
		Sources: []cliproto.Source{
			{
				Handle: "alice",
				PipeInfos: []cliproto.PipeInfo{
					{Pipe: "old-but-read", Unread: 0, LastAt: t(1000)}, // older but read
					{Pipe: "newer-unread", Unread: 1, LastAt: t(5000)},
				},
			},
		},
	}
	h, p, ok := pickAwaitTarget(reply, nil)
	if !ok || h != "alice" || p != "newer-unread" {
		t_.Fatalf("pickAwaitTarget = (%q, %q, %v), want (alice, newer-unread, true)", h, p, ok)
	}
}

func TestPickAwaitTarget_IncludesUncollared(t_ *testing.T) {
	reply := cliproto.ListReply{
		Sources: []cliproto.Source{
			{
				Handle: "alice",
				PipeInfos: []cliproto.PipeInfo{
					{Pipe: "inbox", Unread: 1, LastAt: t(5000)},
				},
			},
		},
		UncollaredPipes: []cliproto.UncollaredPipe{
			{
				Name: "plaza",
				Info: cliproto.PipeInfo{Pipe: "plaza", Unread: 1, LastAt: t(1000)}, // oldest
			},
		},
	}
	h, p, ok := pickAwaitTarget(reply, nil)
	if !ok {
		t_.Fatalf("pickAwaitTarget: ok=false, want uncollared candidate")
	}
	// Uncollared pipes have empty handle and the pipe-path goes in pipe.
	if h != "" || p != "plaza" {
		t_.Fatalf("pickAwaitTarget = (%q, %q), want (\"\", plaza)", h, p)
	}
}

func TestPickAwaitTarget_RespectsPatterns(t_ *testing.T) {
	reply := cliproto.ListReply{
		Sources: []cliproto.Source{
			{
				Handle: "alice",
				PipeInfos: []cliproto.PipeInfo{
					{Pipe: "inbox", Unread: 1, LastAt: t(1000)}, // oldest but filtered out
					{Pipe: "stdout", Unread: 1, LastAt: t(5000)},
				},
			},
		},
	}
	h, p, ok := pickAwaitTarget(reply, []string{"*.stdout"})
	if !ok || h != "alice" || p != "stdout" {
		t_.Fatalf("pickAwaitTarget with pattern *.stdout = (%q, %q, %v), want (alice, stdout, true)", h, p, ok)
	}
}

func TestPickAwaitTarget_NoCandidates(t_ *testing.T) {
	reply := cliproto.ListReply{
		Sources: []cliproto.Source{
			{Handle: "alice", PipeInfos: []cliproto.PipeInfo{{Pipe: "inbox", Unread: 0}}},
		},
	}
	_, _, ok := pickAwaitTarget(reply, nil)
	if ok {
		t_.Fatalf("pickAwaitTarget: ok=true, want false when no pipe has unread")
	}
}

func TestPickAwaitTarget_TieBreakLexicographic(t_ *testing.T) {
	// Both pipes have identical LastAt. Deterministic winner = lexicographically
	// smaller `<handle>.<pipe>`.
	reply := cliproto.ListReply{
		Sources: []cliproto.Source{
			{Handle: "bob", PipeInfos: []cliproto.PipeInfo{{Pipe: "inbox", Unread: 1, LastAt: t(1000)}}},
			{Handle: "alice", PipeInfos: []cliproto.PipeInfo{{Pipe: "inbox", Unread: 1, LastAt: t(1000)}}},
		},
	}
	h, p, ok := pickAwaitTarget(reply, nil)
	if !ok || h != "alice" || p != "inbox" {
		t_.Fatalf("pickAwaitTarget tie-break = (%q, %q, %v), want (alice, inbox, true)", h, p, ok)
	}
}

// defaultPatternsFromSnapshot expands `ppz await` (no args) to the
// concrete pattern list it should watch:
//   - <current>.inbox
//   - every uncollared pipe AT the current manifold (root if empty)
//
// Cross-manifold uncollared pipes are NOT included; the user's example
// in this thread was unambiguous on the namespace-scoped intent.
//
// All sub-cases below should pass once the helper is implemented.
// RED today: the stub returns nil.
func uc(manifold, name string) cliproto.UncollaredPipe {
	displayPath := name
	if manifold != "" {
		displayPath = manifold + "." + name
	}
	return cliproto.UncollaredPipe{
		Manifold: manifold,
		Name:     name,
		Info:     cliproto.PipeInfo{Pipe: displayPath},
	}
}

func TestDefaultPatternsFromSnapshot_InboxOnly_NoUncollared(t_ *testing.T) {
	got := defaultPatternsFromSnapshot("foo", "", nil)
	want := []string{"foo.inbox"}
	if !reflect.DeepEqual(got, want) {
		t_.Fatalf("got %v, want %v", got, want)
	}
}

func TestDefaultPatternsFromSnapshot_RootIncludesRootUncollared(t_ *testing.T) {
	got := defaultPatternsFromSnapshot("foo", "", []cliproto.UncollaredPipe{
		uc("", "room"),
		uc("", "plaza"),
	})
	want := []string{"foo.inbox", "room", "plaza"}
	if !reflect.DeepEqual(got, want) {
		t_.Fatalf("got %v, want %v", got, want)
	}
}

func TestDefaultPatternsFromSnapshot_RootExcludesNamespacedUncollared(t_ *testing.T) {
	got := defaultPatternsFromSnapshot("foo", "", []cliproto.UncollaredPipe{
		uc("", "room"),
		uc("team-a", "chat"),
		uc("team-b", "chat"),
	})
	want := []string{"foo.inbox", "room"}
	if !reflect.DeepEqual(got, want) {
		t_.Fatalf("got %v, want %v", got, want)
	}
}

func TestDefaultPatternsFromSnapshot_NamespaceScoped(t_ *testing.T) {
	got := defaultPatternsFromSnapshot("foo", "team-a", []cliproto.UncollaredPipe{
		uc("", "room"),         // root — must be excluded
		uc("team-a", "chat"),   // current ns — must be included
		uc("team-a", "lobby"),  // current ns — must be included
		uc("team-b", "chat"),   // other ns — must be excluded
	})
	want := []string{"foo.inbox", "team-a.chat", "team-a.lobby"}
	if !reflect.DeepEqual(got, want) {
		t_.Fatalf("got %v, want %v", got, want)
	}
}

