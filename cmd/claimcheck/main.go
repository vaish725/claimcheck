// Command claimcheck keeps numeric claims in a README or resume honest by
// recomputing them from real repo state and failing when a claim has
// drifted beyond its declared tolerance.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/vaish725/claimcheck/internal/verify"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 {
		usage()
		return 2
	}

	switch args[0] {
	case "verify":
		return runVerify(args[1:])
	case "-h", "--help", "help":
		usage()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "claimcheck: unknown command %q\n\n", args[0])
		usage()
		return 2
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `claimcheck verifies that numeric claims in a README or resume still match reality.

Usage:
  claimcheck verify [flags] [repo-path]

repo-path defaults to the current directory.

Flags for verify:
  -claims string
        path to claims.yaml (default "<repo-path>/claims.yaml")
  -soft
        print the drift report without failing (exit 0 even on breach)
`)
}

// runVerify implements the `claimcheck verify` subcommand: load claims.yaml,
// recompute every claim, print the drift report, and translate the result
// into an exit code. Non-zero on any breach or extraction error, unless
// -soft was passed, in which case the report is informational only.
func runVerify(args []string) int {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	claimsFlag := fs.String("claims", "", "path to claims.yaml (default \"<repo-path>/claims.yaml\")")
	soft := fs.Bool("soft", false, "print the drift report without failing (exit 0 even on breach)")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	repoPath := "."
	if fs.NArg() > 0 {
		repoPath = fs.Arg(0)
	}

	claimsPath := *claimsFlag
	if claimsPath == "" {
		claimsPath = filepath.Join(repoPath, "claims.yaml")
	}

	rep, err := verify.Run(context.Background(), repoPath, claimsPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "claimcheck: %v\n", err)
		return 1
	}

	if err := rep.Write(os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "claimcheck: writing report: %v\n", err)
		return 1
	}

	if rep.Breached() && !*soft {
		return 1
	}
	return 0
}
