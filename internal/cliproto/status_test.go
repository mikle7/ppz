package cliproto

import (
	"bytes"
	"testing"
	"time"
)

func TestPrintStatus_IncludesLastTokenRefreshRelativeTime(t *testing.T) {
	now := time.Date(2026, 5, 3, 6, 44, 0, 0, time.UTC)
	oldTimeNow := timeNow
	timeNow = func() time.Time { return now }
	t.Cleanup(func() { timeNow = oldTimeNow })

	lastRefresh := now.Add(-5 * time.Minute)
	var b bytes.Buffer
	PrintStatusWithEnv(&b, StatusReply{
		DaemonPID:          12953,
		LoggedIn:           true,
		URL:                "https://pipescloud.io",
		OrgName:            "jamesmiles",
		LastTokenRefreshAt: &lastRefresh,
		Current:            "foo",
	}, "", "", false)

	want := "" +
		"daemon: logged in (pid=12953)\n" +
		"last token refresh: 5 minutes ago\n" +
		"server: https://pipescloud.io\n" +
		"org: jamesmiles\n" +
		"current source: foo\n"
	if got := b.String(); got != want {
		t.Fatalf("status output mismatch\nwant:\n%q\ngot:\n%q", want, got)
	}
}

func TestPrintStatus_IncludesMissingLastTokenRefreshPlaceholder(t *testing.T) {
	var b bytes.Buffer
	PrintStatusWithEnv(&b, StatusReply{
		DaemonPID: 12953,
		LoggedIn:  true,
		URL:       "https://pipescloud.io",
		OrgName:   "jamesmiles",
		Current:   "foo",
	}, "", "", false)

	want := "" +
		"daemon: logged in (pid=12953)\n" +
		"last token refresh: -\n" +
		"server: https://pipescloud.io\n" +
		"org: jamesmiles\n" +
		"current source: foo\n"
	if got := b.String(); got != want {
		t.Fatalf("status output mismatch\nwant:\n%q\ngot:\n%q", want, got)
	}
}

func TestPrintStatus_ColorsLastTokenRefreshAgeByThreshold(t *testing.T) {
	now := time.Date(2026, 5, 3, 6, 44, 0, 0, time.UTC)
	oldTimeNow := timeNow
	timeNow = func() time.Time { return now }
	t.Cleanup(func() { timeNow = oldTimeNow })

	tests := []struct {
		name        string
		lastRefresh time.Time
		wantLine    string
	}{
		{
			name:        "green under five minutes",
			lastRefresh: now.Add(-4*time.Minute - 59*time.Second),
			wantLine:    "last token refresh: \x1b[32m4 minutes ago\x1b[0m\n",
		},
		{
			name:        "red at five minutes",
			lastRefresh: now.Add(-5 * time.Minute),
			wantLine:    "last token refresh: \x1b[31m5 minutes ago\x1b[0m\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var b bytes.Buffer
			PrintStatusWithEnv(&b, StatusReply{
				DaemonPID:          12953,
				LoggedIn:           true,
				URL:                "https://pipescloud.io",
				OrgName:            "jamesmiles",
				LastTokenRefreshAt: &tt.lastRefresh,
				Current:            "foo",
			}, "", "", true)

			if got := b.String(); !bytes.Contains([]byte(got), []byte(tt.wantLine)) {
				t.Fatalf("status output missing colored refresh line\nwant line: %q\ngot:\n%q", tt.wantLine, got)
			}
		})
	}
}

func TestPrintStatus_ColorsServerAndOrgValuesWhenSet(t *testing.T) {
	var b bytes.Buffer
	PrintStatusWithEnv(&b, StatusReply{
		DaemonPID: 12953,
		LoggedIn:  true,
		URL:       "https://pipescloud.io",
		OrgName:   "jamesmiles",
		Current:   "foo",
	}, "", "", true)

	mustContain := []string{
		"server: \x1b[32mhttps://pipescloud.io\x1b[0m\n",
		"org: \x1b[32mjamesmiles\x1b[0m\n",
	}
	for _, want := range mustContain {
		if got := b.String(); !bytes.Contains([]byte(got), []byte(want)) {
			t.Fatalf("status output missing colored line\nwant line: %q\ngot:\n%q", want, got)
		}
	}
}
