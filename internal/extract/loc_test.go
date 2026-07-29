package extract

import (
	"context"
	"testing"

	"github.com/vaish725/claimcheck/internal/schema"
)

func TestLOCExtractorCountsTrackedFiles(t *testing.T) {
	dir := newGitRepo(t)
	writeFile(t, dir, "main.go", "line1\nline2\nline3\n")   // 3 lines
	writeFile(t, dir, "pkg/util.go", "line1\nline2\n")       // 2 lines
	runGit(t, dir, "add", "main.go", "pkg/util.go")
	runGit(t, dir, "commit", "-q", "-m", "initial")

	got, err := (locExtractor{}).Extract(context.Background(), dir, schema.Claim{})
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if got != 5 {
		t.Errorf("loc = %v, want 5", got)
	}
}

func TestLOCExtractorRespectsGitignore(t *testing.T) {
	dir := newGitRepo(t)
	writeFile(t, dir, ".gitignore", "vendor/\n")
	writeFile(t, dir, "main.go", "line1\nline2\n")     // 2 lines, tracked
	writeFile(t, dir, "vendor/dep.go", "a\nb\nc\nd\n") // 4 lines, ignored
	runGit(t, dir, "add", ".gitignore", "main.go")
	runGit(t, dir, "commit", "-q", "-m", "initial")

	got, err := (locExtractor{}).Extract(context.Background(), dir, schema.Claim{})
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	// main.go (2 lines) + .gitignore itself (1 line); vendor/dep.go's 4
	// lines must not be included.
	if got != 3 {
		t.Errorf("loc = %v, want 3 (vendor/dep.go should be excluded)", got)
	}
}

func TestLOCExtractorIncludesUntrackedNonIgnoredFiles(t *testing.T) {
	dir := newGitRepo(t)
	writeFile(t, dir, "main.go", "line1\nline2\n")
	runGit(t, dir, "add", "main.go")
	runGit(t, dir, "commit", "-q", "-m", "initial")
	// a new file added since the last commit, not yet staged, should still
	// count: claimcheck measures repo state, not just what's committed.
	writeFile(t, dir, "new.go", "line1\nline2\nline3\n")

	got, err := (locExtractor{}).Extract(context.Background(), dir, schema.Claim{})
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if got != 5 {
		t.Errorf("loc = %v, want 5", got)
	}
}
