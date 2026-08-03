package extract

import (
	"context"
	"testing"
	"time"

	"github.com/vaish725/claimcheck/internal/schema"
)

func TestExtractJSONField(t *testing.T) {
	output := []byte(`{"p50_ms": 0.09, "p95_ms": 0.21, "label": "warm"}`)

	got, err := extractJSONField(output, "p50_ms")
	if err != nil {
		t.Fatalf("extractJSONField: %v", err)
	}
	if got != 0.09 {
		t.Errorf("extractJSONField = %v, want 0.09", got)
	}
}

func TestExtractJSONFieldMissing(t *testing.T) {
	output := []byte(`{"p50_ms": 0.09}`)
	if _, err := extractJSONField(output, "p95_ms"); err == nil {
		t.Fatal("expected error for missing field, got none")
	}
}

func TestExtractJSONFieldNotANumber(t *testing.T) {
	output := []byte(`{"p50_ms": "fast"}`)
	if _, err := extractJSONField(output, "p50_ms"); err == nil {
		t.Fatal("expected error for non-numeric field, got none")
	}
}

func TestExtractJSONFieldInvalidJSON(t *testing.T) {
	if _, err := extractJSONField([]byte("not json"), "p50_ms"); err == nil {
		t.Fatal("expected error for invalid JSON, got none")
	}
}

// TestBenchmarkExtractorRunsCommand runs a real command through
// benchmarkExtractor.Extract - the command's own output is the fixture, so
// no testdata directory is needed.
func TestBenchmarkExtractorRunsCommand(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	claim := schema.Claim{
		ID:      "query_p50",
		Type:    schema.Benchmark,
		Command: `echo '{"p50_ms": 0.09}'`,
		Field:   "p50_ms",
	}

	got, err := (benchmarkExtractor{}).Extract(ctx, t.TempDir(), claim)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if got != 0.09 {
		t.Errorf("Extract = %v, want 0.09", got)
	}
}

// TestBenchmarkExtractorNonZeroExitIsFatal differs from the go test/pytest
// extractors: a benchmark script has no "some benchmarks failed but the
// JSON is still trustworthy" case, so any non-zero exit is a hard failure.
func TestBenchmarkExtractorNonZeroExitIsFatal(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	claim := schema.Claim{
		ID:      "query_p50",
		Type:    schema.Benchmark,
		Command: `echo '{"p50_ms": 0.09}'; exit 1`,
		Field:   "p50_ms",
	}

	if _, err := (benchmarkExtractor{}).Extract(ctx, t.TempDir(), claim); err == nil {
		t.Fatal("expected an error for a non-zero exit, got none")
	}
}
