// Package clock returns the current time, honoring PPZ_TEST_CLOCK if set.
// Per WIRE.md §10: PPZ_TEST_CLOCK, when set to an RFC3339 timestamp, freezes
// the clock for deterministic tests.
package clock

import (
	"os"
	"sync"
	"time"
)

var (
	once   sync.Once
	frozen *time.Time
)

func Now() time.Time {
	once.Do(func() {
		if v := os.Getenv("PPZ_TEST_CLOCK"); v != "" {
			if t, err := time.Parse(time.RFC3339, v); err == nil {
				frozen = &t
			}
		}
	})
	if frozen != nil {
		return *frozen
	}
	return time.Now().UTC()
}
