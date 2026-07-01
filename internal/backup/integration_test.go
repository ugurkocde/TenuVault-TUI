package backup

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ugurkocde/TenuVault-TUI/internal/catalog"
	"github.com/ugurkocde/TenuVault-TUI/internal/graph"
	"github.com/ugurkocde/TenuVault-TUI/internal/graphtest"
	"github.com/ugurkocde/TenuVault-TUI/internal/store"
)

func TestRunWritesBackup(t *testing.T) {
	dc, _ := catalog.ByKey("deviceConfigurations")
	dcat, _ := catalog.ByKey("deviceCategories")
	f := &graphtest.Fake{Lists: map[string][]json.RawMessage{
		"beta " + dc.ListPath: {
			[]byte(`{"@odata.type":"#microsoft.graph.windows10GeneralConfiguration","id":"1","displayName":"BitLocker"}`),
			[]byte(`{"@odata.type":"#microsoft.graph.windows10GeneralConfiguration","id":"2","displayName":"Defender"}`),
		},
		"beta " + dcat.ListPath: {
			[]byte(`{"id":"c1","displayName":"Sales"}`),
		},
	}}

	root := t.TempDir()
	res, err := Run(context.Background(), f, []catalog.PolicyType{dc, dcat},
		Options{Root: root, Tenant: graph.Tenant{ID: "t1", DisplayName: "Lab"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != "Success" {
		t.Errorf("status = %q", res.Status)
	}
	if res.ItemCounts["DeviceConfigurations"] != 2 || res.ItemCounts["DeviceCategories"] != 1 {
		t.Errorf("counts = %+v", res.ItemCounts)
	}

	// Files written under category folders.
	if _, err := os.Stat(filepath.Join(res.Path, "DeviceConfigurations", "BitLocker.json")); err != nil {
		t.Errorf("BitLocker.json missing: %v", err)
	}
	// Manifest is valid and reflects the run.
	data, err := os.ReadFile(filepath.Join(res.Path, "metadata.json"))
	if err != nil {
		t.Fatal(err)
	}
	var meta store.Metadata
	if err := json.Unmarshal(data, &meta); err != nil {
		t.Fatal(err)
	}
	if meta.TenantName != "Lab" || meta.Status != "Success" {
		t.Errorf("metadata = %+v", meta)
	}
}

// TestRunCancelledKeepsPartialBackup cancels the run up front: nothing is
// fetched, but the folder still gets a manifest with Status "Cancelled" so it
// is never left as an orphan without metadata.
func TestRunCancelledKeepsPartialBackup(t *testing.T) {
	dc, _ := catalog.ByKey("deviceConfigurations")
	f := &graphtest.Fake{Lists: map[string][]json.RawMessage{
		"beta " + dc.ListPath: {
			[]byte(`{"id":"1","displayName":"BitLocker"}`),
		},
	}}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	root := t.TempDir()
	res, err := Run(ctx, f, []catalog.PolicyType{dc},
		Options{Root: root, Tenant: graph.Tenant{ID: "t1"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != "Cancelled" {
		t.Errorf("status = %q, want Cancelled", res.Status)
	}
	data, err := os.ReadFile(filepath.Join(res.Path, "metadata.json"))
	if err != nil {
		t.Fatalf("metadata.json missing after cancel: %v", err)
	}
	var meta store.Metadata
	if err := json.Unmarshal(data, &meta); err != nil {
		t.Fatal(err)
	}
	if meta.Status != "Cancelled" {
		t.Errorf("manifest status = %q, want Cancelled", meta.Status)
	}
}
