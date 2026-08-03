package schema

import (
	"fmt"
	"os"
	"runtime"

	"gopkg.in/yaml.v3"
)

// ClaimType identifies which extractor should recompute a claim's actual value.
type ClaimType string

const (
	TestCount   ClaimType = "test_count"
	Coverage    ClaimType = "coverage"
	LOC         ClaimType = "loc"
	CommitCount ClaimType = "commit_count"
	Benchmark   ClaimType = "benchmark"
)

// Runner identifies which language toolchain a test_count or coverage claim
// should be recomputed with.
type Runner string

const (
	RunnerGo     Runner = "go"
	RunnerPytest Runner = "pytest"
)

// Claim is one declared, checkable fact about a repo, as written in
// claims.yaml.
type Claim struct {
	ID         string    `yaml:"id"`
	Type       ClaimType `yaml:"type"`
	Runner     Runner    `yaml:"runner,omitempty"`
	Command    string    `yaml:"command,omitempty"` // benchmark: shell command to run
	Field      string    `yaml:"field,omitempty"`   // benchmark: JSON field to read from the command's stdout
	Machine    string    `yaml:"machine,omitempty"` // benchmark: fingerprint this value was last declared on
	Declared   float64   `yaml:"declared"`
	Unit       string    `yaml:"unit,omitempty"`
	Tolerance  string    `yaml:"tolerance"`
	AssertedIn []string  `yaml:"asserted_in,omitempty"`

	// ParsedTolerance is set by Validate; callers should read this, not Tolerance.
	ParsedTolerance Tolerance `yaml:"-"`
}

// CurrentMachineFingerprint identifies this machine well enough to catch
// "this benchmark ran on a different laptop or CI runner" - the one thing
// a tolerance band can't paper over. It deliberately isn't a precise
// hardware ID, just OS/arch/core-count from the stdlib.
func CurrentMachineFingerprint() string {
	return fmt.Sprintf("%s/%s/%dcpu", runtime.GOOS, runtime.GOARCH, runtime.NumCPU())
}

// UnsetMachine is the placeholder a benchmark claim's "machine" field
// should hold before it has ever been established. YAML has no bare-scalar
// spelling of an empty string that update's byte-splice rewriter can
// safely locate and expand (a quoted "" would need offset math a quoted
// scalar's Column doesn't support), so "unset" is an ordinary bare token
// instead. It is treated as "not yet recorded", same as an empty string.
const UnsetMachine = "unset"

// MachineRecorded reports whether this claim's machine field holds a real,
// previously-established fingerprint rather than being empty or the
// UnsetMachine placeholder.
func (c Claim) MachineRecorded() bool {
	return c.Machine != "" && c.Machine != UnsetMachine
}

// ResumeTarget is the reserved asserted_in entry standing for a person's
// résumé rather than a file in the repo. update.go skips it when treating
// asserted_in entries as files to rewrite; resume mode selects for it.
const ResumeTarget = "resume"

// ClaimsFile is the top-level shape of claims.yaml.
type ClaimsFile struct {
	Repo   string  `yaml:"repo"`
	Claims []Claim `yaml:"claims"`
}

// Load reads, parses, and validates a claims.yaml file at path.
func Load(path string) (*ClaimsFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}

	var cf ClaimsFile
	if err := yaml.Unmarshal(data, &cf); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}

	if err := cf.Validate(); err != nil {
		return nil, fmt.Errorf("validating %s: %w", path, err)
	}

	return &cf, nil
}

// Validate checks every claim has the fields it needs to be checkable and
// parses each claim's tolerance into ParsedTolerance.
func (cf *ClaimsFile) Validate() error {
	if len(cf.Claims) == 0 {
		return fmt.Errorf("claims.yaml declares no claims")
	}

	seen := make(map[string]bool, len(cf.Claims))
	for i := range cf.Claims {
		c := &cf.Claims[i]

		if c.ID == "" {
			return fmt.Errorf("claim %d: id is required", i)
		}
		if seen[c.ID] {
			return fmt.Errorf("claim %q: duplicate id", c.ID)
		}
		seen[c.ID] = true

		if c.Type == "" {
			return fmt.Errorf("claim %q: type is required", c.ID)
		}
		if len(c.AssertedIn) == 0 {
			return fmt.Errorf("claim %q: asserted_in is required (where is this claim made?)", c.ID)
		}

		switch c.Type {
		case TestCount, Coverage:
			if c.Runner != RunnerGo && c.Runner != RunnerPytest {
				return fmt.Errorf("claim %q: runner must be %q or %q for type %q", c.ID, RunnerGo, RunnerPytest, c.Type)
			}
		case LOC, CommitCount:
			// language-agnostic: no runner needed.
		case Benchmark:
			if c.Command == "" {
				return fmt.Errorf("claim %q: command is required for type %q", c.ID, c.Type)
			}
			if c.Field == "" {
				return fmt.Errorf("claim %q: field is required for type %q", c.ID, c.Type)
			}
		default:
			return fmt.Errorf("claim %q: unsupported type %q", c.ID, c.Type)
		}

		tol, err := ParseTolerance(c.Tolerance)
		if err != nil {
			return fmt.Errorf("claim %q: %w", c.ID, err)
		}
		c.ParsedTolerance = tol
	}

	return nil
}
