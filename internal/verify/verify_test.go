package verify

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/vaish725/claimcheck/internal/extract"
	"github.com/vaish725/claimcheck/internal/report"
	"github.com/vaish725/claimcheck/internal/schema"
)

// newFixtureRepo creates a tiny, fully-covered Go module in a fresh git
// repo, for verify.Run to be pointed at end to end.
func newFixtureRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	files := map[string]string{
		"go.mod": "module verifyfixture\n\ngo 1.21\n",
		"add.go": "package verifyfixture\n\nfunc Add(a, b int) int {\n\treturn a + b\n}\n",
		"add_test.go": "package verifyfixture\n\nimport \"testing\"\n\n" +
			"func TestAdd(t *testing.T) {\n\tif Add(2, 3) != 5 {\n\t\tt.Fatal(\"bad\")\n\t}\n}\n",
	}
	for name, contents := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(contents), 0o644); err != nil {
			t.Fatalf("writing %s: %v", name, err)
		}
	}

	runGit := func(args ...string) {
		base := []string{"-c", "user.name=claimcheck-test", "-c", "user.email=test@example.com"}
		cmd := exec.Command("git", append(base, args...)...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	runGit("init", "-q")
	runGit("add", ".")
	runGit("commit", "-q", "-m", "initial")

	return dir
}

// TestRunEndToEnd covers all three verdicts in one pass: PASS (test count
// and coverage, learned from the real extractors so expectations can't
// drift), BREACH (a LOC claim declared wrong), and ERROR (a coverage claim
// pointed at the wrong language runner).
func TestRunEndToEnd(t *testing.T) {
	dir := newFixtureRepo(t)
	ctx := context.Background()

	testCountClaim := schema.Claim{ID: "test_count", Type: schema.TestCount, Runner: schema.RunnerGo}
	testCountExtractor, err := extract.Lookup(testCountClaim)
	if err != nil {
		t.Fatal(err)
	}
	actualTestCount, err := testCountExtractor.Extract(ctx, dir, testCountClaim)
	if err != nil {
		t.Fatalf("learning test count: %v", err)
	}

	coverageClaim := schema.Claim{ID: "coverage", Type: schema.Coverage, Runner: schema.RunnerGo}
	coverageExtractor, err := extract.Lookup(coverageClaim)
	if err != nil {
		t.Fatal(err)
	}
	actualCoverage, err := coverageExtractor.Extract(ctx, dir, coverageClaim)
	if err != nil {
		t.Fatalf("learning coverage: %v", err)
	}

	claimsYAML := fmt.Sprintf(`
repo: fixture
claims:
  - id: test_count
    type: test_count
    runner: go
    declared: %v
    tolerance: exact
    asserted_in: [README.md]

  - id: coverage
    type: coverage
    runner: go
    declared: %v
    tolerance: "+-1%%"
    asserted_in: [README.md]

  - id: loc
    type: loc
    declared: 1
    tolerance: exact
    asserted_in: [README.md]

  - id: bad_coverage
    type: coverage
    runner: pytest
    declared: 50
    tolerance: "+-5%%"
    asserted_in: [README.md]
`, actualTestCount, actualCoverage)

	claimsPath := filepath.Join(t.TempDir(), "claims.yaml")
	if err := os.WriteFile(claimsPath, []byte(claimsYAML), 0o644); err != nil {
		t.Fatalf("writing claims.yaml: %v", err)
	}

	rep, err := Run(ctx, dir, claimsPath)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	verdicts := make(map[string]report.Verdict, len(rep.Rows))
	for _, row := range rep.Rows {
		verdicts[row.Claim.ID] = row.Verdict
	}

	if v := verdicts["test_count"]; v != report.Pass {
		t.Errorf("test_count verdict = %v, want Pass", v)
	}
	if v := verdicts["coverage"]; v != report.Pass {
		t.Errorf("coverage verdict = %v, want Pass", v)
	}
	if v := verdicts["loc"]; v != report.Breach {
		t.Errorf("loc verdict = %v, want Breach (declared value is deliberately wrong)", v)
	}
	if v := verdicts["bad_coverage"]; v != report.Error {
		t.Errorf("bad_coverage verdict = %v, want Error (pytest coverage against a Go-only repo)", v)
	}

	if !rep.Breached() {
		t.Error("Report.Breached() = false, want true (loc and bad_coverage both fail)")
	}
	if rep.Repo != "fixture" {
		t.Errorf("Report.Repo = %q, want %q", rep.Repo, "fixture")
	}
}

// TestRunUnknownClaimsFile confirms a missing claims.yaml errors from Run
// itself, rather than returning an empty report.
func TestRunUnknownClaimsFile(t *testing.T) {
	dir := t.TempDir()
	if _, err := Run(context.Background(), dir, filepath.Join(dir, "does-not-exist.yaml")); err == nil {
		t.Fatal("expected error for missing claims.yaml, got none")
	}
}
