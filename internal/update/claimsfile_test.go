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

	spans, err := locateDeclaredSpans([]byte(content))
	if err != nil {
		t.Fatalf("locateDeclaredSpans: %v", err)
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
	if _, err := locateDeclaredSpans([]byte(content)); err == nil {
		t.Fatal("expected an error for a claim missing declared, got none")
	}
}

func TestLocateDeclaredSpansNoClaimsSequence(t *testing.T) {
	content := `repo: example
`
	if _, err := locateDeclaredSpans([]byte(content)); err == nil {
		t.Fatal("expected an error for a document with no claims sequence, got none")
	}
}
