package cliproto

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

// loadFixture reads the captured Claude TUI session bytes — a real
// `peek --raw` dump from a wrapped session containing a "tell me a story"
// prompt and the rendered story output. ~13 KB of OSC/CSI/charset/single-
// shot/bracketed-paste escapes interleaved with the story text.
//
// Re-record by:
//
//	./bin/ppz connect <fresh-handle>
//	./bin/ppz terminal share
//	# inside: claude, ask for a story, exit
//	./bin/ppz read <handle>.stdout --raw > internal/cliproto/testdata/claude-session.bin
//
// Re-recording shouldn't be needed unless protocol noise changes
// meaningfully (Claude version bump emits new escape patterns, etc.).
func loadFixture(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/claude-session.bin")
	if err != nil {
		t.Fatalf("load fixture: %v — re-record by piping `peek --raw` into testdata/claude-session.bin", err)
	}
	if len(b) < 1000 {
		t.Fatalf("fixture suspiciously small (%d bytes); re-record it", len(b))
	}
	return b
}

// TestRender_StoryTextSurvivesIntact: feed the real fixture through the
// emulator and verify specific lines of story text appear verbatim in
// the rendered snapshot — *with proper inter-word spaces* (the regression
// the strip-based approach could never get right).
//
// These literal strings come from the captured session — if you re-record
// against a different prompt, update these assertions to match.
func TestRender_StoryTextSurvivesIntact(t *testing.T) {
	out := RenderTerminal(loadFixture(t), DefaultRenderCols, DefaultRenderRows)

	mustContain := []string{
		"\"You're just control bytes,\" peek said, stripping them down to plain text out of habit.",
		"The stream laughed.",
		"peek thought about this.",
	}
	for _, s := range mustContain {
		if !strings.Contains(out, s) {
			t.Errorf("rendered output missing expected string %q\n--- snapshot ---\n%s\n--- end ---", s, out)
		}
	}
}

// TestRender_NoEscapePollution: the rendered snapshot must contain zero
// ESC bytes and zero leaked OSC/CSI body fragments. (vt10x consumes
// these as part of emulation; this is the "we didn't accidentally pass
// through raw bytes" guard.)
func TestRender_NoEscapePollution(t *testing.T) {
	out := []byte(RenderTerminal(loadFixture(t), DefaultRenderCols, DefaultRenderRows))

	if i := bytes.IndexByte(out, 0x1B); i >= 0 {
		t.Errorf("ESC byte at offset %d in rendered output: context=%q", i, snippet(out, i, 32))
	}
	leaks := []string{"]0;", "]7;", "]8;", "[?2004", "[?1049", "[?25"}
	for _, l := range leaks {
		if idx := bytes.Index(out, []byte(l)); idx >= 0 {
			t.Errorf("escape-body marker %q leaked at offset %d: context=%q", l, idx, snippet(out, idx, 48))
		}
	}
}

// TestRender_ChunkedConcatThenRender confirms the contract documented
// on RenderTerminal: callers feeding chunked input must concatenate
// before rendering. This test is a positive — split the fixture into
// arbitrary chunks, concatenate, render — same result as one-shot. If
// vt10x's Write is ever called per-chunk inside the renderer (it MUST
// not be), this regresses.
func TestRender_ChunkedConcatThenRender(t *testing.T) {
	in := loadFixture(t)
	want := RenderTerminal(in, DefaultRenderCols, DefaultRenderRows)

	// Concatenate "chunks" (any sizes) — should be byte-identical to in,
	// so the render must match.
	for _, chunkSize := range []int{1, 7, 37, 256, 1024} {
		var concat []byte
		for i := 0; i < len(in); i += chunkSize {
			end := i + chunkSize
			if end > len(in) {
				end = len(in)
			}
			concat = append(concat, in[i:end]...)
		}
		if got := RenderTerminal(concat, DefaultRenderCols, DefaultRenderRows); got != want {
			t.Errorf("chunkSize=%d: render of concatenated chunks ≠ render of one-shot", chunkSize)
		}
	}
}

