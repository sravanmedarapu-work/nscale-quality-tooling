package main

import (
	"fmt"
	"os"

	"github.com/sravanmedarapu-work/nscale-quality-tooling/cmd/nscale-test-history/ingest"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: nscale-test-history <command> [flags]")
		fmt.Fprintln(os.Stderr, "commands: ingest")
		os.Exit(1)
	}
	switch os.Args[1] {
	case "ingest":
		ingest.Run(os.Args[2:])
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		os.Exit(1)
	}
}
