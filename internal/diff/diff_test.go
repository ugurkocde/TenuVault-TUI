package diff

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ugurkocde/TenuVault-TUI/internal/store"
)

func writeBackup(t *testing.T, root, folder string, files map[string]map[string]string) store.Backup {
	t.Helper()
	path := filepath.Join(root, folder)
	for cat, policies := range files {
		dir := filepath.Join(path, cat)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		for name, body := range policies {
			if err := os.WriteFile(filepath.Join(dir, name+".json"), []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	return store.Backup{Folder: folder, Path: path}
}

func TestCompare(t *testing.T) {
	root := t.TempDir()
	older := writeBackup(t, root, "backup-2026-01-01-000000", map[string]map[string]string{
		"CompliancePolicies":   {"a": `{"id":"1","displayName":"a","setting":1}`, "gone": `{"id":"2","displayName":"gone"}`},
		"DeviceConfigurations": {"c": `{"id":"3","displayName":"c","x":true}`},
	})
	newer := writeBackup(t, root, "backup-2026-02-01-000000", map[string]map[string]string{
		"CompliancePolicies":   {"a": `{"id":"1","displayName":"a","setting":2}`, "new": `{"id":"4","displayName":"new"}`},
		"DeviceConfigurations": {"c": `{"id":"3","displayName":"c","x":true,"lastModifiedDateTime":"changed"}`},
	})

	changes, err := Compare(older, newer)
	if err != nil {
		t.Fatal(err)
	}

	got := map[string]Change{}
	for _, c := range changes {
		got[c.Category+"/"+c.Name] = c
	}

	if c, ok := got["CompliancePolicies/a"]; !ok || c.Type != Modified {
		t.Errorf("expected a modified, got %+v", c)
	}
	if c, ok := got["CompliancePolicies/new"]; !ok || c.Type != Added {
		t.Errorf("expected new added, got %+v", c)
	}
	if c, ok := got["CompliancePolicies/gone"]; !ok || c.Type != Removed || c.Severity != Critical {
		t.Errorf("expected gone removed+critical, got %+v", c)
	}
	// Only an ignored field changed -> not a drift.
	if _, ok := got["DeviceConfigurations/c"]; ok {
		t.Error("c should be unchanged (only lastModifiedDateTime differs)")
	}
}
