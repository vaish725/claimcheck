package extract

import (
	"context"
	"testing"

	"github.com/vaish725/claimcheck/internal/schema"
)

func TestCommitCountExtractor(t *testing.T) {
	dir := newGitRepo(t)
	for i := 0; i < 4; i++ {
		writeFile(t, dir, "file.txt", "revision")
		runGit(t, dir, "add", "file.txt")
		runGit(t, dir, "commit", "-q", "--allow-empty", "-m", "commit")
	}

	got, err := (commitCountExtractor{}).Extract(context.Background(), dir, schema.Claim{})
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if got != 4 {
		t.Errorf("commit count = %v, want 4", got)
	}
}
