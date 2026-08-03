package update

import (
	"testing"
)

func TestLocateDeclaredSpans(t *testing.T) {
	content := `repo: example
claims:
  - id: test_count
    type: test_count
    runner: go
    declared: 88   # a comment here, must survive untouched
    tolerance: exact
    asserted_in: [README.md]

  - id: coverage
    type: coverage
    runner: go
    declared: 54.3
    unit: percent
    tolerance: "+-3%"
    asserted_in: [README.md, resume]
`

	spans, err := locateFieldSpans([]byte(content), "declared", true)
	if err != nil {
		t.Fatalf("locateFieldSpans: %v", err)
	}
	if len(spans) != 2 {
		t.Fatalf("len(spans) = %d, want 2", len(spans))
	}

	byID := make(map[string]string, len(spans))
	for _, sp := range spans {
		byID[sp.ID] = content[sp.Start:sp.End]
	}

	if got := byID["test_count"]; got != "88" {
		t.Errorf(`spans["test_count"] content = %q, want "88"`, got)
	}
	if got := byID["coverage"]; got != "54.3" {
		t.Errorf(`spans["coverage"] content = %q, want "54.3"`, got)
	}
}

func TestLocateDeclaredSpansMissingDeclared(t *testing.T) {
	content := `repo: example
claims:
  - id: broken
    type: loc
    tolerance: exact
    asserted_in: [README.md]
`
	if _, err := locateFieldSpans([]byte(content), "declared", true); err == nil {
		t.Fatal("expected an error for a claim missing declared, got none")
	}
}

func TestLocateDeclaredSpansNoClaimsSequence(t *testing.T) {
	content := `repo: example
`
	if _, err := locateFieldSpans([]byte(content), "declared", true); err == nil {
		t.Fatal("expected an error for a document with no claims sequence, got none")
	}
}

func TestLocateFieldSpansOptionalFieldSkipsAbsentClaims(t *testing.T) {
	content := `repo: example
claims:
  - id: with_machine
    type: benchmark
    command: "echo hi"
    field: p50_ms
    declared: 0.09
    machine: unset
    tolerance: "+-25%"
    asserted_in: [resume]

  - id: without_machine
    type: loc
    declared: 100
    tolerance: "+-10"
    asserted_in: [README.md]
`

	spans, err := locateFieldSpans([]byte(content), "machine", false)
	if err != nil {
		t.Fatalf("locateFieldSpans: %v", err)
	}
	if len(spans) != 1 {
		t.Fatalf("len(spans) = %d, want 1 (only with_machine has a machine field)", len(spans))
	}
	if spans[0].ID != "with_machine" {
		t.Errorf("span ID = %q, want %q", spans[0].ID, "with_machine")
	}
	if got := content[spans[0].Start:spans[0].End]; got != "unset" {
		t.Errorf("span content = %q, want %q", got, "unset")
	}
}

func TestLocateFieldSpansRejectsQuotedValue(t *testing.T) {
	content := `repo: example
claims:
  - id: quoted
    type: benchmark
    command: "echo hi"
    field: p50_ms
    declared: 0.09
    machine: "darwin/arm64/8cpu"
    tolerance: "+-25%"
    asserted_in: [resume]
`
	if _, err := locateFieldSpans([]byte(content), "machine", false); err == nil {
		t.Fatal("expected an error for a quoted machine value, got none")
	}
}
