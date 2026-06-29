package restore

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ugurkocde/TenuVault-TUI/internal/graphtest"
)

// writeBackupFile writes a policy JSON to a temp file and returns its path.
func writeBackupFile(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "policy.json")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestRestoreCreatesPrefixedPolicy verifies the create path: the backup file is
// read, read-only fields stripped, the [Restored] prefix applied, and the body
// POSTed to the type's create endpoint.
func TestRestoreCreatesPrefixedPolicy(t *testing.T) {
	path := writeBackupFile(t, `{"@odata.type":"#microsoft.graph.windows10GeneralConfiguration","id":"orig-1","displayName":"BitLocker","assignments":[{"x":1}]}`)
	target := &graphtest.Fake{}

	results := Restore(context.Background(), target, []Item{{
		Category: "DeviceConfigurations",
		Name:     "BitLocker",
		Path:     path,
	}})

	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if results[0].Err != nil {
		t.Fatalf("unexpected error: %v", results[0].Err)
	}
	if results[0].NewID == "" {
		t.Error("expected a new id from create")
	}
	if len(target.Posts) != 1 {
		t.Fatalf("got %d POSTs, want 1", len(target.Posts))
	}
	if !strings.Contains(target.Posts[0].Path, "/deviceManagement/deviceConfigurations") {
		t.Errorf("POST path = %q", target.Posts[0].Path)
	}
	var body map[string]any
	if err := json.Unmarshal(target.Posts[0].Body, &body); err != nil {
		t.Fatal(err)
	}
	if body["displayName"] != RestorePrefix+"BitLocker" {
		t.Errorf("displayName = %v, want %q", body["displayName"], RestorePrefix+"BitLocker")
	}
	if _, ok := body["id"]; ok {
		t.Error("id must be stripped before create")
	}
	if _, ok := body["assignments"]; ok {
		t.Error("assignments must be stripped before create")
	}
	if body["@odata.type"] != "#microsoft.graph.windows10GeneralConfiguration" {
		t.Error("@odata.type must be preserved for create routing")
	}
}

// TestRestoreReportsReadError surfaces a per-item error when the backup file is
// missing, and still returns a result for it.
func TestRestoreReportsReadError(t *testing.T) {
	target := &graphtest.Fake{}
	results := Restore(context.Background(), target, []Item{{
		Category: "DeviceConfigurations",
		Name:     "Missing",
		Path:     filepath.Join(t.TempDir(), "does-not-exist.json"),
	}})

	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if results[0].Err == nil {
		t.Error("expected an error for a missing backup file")
	}
	if len(target.Posts) != 0 {
		t.Errorf("nothing should be POSTed when the read fails, got %d", len(target.Posts))
	}
}

// TestRestoreContinuesAfterFailure ensures one bad item doesn't abort the rest.
func TestRestoreContinuesAfterFailure(t *testing.T) {
	good := writeBackupFile(t, `{"@odata.type":"#microsoft.graph.windows10CompliancePolicy","id":"c1","displayName":"Compliance"}`)
	target := &graphtest.Fake{}

	results := Restore(context.Background(), target, []Item{
		{Category: "CompliancePolicies", Name: "Missing", Path: filepath.Join(t.TempDir(), "nope.json")},
		{Category: "CompliancePolicies", Name: "Compliance", Path: good},
	})

	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
	if results[0].Err == nil {
		t.Error("first item should have failed")
	}
	if results[1].Err != nil {
		t.Errorf("second item should have succeeded, got %v", results[1].Err)
	}
	if len(target.Posts) != 1 {
		t.Errorf("only the valid item should be POSTed, got %d", len(target.Posts))
	}
}
