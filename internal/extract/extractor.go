// Package extract recomputes a claim's actual value from real repo state:
// running test suites, parsing coverage reports, walking tracked files,
// and reading git history. Every extractor is expected to fail loudly
// (return an error) rather than guess or silently report zero when it
// cannot determine a value.
package extract

import (
	"context"
	"fmt"

	"github.com/vaish725/claimcheck/internal/schema"
)

// Extractor recomputes the actual value for a claim by inspecting the repo
// at repoPath. Implementations that shell out to a subprocess must run it
// with ctx so a caller-imposed timeout can stop a hung command.
type Extractor interface {
	Extract(ctx context.Context, repoPath string, claim schema.Claim) (float64, error)
}

// key identifies which extractor handles a claim. Runner is empty for
// claim types that are language-agnostic (loc, commit_count).
type key struct {
	Type   schema.ClaimType
	Runner schema.Runner
}

// registry maps a claim's (type, runner) pair to the extractor that knows
// how to recompute it. Built once at package init time.
var registry = map[key]Extractor{
	{Type: schema.TestCount, Runner: schema.RunnerGo}:     goTestCountExtractor{},
	{Type: schema.Coverage, Runner: schema.RunnerGo}:      goCoverageExtractor{},
	{Type: schema.TestCount, Runner: schema.RunnerPytest}: pytestCountExtractor{},
	{Type: schema.Coverage, Runner: schema.RunnerPytest}:  pytestCoverageExtractor{},
	{Type: schema.LOC}:                                    locExtractor{},
	{Type: schema.CommitCount}:                             commitCountExtractor{},
}

// Lookup returns the extractor responsible for claim, or an error if no
// extractor is registered for its (type, runner) combination. Schema
// validation already guarantees runner is set when the claim type needs
// one, so this only fails for genuinely unsupported combinations.
func Lookup(claim schema.Claim) (Extractor, error) {
	e, ok := registry[key{Type: claim.Type, Runner: claim.Runner}]
	if !ok {
		return nil, fmt.Errorf("no extractor registered for type %q runner %q", claim.Type, claim.Runner)
	}
	return e, nil
}
