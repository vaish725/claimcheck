package rewrite

import (
	"fmt"
	"os"
	"path/filepath"
)

// WriteFileAtomic writes data to path by writing a temp file in path's own
// directory and renaming it into place, so a crash, interrupt, or full
// disk mid-write never leaves path partially written or corrupted: the
// rename is the only step that can make the new content visible, and
// rename within a single directory is atomic on every platform Go
// supports. The caller supplies perm (e.g. from stat'ing the file being
// replaced) since this function has no opinion about what the file's mode
// should be.
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
	// Flush to disk before the rename, so the rename can't make a path
	// visible that points at data the OS hasn't actually persisted yet.
	if err = tmp.Sync(); err != nil {
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
