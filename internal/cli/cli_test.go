package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/ugurkocde/TenuVault-TUI/internal/store"
	"github.com/ugurkocde/TenuVault-TUI/internal/syncer"
)

// fixtureBackup writes a one-category backup with a checksums sidecar.
func fixtureBackup(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	cat := filepath.Join(dir, "CompliancePolicies")
	if err := os.MkdirAll(cat, 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{}
	for name, body := range map[string]string{
		"Alpha": `{"@odata.type":"#microsoft.graph.windows10CompliancePolicy","displayName":"Alpha"}`,
		"Beta":  `{"@odata.type":"#microsoft.graph.windows10CompliancePolicy","displayName":"Beta"}`,
	} {
		if err := os.WriteFile(filepath.Join(cat, name+".json"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256([]byte(body))
		files["CompliancePolicies/"+name+".json"] = hex.EncodeToString(sum[:])
	}
	if err := store.WriteChecksums(dir, files); err != nil {
		t.Fatal(err)
	}
	return dir
}

// TestVerifyCmdExitCodes covers intact (0), tampered (1), and missing
// checksums / bad usage (2).
func TestVerifyCmdExitCodes(t *testing.T) {
	dir := fixtureBackup(t)
	if code := Verify([]string{dir}); code != 0 {
		t.Errorf("intact backup: exit = %d, want 0", code)
	}

	if err := os.WriteFile(filepath.Join(dir, "CompliancePolicies", "Alpha.json"), []byte(`{"x":1}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if code := Verify([]string{dir}); code != 1 {
		t.Errorf("tampered backup: exit = %d, want 1", code)
	}

	if code := Verify([]string{t.TempDir()}); code != 2 {
		t.Errorf("backup without checksums: exit = %d, want 2", code)
	}
	if code := Verify(nil); code != 2 {
		t.Errorf("no args: exit = %d, want 2", code)
	}
}

// TestSyncDryRunFromBackup plans a sync from a backup dir without any
// credentials or network: exit 0 and nothing else required.
func TestSyncDryRunFromBackup(t *testing.T) {
	t.Setenv("TENUVAULT_SOURCE_TENANT_ID", "") // ensure no live source leaks in
	dir := fixtureBackup(t)

	if code := Sync([]string{"--from-backup", dir, "--dry-run"}); code != 0 {
		t.Errorf("dry run: exit = %d, want 0", code)
	}
}

// TestSyncRequiresExactlyOneSource rejects zero and two sources with usage
// errors before touching any credentials.
func TestSyncRequiresExactlyOneSource(t *testing.T) {
	t.Setenv("TENUVAULT_SOURCE_TENANT_ID", "")
	if code := Sync([]string{"--dry-run"}); code != 2 {
		t.Errorf("no source: exit = %d, want 2", code)
	}

	t.Setenv("TENUVAULT_SOURCE_TENANT_ID", "aaaa-1111")
	dir := fixtureBackup(t)
	if code := Sync([]string{"--from-backup", dir, "--dry-run"}); code != 2 {
		t.Errorf("two sources: exit = %d, want 2", code)
	}
}

// TestFilterByNames keeps only exact display-name matches.
func TestFilterByNames(t *testing.T) {
	items := []syncer.Item{{Name: "Alpha"}, {Name: "Beta"}, {Name: "Gamma"}}
	got := filterByNames(items, " Alpha, Gamma ")
	if len(got) != 2 || got[0].Name != "Alpha" || got[1].Name != "Gamma" {
		t.Errorf("filtered = %+v", got)
	}
	if got := filterByNames(items, ""); len(got) != 3 {
		t.Errorf("empty filter dropped items: %+v", got)
	}
}

// TestBackupItemsNameFilterIntegration: the sync path builds items from a
// backup dir and the name filter narrows them.
func TestBackupItemsNameFilterIntegration(t *testing.T) {
	dir := fixtureBackup(t)
	all, err := backupItems(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	items := filterByNames(all, "Beta")
	if len(items) != 1 || items[0].Name != "Beta" || items[0].Category != "CompliancePolicies" {
		t.Errorf("items = %+v", items)
	}
}
