package extract

import (
	"context"
	"testing"
	"time"

	"github.com/vaish725/claimcheck/internal/schema"
)

func TestParseJUnitTestCount(t *testing.T) {
	fixture := `<?xml version="1.0" encoding="utf-8"?>
<testsuites>
  <testsuite name="pytest" errors="0" failures="1" skipped="0" tests="3" time="0.012">
    <testcase classname="test_calc" name="test_add" time="0.001"/>
    <testcase classname="test_calc" name="test_sub" time="0.001"/>
    <testcase classname="test_calc" name="test_mul" time="0.001">
      <failure message="assert 5 == 6"/>
    </testcase>
  </testsuite>
</testsuites>`

	got, err := parseJUnitTestCount([]byte(fixture))
	if err != nil {
		t.Fatalf("parseJUnitTestCount: %v", err)
	}
	if got != 3 {
		t.Errorf("parseJUnitTestCount = %d, want 3", got)
	}
}

func TestParseJUnitTestCountBareTestsuite(t *testing.T) {
	// older runners emit a bare <testsuite> with no wrapping <testsuites>
	fixture := `<testsuite name="pytest" tests="5" errors="0" failures="0" skipped="0"></testsuite>`

	got, err := parseJUnitTestCount([]byte(fixture))
	if err != nil {
		t.Fatalf("parseJUnitTestCount: %v", err)
	}
	if got != 5 {
		t.Errorf("parseJUnitTestCount = %d, want 5", got)
	}
}

func TestParseJUnitTestCountMissing(t *testing.T) {
	if _, err := parseJUnitTestCount([]byte(`<testsuites></testsuites>`)); err == nil {
		t.Fatal("expected error for junit xml with no testsuite tests attribute, got none")
	}
}

func TestParseCoverageXML(t *testing.T) {
	fixture := `<?xml version="1.0"?>
<coverage line-rate="0.768" branch-rate="0.5" version="7.15.0">
  <packages></packages>
</coverage>`

	got, err := parseCoverageXML([]byte(fixture))
	if err != nil {
		t.Fatalf("parseCoverageXML: %v", err)
	}
	if got != 76.8 {
		t.Errorf("parseCoverageXML = %v, want 76.8", got)
	}
}

func TestParseCoverageXMLWrongRoot(t *testing.T) {
	if _, err := parseCoverageXML([]byte(`<notcoverage line-rate="0.5"/>`)); err == nil {
		t.Fatal("expected error for wrong root element, got none")
	}
}

// TestPythonExtractorsAgainstFixtureProject runs both extractors for real
// against testdata/pyproject, exercising the full pytest/coverage.py path.
func TestPythonExtractorsAgainstFixtureProject(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	countClaim := schema.Claim{ID: "test_count", Type: schema.TestCount, Runner: schema.RunnerPytest}
	count, err := (pytestCountExtractor{}).Extract(ctx, "../../testdata/pyproject", countClaim)
	if err != nil {
		t.Fatalf("pytestCountExtractor.Extract: %v", err)
	}
	if count != 3 {
		t.Errorf("test count = %v, want 3", count)
	}

	covClaim := schema.Claim{ID: "coverage", Type: schema.Coverage, Runner: schema.RunnerPytest}
	pct, err := (pytestCoverageExtractor{}).Extract(ctx, "../../testdata/pyproject", covClaim)
	if err != nil {
		t.Fatalf("pytestCoverageExtractor.Extract: %v", err)
	}
	if pct <= 0 || pct >= 100 { // div() is untested; add/sub/mul are fully covered
		t.Errorf("coverage = %v%%, want a value strictly between 0 and 100", pct)
	}
}
