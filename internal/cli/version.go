package cli

import (
	"fmt"
	"os"

	"github.com/pipescloud/ppz/internal/version"
)

// cmdVersion prints `ppz <version> (<sha>)` and exits 0. Doesn't talk to
// the daemon or the server — entirely local, useful for diagnosing which
// binary is on PATH.
func cmdVersion(args []string) error {
	_ = args
	fmt.Fprintf(os.Stdout, "ppz %s (%s)\n", version.Version, version.BuildSHA)
	maybeNotifyUpdate()
	return nil
}
