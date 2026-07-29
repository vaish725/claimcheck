package extract

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/vaish725/claimcheck/internal/schema"
)

// commitCountExtractor recomputes a commit_count claim by counting the
// commits reachable from HEAD.
type commitCountExtractor struct{}

func (commitCountExtractor) Extract(ctx context.Context, repoPath string, _ schema.Claim) (float64, error) {
	cmd := exec.CommandContext(ctx, "git", "rev-list", "--count", "HEAD")
	cmd.Dir = repoPath
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return 0, fmt.Errorf("running git rev-list: %w: %s", err, stderr.String())
	}

	count, err := strconv.Atoi(strings.TrimSpace(stdout.String()))
	if err != nil {
		return 0, fmt.Errorf("parsing git rev-list output %q: %w", stdout.String(), err)
	}
	return float64(count), nil
}
