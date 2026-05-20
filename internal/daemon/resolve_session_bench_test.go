package daemon

import "testing"

// Benchmarks for the refined ppid-based resolver. Janitor / heartbeat
// benchmarks from the original draft are gone with those features.

// PF-A: resolver on the hot path — ancestor at depth 1, the common
// case. Runs on every session-using IPC; baseline target <10µs.
func BenchmarkResolveSession_AncestorDepth1(b *testing.B) {
	s := NewState(b.TempDir())
	if _, err := s.RegisterAgentBinding("cindy", 41203); err != nil {
		b.Fatalf("register: %v", err)
	}

	chain := []int{50000, 41203}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = s.ResolveSession("", chain)
	}
}

// PF-B: resolver on the cold path — ancestor not found, falls through
// to the legacy fallback. Budget: still <50µs.
func BenchmarkResolveSession_Fallback(b *testing.B) {
	s := NewState(b.TempDir())
	chain := []int{50000, 50001, 50002}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = s.ResolveSession("", chain)
	}
}