// TestRender_ShellPromptCleanish: the section of the fixture before the
// claude TUI starts is just a shell prompt + `ls -1` output. After
// rendering, that section should look readable: no doubled letters from
// zsh autosuggest redraws, file names on their own lines.
func TestRender_ShellPromptCleanish(t *testing.T) {
	out := RenderTerminal(loadFixture(t), DefaultRenderCols, DefaultRenderRows)

	// The fixture's `ls -1` lists the files at repo root. Pick a few
	// that are stable in this repo and assert each is on its own line.
	for _, name := range []string{"compose", "go.mod", "Makefile", "README.md", "tests"} {
		// Look for "\n<name>\n" so we know it's not embedded in some
		// other line by accident.
		needle := "\n" + name + "\n"
		if !strings.Contains(out, needle) {
			t.Errorf("expected %q on its own line in rendered ls output\n--- snapshot ---\n%s\n--- end ---",
				name, out)
		}
	}
}

// TestRender_ClaudeInlineSessionMatchesGroundTruth is the spec for `--tty`:
// given a real wrapped Claude session's raw PTY bytes, the rendered output
// must match what the user actually sees on screen — Claude header + the
// "tell me another story" input + the full story body + input box + the
// post-exit "Resume this session…" hint + the shell prompt + a couple of
// follow-up commands. ~32 lines of content for this fixture.
//
// Ground truth is `testdata/claude-inline-session.expected.txt`, captured
// by feeding the same fixture bytes through tmux at the same dimensions
// (200×60, history-limit 0). That render matches the user's iTerm 1:1.
//
// Re-record by:
//
//	# capture session bytes:
//	./bin/ppz read <handle>.stdout --raw > /tmp/raw.bin
//	cp /tmp/raw.bin internal/cliproto/testdata/claude-inline-session.bin
//	# regenerate expected via tmux:
//	tmux -L spec -f /dev/null new-session -d -x 200 -y 60 -s s \
//	    "bash -c 'cat internal/cliproto/testdata/claude-inline-session.bin; \
//	              tail -f /dev/null'"
//	tmux -L spec set-option -t s history-limit 0
//	sleep 1
//	tmux -L spec capture-pane -t s -p \
//	    | awk 'NR==FNR{if($0!="")last=NR;next} FNR<=last' /dev/stdin /dev/stdin \
//	    > internal/cliproto/testdata/claude-inline-session.expected.txt
//	tmux -L spec kill-server
func TestRender_ClaudeInlineSessionMatchesGroundTruth(t *testing.T) {
	in, err := os.ReadFile("testdata/claude-inline-session.bin")
	if err != nil {
		t.Fatalf("load fixture: %v", err)
	}
	wantBytes, err := os.ReadFile("testdata/claude-inline-session.expected.txt")
	if err != nil {
		t.Fatalf("load expected: %v", err)
	}
	want := string(wantBytes)
	got := RenderTerminal(in, 200, 60)

	if got == want {
		return
	}
	// Diff-friendly failure: line-by-line so we can see exactly where
	// the render diverges from ground truth.
	gotLines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	wantLines := strings.Split(strings.TrimRight(want, "\n"), "\n")
	maxN := len(gotLines)
	if len(wantLines) > maxN {
		maxN = len(wantLines)
	}
	var diff strings.Builder
	for i := 0; i < maxN; i++ {
		var gl, wl string
		if i < len(gotLines) {
			gl = gotLines[i]
		}
		if i < len(wantLines) {
			wl = wantLines[i]
		}
		if gl == wl {
			continue
		}
		diff.WriteString("line ")
		diff.WriteString(itoa(i + 1))
		diff.WriteString(":\n  want: ")
		diff.WriteString(wl)
		diff.WriteString("\n  got:  ")
		diff.WriteString(gl)
		diff.WriteString("\n")
	}
	t.Errorf("rendered output ≠ ground truth\n%s", diff.String())
}

