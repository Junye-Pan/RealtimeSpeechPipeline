package main

import (
	"fmt"
	"io"
	"os"
	"time"

	livekittransport "github.com/tiger/realtime-speech-pipeline/transports/livekit"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr, time.Now); err != nil {
		fmt.Fprintf(os.Stderr, "rspp-local-runner: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, stdout io.Writer, _ io.Writer, now func() time.Time) error {
	if len(args) > 0 {
		switch args[0] {
		case "help", "-h", "--help":
			printUsage(stdout)
			return nil
		case "livekit":
			args = args[1:]
		}
	}
	if len(args) == 0 {
		args = []string{
			"-dry-run=true",
			"-report=.codex/transports/livekit-local-runner-report.json",
		}
	}

	_, _ = fmt.Fprintln(stdout, "rspp-local-runner: starting single-node runtime + minimal control-plane stub via livekit adapter")
	return livekittransport.RunCLI(args, stdout, now)
}

func printUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "rspp-local-runner usage:")
	_, _ = fmt.Fprintln(w, "  rspp-local-runner [livekit] [livekit-flags]")
	_, _ = fmt.Fprintln(w, "Examples:")
	_, _ = fmt.Fprintln(w, "  rspp-local-runner")
	_, _ = fmt.Fprintln(w, "  rspp-local-runner livekit -dry-run=false -probe=true -url https://livekit.example.com -api-key <key> -api-secret <secret> -room rspp-demo")
}
