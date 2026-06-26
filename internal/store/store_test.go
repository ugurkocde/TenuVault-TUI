package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCleanup(t *testing.T) {
	root := t.TempDir()
	old := "backup-" + time.Now().AddDate(0, 0, -40).Format("2006-01-02-150405")
	recent := "backup-" + time.Now().AddDate(0, 0, -2).Format("2006-01-02-150405")
	for _, f := range []string{old, recent} {
		if err := os.MkdirAll(filepath.Join(root, f), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	removed, err := Cleanup(root, 30)
	if err != nil {
		t.Fatal(err)
	}
	if removed != 1 {
		t.Fatalf("removed = %d, want 1", removed)
	}
	if _, err := os.Stat(filepath.Join(root, old)); !os.IsNotExist(err) {
		t.Error("old backup should be deleted")
	}
	if _, err := os.Stat(filepath.Join(root, recent)); err != nil {
		t.Error("recent backup should remain")
	}
}