// TestRender_ResizeSessionMatchesGroundTruth_AtSourceDims is the spec for
// the dimension-aware `--tty` path: when we render at the source pty's
// actual dimensions (212×57, captured live from `quax.stdctrl`), the
// output must match what tmux produces from the same bytes at the same
// dimensions. Locks the bug as "wiring problem, not vt10x problem".
//
// Re-record by:
//
//	./bin/ppz reread <handle>.stdout --raw \
//	  > internal/cliproto/testdata/claude-resize-session.bin
//	./bin/ppz reread <handle>.stdctrl --json \
//	  > internal/cliproto/testdata/claude-resize-session.stdctrl.jsonl
//	# pull the latest cols/rows from stdctrl, then:
//	tmux -L spec -f /dev/null new-session -d -x <cols> -y <rows> -s s \
//	    "bash -c 'cat internal/cliproto/testdata/claude-resize-session.bin; tail -f /dev/null'"
//	tmux -L spec set-option -t s history-limit 0
//	sleep 1
//	tmux -L spec capture-pane -t s -p \
//	    | awk 'BEGIN{n=0} {a[NR]=$0; if($0 !~ /^[[:space:]]*$/) last=NR} END{for(i=1;i<=last;i++) print a[i]}' \
//	    > internal/cliproto/testdata/claude-resize-session.expected.txt
//	tmux -L spec kill-server
func TestRender_ResizeSessionMatchesGroundTruth_AtSourceDims(t *testing.T) {
	in, err := os.ReadFile("testdata/claude-resize-session.bin")
	if err != nil {
		t.Fatalf("load fixture: %v", err)
	}
	wantBytes, err := os.ReadFile("testdata/claude-resize-session.expected.txt")
	if err != nil {
		t.Fatalf("load expected: %v", err)
	}
	want := string(wantBytes)
	got := RenderTerminal(in, 212, 57)

	if got == want {
		return
	}
	gotLines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	wantLines := strings.Split(strings.TrimRight(want, "\n"), "\n")
	maxN := len(gotLines)
	if len(wantLines) > maxN {
		maxN = len(wantLines)
	}
	var diff strings.Builder
	for i := 0; i < maxN; i++ {
		var gl, wl string
		if i < len(gotLines) {
			gl = gotLines[i]
		}
		if i < len(wantLines) {
			wl = wantLines[i]
		}
		if gl == wl {
			continue
		}
		diff.WriteString("line ")
		diff.WriteString(itoa(i + 1))
		diff.WriteString(":\n  want: ")
		diff.WriteString(wl)
		diff.WriteString("\n  got:  ")
		diff.WriteString(gl)
		diff.WriteString("\n")
	}
	t.Errorf("rendered output ≠ ground truth\n%s", diff.String())
}

// TestRender_AppleSessionMatchesGroundTruth_AtSourceDims is the
// regression pin for the multi-frame-redraw class of vt10x bugs. The
// earlier `claude-resize-session` fixture renders correctly at the
// source dimensions because its byte stream is mostly linear; this
// fixture (captured from a Claude TUI with rotating "Stewing… /
// Churned for Ns / esc to interrupt" status frames) exposes vt10x's
// failure to clear cells when the cursor moves over them — visible
// today as stale animation lines, residual chars merging adjacent
// words ("passenger: a boy" → "passenger:earboy"), and extra divider
// rows that should have been overwritten.
//
// tmux at the same 212×57 produces a clean render. This test diffs
// our RenderTerminal output against tmux's. RED today; will go GREEN
// once vt10x is patched (or replaced — see the renderer plan).
//
// Re-record by:
//
//	./bin/ppz reread <h>.stdout --raw \
//	  > internal/cliproto/testdata/apple-session.bin
//	./bin/ppz reread <h>.stdctrl --json \
//	  > internal/cliproto/testdata/apple-session.stdctrl.jsonl
//	# pull cols/rows from the latest stdctrl entry, then:
//	tmux -L spec -f /dev/null new-session -d -x 212 -y 57 -s s \
//	    "bash -c 'cat internal/cliproto/testdata/apple-session.bin; tail -f /dev/null'"
//	tmux -L spec set-option -t s history-limit 0
//	sleep 1
//	tmux -L spec capture-pane -t s -p \
//	    > internal/cliproto/testdata/apple-session.expected.txt
//	tmux -L spec kill-server
func TestRender_AppleSessionMatchesGroundTruth_AtSourceDims(t *testing.T) {
	in, err := os.ReadFile("testdata/apple-session.bin")
	if err != nil {
		t.Fatalf("load fixture: %v", err)
	}
	wantBytes, err := os.ReadFile("testdata/apple-session.expected.txt")
	if err != nil {
		t.Fatalf("load expected: %v", err)
	}
	want := string(wantBytes)
	got := RenderTerminal(in, 212, 57)

	if got == want {
		return
	}
	gotLines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	wantLines := strings.Split(strings.TrimRight(want, "\n"), "\n")
	maxN := len(gotLines)
	if len(wantLines) > maxN {
		maxN = len(wantLines)
	}
	var diff strings.Builder
	for i := 0; i < maxN; i++ {
		var gl, wl string
		if i < len(gotLines) {
			gl = gotLines[i]
		}
		if i < len(wantLines) {
			wl = wantLines[i]
		}
		if gl == wl {
			continue
		}
		diff.WriteString("line ")
		diff.WriteString(itoa(i + 1))
		diff.WriteString(":\n  want: ")
		diff.WriteString(wl)
		diff.WriteString("\n  got:  ")
		diff.WriteString(gl)
		diff.WriteString("\n")
	}
	t.Errorf("rendered output ≠ ground truth\n%s", diff.String())
}

