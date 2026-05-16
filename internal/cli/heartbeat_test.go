package cli

import (
	"encoding/json"
	"reflect"
	"sort"
	"testing"
	"time"
)

func TestBuildHeartbeatPayload_PopulatesAllFields(t *testing.T) {
	now := time.Date(2026, 5, 16, 12, 34, 56, 0, time.UTC)
	started := time.Date(2026, 5, 16, 11, 0, 0, 0, time.UTC)
	in := heartbeatInputs{
		Now:         now,
		Seq:         42,
		Harness:     "claude",
		Model:       "opus",
		Hostname:    "jimmy-mbp",
		OS:          "darwin",
		Arch:        "arm64",
		PID:         12345,
		PPZVersion:  "0.32.0",
		StartedAt:   started,
		IntervalSec: 60,
	}

	got := buildHeartbeatPayload(in)

	if got.TS != "2026-05-16T12:34:56Z" {
		t.Errorf("ts = %q, want 2026-05-16T12:34:56Z", got.TS)
	}
	if got.Seq != 42 {
		t.Errorf("seq = %d, want 42", got.Seq)
	}
	if got.Harness != "claude" {
		t.Errorf("harness = %q, want claude", got.Harness)
	}
	if got.Model != "opus" {
		t.Errorf("model = %q, want opus", got.Model)
	}
	if got.Hostname != "jimmy-mbp" {
		t.Errorf("hostname = %q, want jimmy-mbp", got.Hostname)
	}
	if got.OS != "darwin" {
		t.Errorf("os = %q, want darwin", got.OS)
	}
	if got.Arch != "arm64" {
		t.Errorf("arch = %q, want arm64", got.Arch)
	}
	if got.PID != 12345 {
		t.Errorf("pid = %d, want 12345", got.PID)
	}
	if got.PPZVersion != "0.32.0" {
		t.Errorf("ppz_version = %q, want 0.32.0", got.PPZVersion)
	}
	if got.StartedAt != "2026-05-16T11:00:00Z" {
		t.Errorf("started_at = %q, want 2026-05-16T11:00:00Z", got.StartedAt)
	}
	if got.IntervalSec != 60 {
		t.Errorf("interval_sec = %d, want 60", got.IntervalSec)
	}
}

func TestBuildHeartbeatPayload_JSONShape(t *testing.T) {
	in := heartbeatInputs{
		Now:         time.Date(2026, 5, 16, 12, 34, 56, 0, time.UTC),
		Seq:         1,
		Harness:     "claude",
		Model:       "opus",
		Hostname:    "host",
		OS:          "darwin",
		Arch:        "arm64",
		PID:         1,
		PPZVersion:  "0.0.0",
		StartedAt:   time.Date(2026, 5, 16, 12, 34, 56, 0, time.UTC),
		IntervalSec: 60,
	}

	raw, err := json.Marshal(buildHeartbeatPayload(in))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	wantKeys := []string{
		"arch", "harness", "hostname", "interval_sec", "model",
		"os", "pid", "ppz_version", "seq", "started_at", "ts",
	}
	gotKeys := make([]string, 0, len(m))
	for k := range m {
		gotKeys = append(gotKeys, k)
	}
	sort.Strings(gotKeys)
	if !reflect.DeepEqual(gotKeys, wantKeys) {
		t.Fatalf("json keys = %v, want %v", gotKeys, wantKeys)
	}
}

func TestBuildHeartbeatPayload_EmptyHarnessAndModel(t *testing.T) {
	// Non-agent PTY sources (plain `ppz terminal share`) leave the env
	// vars unset. The payload should still include the keys, with empty
	// string values, so consumers see a consistent schema.
	in := heartbeatInputs{
		Now:         time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC),
		Seq:         1,
		Harness:     "",
		Model:       "",
		Hostname:    "h",
		OS:          "linux",
		Arch:        "amd64",
		PID:         1,
		PPZVersion:  "0.0.0",
		StartedAt:   time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC),
		IntervalSec: 60,
	}
	got := buildHeartbeatPayload(in)
	if got.Harness != "" {
		t.Errorf("harness = %q, want empty", got.Harness)
	}
	if got.Model != "" {
		t.Errorf("model = %q, want empty", got.Model)
	}

	raw, _ := json.Marshal(got)
	var m map[string]any
	_ = json.Unmarshal(raw, &m)
	if _, ok := m["harness"]; !ok {
		t.Errorf("harness key missing from JSON when empty")
	}
	if _, ok := m["model"]; !ok {
		t.Errorf("model key missing from JSON when empty")
	}
}
