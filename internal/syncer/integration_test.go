package syncer

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/ugurkocde/TenuVault-TUI/internal/graphtest"
)

// TestRunSyncLive copies a live source policy into the target, asserting the
// create payload is cleaned, prefixed, and POSTed to the right endpoint.
func TestRunSyncLive(t *testing.T) {
	source := &graphtest.Fake{}
	target := &graphtest.Fake{}
	items := []Item{{
		Category: "DeviceConfigurations",
		TypeKey:  "deviceConfigurations",
		Name:     "BitLocker",
		Raw:      []byte(`{"@odata.type":"#microsoft.graph.windows10GeneralConfiguration","id":"src-1","displayName":"BitLocker","assignments":[{"x":1}]}`),
	}}

	results := Run(context.Background(), target, source, items, "[Synced] ", nil)
	if len(results) != 1 || results[0].Err != nil {
		t.Fatalf("results = %+v", results)
	}
	if len(target.Posts) != 1 {
		t.Fatalf("target POSTs = %d, want 1", len(target.Posts))
	}
	if !strings.Contains(target.Posts[0].Path, "/deviceManagement/deviceConfigurations") {
		t.Errorf("path = %q", target.Posts[0].Path)
	}
	var body map[string]any
	_ = json.Unmarshal(target.Posts[0].Body, &body)
	if body["displayName"] != "[Synced] BitLocker" {
		t.Errorf("displayName = %v", body["displayName"])
	}
	if _, ok := body["id"]; ok {
		t.Error("id must be stripped")
	}
	if _, ok := body["assignments"]; ok {
		t.Error("assignments must be stripped on sync")
	}
	if body["@odata.type"] != "#microsoft.graph.windows10GeneralConfiguration" {
		t.Error("@odata.type must be preserved for create routing")
	}
}

// TestRunSyncWarnsOnPartialContent: a live source type that needs a per-item
// detail fetch (configurationPolicies / DetailByID) but whose detail GET fails
// is still created, with Warn set so the incomplete copy is surfaced.
func TestRunSyncWarnsOnPartialContent(t *testing.T) {
	source := &graphtest.Fake{} // no GET stub registered -> detail fetch fails
	target := &graphtest.Fake{}
	items := []Item{{
		Category: "ConfigurationPolicies",
		TypeKey:  "configurationPolicies",
		Name:     "Catalog",
		Raw:      []byte(`{"id":"src-1","name":"Catalog"}`),
	}}

	results := Run(context.Background(), target, source, items, "", nil)
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if results[0].Err != nil {
		t.Fatalf("item should still be created, got error: %v", results[0].Err)
	}
	if !results[0].Warn {
		t.Error("expected Warn=true when source content could not be fully resolved")
	}
	if len(target.Posts) != 1 {
		t.Fatalf("the policy should still be created, got %d POSTs", len(target.Posts))
	}
}

// TestRunSyncNoWarnOnBackupSource: backup-sourced items never set Warn (nothing
// to enrich; the file is the full content).
func TestRunSyncNoWarnOnBackupSource(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/p.json"
	if err := os.WriteFile(path, []byte(`{"@odata.type":"#microsoft.graph.windows10CompliancePolicy","id":"b1","displayName":"Compliance"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	target := &graphtest.Fake{}
	results := Run(context.Background(), target, nil,
		[]Item{{Category: "CompliancePolicies", Name: "Compliance", Path: path}}, "", nil)
	if len(results) != 1 || results[0].Err != nil {
		t.Fatalf("results = %+v", results)
	}
	if results[0].Warn {
		t.Error("backup-sourced item should not set Warn")
	}
}

// TestRunSyncFromBackup copies a policy read from a backup file (no source).
func TestRunSyncFromBackup(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/p.json"
	if err := os.WriteFile(path, []byte(`{"@odata.type":"#microsoft.graph.windows10CompliancePolicy","id":"b1","displayName":"Compliance"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	target := &graphtest.Fake{}
	results := Run(context.Background(), target, nil,
		[]Item{{Category: "CompliancePolicies", Name: "Compliance", Path: path}}, "", nil)
	if len(results) != 1 || results[0].Err != nil {
		t.Fatalf("results = %+v", results)
	}
	if len(target.Posts) != 1 || !strings.Contains(target.Posts[0].Path, "deviceCompliancePolicies") {
		t.Errorf("posts = %+v", target.Posts)
	}
}
