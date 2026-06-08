package daemon

import (
	"errors"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
)

// TestWatchRegistry_RearmAll_ReplacesSubAndDeliversOnNewNC pins the
// core fix: a wait/watch subscription anchored to a NC that's about to
// be swapped out gets rebound to the replacement NC, so the callback
// continues firing on publishes to the same subject.
//
// RED until watch_registry.go exists.
func TestWatchRegistry_RearmAll_ReplacesSubAndDeliversOnNewNC(t *testing.T) {
	url := startEmbeddedNATSURL(t)
	ncA, err := nats.Connect(url)
	if err != nil {
		t.Fatalf("connect ncA: %v", err)
	}
	t.Cleanup(func() { ncA.Close() })
	ncB, err := nats.Connect(url)
	if err != nil {
		t.Fatalf("connect ncB: %v", err)
	}
	t.Cleanup(func() { ncB.Close() })

	r := newWatchRegistry()
	fired := make(chan []byte, 16)
	cb := func(msg *nats.Msg) { fired <- msg.Data }

	subj := "TEST.rearm.subject"
	originalSub, err := ncA.Subscribe(subj, cb)
	if err != nil {
		t.Fatalf("initial subscribe on ncA: %v", err)
	}
	entry := &watchEntry{subject: subj, cb: cb, sub: originalSub}
	r.add(entry)

	// Sanity: pre-rearm, the sub on ncA delivers.
	if err := ncA.Publish(subj, []byte("via-A-before")); err != nil {
		t.Fatalf("publish on ncA: %v", err)
	}
	if got, err := waitForPayload(fired, 500*time.Millisecond); err != nil {
		t.Fatalf("pre-rearm: %v", err)
	} else if string(got) != "via-A-before" {
		t.Fatalf("pre-rearm payload: got %q want via-A-before", got)
	}

	// Rearm onto ncB.
	r.rearmAll(ncB, nil)

	// Contract 1: entry.sub now points to a different, valid sub
	// (the one on ncB).
	if entry.sub == originalSub {
		t.Fatalf("entry.sub not replaced — rearmAll left the old sub in place")
	}
	if entry.sub == nil || !entry.sub.IsValid() {
		t.Fatalf("entry.sub after rearm: nil or invalid (want a live sub on ncB)")
	}

	// Contract 2: the original sub on ncA was Unsubscribed. nats.go
	// flips IsValid() to false at the moment Unsubscribe() locally
	// detaches the sub, BEFORE the server ack — so this is reliable
	// without a Flush.
	if originalSub.IsValid() {
		t.Fatalf("original sub on ncA should have been Unsubscribed by rearmAll")
	}

	// Contract 3: publishes on the new NC reach the new sub.
	drainPayloads(fired)
	if err := ncB.Publish(subj, []byte("via-B-after")); err != nil {
		t.Fatalf("publish on ncB: %v", err)
	}
	if got, err := waitForPayload(fired, 500*time.Millisecond); err != nil {
		t.Fatalf("post-rearm via ncB: %v", err)
	} else if string(got) != "via-B-after" {
		t.Fatalf("post-rearm payload: got %q want via-B-after", got)
	}
}

// TestWatchRegistry_RearmAll_FailureKeepsEntry covers the fail-soft
// contract: if Subscribe on the new NC errors, the entry stays in the
// registry with its old (about-to-die) sub untouched, and the warn
// callback fires. Worst-case the handler hits its IPC deadline — same
// as pre-fix behaviour, never worse.
//
// RED until watch_registry.go exists.
func TestWatchRegistry_RearmAll_FailureKeepsEntry(t *testing.T) {
	url := startEmbeddedNATSURL(t)
	ncA, err := nats.Connect(url)
	if err != nil {
		t.Fatalf("connect ncA: %v", err)
	}
	t.Cleanup(func() { ncA.Close() })

	r := newWatchRegistry()
	subj := "TEST.rearm.fail"
	sub, err := ncA.Subscribe(subj, func(*nats.Msg) {})
	if err != nil {
		t.Fatalf("initial subscribe: %v", err)
	}
	entry := &watchEntry{subject: subj, cb: func(*nats.Msg) {}, sub: sub}
	r.add(entry)
	originalSub := entry.sub

	// Closed connection → Subscribe returns ErrConnectionClosed.
	closedNC, err := nats.Connect(url)
	if err != nil {
		t.Fatalf("connect closedNC: %v", err)
	}
	closedNC.Close()

	var warns []string
	r.rearmAll(closedNC, func(reason string) { warns = append(warns, reason) })

	if len(warns) != 1 {
		t.Fatalf("expected exactly 1 warn callback (Subscribe failure), got %d: %v", len(warns), warns)
	}
	if entry.sub != originalSub {
		t.Fatalf("entry.sub should be unchanged on rearm failure; want %p got %p", originalSub, entry.sub)
	}
	// Entry should still be registered (next swap retries).
	if !r.contains(entry) {
		t.Fatalf("entry should still be in registry after rearm failure")
	}
}

