package diff

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/ugurkocde/TenuVault-TUI/internal/store"
)

func writeBackup(t *testing.T, root, folder string, files map[string]map[string]string) store.Backup {
	t.Helper()
	path := filepath.Join(root, folder)
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
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

func TestEqualPolicyReportsReadFailure(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "good.json")
	if err := os.WriteFile(good, []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := equalPolicy(good, filepath.Join(dir, "missing.json")); err == nil {
		t.Fatal("missing policy file should return an error")
	}
}

func TestCompareHandlesCategoryPresentInOnlyOneBackup(t *testing.T) {
	root := t.TempDir()
	older := writeBackup(t, root, "backup-2026-01-01-000000", map[string]map[string]string{
		"RemovedCategory": {"old": `{}`},
	})
	newer := writeBackup(t, root, "backup-2026-02-01-000000", map[string]map[string]string{
		"AddedCategory": {"new": `{}`},
	})

	changes, err := Compare(older, newer)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]ChangeType{}
	for _, change := range changes {
		got[change.Category+"/"+change.Name] = change.Type
	}
	if got["AddedCategory/new"] != Added || got["RemovedCategory/old"] != Removed {
		t.Fatalf("changes=%+v", changes)
	}
}

func TestCompareReturnsStableSortedChanges(t *testing.T) {
	root := t.TempDir()
	older := writeBackup(t, root, "backup-2026-01-01-000000", map[string]map[string]string{})
	newer := writeBackup(t, root, "backup-2026-02-01-000000", map[string]map[string]string{
		"ZCategory": {"z": `{}`},
		"ACategory": {"b": `{}`, "a": `{}`},
	})
	changes, err := Compare(older, newer)
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, 0, len(changes))
	for _, change := range changes {
		got = append(got, change.Category+"/"+change.Name)
	}
	want := []string{"ACategory/a", "ACategory/b", "ZCategory/z"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got=%v want=%v", got, want)
	}
}

func TestIndexPoliciesReportsDisappearingCategory(t *testing.T) {
	backup := store.Backup{Path: t.TempDir()}
	if _, err := indexPolicies(backup, "DeletedDuringCompare"); err == nil {
		t.Fatal("a category expected to exist must not be treated as empty")
	}
}

// TestCompareTracksRenameAsModified: pairing is by Graph id, so a pure
// displayName change is one Modified row under the new name, never a
// misleading Removed + Added pair.
func TestCompareTracksRenameAsModified(t *testing.T) {
	root := t.TempDir()
	a := writeBackup(t, root, "backup-a", map[string]map[string]string{
		"ConfigurationPolicies": {"Old Name": `{"id":"p1","name":"Old Name","x":1}`},
	})
	b := writeBackup(t, root, "backup-b", map[string]map[string]string{
		"ConfigurationPolicies": {"New Name": `{"id":"p1","name":"New Name","x":1}`},
	})
	changes, err := Compare(a, b)
	if err != nil {
		t.Fatal(err)
	}
	want := []Change{{"ConfigurationPolicies", "New Name", Modified, Warning}}
	if !reflect.DeepEqual(changes, want) {
		t.Fatalf("changes = %+v, want single Modified under the new name", changes)
	}
}

// TestCompareSurvivesDuplicateNameSuffixShuffle: two policies share a display
// name; which file carries the " (2)" suffix flips between backup runs. Id
// pairing must still match each policy to itself and report no drift.
func TestCompareSurvivesDuplicateNameSuffixShuffle(t *testing.T) {
	root := t.TempDir()
	a := writeBackup(t, root, "backup-a", map[string]map[string]string{
		"ConfigurationPolicies": {
			"Dup":     `{"id":"p1","name":"Dup","x":1}`,
			"Dup (2)": `{"id":"p2","name":"Dup","x":2}`,
		},
	})
	b := writeBackup(t, root, "backup-b", map[string]map[string]string{
		"ConfigurationPolicies": {
			"Dup":     `{"id":"p2","name":"Dup","x":2}`,
			"Dup (2)": `{"id":"p1","name":"Dup","x":1}`,
		},
	})
	changes, err := Compare(a, b)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 0 {
		t.Fatalf("changes = %+v, want none for a suffix shuffle", changes)
	}
}

// TestCompareIgnoresAnnotationFormatChurn: backups taken by different
// TenuVault versions differ only in OData annotation bookkeeping (old files
// carry settings@odata.context, new ones a stamped @odata.context). That must
// never register as drift.
func TestCompareIgnoresAnnotationFormatChurn(t *testing.T) {
	root := t.TempDir()
	a := writeBackup(t, root, "backup-a", map[string]map[string]string{
		"ConfigurationPolicies": {"Baseline": `{"id":"p1","name":"Baseline","settings@odata.context":"https://old","settings":[{"id":"0"}]}`},
	})
	b := writeBackup(t, root, "backup-b", map[string]map[string]string{
		"ConfigurationPolicies": {"Baseline": `{"@odata.context":"https://new","id":"p1","name":"Baseline","settings":[{"id":"0"}]}`},
	})
	changes, err := Compare(a, b)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 0 {
		t.Fatalf("changes = %+v, want none for annotation churn", changes)
	}
}