// TestRender_ResizeSessionGarblesAtDefaultDims pins the diagnosis: the
// same bytes rendered at the hardcoded 200×60 grid do NOT match the
// 212-col ground truth. If this test ever starts passing it means we
// either fixed the underlying vt10x wrap bug or accidentally widened
// DefaultRenderCols — either way, worth a human look.
func TestRender_ResizeSessionGarblesAtDefaultDims(t *testing.T) {
	in, err := os.ReadFile("testdata/claude-resize-session.bin")
	if err != nil {
		t.Fatalf("load fixture: %v", err)
	}
	wantBytes, err := os.ReadFile("testdata/claude-resize-session.expected.txt")
	if err != nil {
		t.Fatalf("load expected: %v", err)
	}
	want := string(wantBytes)
	got := RenderTerminal(in, DefaultRenderCols, DefaultRenderRows)
	if got == want {
		t.Fatal("expected garbled output at 200×60 (source was 212×57); " +
			"got an exact match — has the underlying bug been fixed?")
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

// TestRender_EmptyInput keeps the trivial case honest.
func TestRender_EmptyInput(t *testing.T) {
	if got := RenderTerminal(nil, 80, 24); got != "" {
		t.Errorf("empty input should render as empty string; got %q", got)
	}
	if got := RenderTerminal([]byte{}, 80, 24); got != "" {
		t.Errorf("empty input (zero-length slice) should render empty; got %q", got)
	}
}

// TestRender_TrailingBlankRowsTrimmed: when a small payload doesn't fill
// the virtual screen, the snapshot should be tight — just the rows with
// content, not 60 newlines. Note: VT100 `\n` is line-feed only (cursor
// moves down, column unchanged); real PTY output uses `\r\n` (OPOST
// expands `\n` to that). Test uses both to mirror real behaviour.
func TestRender_TrailingBlankRowsTrimmed(t *testing.T) {
	out := RenderTerminal([]byte("hello\r\nworld"), 80, 24)
	if got, want := strings.Count(out, "\n"), 2; got != want {
		t.Errorf("trailing rows not trimmed: want %d newlines, got %d (output=%q)", want, got, out)
	}
	if !strings.HasPrefix(out, "hello\nworld\n") {
		t.Errorf("unexpected prefix: %q", out)
	}
}

// --- helpers ---

func snippet(b []byte, at, width int) string {
	if at < 0 {
		at = 0
	}
	end := at + width
	if end > len(b) {
		end = len(b)
	}
	return strings.Map(func(r rune) rune {
		if r == 0x1B {
			return '␛'
		}
		if r < 0x20 || r == 0x7F {
			return '·'
		}
		return r
	}, string(b[at:end]))
}
