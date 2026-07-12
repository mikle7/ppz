package cli

import (
	"testing"
	"time"
)

// stdinRelaySinceMS must cap the replay-filter lookback so a long-lived
// wrapper never re-reads its whole retained stdin on redial (mikle7/ppz#1
// duplicate variant). A fresh wrapper is unaffected; a long-lived one is
// pinned to at most stdinRelayMaxLookback.
func TestStdinRelaySinceMS_CapsLookback(t *testing.T) {
	capMS := stdinRelayMaxLookback.Milliseconds()

	// Fresh wrapper: elapsed is tiny -> SinceMS ~= elapsed, well under the cap.
	if got := stdinRelaySinceMS(time.Now().Add(-500 * time.Millisecond)); got < 400 || got > capMS {
		t.Errorf("fresh wrapper: SinceMS=%d, want ~500 and <= cap %d", got, capMS)
	}

	// Long-lived wrapper (hours old): SinceMS MUST be clamped to the cap, not
	// grow to hours — that clamp is the whole fix.
	if got := stdinRelaySinceMS(time.Now().Add(-3 * time.Hour)); got != capMS+1 {
		t.Errorf("long-lived wrapper: SinceMS=%d, want capMS+1=%d (clamped)", got, capMS+1)
	}

	// Exactly at the cap boundary stays within the cap (+1 slack).
	if got := stdinRelaySinceMS(time.Now().Add(-stdinRelayMaxLookback)); got > capMS+1 {
		t.Errorf("at-cap wrapper: SinceMS=%d exceeds cap+1 %d", got, capMS+1)
	}
}
