package main

import (
	"fmt"
	"os"

	"github.com/pipescloud/ppz/internal/cli"
	"github.com/pipescloud/ppz/internal/cliproto"
)

func main() {
	if err := cli.Run(os.Args[1:]); err != nil {
		if e, ok := err.(*cliproto.Error); ok {
			cliproto.FprintError(os.Stderr, e)
			os.Exit(cliproto.ExitCode(e.Code))
		}
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
