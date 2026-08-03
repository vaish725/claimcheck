// Package update recomputes every claim and rewrites claims.yaml's
// declared values plus every marked span in each claim's asserted_in files.
package update

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/vaish725/claimcheck/internal/report"
	"github.com/vaish725/claimcheck/internal/rewrite"
	"github.com/vaish725/claimcheck/internal/schema"
	"github.com/vaish725/claimcheck/internal/verify"
)

// Change is one claim's before/after value. Err is set if the claim could
// not be safely updated - extraction failed, or (for a benchmark claim) it
// was declared on a different machine - in which case NewValue is
// meaningless and the claim is left untouched everywhere.
type Change struct {
	Claim    schema.Claim // Claim.Declared is the OLD value
	NewValue float64
	Err      error
}

// FileChange is the before/after content of one rewritten file.
type FileChange struct {
	Path             string
	OldData, NewData []byte
}

// Plan is the result of recomputing every claim and computing what
// claims.yaml and every asserted_in file would look like afterward.
// Building a Plan never writes to disk.
type Plan struct {
	Changes []Change
	Files   []FileChange
}

// Changed reports whether applying this plan would modify anything on disk.
func (p *Plan) Changed() bool { return len(p.Files) > 0 }

// Failed reports whether any claim's actual value could not be recomputed.
func (p *Plan) Failed() bool {
	for _, c := range p.Changes {
		if c.Err != nil {
			return true
		}
	}
	return false
}

// BuildPlan recomputes every claim (reusing verify.Run's extraction) and
// computes the new contents of claims.yaml and every asserted_in file,
// without writing to disk; call Apply for that.
func BuildPlan(ctx context.Context, repoPath, claimsPath string) (*Plan, error) {
	rep, err := verify.Run(ctx, repoPath, claimsPath)
	if err != nil {
		return nil, err
	}

	rawClaims, err := os.ReadFile(claimsPath)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", claimsPath, err)
	}

	plan := &Plan{}
	declaredValues := make(map[string]string)
	machineValues := make(map[string]string)
	markerValuesByFile := make(map[string]map[string]string)

	for _, row := range rep.Rows {
		change := Change{Claim: row.Claim}
		if row.Err != nil {
			change.Err = row.Err
			plan.Changes = append(plan.Changes, change)
			continue
		}
		if row.Verdict == report.Skip {
			// A benchmark claim declared on a different machine: writing
			// this machine's number into "declared" would silently launder
			// a cross-machine comparison, exactly what claimcheck refuses
			// to do. Leave it untouched, same as an extraction failure.
			change.Err = fmt.Errorf("declared on a different machine (%s); run update there, or clear the machine field to adopt this one", row.Claim.Machine)
			plan.Changes = append(plan.Changes, change)
			continue
		}
		change.NewValue = row.Actual
		plan.Changes = append(plan.Changes, change)

		valueText := report.FormatFloat(row.Actual)
		declaredValues[row.Claim.ID] = valueText
		if row.Claim.Type == schema.Benchmark {
			machineValues[row.Claim.ID] = schema.CurrentMachineFingerprint()
		}

		for _, target := range row.Claim.AssertedIn {
			if target == schema.ResumeTarget {
				continue
			}
			if markerValuesByFile[target] == nil {
				markerValuesByFile[target] = make(map[string]string)
			}
			markerValuesByFile[target][row.Claim.ID] = valueText
		}
	}

	declaredSpans, err := locateFieldSpans(rawClaims, "declared", true)
	if err != nil {
		return nil, fmt.Errorf("locating declared values in %s: %w", claimsPath, err)
	}
	afterDeclared, err := rewrite.ReplaceSpans(rawClaims, declaredSpans, declaredValues)
	if err != nil {
		return nil, fmt.Errorf("rewriting %s: %w", claimsPath, err)
	}

	// A second pass against the already-patched bytes: machine is optional
	// (only claims that opted in by including the key get touched), and
	// its byte offsets must be found after the declared-value rewrite,
	// since that rewrite can shift everything after it on the same line.
	machineSpans, err := locateFieldSpans(afterDeclared, "machine", false)
	if err != nil {
		return nil, fmt.Errorf("locating machine fingerprints in %s: %w", claimsPath, err)
	}
	newClaims, err := rewrite.ReplaceSpans(afterDeclared, machineSpans, machineValues)
	if err != nil {
		return nil, fmt.Errorf("rewriting %s: %w", claimsPath, err)
	}

	if !bytes.Equal(newClaims, rawClaims) {
		plan.Files = append(plan.Files, FileChange{Path: claimsPath, OldData: rawClaims, NewData: newClaims})
	}

	// sorted for deterministic Plan.Files / Write output
	targets := make([]string, 0, len(markerValuesByFile))
	for target := range markerValuesByFile {
		targets = append(targets, target)
	}
	sort.Strings(targets)

	for _, target := range targets {
		fullPath := filepath.Join(repoPath, target)
		old, err := os.ReadFile(fullPath)
		if err != nil {
			return nil, fmt.Errorf("reading %s (listed in asserted_in): %w", target, err)
		}
		rewriteFunc := rewrite.Replace
		if strings.EqualFold(filepath.Ext(target), ".tex") {
			rewriteFunc = rewrite.ReplaceLaTeX
		}
		newData, err := rewriteFunc(old, markerValuesByFile[target])
		if err != nil {
			return nil, fmt.Errorf("rewriting markers in %s: %w", target, err)
		}
		if !bytes.Equal(newData, old) {
			plan.Files = append(plan.Files, FileChange{Path: fullPath, OldData: old, NewData: newData})
		}
	}

	return plan, nil
}

// Apply writes every FileChange in the plan to disk atomically, preserving
// each file's existing permission bits.
func Apply(plan *Plan) error {
	for _, fc := range plan.Files {
		perm := os.FileMode(0o644)
		if info, err := os.Stat(fc.Path); err == nil {
			perm = info.Mode().Perm()
		}
		if err := rewrite.WriteFileAtomic(fc.Path, fc.NewData, perm); err != nil {
			return fmt.Errorf("writing %s: %w", fc.Path, err)
		}
	}
	return nil
}

// Write renders a summary: one line per claim (old -> new, unchanged, or
// error), then which files changed. applied must reflect whether Apply
// was actually called, or a dry run would misleadingly say "updated".
func (p *Plan) Write(w io.Writer, applied bool) error {
	for _, c := range p.Changes {
		var line string
		switch {
		case c.Err != nil:
			line = fmt.Sprintf("%s: ERROR: %v (not updated)\n", c.Claim.ID, c.Err)
		case c.Claim.Declared == c.NewValue:
			line = fmt.Sprintf("%s: %s (unchanged)\n", c.Claim.ID, report.FormatFloat(c.NewValue))
		default:
			line = fmt.Sprintf("%s: %s -> %s\n", c.Claim.ID, report.FormatFloat(c.Claim.Declared), report.FormatFloat(c.NewValue))
		}
		if _, err := io.WriteString(w, line); err != nil {
			return err
		}
	}

	if len(p.Files) == 0 {
		_, err := fmt.Fprintln(w, "no files changed")
		return err
	}
	verb := "would update"
	if applied {
		verb = "updated"
	}
	for _, fc := range p.Files {
		if _, err := fmt.Fprintf(w, "%s %s\n", verb, fc.Path); err != nil {
			return err
		}
	}
	return nil
}