// TestWatchRegistry_RemoveUnsubscribes covers handler-exit cleanup:
// remove() must Unsubscribe the live sub AND drop the entry from the
// map, so a subsequent rearmAll doesn't touch a dead handler.
//
// RED until watch_registry.go exists.
func TestWatchRegistry_RemoveUnsubscribes(t *testing.T) {
	url := startEmbeddedNATSURL(t)
	nc, err := nats.Connect(url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { nc.Close() })

	r := newWatchRegistry()
	fired := make(chan struct{}, 1)
	subj := "TEST.remove"
	sub, err := nc.Subscribe(subj, func(*nats.Msg) { fired <- struct{}{} })
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	entry := &watchEntry{subject: subj, cb: func(*nats.Msg) {}, sub: sub}
	r.add(entry)

	r.remove(entry)

	// Sub should be dead.
	if err := nc.Publish(subj, []byte("ping")); err != nil {
		t.Fatalf("publish: %v", err)
	}
	select {
	case <-fired:
		t.Fatalf("remove() did not Unsubscribe — publish fired the callback")
	case <-time.After(100 * time.Millisecond):
	}
	if r.contains(entry) {
		t.Fatalf("remove() did not drop entry from registry")
	}
	// remove() is idempotent — calling again must not panic.
	r.remove(entry)
}

// TestWatchRegistry_RearmAll_SkipsRemovedEntryWithoutLeak covers the
// race where a handler exits (calls remove) between rearmAll's
// snapshot iteration and its swapSub. The newly-created sub on the
// replacement NC must be Unsubscribed instead of installed onto a
// dead entry — otherwise the new NC accumulates orphaned subs across
// every swap of a handler that's exiting.
//
// RED until watch_registry.go exists.
func TestWatchRegistry_RearmAll_SkipsRemovedEntryWithoutLeak(t *testing.T) {
	url := startEmbeddedNATSURL(t)
	ncA, err := nats.Connect(url)
	if err != nil {
		t.Fatalf("connect ncA: %v", err)
	}
	t.Cleanup(func() { ncA.Close() })
	ncB, err := nats.Connect(url)
	if err != nil {
		t.Fatalf("connect ncB: %v", err)
	}
	t.Cleanup(func() { ncB.Close() })

	r := newWatchRegistry()
	subj := "TEST.removed.during.rearm"
	sub, err := ncA.Subscribe(subj, func(*nats.Msg) {})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	entry := &watchEntry{subject: subj, cb: func(*nats.Msg) {}, sub: sub}
	r.add(entry)

	// Baseline subscription count on ncB before rearm.
	beforeRearm, err := ncBSubsToSubject(ncB, subj)
	if err != nil {
		t.Fatalf("baseline ncB sub count: %v", err)
	}

	// Simulate the race: handler exits FIRST (remove() runs before
	// rearmAll iterates). swapSub must return nil and the just-created
	// sub on ncB must be Unsubscribed.
	r.remove(entry)
	r.rearmAll(ncB, nil)

	afterRearm, err := ncBSubsToSubject(ncB, subj)
	if err != nil {
		t.Fatalf("post-rearm ncB sub count: %v", err)
	}
	if afterRearm != beforeRearm {
		t.Fatalf("rearmAll leaked a sub on ncB: before=%d after=%d", beforeRearm, afterRearm)
	}
}

// waitForPayload returns the next payload on c, or an error on
// timeout. Lets tests disambiguate which sub fired by content.
func waitForPayload(c <-chan []byte, timeout time.Duration) ([]byte, error) {
	select {
	case b := <-c:
		return b, nil
	case <-time.After(timeout):
		return nil, errors.New("timed out waiting for callback")
	}
}

// drainPayloads non-blockingly discards anything already queued in c.
func drainPayloads(c <-chan []byte) {
	for {
		select {
		case <-c:
		default:
			return
		}
	}
}

// ncBSubsToSubject counts how many subscriptions on nc match subj.
// nats.go doesn't expose a per-subject count, but a fresh sub +
// Flush + a single Publish + count-of-fires within a short window is
// a reliable probe: each live sub fires once per publish.
func ncBSubsToSubject(nc *nats.Conn, subj string) (int, error) {
	fires := make(chan struct{}, 16)
	probe, err := nc.Subscribe(subj, func(*nats.Msg) { fires <- struct{}{} })
	if err != nil {
		return 0, err
	}
	defer probe.Unsubscribe()
	if err := nc.Flush(); err != nil {
		return 0, err
	}
	if err := nc.Publish(subj, []byte("probe")); err != nil {
		return 0, err
	}
	if err := nc.Flush(); err != nil {
		return 0, err
	}
	// Settle window: at most one fire per live sub. We include the
	// probe itself, so subtract 1 below.
	deadline := time.After(150 * time.Millisecond)
	count := 0
Loop:
	for {
		select {
		case <-fires:
			count++
		case <-deadline:
			break Loop
		}
	}
	return count - 1, nil // exclude the probe
}
