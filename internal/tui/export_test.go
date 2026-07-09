package tui

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ugurkocde/TenuVault-TUI/internal/restore"
	"github.com/ugurkocde/TenuVault-TUI/internal/syncer"
)

// TestExportSyncResults presses e on the sync results screen and checks the
// written report and the transient status message.
func TestExportSyncResults(t *testing.T) {
	m := seededModel()
	m.cfg.BackupRoot = t.TempDir()
	m.screen = screenSyncResults
	m.syncNamePrefix = syncPrefix
	m.syncResults = []syncer.Result{
		{Item: syncer.Item{Category: "DeviceConfigurations", Name: "BitLocker"}, NewID: "9f2a"},
		{Item: syncer.Item{Category: "CompliancePolicies", Name: "Broken"}, Err: errors.New("graph POST /x: boom")},
		{Item: syncer.Item{Category: "DeviceConfigurations", Name: "Partial"}, NewID: "1", Warn: true},
	}

	out, cmd := m.handleKey("e")
	m = out.(model)
	if cmd == nil {
		t.Fatal("e on sync results returned no command")
	}
	upd, _ := m.Update(cmd())
	m2 := upd.(model)
	if !strings.HasPrefix(m2.status, "Exported to ") || m2.statusKind != "ok" {
		t.Fatalf("status = %q (%s), want an Exported-to confirmation", m2.status, m2.statusKind)
	}

	entries, err := os.ReadDir(m.cfg.BackupRoot)
	if err != nil || len(entries) != 1 {
		t.Fatalf("backup root entries = %v, err %v; want one report file", entries, err)
	}
	name := entries[0].Name()
	if !strings.HasPrefix(name, "tenuvault-sync-") || !strings.HasSuffix(name, ".json") {
		t.Errorf("report file name = %q", name)
	}
	data, err := os.ReadFile(filepath.Join(m.cfg.BackupRoot, name))
	if err != nil {
		t.Fatal(err)
	}
	var doc resultReportDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("report is not valid JSON: %v", err)
	}
	if doc.Kind != "sync" || doc.Target != "Contoso Prod" || doc.NamePrefix != syncPrefix {
		t.Errorf("header = %+v", doc)
	}
	if len(doc.Results) != 3 {
		t.Fatalf("results = %d, want 3", len(doc.Results))
	}
	if doc.Results[1].Error == "" || !strings.Contains(doc.Results[1].Error, "boom") {
		t.Errorf("failed item error = %q, want the failure text", doc.Results[1].Error)
	}
	if !doc.Results[2].PartialContent {
		t.Error("partial-content flag not exported")
	}
}

func TestRestoreReportPreservesPartialContentWarning(t *testing.T) {
	m := seededModel()
	m.restoreResults = []restore.Result{{
		Item:  restore.Item{Category: "DeviceConfigurations", Name: "Partial"},
		NewID: "new-1",
		Warn:  true,
	}}
	doc := m.restoreReport()
	if len(doc.Results) != 1 || !doc.Results[0].PartialContent {
		t.Fatalf("results=%+v", doc.Results)
	}
}

// TestExportDiffReport presses e on the compare screen.
func TestExportDiffReport(t *testing.T) {
	m := seededModel()
	m.cfg.BackupRoot = t.TempDir()
	m.screen = screenDiffResult

	out, cmd := m.handleKey("e")
	m = out.(model)
	upd, _ := m.Update(cmd())
	if m2 := upd.(model); !strings.HasPrefix(m2.status, "Exported to ") {
		t.Fatalf("status = %q", m2.status)
	}
	entries, _ := os.ReadDir(m.cfg.BackupRoot)
	if len(entries) != 1 || !strings.HasPrefix(entries[0].Name(), "tenuvault-diff-") {
		t.Fatalf("expected one tenuvault-diff-*.json, got %v", entries)
	}
	data, _ := os.ReadFile(filepath.Join(m.cfg.BackupRoot, entries[0].Name()))
	var doc diffReportDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Kind != "diff" || len(doc.Changes) != 1 || doc.Changes[0].Type != "removed" {
		t.Errorf("diff report = %+v", doc)
	}
}

// TestExportFailureSurfaces reports a write failure as an error status.
func TestExportFailureSurfaces(t *testing.T) {
	m := seededModel()
	// A file (not a dir) as backup root makes MkdirAll fail.
	f := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	m.cfg.BackupRoot = f
	m.screen = screenRestoreResult

	out, cmd := m.handleKey("e")
	m = out.(model)
	upd, _ := m.Update(cmd())
	m2 := upd.(model)
	if !strings.HasPrefix(m2.status, "Export failed: ") || m2.statusKind != "err" {
		t.Errorf("status = %q (%s), want an export failure", m2.status, m2.statusKind)
	}
}

// TestExportRestoreReport presses e on the restore results screen.
func TestExportRestoreReport(t *testing.T) {
	m := seededModel()
	m.cfg.BackupRoot = t.TempDir()
	m.screen = screenRestoreResult

	out, cmd := m.handleKey("e")
	m = out.(model)
	upd, _ := m.Update(cmd())
	if m2 := upd.(model); !strings.HasPrefix(m2.status, "Exported to ") {
		t.Fatalf("status = %q", m2.status)
	}
	entries, _ := os.ReadDir(m.cfg.BackupRoot)
	if len(entries) != 1 || !strings.HasPrefix(entries[0].Name(), "tenuvault-restore-") {
		t.Fatalf("expected one tenuvault-restore-*.json, got %v", entries)
	}
}
