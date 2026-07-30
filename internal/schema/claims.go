package schema

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// ClaimType identifies which extractor should recompute a claim's actual value.
type ClaimType string

const (
	TestCount   ClaimType = "test_count"
	Coverage    ClaimType = "coverage"
	LOC         ClaimType = "loc"
	CommitCount ClaimType = "commit_count"
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
	Declared   float64   `yaml:"declared"`
	Unit       string    `yaml:"unit,omitempty"`
	Tolerance  string    `yaml:"tolerance"`
	AssertedIn []string  `yaml:"asserted_in,omitempty"`

	// ParsedTolerance is set by Validate; callers should read this, not Tolerance.
	ParsedTolerance Tolerance `yaml:"-"`
}

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
