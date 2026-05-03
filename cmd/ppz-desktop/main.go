package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/pipescloud/ppz/internal/desktop"
)

func main() {
	headless := flag.Bool("headless", false, "run as a long-lived service (no window). Pair with --http-addr to expose a browser viewer.")
	dump := flag.Bool("dump-state", false, "print one JSON snapshot of pipe state and exit")
	ipc := flag.String("ipc", "", "path to the daemon IPC socket")
	httpAddr := flag.String("http-addr", "", "in --headless mode, also serve the browser viewer on this addr (e.g. :9090)")
	flag.Parse()

	if *ipc == "" {
		if v := os.Getenv("PPZ_IPC_SOCKET"); v != "" {
			*ipc = v
		}
	}
	if *ipc == "" {
		fmt.Fprintln(os.Stderr, "ppz-desktop: --ipc=<sock> or PPZ_IPC_SOCKET required")
		os.Exit(2)
	}

	if *dump {
		if err := desktop.DumpState(*ipc, os.Stdout); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		return
	}
	if *headless {
		if err := desktop.RunHeadless(*ipc, *httpAddr); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// Native windowed mode (Wails) would go here. Out-of-docker, the same
	// HTML viewer can be wrapped by Wails to render in a real OS window;
	// for Phase 1 we ship the HTTP-served viewer (works in docker too).
	fmt.Fprintln(os.Stderr, "ppz-desktop: windowed mode not built; pass --headless --http-addr=:9090 and open the URL in a browser")
	os.Exit(2)
}
