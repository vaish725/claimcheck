package extract

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/vaish725/claimcheck/internal/schema"
)

// goTestEvent mirrors the subset of `go test -json` event fields this
// package cares about. The full schema has more fields (Package, Elapsed,
// Output, ...) that are irrelevant to counting tests.
type goTestEvent struct {
	Action string `json:"Action"`
	Test   string `json:"Test"`
}

// goTestCountExtractor recomputes a test_count claim for a Go repo by
// running the test suite and counting distinct tests that reached a
// terminal result.
type goTestCountExtractor struct{}

func (goTestCountExtractor) Extract(ctx context.Context, repoPath string, _ schema.Claim) (float64, error) {
	output, err := runGoTestJSON(ctx, repoPath)
	if err != nil {
		return 0, err
	}
	count, err := countGoTests(output)
	if err != nil {
		return 0, err
	}
	return float64(count), nil
}

// runGoTestJSON runs `go test -json ./...` in repoPath and returns stdout.
// A non-zero exit from failing tests is not treated as an extraction
// failure: the JSON event stream is still complete and parseable, and
// deciding whether failures matter is the tolerance's job, not this
// extractor's.
func runGoTestJSON(ctx context.Context, repoPath string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "go", "test", "-json", "./...")
	cmd.Dir = repoPath
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if _, isExitErr := err.(*exec.ExitError); !isExitErr {
			return nil, fmt.Errorf("running go test: %w: %s", err, stderr.String())
		}
	}
	return stdout.Bytes(), nil
}

// countGoTests counts distinct top-level tests (subtests, whose names
// contain "/", are folded into their parent) that reached a pass, fail, or
// skip action in a `go test -json` event stream.
func countGoTests(output []byte) (int, error) {
	seen := make(map[string]bool)
	dec := json.NewDecoder(bytes.NewReader(output))
	for {
		var ev goTestEvent
		if err := dec.Decode(&ev); err != nil {
			if err == io.EOF {
				break
			}
			return 0, fmt.Errorf("parsing go test -json output: %w", err)
		}
		if ev.Test == "" || strings.Contains(ev.Test, "/") {
			continue
		}
		switch ev.Action {
		case "pass", "fail", "skip":
			seen[ev.Test] = true
		}
	}
	if len(seen) == 0 {
		return 0, fmt.Errorf("go test -json produced no test results (no tests found, or the module failed to build)")
	}
	return len(seen), nil
}

// goCoverageExtractor recomputes a coverage claim for a Go repo by running
// the test suite with a coverage profile and reading the aggregate total
// out of `go tool cover -func`.
type goCoverageExtractor struct{}

func (goCoverageExtractor) Extract(ctx context.Context, repoPath string, claim schema.Claim) (float64, error) {
	profilePath, cleanup, err := tempFilePath("claimcheck-coverprofile-*.out")
	if err != nil {
		return 0, err
	}
	defer cleanup()

	if err := runGoCoverProfile(ctx, repoPath, profilePath); err != nil {
		return 0, err
	}

	funcOutput, err := runGoToolCoverFunc(ctx, repoPath, profilePath)
	if err != nil {
		return 0, err
	}

	pct, err := parseCoverTotal(funcOutput)
	if err != nil {
		return 0, fmt.Errorf("claim %q: %w", claim.ID, err)
	}
	return pct, nil
}

// runGoCoverProfile runs `go test -coverprofile=<profilePath> ./...`. As
// with test count, a failing-test exit code doesn't invalidate the
// coverage profile that was still written.
func runGoCoverProfile(ctx context.Context, repoPath, profilePath string) error {
	cmd := exec.CommandContext(ctx, "go", "test", "-coverprofile="+profilePath, "./...")
	cmd.Dir = repoPath
	var stderr bytes.Buffer
	cmd.Stdout = io.Discard
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if _, isExitErr := err.(*exec.ExitError); !isExitErr {
			return fmt.Errorf("running go test -coverprofile: %w: %s", err, stderr.String())
		}
	}

	data, err := os.ReadFile(profilePath)
	if err != nil {
		return fmt.Errorf("reading coverage profile: %w", err)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return fmt.Errorf("go test produced an empty coverage profile (no coverable statements found)")
	}
	return nil
}

func runGoToolCoverFunc(ctx context.Context, repoPath, profilePath string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "go", "tool", "cover", "-func="+profilePath)
	cmd.Dir = repoPath
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("running go tool cover: %w: %s", err, stderr.String())
	}
	return stdout.Bytes(), nil
}

// parseCoverTotal extracts the percentage from the "total:" summary line
// that `go tool cover -func` prints last, e.g.
// "total:\t\t\t\t\t(statements)\t54.3%".
func parseCoverTotal(output []byte) (float64, error) {
	for _, line := range strings.Split(string(output), "\n") {
		if !strings.HasPrefix(line, "total:") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			break
		}
		last := strings.TrimSuffix(fields[len(fields)-1], "%")
		pct, err := strconv.ParseFloat(last, 64)
		if err != nil {
			return 0, fmt.Errorf("parsing coverage percentage from %q: %w", line, err)
		}
		return pct, nil
	}
	return 0, fmt.Errorf("no \"total:\" line found in go tool cover output")
}
