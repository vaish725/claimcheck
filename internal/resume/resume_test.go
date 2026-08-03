package resume

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeFixtureClaims writes a claims.yaml with one claim asserted_in
// resume (declared as resumeDeclared, actual is always 1 via a trivial
// echo "benchmark") and one claim that is not, so tests can confirm
// filtering. No git repo is needed - benchmark claims just run a shell
// command.
func writeFixtureClaims(t *testing.T, dir string, resumeDeclared float64) {
	t.Helper()
	content := fmt.Sprintf(`repo: %s
claims:
  - id: resume_claim
    type: benchmark
    command: "echo '{\"v\": 1}'"
    field: v
    declared: %v
    tolerance: exact
    asserted_in: [resume]

  - id: other_claim
    type: benchmark
    command: "echo '{\"v\": 1}'"
    field: v
    declared: 1
    tolerance: exact
    asserted_in: [README.md]
`, filepath.Base(dir), resumeDeclared)
	if err := os.WriteFile(filepath.Join(dir, "claims.yaml"), []byte(content), 0o644); err != nil {
		t.Fatalf("writing claims.yaml: %v", err)
	}
}

func TestLoadFileResolvesPaths(t *testing.T) {
	dir := t.TempDir()
	resumePath := filepath.Join(dir, "resume.yaml")

	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("no home directory available: %v", err)
	}

	content := "repos:\n" +
		"  - path: ~/somewhere\n" +
		"  - path: relative-repo\n" +
		"  - path: " + filepath.Join(dir, "absolute-repo") + "\n"
	if err := os.WriteFile(resumePath, []byte(content), 0o644); err != nil {
		t.Fatalf("writing resume.yaml: %v", err)
	}

	f, err := LoadFile(resumePath)
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	if len(f.Repos) != 3 {
		t.Fatalf("len(Repos) = %d, want 3", len(f.Repos))
	}

	wantTilde := filepath.Join(home, "somewhere")
	if f.Repos[0].Path != wantTilde {
		t.Errorf("tilde path = %q, want %q", f.Repos[0].Path, wantTilde)
	}
	wantRelative := filepath.Join(dir, "relative-repo")
	if f.Repos[1].Path != wantRelative {
		t.Errorf("relative path = %q, want %q", f.Repos[1].Path, wantRelative)
	}
	wantAbsolute := filepath.Join(dir, "absolute-repo")
	if f.Repos[2].Path != wantAbsolute {
		t.Errorf("absolute path = %q, want %q", f.Repos[2].Path, wantAbsolute)
	}
}

func TestLoadFileNoRepos(t *testing.T) {
	dir := t.TempDir()
	resumePath := filepath.Join(dir, "resume.yaml")
	if err := os.WriteFile(resumePath, []byte("repos: []\n"), 0o644); err != nil {
		t.Fatalf("writing resume.yaml: %v", err)
	}
	if _, err := LoadFile(resumePath); err == nil {
		t.Fatal("expected an error for an empty repos list, got none")
	}
}

func TestRunFiltersToResumeClaimsAndHandlesUnreachableRepo(t *testing.T) {
	dir := t.TempDir()

	goodRepo := filepath.Join(dir, "good")
	if err := os.MkdirAll(goodRepo, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	writeFixtureClaims(t, goodRepo, 1) // matches actual, so it passes

	missingRepo := filepath.Join(dir, "does-not-exist")

	resumePath := filepath.Join(dir, "resume.yaml")
	content := "repos:\n" +
		"  - path: " + goodRepo + "\n" +
		"  - path: " + missingRepo + "\n"
	if err := os.WriteFile(resumePath, []byte(content), 0o644); err != nil {
		t.Fatalf("writing resume.yaml: %v", err)
	}

	results, err := Run(context.Background(), resumePath)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(results))
	}

	byPath := make(map[string]RepoResult, len(results))
	for _, r := range results {
		byPath[r.Path] = r
	}

	good := byPath[goodRepo]
	if good.Err != nil {
		t.Errorf("good repo Err = %v, want nil", good.Err)
	}
	if len(good.Report.Rows) != 1 || good.Report.Rows[0].Claim.ID != "resume_claim" {
		t.Errorf("good repo rows = %+v, want exactly the resume_claim row", good.Report.Rows)
	}

	missing := byPath[missingRepo]
	if missing.Err == nil {
		t.Error("missing repo Err = nil, want an error (no claims.yaml there)")
	}
}

func TestBreached(t *testing.T) {
	dir := t.TempDir()
	passRepo := filepath.Join(dir, "pass")
	if err := os.MkdirAll(passRepo, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	writeFixtureClaims(t, passRepo, 1)

	breachRepo := filepath.Join(dir, "breach")
	if err := os.MkdirAll(breachRepo, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	writeFixtureClaims(t, breachRepo, 999) // won't match the actual value of 1

	ctx := context.Background()
	passResult := checkRepo(ctx, passRepo)
	breachResult := checkRepo(ctx, breachRepo)
	errResult := RepoResult{Path: "nowhere", Err: fmt.Errorf("boom")}

	if Breached([]RepoResult{passResult}) {
		t.Error("Breached([pass]) = true, want false")
	}
	if !Breached([]RepoResult{passResult, breachResult}) {
		t.Error("Breached([pass, breach]) = false, want true")
	}
	if !Breached([]RepoResult{passResult, errResult}) {
		t.Error("Breached([pass, err]) = false, want true")
	}
}

func TestWriteSummary(t *testing.T) {
	dir := t.TempDir()
	goodRepo := filepath.Join(dir, "good")
	if err := os.MkdirAll(goodRepo, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	writeFixtureClaims(t, goodRepo, 1)

	results := []RepoResult{
		checkRepo(context.Background(), goodRepo),
		{Path: "/nowhere", Err: fmt.Errorf("no such file or directory")},
	}

	var buf strings.Builder
	if err := WriteSummary(&buf, results); err != nil {
		t.Fatalf("WriteSummary: %v", err)
	}
	out := buf.String()

	for _, want := range []string{goodRepo, "resume_claim", "PASS", "/nowhere", "ERROR", "no such file or directory"} {
		if !strings.Contains(out, want) {
			t.Errorf("summary output missing %q; got:\n%s", want, out)
		}
	}
	if strings.Contains(out, "other_claim") {
		t.Errorf("summary output includes a non-resume claim; got:\n%s", out)
	}
}
