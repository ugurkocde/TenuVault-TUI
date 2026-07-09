// Package fileutil contains shared filesystem helpers.
package fileutil

import (
	"errors"
	"os"
	"path/filepath"
)

// WriteFileAtomic writes data to a temporary file in the destination directory,
// flushes it, and renames it over path. A same-directory rename prevents readers
// from observing a partially written file.
func WriteFileAtomic(path string, data []byte, perm os.FileMode) (err error) {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	closed := false
	defer func() {
		if !closed {
			err = errors.Join(err, tmp.Close())
		}
		if err != nil {
			_ = os.Remove(tmpPath)
		}
	}()

	if err = tmp.Chmod(perm); err != nil {
		return err
	}
	if _, err = tmp.Write(data); err != nil {
		return err
	}
	if err = tmp.Sync(); err != nil {
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	closed = true
	if err = os.Rename(tmpPath, path); err != nil {
		return err
	}

	// Best effort: some platforms/filesystems do not support syncing a
	// directory, while the file-level atomicity above remains valid.
	if d, openErr := os.Open(dir); openErr == nil {
		_ = d.Sync()
		_ = d.Close()
	}
	return nil
}
