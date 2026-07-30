package report

import (
	"errors"
	"strings"
	"testing"

	"github.com/vaish725/claimcheck/internal/schema"
)

func mustTolerance(t *testing.T, raw string) schema.Tolerance {
	t.Helper()
	tol, err := schema.ParseTolerance(raw)
	if err != nil {
		t.Fatalf("ParseTolerance(%q): %v", raw, err)
	}
	return tol
}

func TestNewRowPass(t *testing.T) {
	claim := schema.Claim{ID: "test_count", Declared: 88, ParsedTolerance: mustTolerance(t, "exact")}
	row := NewRow(claim, 88, nil)
	if row.Verdict != Pass {
		t.Errorf("Verdict = %v, want Pass", row.Verdict)
	}
}

func TestNewRowBreach(t *testing.T) {
	claim := schema.Claim{ID: "test_count", Declared: 88, ParsedTolerance: mustTolerance(t, "exact")}
	row := NewRow(claim, 91, nil)
	if row.Verdict != Breach {
		t.Errorf("Verdict = %v, want Breach", row.Verdict)
	}
}

func TestNewRowError(t *testing.T) {
	claim := schema.Claim{ID: "coverage", Declared: 54, ParsedTolerance: mustTolerance(t, "+-3%")}
	row := NewRow(claim, 0, errors.New("could not run coverage"))
	if row.Verdict != Error {
		t.Errorf("Verdict = %v, want Error", row.Verdict)
	}
}

func TestReportBreached(t *testing.T) {
	passClaim := schema.Claim{ID: "a", Declared: 10, ParsedTolerance: mustTolerance(t, "exact")}
	breachClaim := schema.Claim{ID: "b", Declared: 10, ParsedTolerance: mustTolerance(t, "exact")}

	allPass := Report{Rows: []Row{NewRow(passClaim, 10, nil)}}
	if allPass.Breached() {
		t.Error("Breached() = true for an all-passing report, want false")
	}

	oneBreach := Report{Rows: []Row{NewRow(passClaim, 10, nil), NewRow(breachClaim, 12, nil)}}
	if !oneBreach.Breached() {
		t.Error("Breached() = false with a breaching claim present, want true")
	}

	oneError := Report{Rows: []Row{NewRow(passClaim, 10, nil), NewRow(breachClaim, 0, errors.New("boom"))}}
	if !oneError.Breached() {
		t.Error("Breached() = false with an error claim present, want true")
	}
}

func TestReportWriteAvoidsFloatNoise(t *testing.T) {
	// 82.35 - 75 in float64 is 7.349999999999994; must display as "7.35"
	claim := schema.Claim{ID: "coverage", Declared: 75, Tolerance: "+-10%", ParsedTolerance: mustTolerance(t, "+-10%")}
	r := Report{Rows: []Row{NewRow(claim, 82.35, nil)}}

	var buf strings.Builder
	if err := r.Write(&buf); err != nil {
		t.Fatalf("Write: %v", err)
	}
	out := buf.String()

	if strings.Contains(out, "999999") {
		t.Errorf("report output contains floating point rounding noise:\n%s", out)
	}
	if !strings.Contains(out, "+7.35") {
		t.Errorf("report output missing expected delta %q; got:\n%s", "+7.35", out)
	}
}

func TestReportWrite(t *testing.T) {
	claim := schema.Claim{ID: "test_count", Declared: 88, Tolerance: "exact", ParsedTolerance: mustTolerance(t, "exact")}
	errClaim := schema.Claim{ID: "coverage", Declared: 54, Tolerance: "+-3%", ParsedTolerance: mustTolerance(t, "+-3%")}

	r := Report{
		Repo: "example",
		Rows: []Row{
			NewRow(claim, 88, nil),
			NewRow(errClaim, 0, errors.New("pytest not found")),
		},
	}

	var buf strings.Builder
	if err := r.Write(&buf); err != nil {
		t.Fatalf("Write: %v", err)
	}
	out := buf.String()

	for _, want := range []string{"repo: example", "test_count", "PASS", "coverage", "ERROR", "pytest not found"} {
		if !strings.Contains(out, want) {
			t.Errorf("report output missing %q; got:\n%s", want, out)
		}
	}
}
