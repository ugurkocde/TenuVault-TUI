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
		dir := filepath.Join(root, f)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "metadata.json"), []byte(`{}`), 0o644); err != nil {
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

func TestListRejectsCorruptMetadata(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "backup-2026-01-01-000000")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "metadata.json"), []byte("{bad"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := List(root); err == nil {
		t.Fatal("corrupt metadata should be reported")
	}
}

func TestBackupFolderTimeSupportsUniqueSuffix(t *testing.T) {
	got, err := backupFolderTime("backup-2026-07-09-120000.000000123-2")
	if err != nil {
		t.Fatal(err)
	}
	if got.Nanosecond() != 123 {
		t.Fatalf("nanoseconds=%d, want 123", got.Nanosecond())
	}
}

func TestListSkipsBackupWithoutCommitManifest(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "backup-2026-07-09-120000"), 0o755); err != nil {
		t.Fatal(err)
	}
	backups, err := List(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(backups) != 0 {
		t.Fatalf("backups=%v, want incomplete directory hidden", backups)
	}
}
