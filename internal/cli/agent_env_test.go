package cli

import (
	"os"
	"reflect"
	"strings"
	"testing"
)

// agentEnvPairs builds the env-var assignments the pty wrapper reads
// to stamp every heartbeat. Always two keys: harness + model. Model may
// be empty when the agent harness has no default (e.g. copilot/codex
// without --model).
func TestAgentEnvPairs_ClaudeOpus(t *testing.T) {
	got := agentEnvPairs(agentSpec{harness: "claude", model: "opus"})
	want := []string{"PPZ_AGENT_HARNESS=claude", "PPZ_AGENT_MODEL=opus"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("agentEnvPairs = %v, want %v", got, want)
	}
}

func TestAgentEnvPairs_CodexNoModel(t *testing.T) {
	got := agentEnvPairs(agentSpec{harness: "codex", model: ""})
	want := []string{"PPZ_AGENT_HARNESS=codex", "PPZ_AGENT_MODEL="}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("agentEnvPairs = %v, want %v", got, want)
	}
}

// setAgentEnv exports PPZ_AGENT_HARNESS + PPZ_AGENT_MODEL into the
// current process env so the in-process cmdTerminalShare call inherits
// them via os.Environ() inside terminalShareEnv.
func TestSetAgentEnv_ExportsHarnessAndModel(t *testing.T) {
	t.Setenv("PPZ_AGENT_HARNESS", "")
	t.Setenv("PPZ_AGENT_MODEL", "")
	setAgentEnv(agentSpec{harness: "claude", model: "opus"})
	if got := os.Getenv("PPZ_AGENT_HARNESS"); got != "claude" {
		t.Errorf("PPZ_AGENT_HARNESS = %q, want claude", got)
	}
	if got := os.Getenv("PPZ_AGENT_MODEL"); got != "opus" {
		t.Errorf("PPZ_AGENT_MODEL = %q, want opus", got)
	}
}

// Once setAgentEnv has run, terminalShareEnv picks the agent env up via
// os.Environ() and threads it to the wrapped child.
func TestTerminalShareEnv_IncludesAgentEnvOnceSet(t *testing.T) {
	t.Setenv("PPZ_AGENT_HARNESS", "claude")
	t.Setenv("PPZ_AGENT_MODEL", "opus")
	env := terminalShareEnv("alice")
	if !envContains(env, "PPZ_AGENT_HARNESS=claude") {
		t.Errorf("terminalShareEnv missing PPZ_AGENT_HARNESS=claude; got %v", env)
	}
	if !envContains(env, "PPZ_AGENT_MODEL=opus") {
		t.Errorf("terminalShareEnv missing PPZ_AGENT_MODEL=opus; got %v", env)
	}
}

// --new-window builders inject env-var assignments before the
// `ppz terminal share` invocation so the spawned wrapper sees the
// agent identity. Format: `env KEY=VAL KEY=VAL ppz terminal share ...`.
// The `env` prefix is portable across bash/zsh/dash without needing a
// shell builtin.
func TestBuildNewWindowScript_InjectsAgentEnvPairs(t *testing.T) {
	pairs := []string{"PPZ_AGENT_HARNESS=claude", "PPZ_AGENT_MODEL=opus"}
	cmd := buildNewWindowScript("Apple_Terminal", "alice", "", pairs, []string{"claude"})
	if !strings.Contains(cmd, "env PPZ_AGENT_HARNESS=claude PPZ_AGENT_MODEL=opus ppz terminal share") {
		t.Errorf("expected env-prefixed share invocation, got:\n%s", cmd)
	}
}

func TestBuildLinuxNewWindowArgv_InjectsAgentEnvPairs(t *testing.T) {
	pairs := []string{"PPZ_AGENT_HARNESS=claude", "PPZ_AGENT_MODEL=opus"}
	argv, err := buildLinuxNewWindowArgv("xterm", "alice", "", pairs, []string{"claude"})
	if err != nil {
		t.Fatalf("buildLinuxNewWindowArgv: %v", err)
	}
	joined := strings.Join(argv, " ")
	if !strings.Contains(joined, "env PPZ_AGENT_HARNESS=claude PPZ_AGENT_MODEL=opus ppz terminal share") {
		t.Errorf("expected env-prefixed share invocation in argv, got:\n%s", joined)
	}
}

func TestBuildWSLNewWindowArgv_InjectsAgentEnvPairs(t *testing.T) {
	pairs := []string{"PPZ_AGENT_HARNESS=claude", "PPZ_AGENT_MODEL=opus"}
	argv, err := buildWSLNewWindowArgv("Ubuntu", "alice", "", pairs, []string{"claude"})
	if err != nil {
		t.Fatalf("buildWSLNewWindowArgv: %v", err)
	}
	joined := strings.Join(argv, " ")
	if !strings.Contains(joined, "env PPZ_AGENT_HARNESS=claude PPZ_AGENT_MODEL=opus ppz terminal share") {
		t.Errorf("expected env-prefixed share invocation in argv, got:\n%s", joined)
	}
}
