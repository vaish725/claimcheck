package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

// newUpdateFixture creates a minimal git repo with a claims.yaml (a
// deliberately wrong commit_count claim) and a README.md marker for it, so
// `update` has something real to recompute without needing a Go/pytest
// toolchain run - that extraction path is already covered by
// internal/update's own tests; this just exercises the CLI wiring.
func newUpdateFixture(t *testing.T) (dir string) {
	t.Helper()
	dir = t.TempDir()

	readme := "# fixture\n\nCommits: <!-- claimcheck:commits -->999<!-- /claimcheck:commits -->\n"
	claims := `repo: fixture
claims:
  - id: commits
    type: commit_count
    declared: 999
    tolerance: exact
    asserted_in: [README.md]
`
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte(readme), 0o644); err != nil {
		t.Fatalf("writing README.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "claims.yaml"), []byte(claims), 0o644); err != nil {
		t.Fatalf("writing claims.yaml: %v", err)
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

func TestRunUpdateMissingClaimsFile(t *testing.T) {
	dir := t.TempDir()
	code := run([]string{"update", dir})
	if code != 1 {
		t.Errorf("run update with no claims.yaml = %d, want 1", code)
	}
}

func TestRunUpdateDryRunLeavesFilesUntouched(t *testing.T) {
	dir := newUpdateFixture(t)

	before, err := os.ReadFile(filepath.Join(dir, "README.md"))
	if err != nil {
		t.Fatalf("reading README.md: %v", err)
	}

	code := run([]string{"update", "-dry-run", dir})
	if code != 0 {
		t.Errorf("run update -dry-run = %d, want 0", code)
	}

	after, err := os.ReadFile(filepath.Join(dir, "README.md"))
	if err != nil {
		t.Fatalf("reading README.md: %v", err)
	}
	if string(before) != string(after) {
		t.Errorf("update -dry-run modified README.md on disk")
	}
	if !strings.Contains(string(after), "999") {
		t.Errorf("README.md no longer contains the original (unwritten) value 999")
	}
}

// TestRunUpdateDryRunSummaryDoesNotClaimFilesWereWritten guards against a
// real bug this feature shipped with once: -dry-run must never print
// "updated X" when X was never actually touched on disk.
func TestRunUpdateDryRunSummaryDoesNotClaimFilesWereWritten(t *testing.T) {
	dir := newUpdateFixture(t)

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("creating pipe: %v", err)
	}
	origStdout := os.Stdout
	os.Stdout = w
	code := run([]string{"update", "-dry-run", dir})
	w.Close()
	os.Stdout = origStdout

	var buf bytes.Buffer
	buf.ReadFrom(r)
	out := buf.String()

	if code != 0 {
		t.Errorf("run update -dry-run = %d, want 0", code)
	}
	if strings.Contains(out, "updated ") {
		t.Errorf("dry-run summary claims files were updated; got:\n%s", out)
	}
	if !strings.Contains(out, "would update") {
		t.Errorf("dry-run summary missing expected \"would update\" wording; got:\n%s", out)
	}
}

func TestRunUpdateWritesChanges(t *testing.T) {
	dir := newUpdateFixture(t)

	code := run([]string{"update", dir})
	if code != 0 {
		t.Errorf("run update = %d, want 0", code)
	}

	readme, err := os.ReadFile(filepath.Join(dir, "README.md"))
	if err != nil {
		t.Fatalf("reading README.md: %v", err)
	}
	if strings.Contains(string(readme), "999") {
		t.Errorf("README.md still contains the deliberately-wrong declared value 999 after update; got:\n%s", readme)
	}

	claims, err := os.ReadFile(filepath.Join(dir, "claims.yaml"))
	if err != nil {
		t.Fatalf("reading claims.yaml: %v", err)
	}
	if strings.Contains(string(claims), "declared: 999") {
		t.Errorf("claims.yaml still contains the deliberately-wrong declared value after update; got:\n%s", claims)
	}
}
