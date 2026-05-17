package daemon

import (
	"reflect"
	"testing"
	"time"

	"github.com/pipescloud/ppz/internal/cliproto"
)

// enrichEntriesWithOwners joins the daemon's in-memory heartbeat cache
// with a server-side source listing (which carries the per-source
// CreatedBy / owner username) and produces the wire shape `ppz who`
// consumes. Pure function — no IPC, no network, deterministic.
//
// Rules:
//   - Order is preserved from the cache snapshot.
//   - An entry whose handle isn't in the source map (orphan: server
//     deleted the source but the daemon's cache still has a beat) gets
//     Owner == "" — the renderer falls back to "-".
//   - Cache fields (Handle, Payload, ArrivedAt) flow through verbatim.

func TestEnrichEntriesWithOwners_JoinsByHandle(t *testing.T) {
	t1 := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)
	cache := []HeartbeatEntry{
		{Handle: "alice", Payload: `{"seq":1}`, ArrivedAt: t1},
		{Handle: "bob", Payload: `{"seq":2}`, ArrivedAt: t1},
	}
	sources := []cliproto.Source{
		{Handle: "alice", CreatedBy: "alice-owner"},
		{Handle: "bob", CreatedBy: "bob-owner"},
	}
	got := enrichEntriesWithOwners(cache, sources)
	want := []cliproto.WhoEntry{
		{Handle: "alice", Owner: "alice-owner", Payload: `{"seq":1}`, ArrivedAt: t1},
		{Handle: "bob", Owner: "bob-owner", Payload: `{"seq":2}`, ArrivedAt: t1},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("enrichEntriesWithOwners mismatch:\n got=%#v\nwant=%#v", got, want)
	}
}

func TestEnrichEntriesWithOwners_MissingSourceFallsBackToEmpty(t *testing.T) {
	now := time.Now()
	cache := []HeartbeatEntry{
		{Handle: "orphan", Payload: `{"seq":1}`, ArrivedAt: now},
	}
	got := enrichEntriesWithOwners(cache, nil)
	if len(got) != 1 {
		t.Fatalf("want 1 entry, got %d", len(got))
	}
	if got[0].Owner != "" {
		t.Errorf("Owner = %q, want empty (orphan source)", got[0].Owner)
	}
	if got[0].Handle != "orphan" {
		t.Errorf("Handle round-trip: got %q, want orphan", got[0].Handle)
	}
}

func TestEnrichEntriesWithOwners_PreservesCacheOrder(t *testing.T) {
	now := time.Now()
	cache := []HeartbeatEntry{
		{Handle: "zulu", ArrivedAt: now},
		{Handle: "alpha", ArrivedAt: now},
		{Handle: "mike", ArrivedAt: now},
	}
	got := enrichEntriesWithOwners(cache, nil)
	handles := []string{got[0].Handle, got[1].Handle, got[2].Handle}
	want := []string{"zulu", "alpha", "mike"} // cache already sorted by Snapshot — function must not re-sort
	if !reflect.DeepEqual(handles, want) {
		t.Errorf("order changed: got %v, want %v", handles, want)
	}
}
