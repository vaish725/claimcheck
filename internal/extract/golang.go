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

// goTestEvent is the subset of a `go test -json` event this package needs.
type goTestEvent struct {
	Action string `json:"Action"`
	Test   string `json:"Test"`
}

// goTestCountExtractor counts a Go repo's tests via `go test -json`.
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
// Failing tests still produce a complete event stream, so a non-zero exit
// alone is not treated as an extraction failure.
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

// countGoTests counts distinct top-level tests (subtests, named with "/",
// fold into their parent) with a pass/fail/skip action.
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

// goCoverageExtractor recomputes coverage for a Go repo via a coverage
// profile and `go tool cover -func`'s aggregate total.
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

// runGoCoverProfile runs `go test -coverprofile=<profilePath> ./...`; a
// failing-test exit code doesn't invalidate the profile that was written.
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

// parseCoverTotal extracts the percentage from `go tool cover -func`'s
// final "total:" line, e.g. "total:\t\t\t(statements)\t54.3%".
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
