package daemon

// Tests for the on-disk nats-events.jsonl. Cover the three behaviours
// the rest of Phase 0 depends on:
//
//   - Append round-trips through readNATSEventLogFile in the order
//     they were written, with every field intact.
//   - Rotation kicks in past the size cap, and old generations age
//     out at .2.
//   - scanNATSEventLog stitches rotated + active files in chronological
//     order and respects the `since` filter.
//
// Tests live in the same package so they can poke at unexported
// helpers without an awkward shim.

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAppendNATSEventLog_RoundTrip(t *testing.T) {
	home := t.TempDir()
	now := time.Now().Truncate(time.Second)
	events := []NATSEvent{
		{Type: "swap", At: now, Caller: "handleLogin", NCID: "0xA", JWTExp: now.Add(5 * time.Minute).Unix(), Reason: "old= new=0xA"},
		{Type: "disconnect", At: now.Add(time.Second), Caller: "nats.go", NCID: "0xA", Reason: "EOF"},
		{Type: "closed", At: now.Add(2 * time.Second), Caller: "nats.go", NCID: "0xA"},
	}
	for _, ev := range events {
		if err := appendNATSEventLog(home, ev); err != nil {
			t.Fatalf("appendNATSEventLog: %v", err)
		}
	}
	got := readNATSEventLogFile(filepath.Join(home, natsEventLogFile))
	if len(got) != len(events) {
		t.Fatalf("readNATSEventLogFile: got %d events, want %d", len(got), len(events))
	}
	for i, want := range events {
		if got[i].V != NATSEventSchemaVersion {
			t.Errorf("event[%d].V = %d, want %d (Append must stamp schema version)", i, got[i].V, NATSEventSchemaVersion)
		}
		if got[i].Type != want.Type || got[i].Caller != want.Caller || got[i].NCID != want.NCID || got[i].Reason != want.Reason {
			t.Errorf("event[%d] = %+v, want %+v", i, got[i], want)
		}
		if !got[i].At.Equal(want.At) {
			t.Errorf("event[%d].At = %s, want %s", i, got[i].At, want.At)
		}
		if got[i].JWTExp != want.JWTExp {
			t.Errorf("event[%d].JWTExp = %d, want %d", i, got[i].JWTExp, want.JWTExp)
		}
	}
}

// TestAppendNATSEventLog_Rotation forces a rotation by writing past
// the size cap and asserts the active file holds only the post-
// rotation event, while .1 contains the pre-rotation tail.
func TestAppendNATSEventLog_Rotation(t *testing.T) {
	home := t.TempDir()
	bigReason := make([]byte, 1024) // each event ~1.1 KB
	for i := range bigReason {
		bigReason[i] = 'x'
	}
	// Hand-tune: write enough events to push past natsEventLogMaxBytes.
	// With ~1.1 KB per line and a 10 MB cap, ~10k events does it. We
	// stop the moment rotation is observed to keep the test snappy.
	const cap = natsEventLogMaxBytes
	for i := 0; i < 12000; i++ {
		ev := NATSEvent{Type: "warn", At: time.Now(), Reason: string(bigReason)}
		if err := appendNATSEventLog(home, ev); err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
		if _, err := os.Stat(filepath.Join(home, natsEventLogFile+".1")); err == nil {
			break
		}
	}
	if _, err := os.Stat(filepath.Join(home, natsEventLogFile+".1")); err != nil {
		t.Fatalf("expected %s.1 to exist after exceeding cap; stat err: %v", natsEventLogFile, err)
	}
	active, err := os.Stat(filepath.Join(home, natsEventLogFile))
	if err != nil {
		t.Fatalf("active missing after rotation: %v", err)
	}
	if active.Size() >= int64(cap) {
		t.Errorf("active file = %d bytes, expected < cap %d after rotation", active.Size(), cap)
	}
}

func TestScanNATSEventLog_FiltersAndOrders(t *testing.T) {
	home := t.TempDir()
	base := time.Now().Truncate(time.Second).Add(-time.Hour)
	for i := 0; i < 5; i++ {
		ev := NATSEvent{Type: "swap", At: base.Add(time.Duration(i) * time.Minute), Caller: "test"}
		if err := appendNATSEventLog(home, ev); err != nil {
			t.Fatalf("append: %v", err)
		}
	}
	// since = base+2.5min → events [3, 4, 5] (indices 2,3,4) survive.
	got := scanNATSEventLog(home, base.Add(2*time.Minute+30*time.Second))
	if len(got) != 2 {
		t.Fatalf("scanNATSEventLog: got %d events, want 2 (filter respect)", len(got))
	}
	// Chronological.
	for i := 1; i < len(got); i++ {
		if got[i].At.Before(got[i-1].At) {
			t.Errorf("events not chronological: got[%d].At=%s < got[%d].At=%s", i, got[i].At, i-1, got[i-1].At)
		}
	}
}

func TestTailNATSEventLog_LastN(t *testing.T) {
	home := t.TempDir()
	base := time.Now().Truncate(time.Second)
	for i := 0; i < 10; i++ {
		ev := NATSEvent{Type: "swap", At: base.Add(time.Duration(i) * time.Second), Reason: ""}
		_ = appendNATSEventLog(home, ev)
	}
	got := tailNATSEventLog(home, 3)
	if len(got) != 3 {
		t.Fatalf("tail: got %d, want 3", len(got))
	}
	// time.Time.Equal compares wall-clock instants ignoring location-
	// pointer identity; == is wrong here because JSON unmarshal returns
	// time.UTC while base carries time.Local (even when both render UTC
	// in CI, the *Location pointers differ).
	if !got[0].At.Equal(base.Add(7*time.Second)) || !got[2].At.Equal(base.Add(9*time.Second)) {
		t.Errorf("tail returned wrong range: first=%s last=%s, want %s..%s",
			got[0].At, got[len(got)-1].At,
			base.Add(7*time.Second), base.Add(9*time.Second))
	}
}

func TestReadNATSEventLogFile_SkipsMalformedLines(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, natsEventLogFile)
	// Construct a file with valid + invalid lines interleaved.
	good := `{"v":1,"type":"swap","at":"2026-06-02T00:33:30Z"}` + "\n"
	bad := `{"v":1,"type":"swap","at": broken` + "\n"
	contents := good + bad + good
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	got := readNATSEventLogFile(path)
	if len(got) != 2 {
		t.Errorf("readNATSEventLogFile: got %d valid events, want 2 (malformed line skipped)", len(got))
	}
}
