// Package resume checks every repo listed in a central resume.yaml and
// reports which résumé-asserted claims have drifted, across all of them in
// one pass.
package resume

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sync/errgroup"
	"gopkg.in/yaml.v3"

	"github.com/vaish725/claimcheck/internal/report"
	"github.com/vaish725/claimcheck/internal/schema"
	"github.com/vaish725/claimcheck/internal/verify"
)

// RepoEntry is one repo listed in resume.yaml.
type RepoEntry struct {
	Path string `yaml:"path"`
}

// File is the top-level shape of resume.yaml.
type File struct {
	Repos []RepoEntry `yaml:"repos"`
}

// LoadFile reads and parses resume.yaml. Each repo path is resolved
// eagerly: a leading "~/" expands to the user's home directory, and any
// path still relative after that is resolved against resume.yaml's own
// directory, so the file can be checked out or moved around and its
// relative paths keep working.
func LoadFile(path string) (*File, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}

	var f File
	if err := yaml.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	if len(f.Repos) == 0 {
		return nil, fmt.Errorf("%s declares no repos", path)
	}

	baseDir := filepath.Dir(path)
	for i := range f.Repos {
		resolved, err := resolveRepoPath(f.Repos[i].Path, baseDir)
		if err != nil {
			return nil, fmt.Errorf("%s: repo %d: %w", path, i, err)
		}
		f.Repos[i].Path = resolved
	}

	return &f, nil
}

func resolveRepoPath(p, baseDir string) (string, error) {
	if p == "" {
		return "", fmt.Errorf("empty repo path")
	}
	if p == "~" || strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("expanding ~: %w", err)
		}
		p = filepath.Join(home, strings.TrimPrefix(p, "~"))
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(baseDir, p)
	}
	return p, nil
}

// RepoResult is one repo's outcome. Err is set if the repo itself could
// not be checked at all (missing directory, missing or invalid
// claims.yaml) - distinct from a claim within the repo breaching or
// erroring, which shows up as an ordinary row in Report.
type RepoResult struct {
	Path   string
	Report report.Report // only rows whose claim is asserted_in resume
	Err    error
}

// Run loads resumePath and checks every listed repo concurrently, reusing
// verify.Run's own per-repo concurrent claim extraction - two levels of
// concurrency, across repos and across claims within each repo.
func Run(ctx context.Context, resumePath string) ([]RepoResult, error) {
	file, err := LoadFile(resumePath)
	if err != nil {
		return nil, err
	}

	results := make([]RepoResult, len(file.Repos))
	var g errgroup.Group
	for i, repo := range file.Repos {
		i, repo := i, repo
		g.Go(func() error {
			results[i] = checkRepo(ctx, repo.Path)
			return nil // a repo failing to check doesn't abort the others
		})
	}
	_ = g.Wait()

	return results, nil
}

// checkRepo runs verify.Run for one repo and filters its rows down to
// résumé-asserted claims.
func checkRepo(ctx context.Context, repoPath string) RepoResult {
	claimsPath := filepath.Join(repoPath, "claims.yaml")
	rep, err := verify.Run(ctx, repoPath, claimsPath)
	if err != nil {
		return RepoResult{Path: repoPath, Err: err}
	}

	var rows []report.Row
	for _, row := range rep.Rows {
		for _, target := range row.Claim.AssertedIn {
			if target == schema.ResumeTarget {
				rows = append(rows, row)
				break
			}
		}
	}

	// Repo is left unset: WriteSummary prints its own "repo: <path>" header
	// using RepoResult.Path (the actual local checkout, unambiguous across
	// repos), rather than claims.yaml's self-declared, possibly-duplicated
	// repo name.
	return RepoResult{Path: repoPath, Report: report.Report{Rows: rows}}
}

// Breached reports whether any repo failed outright, or has a résumé claim
// that breached tolerance or failed to extract.
func Breached(results []RepoResult) bool {
	for _, r := range results {
		if r.Err != nil || r.Report.Breached() {
			return true
		}
	}
	return false
}

// WriteSummary renders every repo's filtered report, or its top-level
// error if the repo couldn't be checked at all.
func WriteSummary(w io.Writer, results []RepoResult) error {
	for i, r := range results {
		if i > 0 {
			if _, err := fmt.Fprintln(w); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintf(w, "repo: %s\n", r.Path); err != nil {
			return err
		}
		if r.Err != nil {
			if _, err := fmt.Fprintf(w, "  ERROR: %v\n", r.Err); err != nil {
				return err
			}
			continue
		}
		if len(r.Report.Rows) == 0 {
			if _, err := fmt.Fprintln(w, "  (no claims asserted in resume)"); err != nil {
				return err
			}
			continue
		}
		if err := r.Report.Write(w); err != nil {
			return err
		}
	}
	return nil
}
