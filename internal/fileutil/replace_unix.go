//go:build !windows

package fileutil

import "os"

func replaceFile(source, destination string) error {
	return os.Rename(source, destination)
}
