package rewrite

import (
	"fmt"
	"os"
	"path/filepath"
)

// WriteFileAtomic writes data to path via a same-directory temp file plus
// rename, so a crash or interrupt mid-write never leaves path corrupted.
// Caller supplies perm (e.g. from stat'ing the file being replaced).
func WriteFileAtomic(path string, data []byte, perm os.FileMode) (err error) {
	dir := filepath.Dir(path)

	tmp, err := os.CreateTemp(dir, ".claimcheck-tmp-*")
	if err != nil {
		return fmt.Errorf("creating temp file in %s: %w", dir, err)
	}
	tmpPath := tmp.Name()
	defer func() {
		if err != nil {
			os.Remove(tmpPath)
		}
	}()
	defer tmp.Close()

	if _, err = tmp.Write(data); err != nil {
		return fmt.Errorf("writing temp file: %w", err)
	}
	if err = tmp.Sync(); err != nil { // flush before rename so the visible path always has persisted data
		return fmt.Errorf("syncing temp file: %w", err)
	}
	if err = tmp.Close(); err != nil {
		return fmt.Errorf("closing temp file: %w", err)
	}
	if err = os.Chmod(tmpPath, perm); err != nil {
		return fmt.Errorf("setting permissions on temp file: %w", err)
	}
	if err = os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("renaming temp file into place: %w", err)
	}
	return nil
}
