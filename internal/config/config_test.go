package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExpandPath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir: %v", err)
	}
	cases := []struct {
		in, want string
	}{
		{"", ""},
		{"   ", ""},
		{"~", home},
		{"~/Backups", filepath.Join(home, "Backups")},
		{"  /a/b  ", filepath.Clean("/a/b")},
		{"./x", "x"},
		{"a/../b", "b"},
		{"~weird", "~weird"},
	}
	for _, c := range cases {
		if got := ExpandPath(c.in); got != c.want {
			t.Errorf("ExpandPath(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestLoadExpandsBackupRoot(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TENUVAULT_CONFIG_DIR", dir)
	data := []byte(`{"backupRoot": "~/TenuVaultTest"}`)
	if err := os.WriteFile(filepath.Join(dir, "config.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir: %v", err)
	}
	c := Load()
	if want := filepath.Join(home, "TenuVaultTest"); c.BackupRoot != want {
		t.Errorf("Load BackupRoot = %q, want %q", c.BackupRoot, want)
	}
}

func TestLoadEmptyBackupRootDefaults(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TENUVAULT_CONFIG_DIR", dir)
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"backupRoot": ""}`), 0o600); err != nil {
		t.Fatal(err)
	}
	c := Load()
	if c.BackupRoot == "" {
		t.Error("Load left BackupRoot empty, want default")
	}
}
