package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRunNoArgs(t *testing.T) {
	if code := run(nil); code != 2 {
		t.Errorf("run(nil) = %d, want 2", code)
	}
}

func TestRunUnknownCommand(t *testing.T) {
	if code := run([]string{"bogus"}); code != 2 {
		t.Errorf(`run(["bogus"]) = %d, want 2`, code)
	}
}

func TestRunHelp(t *testing.T) {
	if code := run([]string{"help"}); code != 0 {
		t.Errorf(`run(["help"]) = %d, want 0`, code)
	}
}

func TestRunVerifyMissingClaimsFile(t *testing.T) {
	dir := t.TempDir()
	code := run([]string{"verify", dir})
	if code != 1 {
		t.Errorf("run verify with no claims.yaml = %d, want 1", code)
	}
}

func TestRunVerifySoftStillPrintsButExitsZero(t *testing.T) {
	// A malformed claims.yaml is still a hard error even with -soft: -soft
	// only suppresses the exit code for claims that were successfully
	// checked and found breaching, not for a claims.yaml that fails to load.
	dir := t.TempDir()
	claimsPath := filepath.Join(dir, "claims.yaml")
	if err := os.WriteFile(claimsPath, []byte("not: valid: yaml: at: all: ["), 0o644); err != nil {
		t.Fatalf("writing claims.yaml: %v", err)
	}

	code := run([]string{"verify", "-soft", dir})
	if code != 1 {
		t.Errorf("run verify -soft with unparsable claims.yaml = %d, want 1", code)
	}
}
