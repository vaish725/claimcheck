package extract

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// runGit runs a git command in dir, failing the test on error, with a
// throwaway identity so commits work without global git config.
func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	base := []string{"-c", "user.name=claimcheck-test", "-c", "user.email=test@example.com"}
	cmd := exec.Command("git", append(base, args...)...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// writeFile writes contents to a file under dir, creating parent
// directories as needed.
func writeFile(t *testing.T, dir, relPath, contents string) {
	t.Helper()
	full := filepath.Join(dir, relPath)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("creating parent dir for %s: %v", relPath, err)
	}
	if err := os.WriteFile(full, []byte(contents), 0o644); err != nil {
		t.Fatalf("writing %s: %v", relPath, err)
	}
}

// newGitRepo creates a fresh git repo in a temp directory and returns its
// path, ready for files to be added and committed by the caller.
func newGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "init", "-q")
	return dir
}
