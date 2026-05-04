package cli

import (
	"fmt"
	"os"
	"strings"
)

func cmdAlert(args []string) error {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: ppz alert MESSAGE")
		os.Exit(2)
	}
	fmt.Fprintln(os.Stdout, strings.Join(args, " "))
	return nil
}
