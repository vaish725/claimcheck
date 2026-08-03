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
	Pass   Verdict = iota // actual fell within tolerance
	Breach                // extraction succeeded but actual fell outside tolerance
	Error                 // actual could not be determined at all
	Skip                  // benchmark claim declared on a different machine; not compared
)

func (v Verdict) String() string {
	switch v {
	case Pass:
		return "PASS"
	case Breach:
		return "BREACH"
	case Error:
		return "ERROR"
	case Skip:
		return "SKIP"
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

// NewRow compares a claim's declared value against its extracted actual
// value (or extraction error) and produces the resulting row.
func NewRow(claim schema.Claim, actual float64, extractErr error) Row {
	if extractErr != nil {
		return Row{Claim: claim, Err: extractErr, Verdict: Error}
	}
	// A benchmark claim declared on a recorded machine other than this one
	// is never compared - refused, not fudged, per the tolerance a number
	// this machine-dependent can't otherwise be trusted across. A claim
	// that has never been recorded (empty or UnsetMachine) compares
	// normally, so `update` can establish it on the first run.
	if claim.Type == schema.Benchmark && claim.MachineRecorded() && claim.Machine != schema.CurrentMachineFingerprint() {
		return Row{Claim: claim, Actual: actual, Verdict: Skip}
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

// Breached reports whether any claim drifted beyond tolerance or failed to
// extract. Skip is an intentional non-verification, not a failure.
func (r Report) Breached() bool {
	for _, row := range r.Rows {
		if row.Verdict == Breach || row.Verdict == Error {
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
		if row.Verdict == Skip {
			fmt.Fprintf(tw, "%s\t%s\t%s\t-\t%s\t%s (declared on %s, running on %s)\n",
				row.Claim.ID, FormatFloat(row.Claim.Declared), FormatFloat(row.Actual), row.Claim.Tolerance,
				row.Verdict, row.Claim.Machine, schema.CurrentMachineFingerprint())
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

// FormatFloat rounds to 4 decimal places and trims trailing zeros, so
// "88.0" prints as "88" and float subtraction noise like
// "7.349999999999994" prints as "7.35". Exported for update to reuse.
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
