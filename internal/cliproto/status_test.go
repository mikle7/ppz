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
		AccountName:            "jamesmiles",
		LastTokenRefreshAt: &lastRefresh,
		Current:            "foo",
	}, "", "", false)

	want := "" +
		"daemon: logged in (pid=12953)\n" +
		"last token refresh: 5 minutes ago\n" +
		"server: https://pipescloud.io\n" +
		"account: jamesmiles\n" +
		"nats: unknown\n" +
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
		AccountName:   "jamesmiles",
		Current:   "foo",
	}, "", "", false)

	want := "" +
		"daemon: logged in (pid=12953)\n" +
		"last token refresh: -\n" +
		"server: https://pipescloud.io\n" +
		"account: jamesmiles\n" +
		"nats: unknown\n" +
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
				AccountName:            "jamesmiles",
				LastTokenRefreshAt: &tt.lastRefresh,
				Current:            "foo",
			}, "", "", true)

			if got := b.String(); !bytes.Contains([]byte(got), []byte(tt.wantLine)) {
				t.Fatalf("status output missing colored refresh line\nwant line: %q\ngot:\n%q", tt.wantLine, got)
			}
		})
	}
}

func TestPrintStatus_ColorsServerAndAccountValuesWhenSet(t *testing.T) {
	var b bytes.Buffer
	PrintStatusWithEnv(&b, StatusReply{
		DaemonPID: 12953,
		LoggedIn:  true,
		URL:       "https://pipescloud.io",
		AccountName:   "jamesmiles",
		Current:   "foo",
	}, "", "", true)

	mustContain := []string{
		"server: \x1b[32mhttps://pipescloud.io\x1b[0m\n",
		"account: \x1b[32mjamesmiles\x1b[0m\n",
	}
	for _, want := range mustContain {
		if got := b.String(); !bytes.Contains([]byte(got), []byte(want)) {
			t.Fatalf("status output missing colored line\nwant line: %q\ngot:\n%q", want, got)
		}
	}
}

func TestPrintStatus_IncludesDaemonVersionMatch(t *testing.T) {
	var b bytes.Buffer
	PrintStatusWithEnvAndCLIVersion(&b, StatusReply{
		DaemonPID:     12953,
		DaemonVersion: "v0.18.0",
		LoggedIn:      true,
		URL:           "https://pipescloud.io",
		AccountName:       "jamesmiles",
		Current:       "foo",
	}, "", "", false, "v0.18.0")

	want := "" +
		"daemon: logged in (pid=12953), v0.18.0 (latest)\n" +
		"last token refresh: -\n" +
		"server: https://pipescloud.io\n" +
		"account: jamesmiles\n" +
		"nats: unknown\n" +
		"current source: foo\n"
	if got := b.String(); got != want {
		t.Fatalf("status output mismatch\nwant:\n%q\ngot:\n%q", want, got)
	}
}

func TestPrintStatus_ColorsDaemonVersionByCLIMatch(t *testing.T) {
	tests := []struct {
		name          string
		daemonVersion string
		cliVersion    string
		wantLine      string
	}{
		{
			name:          "green when daemon matches cli",
			daemonVersion: "v0.18.0",
			cliVersion:    "v0.18.0",
			wantLine:      "daemon: \x1b[32mlogged in\x1b[0m (pid=12953), \x1b[32mv0.18.0\x1b[0m (latest)\n",
		},
		{
			name:          "red when daemon differs from cli",
			daemonVersion: "v0.17.9",
			cliVersion:    "v0.18.0",
			wantLine:      "daemon: \x1b[32mlogged in\x1b[0m (pid=12953), \x1b[31mv0.17.9\x1b[0m (not latest)\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var b bytes.Buffer
			PrintStatusWithEnvAndCLIVersion(&b, StatusReply{
				DaemonPID:     12953,
				DaemonVersion: tt.daemonVersion,
				LoggedIn:      true,
				URL:           "https://pipescloud.io",
				AccountName:       "jamesmiles",
				Current:       "foo",
			}, "", "", true, tt.cliVersion)

			if got := b.String(); !bytes.Contains([]byte(got), []byte(tt.wantLine)) {
				t.Fatalf("status output missing colored daemon version line\nwant line: %q\ngot:\n%q", tt.wantLine, got)
			}
		})
	}
}

func TestPrintStatus_ClarifiesUnknownDaemonVersion(t *testing.T) {
	var b bytes.Buffer
	PrintStatusWithEnvAndCLIVersion(&b, StatusReply{
		DaemonPID: 12953,
		LoggedIn:  true,
		URL:       "https://pipescloud.io",
		AccountName:   "jamesmiles",
		Current:   "foo",
	}, "", "", false, "v0.18.0")

	wantLine := "daemon: logged in (pid=12953), version unknown (not latest)\n"
	if got := b.String(); !bytes.Contains([]byte(got), []byte(wantLine)) {
		t.Fatalf("status output missing clarified unknown daemon version\nwant line: %q\ngot:\n%q", wantLine, got)
	}
}
