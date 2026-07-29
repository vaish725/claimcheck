// Package report turns extracted claim values into a human-readable drift
// report and the pass/fail verdict that decides claimcheck's exit code.
package report

import (
	"fmt"
	"io"
	"math"
	"strconv"
	"text/tabwriter"

	"github.com/vaish725/claimcheck/internal/schema"
)

// Verdict is the outcome of checking one claim.
type Verdict int

const (
	// Pass means the actual value fell within the claim's tolerance.
	Pass Verdict = iota
	// Breach means extraction succeeded but the actual value fell outside
	// tolerance: the claim has drifted.
	Breach
	// Error means the actual value could not be determined at all. This is
	// always reported loudly and never silently treated as zero or as a pass.
	Error
)

func (v Verdict) String() string {
	switch v {
	case Pass:
		return "PASS"
	case Breach:
		return "BREACH"
	case Error:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

// Row is the checked outcome for a single claim.
type Row struct {
	Claim   schema.Claim
	Actual  float64 // meaningless when Verdict is Error
	Err     error   // set only when Verdict is Error
	Verdict Verdict
}

// NewRow compares a claim's declared value against its freshly extracted
// actual value (or the error that prevented extraction) and produces the
// row that should appear in the drift report.
func NewRow(claim schema.Claim, actual float64, extractErr error) Row {
	if extractErr != nil {
		return Row{Claim: claim, Err: extractErr, Verdict: Error}
	}
	verdict := Pass
	if !claim.ParsedTolerance.Within(claim.Declared, actual) {
		verdict = Breach
	}
	return Row{Claim: claim, Actual: actual, Verdict: verdict}
}

// Report is the full drift report for one repo's claims.yaml.
type Report struct {
	Repo string
	Rows []Row
}

// Breached reports whether any claim failed to pass, either because it
// drifted beyond tolerance or because it could not be extracted. This is
// what `claimcheck verify` uses to decide its exit code.
func (r Report) Breached() bool {
	for _, row := range r.Rows {
		if row.Verdict != Pass {
			return true
		}
	}
	return false
}

// Write renders the report as an aligned, human-readable table: claim,
// declared, actual, delta, tolerance, verdict.
func (r Report) Write(w io.Writer) error {
	if r.Repo != "" {
		if _, err := fmt.Fprintf(w, "repo: %s\n\n", r.Repo); err != nil {
			return err
		}
	}

	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "CLAIM\tDECLARED\tACTUAL\tDELTA\tTOLERANCE\tVERDICT")
	for _, row := range r.Rows {
		if row.Verdict == Error {
			fmt.Fprintf(tw, "%s\t%s\t-\t-\t%s\t%s (%s)\n",
				row.Claim.ID, FormatFloat(row.Claim.Declared), row.Claim.Tolerance, row.Verdict, row.Err)
			continue
		}
		delta := row.Actual - row.Claim.Declared
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
			row.Claim.ID,
			FormatFloat(row.Claim.Declared),
			FormatFloat(row.Actual),
			formatSignedFloat(delta),
			row.Claim.Tolerance,
			row.Verdict,
		)
	}
	return tw.Flush()
}

// FormatFloat rounds to 4 decimal places (enough to preserve any real
// precision a claim cares about) before trimming trailing zeros, so
// "88.0" prints as "88" and "76.8" prints as "76.8" rather than the
// float64 rounding noise that plain subtraction can introduce (e.g.
// "7.349999999999994" instead of "7.35"). Exported so the update package
// can format the same values it writes back into claims.yaml and marked
// spans exactly as the drift report would show them.
func FormatFloat(f float64) string {
	rounded := math.Round(f*1e4) / 1e4
	return strconv.FormatFloat(rounded, 'f', -1, 64)
}

// formatSignedFloat is FormatFloat with an explicit "+" for non-negative
// deltas, so the drift report reads as "+3" / "-2" rather than "3" / "-2".
func formatSignedFloat(f float64) string {
	if f >= 0 {
		return "+" + FormatFloat(f)
	}
	return FormatFloat(f)
}
