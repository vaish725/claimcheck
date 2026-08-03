package schema

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func writeTempClaims(t *testing.T, contents string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "claims.yaml")
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}
	return path
}

func TestLoadValid(t *testing.T) {
	path := writeTempClaims(t, `
repo: example
claims:
  - id: test_count
    type: test_count
    runner: go
    declared: 88
    tolerance: exact
    asserted_in: [README.md]

  - id: coverage
    type: coverage
    runner: go
    declared: 54
    unit: percent
    tolerance: "+-3%"
    asserted_in: [README.md, resume]

  - id: loc
    type: loc
    declared: 5718
    tolerance: "+-100"
    asserted_in: [resume]

  - id: commits
    type: commit_count
    declared: 26
    tolerance: "+-5"
    asserted_in: [resume]
`)

	cf, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cf.Repo != "example" {
		t.Errorf("Repo = %q, want %q", cf.Repo, "example")
	}
	if len(cf.Claims) != 4 {
		t.Fatalf("len(Claims) = %d, want 4", len(cf.Claims))
	}
	if cf.Claims[0].ParsedTolerance.Kind != Exact {
		t.Errorf("claim 0 ParsedTolerance.Kind = %v, want Exact", cf.Claims[0].ParsedTolerance.Kind)
	}
}

func TestLoadMissingTolerance(t *testing.T) {
	path := writeTempClaims(t, `
repo: example
claims:
  - id: test_count
    type: test_count
    runner: go
    declared: 88
    asserted_in: [README.md]
`)

	if _, err := Load(path); err == nil {
		t.Fatal("expected error for missing tolerance, got none")
	}
}

func TestLoadMissingAssertedIn(t *testing.T) {
	path := writeTempClaims(t, `
repo: example
claims:
  - id: test_count
    type: test_count
    runner: go
    declared: 88
    tolerance: exact
`)

	if _, err := Load(path); err == nil {
		t.Fatal("expected error for missing asserted_in, got none")
	}
}

func TestLoadDuplicateID(t *testing.T) {
	path := writeTempClaims(t, `
repo: example
claims:
  - id: dup
    type: loc
    declared: 100
    tolerance: "+-10"
    asserted_in: [README.md]
  - id: dup
    type: commit_count
    declared: 5
    tolerance: "+-1"
    asserted_in: [README.md]
`)

	if _, err := Load(path); err == nil {
		t.Fatal("expected error for duplicate id, got none")
	}
}

func TestLoadMissingRunnerForTestCount(t *testing.T) {
	path := writeTempClaims(t, `
repo: example
claims:
  - id: test_count
    type: test_count
    declared: 88
    tolerance: exact
    asserted_in: [README.md]
`)

	if _, err := Load(path); err == nil {
		t.Fatal("expected error for missing runner on test_count claim, got none")
	}
}

func TestLoadNoClaims(t *testing.T) {
	path := writeTempClaims(t, `
repo: example
claims: []
`)

	if _, err := Load(path); err == nil {
		t.Fatal("expected error for empty claims list, got none")
	}
}

func TestLoadBenchmarkValid(t *testing.T) {
	path := writeTempClaims(t, `
repo: example
claims:
  - id: query_p50
    type: benchmark
    command: "echo '{\"p50_ms\": 0.09}'"
    field: p50_ms
    declared: 0.09
    machine: ""
    tolerance: "+-25%"
    asserted_in: [resume]
`)

	cf, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	c := cf.Claims[0]
	if c.Command == "" || c.Field != "p50_ms" {
		t.Errorf("benchmark claim fields not loaded: %+v", c)
	}
}

func TestLoadBenchmarkMissingCommand(t *testing.T) {
	path := writeTempClaims(t, `
repo: example
claims:
  - id: query_p50
    type: benchmark
    field: p50_ms
    declared: 0.09
    tolerance: "+-25%"
    asserted_in: [resume]
`)

	if _, err := Load(path); err == nil {
		t.Fatal("expected error for benchmark claim missing command, got none")
	}
}

func TestLoadBenchmarkMissingField(t *testing.T) {
	path := writeTempClaims(t, `
repo: example
claims:
  - id: query_p50
    type: benchmark
    command: "echo '{\"p50_ms\": 0.09}'"
    declared: 0.09
    tolerance: "+-25%"
    asserted_in: [resume]
`)

	if _, err := Load(path); err == nil {
		t.Fatal("expected error for benchmark claim missing field, got none")
	}
}

func TestMachineRecorded(t *testing.T) {
	cases := []struct {
		machine string
		want    bool
	}{
		{"", false},
		{UnsetMachine, false},
		{"darwin/arm64/8cpu", true},
	}
	for _, tc := range cases {
		c := Claim{Machine: tc.machine}
		if got := c.MachineRecorded(); got != tc.want {
			t.Errorf("MachineRecorded() for %q = %v, want %v", tc.machine, got, tc.want)
		}
	}
}

func TestCurrentMachineFingerprint(t *testing.T) {
	fp := CurrentMachineFingerprint()
	if fp == "" {
		t.Fatal("CurrentMachineFingerprint() returned an empty string")
	}
	if !strings.Contains(fp, runtime.GOOS) {
		t.Errorf("fingerprint %q does not contain GOOS %q", fp, runtime.GOOS)
	}
	if !strings.Contains(fp, runtime.GOARCH) {
		t.Errorf("fingerprint %q does not contain GOARCH %q", fp, runtime.GOARCH)
	}
}
