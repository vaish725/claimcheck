package extract

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/vaish725/claimcheck/internal/schema"
)

func TestCountGoTests(t *testing.T) {
	// A captured (trimmed) `go test -json` event stream: three top-level
	// tests, one of which has a subtest that should not be double-counted.
	fixture := strings.Join([]string{
		`{"Action":"run","Test":"TestAdd"}`,
		`{"Action":"pass","Test":"TestAdd"}`,
		`{"Action":"run","Test":"TestSub"}`,
		`{"Action":"run","Test":"TestSub/negative"}`,
		`{"Action":"pass","Test":"TestSub/negative"}`,
		`{"Action":"pass","Test":"TestSub"}`,
		`{"Action":"run","Test":"TestMul"}`,
		`{"Action":"fail","Test":"TestMul"}`,
		`{"Action":"pass"}`, // package-level event with no Test name; must be ignored
	}, "\n")

	got, err := countGoTests([]byte(fixture))
	if err != nil {
		t.Fatalf("countGoTests: %v", err)
	}
	if got != 3 {
		t.Errorf("countGoTests = %d, want 3", got)
	}
}

func TestCountGoTestsEmpty(t *testing.T) {
	if _, err := countGoTests([]byte("")); err == nil {
		t.Fatal("expected error for empty output, got none")
	}
}

func TestParseCoverTotal(t *testing.T) {
	fixture := "goproject/mathutil.go:9:\tAdd\t\t100.0%\n" +
		"goproject/mathutil.go:13:\tSub\t\t100.0%\n" +
		"total:\t\t\t\t\t(statements)\t75.0%\n"

	got, err := parseCoverTotal([]byte(fixture))
	if err != nil {
		t.Fatalf("parseCoverTotal: %v", err)
	}
	if got != 75.0 {
		t.Errorf("parseCoverTotal = %v, want 75.0", got)
	}
}

func TestParseCoverTotalMissing(t *testing.T) {
	if _, err := parseCoverTotal([]byte("no total line here\n")); err == nil {
		t.Fatal("expected error for missing total line, got none")
	}
}

// TestGoExtractorsAgainstFixtureProject runs both Go extractors for real
// against testdata/goproject, which has three passing tests and one
// deliberately untested function (Div), so this exercises the full
// exec.CommandContext + go test -json / go tool cover path, not just the
// parsing logic above.
func TestGoExtractorsAgainstFixtureProject(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	claim := schema.Claim{ID: "test_count", Type: schema.TestCount, Runner: schema.RunnerGo}

	count, err := (goTestCountExtractor{}).Extract(ctx, "../../testdata/goproject", claim)
	if err != nil {
		t.Fatalf("goTestCountExtractor.Extract: %v", err)
	}
	if count != 3 {
		t.Errorf("test count = %v, want 3", count)
	}

	covClaim := schema.Claim{ID: "coverage", Type: schema.Coverage, Runner: schema.RunnerGo}
	pct, err := (goCoverageExtractor{}).Extract(ctx, "../../testdata/goproject", covClaim)
	if err != nil {
		t.Fatalf("goCoverageExtractor.Extract: %v", err)
	}
	// Div is untested, so coverage should be meaningfully short of 100%
	// but still well above zero (Add/Sub/Mul are fully exercised).
	if pct <= 0 || pct >= 100 {
		t.Errorf("coverage = %v%%, want a value strictly between 0 and 100", pct)
	}
}
