//go:build linux

package cli

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

func init() {
	parentPIDForPID = linuxParentPIDForPID
}

// linuxParentPIDForPID reads /proc/<pid>/status and extracts the PPid:
// field. Returns 0 + error when the process no longer exists (ESRCH
// would surface as "no such file or directory" when reading
// /proc/<pid>/status).
func linuxParentPIDForPID(pid int) (int, error) {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
	if err != nil {
		return 0, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(line, "PPid:") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return 0, errors.New("malformed PPid line")
		}
		ppid, err := strconv.Atoi(fields[1])
		if err != nil {
			return 0, err
		}
		return ppid, nil
	}
	return 0, errors.New("PPid field not found")
}
