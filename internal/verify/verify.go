// Package verify orchestrates a single `claimcheck verify` run: load a
// repo's claims.yaml, recompute every claim's actual value, and assemble
// the resulting drift report.
package verify

import (
	"context"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/vaish725/claimcheck/internal/extract"
	"github.com/vaish725/claimcheck/internal/report"
	"github.com/vaish725/claimcheck/internal/schema"
)

// DefaultClaimTimeout bounds how long any single claim's extraction may
// run. Extractors shell out to test runners and coverage tools, any of
// which can hang; without a per-claim deadline, one hung extractor would
// hang the entire verify run, and by extension, CI.
const DefaultClaimTimeout = 60 * time.Second

// Run loads claimsPath, recomputes every declared claim's actual value by
// inspecting repoPath, and returns the resulting drift report. Claims are
// extracted concurrently: résumé mode will eventually run this across nine
// repos, and doing that serially would be slow enough to be annoying, so
// the concurrency is built in at the single-repo level from the start.
//
// A claim that fails to extract does not abort the run; it becomes an
// ERROR row in the report so the caller sees every claim's status in one
// pass rather than stopping at the first failure.
func Run(ctx context.Context, repoPath, claimsPath string) (report.Report, error) {
	cf, err := schema.Load(claimsPath)
	if err != nil {
		return report.Report{}, err
	}

	rows := make([]report.Row, len(cf.Claims))

	var g errgroup.Group
	for i, claim := range cf.Claims {
		i, claim := i, claim
		g.Go(func() error {
			rows[i] = extractRow(ctx, repoPath, claim)
			return nil // extraction failures become ERROR rows, not fatal errors
		})
	}
	_ = g.Wait()

	return report.Report{Repo: cf.Repo, Rows: rows}, nil
}

// extractRow recomputes a single claim's actual value under its own bounded
// context and turns the outcome into a report row.
func extractRow(ctx context.Context, repoPath string, claim schema.Claim) report.Row {
	extractor, err := extract.Lookup(claim)
	if err != nil {
		return report.NewRow(claim, 0, err)
	}

	claimCtx, cancel := context.WithTimeout(ctx, DefaultClaimTimeout)
	defer cancel()

	actual, err := extractor.Extract(claimCtx, repoPath, claim)
	return report.NewRow(claim, actual, err)
}
