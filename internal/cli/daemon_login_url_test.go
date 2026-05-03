package cli

import "testing"

func TestNormaliseLoginURL(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		// Bare hostname → assume https.
		{"pipescloud.io", "https://pipescloud.io"},

		// Hostname + path → assume https, preserve path.
		{"pipescloud.io/api", "https://pipescloud.io/api"},

		// localhost defaults to http (no TLS in local dev).
		{"localhost:8080", "http://localhost:8080"},
		{"localhost", "http://localhost"},

		// 127.0.0.1 / 0.0.0.0 → http (local dev).
		{"127.0.0.1:8080", "http://127.0.0.1:8080"},

		// Compose-style internal hostnames → http (no TLS inside the network).
		{"ppz-server:8080", "http://ppz-server:8080"},

		// Explicit schemes pass through unchanged (modulo trailing slash trim).
		{"https://pipescloud.io", "https://pipescloud.io"},
		{"https://pipescloud.io/", "https://pipescloud.io"},
		{"http://localhost:8080", "http://localhost:8080"},
		{"http://localhost:8080/", "http://localhost:8080"},

		// Trailing slash is trimmed off the bare-host form too.
		{"pipescloud.io/", "https://pipescloud.io"},
	}
	for _, c := range cases {
		got := normaliseLoginURL(c.in)
		if got != c.want {
			t.Errorf("normaliseLoginURL(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
