//go:build darwin

package cli

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

func init() {
	parentPIDForPID = darwinParentPIDForPID
}

// darwinParentPIDForPID returns the parent pid by shelling out to
// `ps -o ppid= -p <pid>`. ps reads the kernel proc table via the same
// sysctl path the kernel exposes for ps(1); we delegate to keep the
// code portable across darwin versions without having to chase struct
// layout changes in kinfo_proc.
//
// Walk depth is capped at MaxAncestorWalkDepth (8), so worst case is
// 8 ps invocations per IPC. ps is a small statically-linked binary;
// per-call cost is ~1-2ms on macOS — acceptable for the CLI hot path
// (each `ppz` invocation calls this once).
func darwinParentPIDForPID(pid int) (int, error) {
	out, err := exec.Command("ps", "-o", "ppid=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return 0, fmt.Errorf("ps: %w", err)
	}
	s := strings.TrimSpace(string(out))
	if s == "" {
		return 0, fmt.Errorf("ps returned no ppid for pid %d", pid)
	}
	ppid, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("ps returned unparseable ppid %q: %w", s, err)
	}
	return ppid, nil
}
