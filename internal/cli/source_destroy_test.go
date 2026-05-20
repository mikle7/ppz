package cli

import (
	"reflect"
	"sort"
	"testing"

	"github.com/pipescloud/ppz/internal/cliproto"
)

// testSources is a fixed source list used across resolveDestroyTargets cases.
var testSources = []cliproto.Source{
	{
		Handle: "apple",
		Kind:   "pty",
		PipeInfos: []cliproto.PipeInfo{
			{Pipe: "broadcast"}, {Pipe: "stdin"}, {Pipe: "stdctrl"}, {Pipe: "stdout"},
		},
	},
	{
		Handle: "banana",
		Kind:   "message",
		PipeInfos: []cliproto.PipeInfo{
			{Pipe: "broadcast"},
		},
	},
	{
		Handle: "agent-one",
		Kind:   "message",
		PipeInfos: []cliproto.PipeInfo{
			{Pipe: "broadcast"}, {Pipe: "results"},
		},
	},
	{
		Handle: "agent-two",
		Kind:   "message",
		PipeInfos: []cliproto.PipeInfo{
			{Pipe: "broadcast"},
		},
	},
}

func TestResolveDestroyTargets_StarDestroysAllSources(t *testing.T) {
	srcs, pipes, err := resolveDestroyTargets("*", testSources)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pipes) != 0 {
		t.Errorf("expected no pipe targets, got %v", pipes)
	}
	sort.Strings(srcs)
	want := []string{"agent-one", "agent-two", "apple", "banana"}
	if !slicesEqual(srcs, want) {
		t.Errorf("got sources %v, want %v", srcs, want)
	}
}

func TestResolveDestroyTargets_LiteralHandleMatchesOne(t *testing.T) {
	srcs, pipes, err := resolveDestroyTargets("apple", testSources)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pipes) != 0 {
		t.Errorf("expected no pipe targets, got %v", pipes)
	}
	if len(srcs) != 1 || srcs[0] != "apple" {
		t.Errorf("got %v, want [apple]", srcs)
	}
}

func TestResolveDestroyTargets_HandleGlobMatchesSubset(t *testing.T) {
	srcs, pipes, err := resolveDestroyTargets("agent-*", testSources)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pipes) != 0 {
		t.Errorf("expected no pipe targets, got %v", pipes)
	}
	sort.Strings(srcs)
	want := []string{"agent-one", "agent-two"}
	if !slicesEqual(srcs, want) {
		t.Errorf("got %v, want %v", srcs, want)
	}
}

func TestResolveDestroyTargets_NoMatchIsEmpty(t *testing.T) {
	srcs, pipes, err := resolveDestroyTargets("cherry", testSources)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(srcs) != 0 || len(pipes) != 0 {
		t.Errorf("expected empty results, got srcs=%v pipes=%v", srcs, pipes)
	}
}

func TestResolveDestroyTargets_StarDotPipeDestroysMatchingPipesOnly(t *testing.T) {
	srcs, pipes, err := resolveDestroyTargets("*.stdout", testSources)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(srcs) != 0 {
		t.Errorf("expected no source targets, got %v", srcs)
	}
	if len(pipes) != 1 || pipes[0].Handle != "apple" || pipes[0].Name != "stdout" {
		t.Errorf("got %v, want [{apple stdout}]", pipes)
	}
}

func TestResolveDestroyTargets_LiteralHandleDotPipe(t *testing.T) {
	srcs, pipes, err := resolveDestroyTargets("apple.stdout", testSources)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(srcs) != 0 {
		t.Errorf("expected no source targets, got %v", srcs)
	}
	if len(pipes) != 1 || pipes[0].Handle != "apple" || pipes[0].Name != "stdout" {
		t.Errorf("got %v, want [{apple stdout}]", pipes)
	}
}

func TestResolveDestroyTargets_StarDotBroadcastMatchesAllSources(t *testing.T) {
	srcs, pipes, err := resolveDestroyTargets("*.broadcast", testSources)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(srcs) != 0 {
		t.Errorf("expected no source targets, got %v", srcs)
	}
	sort.Slice(pipes, func(i, j int) bool { return pipes[i].Handle < pipes[j].Handle })
	want := []cliproto.PipeDestroyRequest{
		{Handle: "agent-one", Name: "broadcast"},
		{Handle: "agent-two", Name: "broadcast"},
		{Handle: "apple", Name: "broadcast"},
		{Handle: "banana", Name: "broadcast"},
	}
	if !pipeRequestsEqual(pipes, want) {
		t.Errorf("got %v, want %v", pipes, want)
	}
}

func TestResolveDestroyTargets_HandleDotStarMatchesAllPipesOnSource(t *testing.T) {
	srcs, pipes, err := resolveDestroyTargets("apple.*", testSources)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(srcs) != 0 {
		t.Errorf("expected no source targets, got %v", srcs)
	}
	sort.Slice(pipes, func(i, j int) bool { return pipes[i].Name < pipes[j].Name })
	want := []cliproto.PipeDestroyRequest{
		{Handle: "apple", Name: "broadcast"},
		{Handle: "apple", Name: "stdctrl"},
		{Handle: "apple", Name: "stdin"},
		{Handle: "apple", Name: "stdout"},
	}
	if !pipeRequestsEqual(pipes, want) {
		t.Errorf("got %v, want %v", pipes, want)
	}
}

func TestResolveDestroyTargets_PipeGlobOnlyMatchesSourcesThatHaveIt(t *testing.T) {
	// Only agent-one has "results"; agent-two does not.
	srcs, pipes, err := resolveDestroyTargets("agent-*.results", testSources)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(srcs) != 0 {
		t.Errorf("expected no source targets, got %v", srcs)
	}
	if len(pipes) != 1 || pipes[0].Handle != "agent-one" || pipes[0].Name != "results" {
		t.Errorf("got %v, want [{agent-one results}]", pipes)
	}
}

func TestResolveDestroyTargets_EmptyPatternIsError(t *testing.T) {
	_, _, err := resolveDestroyTargets("", testSources)
	if err == nil {
		t.Fatal("expected error for empty pattern, got nil")
	}
}

func TestResolveDestroyTargets_EmptyHandleSideIsError(t *testing.T) {
	_, _, err := resolveDestroyTargets(".stdout", testSources)
	if err == nil {
		t.Fatal("expected error for '.stdout', got nil")
	}
}

func TestResolveDestroyTargets_EmptyPipeSideIsError(t *testing.T) {
	_, _, err := resolveDestroyTargets("apple.", testSources)
	if err == nil {
		t.Fatal("expected error for 'apple.', got nil")
	}
}

func TestResolveDestroyTargets_BadGlobIsError(t *testing.T) {
	_, _, err := resolveDestroyTargets("[bad", testSources)
	if err == nil {
		t.Fatal("expected error for invalid glob '[bad', got nil")
	}
}

func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func pipeRequestsEqual(a, b []cliproto.PipeDestroyRequest) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !reflect.DeepEqual(a[i], b[i]) {
			return false
		}
	}
	return true
}
