// Package update recomputes every claim in a repo's claims.yaml and
// rewrites both claims.yaml's declared values and every marked span in the
// files each claim's asserted_in lists, so refreshing a README or resume's
// numbers is one command instead of manual, repo-by-repo editing.
package update

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/vaish725/claimcheck/internal/report"
	"github.com/vaish725/claimcheck/internal/rewrite"
	"github.com/vaish725/claimcheck/internal/schema"
	"github.com/vaish725/claimcheck/internal/verify"
)

// resumeTarget is the reserved asserted_in entry standing in for the
// future cross-repo résumé mode. It names no file in this repo, so update
// never tries to open or rewrite it.
const resumeTarget = "resume"

// Change is one claim's before/after value. Err is set if the claim's
// actual value could not be recomputed, in which case NewValue is
// meaningless and the claim is left untouched everywhere - a value update
// can't safely compute is a value update skips, not guesses at.
type Change struct {
	Claim    schema.Claim // Claim.Declared is the OLD value
	NewValue float64
	Err      error
}

// FileChange is the before/after content of one rewritten file: either
// claims.yaml itself, or a file some claim's asserted_in lists.
type FileChange struct {
	Path             string
	OldData, NewData []byte
}

// Plan is the full result of recomputing every claim and computing what
// claims.yaml and every asserted_in file would look like afterward.
// Building a Plan never writes anything to disk.
type Plan struct {
	Changes []Change
	Files   []FileChange
}

// Changed reports whether applying this plan would modify anything on disk.
func (p *Plan) Changed() bool { return len(p.Files) > 0 }

// Failed reports whether any claim's actual value could not be
// recomputed, meaning this plan is incomplete.
func (p *Plan) Failed() bool {
	for _, c := range p.Changes {
		if c.Err != nil {
			return true
		}
	}
	return false
}

// BuildPlan recomputes every claim declared in claimsPath, reusing the
// same concurrent extraction verify.Run uses, and computes the new
// contents of claims.yaml and every file its claims' asserted_in lists.
// It does not write anything to disk; call Apply for that.
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
	markerValuesByFile := make(map[string]map[string]string)

	for _, row := range rep.Rows {
		change := Change{Claim: row.Claim}
		if row.Err != nil {
			change.Err = row.Err
			plan.Changes = append(plan.Changes, change)
			continue
		}
		change.NewValue = row.Actual
		plan.Changes = append(plan.Changes, change)

		valueText := report.FormatFloat(row.Actual)
		declaredValues[row.Claim.ID] = valueText

		for _, target := range row.Claim.AssertedIn {
			if target == resumeTarget {
				continue
			}
			if markerValuesByFile[target] == nil {
				markerValuesByFile[target] = make(map[string]string)
			}
			markerValuesByFile[target][row.Claim.ID] = valueText
		}
	}

	spans, err := locateDeclaredSpans(rawClaims)
	if err != nil {
		return nil, fmt.Errorf("locating declared values in %s: %w", claimsPath, err)
	}
	newClaims, err := rewrite.ReplaceSpans(rawClaims, spans, declaredValues)
	if err != nil {
		return nil, fmt.Errorf("rewriting %s: %w", claimsPath, err)
	}
	if !bytes.Equal(newClaims, rawClaims) {
		plan.Files = append(plan.Files, FileChange{Path: claimsPath, OldData: rawClaims, NewData: newClaims})
	}

	// Sorted so Plan.Files (and therefore Write's output) is deterministic
	// rather than following Go's randomized map iteration order.
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
		newData, err := rewrite.Replace(old, markerValuesByFile[target])
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

// Write renders a human-readable summary of the plan: one line per claim
// (old -> new, unchanged, or error), followed by which files were changed.
// applied must reflect whether Apply was actually called - Write has no
// other way to know, and printing "updated" for a dry run that wrote
// nothing would be actively misleading.
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
