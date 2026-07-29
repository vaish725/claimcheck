package rewrite

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteFileAtomicRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "README.md")

	if err := WriteFileAtomic(path, []byte("hello"), 0o644); err != nil {
		t.Fatalf("WriteFileAtomic: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading written file: %v", err)
	}
	if string(got) != "hello" {
		t.Errorf("content = %q, want %q", got, "hello")
	}
}

func TestWriteFileAtomicOverwritesExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "README.md")
	if err := os.WriteFile(path, []byte("old content, longer than the new content"), 0o644); err != nil {
		t.Fatalf("seeding existing file: %v", err)
	}

	if err := WriteFileAtomic(path, []byte("new"), 0o644); err != nil {
		t.Fatalf("WriteFileAtomic: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading written file: %v", err)
	}
	if string(got) != "new" {
		t.Errorf("content = %q, want %q (old content must not linger)", got, "new")
	}
}

func TestWriteFileAtomicSetsPermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "README.md")

	if err := WriteFileAtomic(path, []byte("hello"), 0o600); err != nil {
		t.Fatalf("WriteFileAtomic: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("permissions = %v, want %v", got, os.FileMode(0o600))
	}
}

func TestWriteFileAtomicCleansUpOnFailure(t *testing.T) {
	dir := t.TempDir()
	// A target path inside a directory that doesn't exist means the final
	// rename must fail, since CreateTemp still succeeds in dir itself.
	badPath := filepath.Join(dir, "no-such-subdir", "README.md")

	err := WriteFileAtomic(badPath, []byte("hello"), 0o644)
	if err == nil {
		t.Fatal("expected an error for a rename into a nonexistent directory, got none")
	}

	// os.Rename fails without creating "no-such-subdir", so dir should be
	// left exactly as it was: the temp file WriteFileAtomic created must
	// have been cleaned up on this failure path.
	entries, readErr := os.ReadDir(dir)
	if readErr != nil {
		t.Fatalf("reading temp dir: %v", readErr)
	}
	if len(entries) != 0 {
		names := make([]string, len(entries))
		for i, e := range entries {
			names[i] = e.Name()
		}
		t.Errorf("leftover files in dir after failed write: %v", names)
	}
}
