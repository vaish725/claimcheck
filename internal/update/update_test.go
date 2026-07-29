package update

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// newFixtureRepo creates a tiny, fully-covered Go module in a fresh git
// repo, plus a claims.yaml (with declared values deliberately wrong, and a
// comment that must survive any rewrite) and a README.md with a marker
// for the test_count claim only. The coverage claim asserts itself in
// both README.md and the reserved "resume" placeholder, so BuildPlan must
// skip "resume" without trying to open it as a file.
func newFixtureRepo(t *testing.T) (dir, claimsPath string) {
	t.Helper()
	dir = t.TempDir()

	files := map[string]string{
		"go.mod": "module updatefixture\n\ngo 1.21\n",
		"add.go": "package updatefixture\n\nfunc Add(a, b int) int {\n\treturn a + b\n}\n",
		"add_test.go": "package updatefixture\n\nimport \"testing\"\n\n" +
			"func TestAdd(t *testing.T) {\n\tif Add(2, 3) != 5 {\n\t\tt.Fatal(\"bad\")\n\t}\n}\n",
		"claims.yaml": `repo: fixture
claims:
  - id: test_count
    type: test_count
    runner: go
    declared: 999   # deliberately wrong, comment must survive
    tolerance: exact
    asserted_in: [README.md]

  - id: coverage
    type: coverage
    runner: go
    declared: 1
    tolerance: "+-1%"
    asserted_in: [README.md, resume]
`,
		"README.md": "# fixture\n\n" +
			"Tests: <!-- claimcheck:test_count -->999<!-- /claimcheck:test_count --> passing.\n" +
			"Coverage not shown here.\n",
	}
	for name, contents := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(contents), 0o644); err != nil {
			t.Fatalf("writing %s: %v", name, err)
		}
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

	return dir, filepath.Join(dir, "claims.yaml")
}

func TestBuildPlanAndApplyEndToEnd(t *testing.T) {
	dir, claimsPath := newFixtureRepo(t)
	ctx := context.Background()

	plan, err := BuildPlan(ctx, dir, claimsPath)
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	if plan.Failed() {
		for _, c := range plan.Changes {
			if c.Err != nil {
				t.Errorf("claim %q failed to extract: %v", c.Claim.ID, c.Err)
			}
		}
		t.Fatal("Plan.Failed() = true, want false")
	}
	if !plan.Changed() {
		t.Fatal("Plan.Changed() = false, want true (declared values are deliberately wrong)")
	}

	changesByID := make(map[string]Change, len(plan.Changes))
	for _, c := range plan.Changes {
		changesByID[c.Claim.ID] = c
	}
	if got := changesByID["test_count"].NewValue; got != 1 {
		t.Errorf("test_count NewValue = %v, want 1", got)
	}
	if got := changesByID["coverage"].NewValue; got != 100 {
		t.Errorf("coverage NewValue = %v, want 100 (Add is fully covered by its one test)", got)
	}

	if len(plan.Files) != 2 {
		t.Fatalf("len(Files) = %d, want 2 (claims.yaml and README.md)", len(plan.Files))
	}
	filesByPath := make(map[string]FileChange, len(plan.Files))
	for _, fc := range plan.Files {
		filesByPath[fc.Path] = fc
	}

	claimsChange, ok := filesByPath[claimsPath]
	if !ok {
		t.Fatalf("no FileChange for %s", claimsPath)
	}
	newClaims := string(claimsChange.NewData)
	if !strings.Contains(newClaims, "declared: 1   # deliberately wrong, comment must survive") {
		t.Errorf("claims.yaml rewrite did not preserve the trailing comment; got:\n%s", newClaims)
	}
	if !strings.Contains(newClaims, "declared: 100\n") {
		t.Errorf("claims.yaml rewrite did not update the coverage declared value; got:\n%s", newClaims)
	}

	readmePath := filepath.Join(dir, "README.md")
	readmeChange, ok := filesByPath[readmePath]
	if !ok {
		t.Fatalf("no FileChange for %s", readmePath)
	}
	wantReadme := "# fixture\n\n" +
		"Tests: <!-- claimcheck:test_count -->1<!-- /claimcheck:test_count --> passing.\n" +
		"Coverage not shown here.\n"
	if string(readmeChange.NewData) != wantReadme {
		t.Errorf("README.md rewrite = %q, want %q", readmeChange.NewData, wantReadme)
	}

	// BuildPlan alone must not have touched disk.
	onDisk, err := os.ReadFile(claimsPath)
	if err != nil {
		t.Fatalf("reading claims.yaml: %v", err)
	}
	if string(onDisk) != string(claimsChange.OldData) {
		t.Errorf("BuildPlan modified claims.yaml on disk before Apply was called")
	}

	if err := Apply(plan); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	gotClaims, err := os.ReadFile(claimsPath)
	if err != nil {
		t.Fatalf("reading claims.yaml after Apply: %v", err)
	}
	if string(gotClaims) != newClaims {
		t.Errorf("claims.yaml on disk after Apply does not match the planned content")
	}
	gotReadme, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("reading README.md after Apply: %v", err)
	}
	if string(gotReadme) != wantReadme {
		t.Errorf("README.md on disk after Apply does not match the planned content")
	}

	// A second BuildPlan, now that declared values match reality, should
	// report nothing left to change.
	plan2, err := BuildPlan(ctx, dir, claimsPath)
	if err != nil {
		t.Fatalf("second BuildPlan: %v", err)
	}
	if plan2.Changed() {
		t.Errorf("second Plan.Changed() = true, want false (update should be idempotent)")
	}
}

func TestBuildPlanMissingAssertedInFile(t *testing.T) {
	dir := t.TempDir()
	claimsPath := filepath.Join(dir, "claims.yaml")
	content := `repo: fixture
claims:
  - id: commits
    type: commit_count
    declared: 1
    tolerance: exact
    asserted_in: [MISSING.md]
`
	if err := os.WriteFile(claimsPath, []byte(content), 0o644); err != nil {
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

	if _, err := BuildPlan(context.Background(), dir, claimsPath); err == nil {
		t.Fatal("expected an error for a claim asserting itself in a file that doesn't exist, got none")
	}
}

func TestBuildPlanResumeOnlyAssertedInIsNotTreatedAsAFile(t *testing.T) {
	dir := t.TempDir()
	claimsPath := filepath.Join(dir, "claims.yaml")
	content := `repo: fixture
claims:
  - id: commits
    type: commit_count
    declared: 1
    tolerance: exact
    asserted_in: [resume]
`
	if err := os.WriteFile(claimsPath, []byte(content), 0o644); err != nil {
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
	runGit("commit", "-q", "--allow-empty", "-m", "initial")
	runGit("commit", "-q", "--allow-empty", "-m", "second") // commit_count should become 2

	plan, err := BuildPlan(context.Background(), dir, claimsPath)
	if err != nil {
		t.Fatalf("BuildPlan: %v (a \"resume\"-only asserted_in must not be opened as a file)", err)
	}
	if plan.Failed() {
		t.Fatalf("Plan.Failed() = true: %+v", plan.Changes)
	}
	// claims.yaml itself is the only file that should have changed; there
	// is no real file named "resume" to have been touched.
	for _, fc := range plan.Files {
		if fc.Path == "resume" || strings.HasSuffix(fc.Path, string(os.PathSeparator)+"resume") {
			t.Errorf("plan attempted to rewrite a file for the reserved \"resume\" target: %s", fc.Path)
		}
	}
}
