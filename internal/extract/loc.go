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

// locExtractor counts lines across every file `git ls-files` considers
// part of the working tree, so .gitignore rules are honored for free.
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
			continue // unreadable file (broken symlink, permissions) shouldn't abort the whole count
		}
		total += n
	}
	return float64(total), nil
}

// listRepoFiles lists tracked plus untracked-but-not-ignored files - the
// set a contributor would consider "the code in this repo".
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
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024) // handle minified/generated files with very long lines

	count := 0
	for scanner.Scan() {
		count++
	}
	return count, scanner.Err()
}
