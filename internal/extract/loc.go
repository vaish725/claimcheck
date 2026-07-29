package extract

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/vaish725/claimcheck/internal/schema"
)

// locExtractor recomputes a loc claim by counting lines across every file
// git considers part of the working tree: tracked files plus untracked
// files that aren't ignored. Using `git ls-files` instead of a hand-rolled
// walker means .gitignore rules (including nested ones) are honored for
// free, with no gitignore-parsing code of our own to maintain.
type locExtractor struct{}

func (locExtractor) Extract(ctx context.Context, repoPath string, _ schema.Claim) (float64, error) {
	files, err := listRepoFiles(ctx, repoPath)
	if err != nil {
		return 0, err
	}
	if len(files) == 0 {
		return 0, fmt.Errorf("git ls-files returned no files in %s", repoPath)
	}

	total := 0
	for _, rel := range files {
		n, err := countLines(filepath.Join(repoPath, rel))
		if err != nil {
			// A file git tracks but the OS can't read (broken symlink,
			// permissions, or a binary that isn't really text) shouldn't
			// abort the whole claim; skip it and keep counting the rest.
			continue
		}
		total += n
	}
	return float64(total), nil
}

// listRepoFiles runs `git ls-files --cached --others --exclude-standard`,
// which lists tracked files plus untracked-but-not-ignored files, exactly
// the set a contributor would consider "the code in this repo".
func listRepoFiles(ctx context.Context, repoPath string) ([]string, error) {
	cmd := exec.CommandContext(ctx, "git", "ls-files", "--cached", "--others", "--exclude-standard")
	cmd.Dir = repoPath
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("running git ls-files: %w: %s", err, stderr.String())
	}

	var files []string
	scanner := bufio.NewScanner(&stdout)
	for scanner.Scan() {
		line := scanner.Text()
		if line != "" {
			files = append(files, line)
		}
	}
	return files, scanner.Err()
}

func countLines(path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	// bufio.Scanner's default 64KiB line limit can't handle minified or
	// generated files with extremely long lines; grow the buffer rather
	// than fail the whole LOC count over one outlier file.
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)

	count := 0
	for scanner.Scan() {
		count++
	}
	return count, scanner.Err()
}
