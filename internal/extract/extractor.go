// Package extract recomputes a claim's actual value from real repo state.
// Extractors fail loudly rather than guess or report zero.
package extract

import (
	"context"
	"fmt"

	"github.com/vaish725/claimcheck/internal/schema"
)

// Extractor recomputes a claim's actual value by inspecting repoPath.
// Subprocess-based implementations must run under ctx so a caller-imposed
// timeout can stop a hung command.
type Extractor interface {
	Extract(ctx context.Context, repoPath string, claim schema.Claim) (float64, error)
}

// key identifies which extractor handles a claim; Runner is empty for
// language-agnostic types (loc, commit_count).
type key struct {
	Type   schema.ClaimType
	Runner schema.Runner
}

// registry maps a claim's (type, runner) to the extractor that recomputes it.
var registry = map[key]Extractor{
	{Type: schema.TestCount, Runner: schema.RunnerGo}:     goTestCountExtractor{},
	{Type: schema.Coverage, Runner: schema.RunnerGo}:      goCoverageExtractor{},
	{Type: schema.TestCount, Runner: schema.RunnerPytest}: pytestCountExtractor{},
	{Type: schema.Coverage, Runner: schema.RunnerPytest}:  pytestCoverageExtractor{},
	{Type: schema.LOC}:         locExtractor{},
	{Type: schema.CommitCount}: commitCountExtractor{},
	{Type: schema.Benchmark}:   benchmarkExtractor{},
}

// Lookup returns the extractor for claim's (type, runner), or an error if
// none is registered.
func Lookup(claim schema.Claim) (Extractor, error) {
	e, ok := registry[key{Type: claim.Type, Runner: claim.Runner}]
	if !ok {
		return nil, fmt.Errorf("no extractor registered for type %q runner %q", claim.Type, claim.Runner)
	}
	return e, nil
}
