// Package verify loads a repo's claims.yaml, recomputes every claim, and
// assembles the resulting drift report.
package verify

import (
	"context"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/vaish725/claimcheck/internal/extract"
	"github.com/vaish725/claimcheck/internal/report"
	"github.com/vaish725/claimcheck/internal/schema"
)

// DefaultClaimTimeout bounds a single claim's extraction, so one hung
// subprocess can't hang the whole verify run.
const DefaultClaimTimeout = 60 * time.Second

// Run recomputes every claim in claimsPath against repoPath, concurrently,
// and returns the resulting drift report. A claim that fails to extract
// becomes an ERROR row rather than aborting the whole run.
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

// extractRow recomputes one claim under its own bounded context.
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
