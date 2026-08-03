// Command claimcheck keeps numeric README/resume claims honest by
// recomputing them from repo state and failing on drift.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/vaish725/claimcheck/internal/resume"
	"github.com/vaish725/claimcheck/internal/update"
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
	case "update":
		return runUpdate(args[1:])
	case "resume":
		return runResume(args[1:])
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
  claimcheck update [flags] [repo-path]
  claimcheck resume [flags]

repo-path defaults to the current directory.

Flags for verify:
  -claims string
        path to claims.yaml (default "<repo-path>/claims.yaml")
  -soft
        print the drift report without failing (exit 0 even on breach)

Flags for update:
  -claims string
        path to claims.yaml (default "<repo-path>/claims.yaml")
  -dry-run
        print what would change without writing anything

Flags for resume:
  -file string
        path to resume.yaml (default "resume.yaml")
  -soft
        print the summary without failing (exit 0 even on breach)
`)
}

// resolvePaths applies the shared convention: first positional arg is the
// repo path (default "."), claims.yaml lives at its root unless -claims
// overrides it.
func resolvePaths(fs *flag.FlagSet, claimsFlag string) (repoPath, claimsPath string) {
	repoPath = "."
	if fs.NArg() > 0 {
		repoPath = fs.Arg(0)
	}
	claimsPath = claimsFlag
	if claimsPath == "" {
		claimsPath = filepath.Join(repoPath, "claims.yaml")
	}
	return repoPath, claimsPath
}

// runVerify recomputes every claim and prints the drift report. Exits
// non-zero on any breach or extraction error, unless -soft is set.
func runVerify(args []string) int {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	claimsFlag := fs.String("claims", "", "path to claims.yaml (default \"<repo-path>/claims.yaml\")")
	soft := fs.Bool("soft", false, "print the drift report without failing (exit 0 even on breach)")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	repoPath, claimsPath := resolvePaths(fs, *claimsFlag)

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

// runUpdate recomputes every claim and rewrites claims.yaml plus every
// asserted-in file, unless -dry-run is set. Exits non-zero if any claim
// failed to recompute, since the update is then incomplete.
func runUpdate(args []string) int {
	fs := flag.NewFlagSet("update", flag.ContinueOnError)
	claimsFlag := fs.String("claims", "", "path to claims.yaml (default \"<repo-path>/claims.yaml\")")
	dryRun := fs.Bool("dry-run", false, "print what would change without writing anything")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	repoPath, claimsPath := resolvePaths(fs, *claimsFlag)

	plan, err := update.BuildPlan(context.Background(), repoPath, claimsPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "claimcheck: %v\n", err)
		return 1
	}

	applied := false
	if !*dryRun && plan.Changed() {
		if err := update.Apply(plan); err != nil {
			fmt.Fprintf(os.Stderr, "claimcheck: %v\n", err)
			return 1
		}
		applied = true
	}

	if err := plan.Write(os.Stdout, applied); err != nil {
		fmt.Fprintf(os.Stderr, "claimcheck: writing summary: %v\n", err)
		return 1
	}

	if plan.Failed() {
		return 1
	}
	return 0
}

// runResume checks every repo listed in resume.yaml and prints which
// résumé-asserted claims have drifted. Exits non-zero if any repo failed
// outright or has a breaching/erroring claim, unless -soft is set.
func runResume(args []string) int {
	fs := flag.NewFlagSet("resume", flag.ContinueOnError)
	fileFlag := fs.String("file", "resume.yaml", "path to resume.yaml")
	soft := fs.Bool("soft", false, "print the summary without failing (exit 0 even on breach)")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	results, err := resume.Run(context.Background(), *fileFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "claimcheck: %v\n", err)
		return 1
	}

	if err := resume.WriteSummary(os.Stdout, results); err != nil {
		fmt.Fprintf(os.Stderr, "claimcheck: writing summary: %v\n", err)
		return 1
	}

	if resume.Breached(results) && !*soft {
		return 1
	}
	return 0
}
