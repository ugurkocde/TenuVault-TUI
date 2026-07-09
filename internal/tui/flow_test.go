package tui

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/ugurkocde/TenuVault-TUI/internal/config"
	"github.com/ugurkocde/TenuVault-TUI/internal/connection"
	"github.com/ugurkocde/TenuVault-TUI/internal/fileutil"
	"github.com/ugurkocde/TenuVault-TUI/internal/graph"
	"github.com/ugurkocde/TenuVault-TUI/internal/store"
	"github.com/ugurkocde/TenuVault-TUI/internal/syncer"
)

func testConfig(t *testing.T) config.Config {
	t.Helper()
	cfg := config.Default()
	cfg.BackupRoot = t.TempDir()
	return cfg
}

func TestAuthEscapeReturnsToTenantList(t *testing.T) {
	m := New(testConfig(t))
	m.screen = screenAuth
	m.addingConn = true

	next, _ := m.keyAuth("esc")
	got := next.(model)
	if got.screen != screenConnections || got.addingConn {
		t.Fatalf("screen=%v addingConn=%v, want tenant list with add flow cleared", got.screen, got.addingConn)
	}
}

func TestBackupStartExplainsEmptySelection(t *testing.T) {
	m := New(testConfig(t))
	started, _ := m.startBackupSelect()
	m = started.(model)
	for i := range m.cats {
		m.cats[i].sel = false
	}

	next, _ := m.keyBackupSelect("enter")
	got := next.(model)
	if got.statusKind != "err" || got.status == "" || got.screen != screenBackupSelect {
		t.Fatalf("status=%q kind=%q screen=%v", got.status, got.statusKind, got.screen)
	}
}

func TestBackupSyncBlocksSameTenantWithoutPrefix(t *testing.T) {
	dir := t.TempDir()
	meta, err := json.Marshal(store.Metadata{TenantID: "tenant-1"})
	if err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteFileAtomic(filepath.Join(dir, "metadata.json"), meta, 0o644); err != nil {
		t.Fatal(err)
	}

	m := New(testConfig(t))
	m.screen = screenSyncConfirm
	m.syncFromBackup = true
	m.syncTargetConn = 0
	m.syncNamePrefix = ""
	m.syncItems = []syncer.Item{{Name: "Policy"}}
	m.restoreBackup = &store.Backup{Path: dir}
	m.conns = []connection.Connection{{Tenant: graph.Tenant{ID: "tenant-1"}}}

	next, _ := m.keySyncConfirm("y")
	got := next.(model)
	if got.screen != screenSyncNaming || got.syncRunning || got.statusKind != "err" {
		t.Fatalf("screen=%v running=%v status=%q", got.screen, got.syncRunning, got.status)
	}
}

func TestCompletedBackgroundDiffDoesNotHijackNavigation(t *testing.T) {
	m := New(testConfig(t))
	m.screen = screenDashboard
	m.diffLoading = false

	next, _ := m.Update(diffDoneMsg{})
	got := next.(model)
	if got.screen != screenDashboard || got.status != "Compare finished" {
		t.Fatalf("screen=%v status=%q", got.screen, got.status)
	}
}

func TestStaleDiffResultCannotReplaceActiveComparison(t *testing.T) {
	m := New(testConfig(t))
	m.screen = screenBrowse
	m.diffLoading = true
	m.diffGen = 2

	next, _ := m.Update(diffDoneMsg{gen: 1})
	got := next.(model)
	if !got.diffLoading || got.screen != screenBrowse || len(got.diffChanges) != 0 {
		t.Fatalf("stale result changed active flow: loading=%v screen=%v changes=%v",
			got.diffLoading, got.screen, got.diffChanges)
	}
}

func TestErrorReturnsToOriginatingScreen(t *testing.T) {
	m := New(testConfig(t))
	m.connected = true
	m.screen = screenError
	m.errorReturn = screenBrowseDetail

	next, _ := m.handleKey("esc")
	got := next.(model)
	if got.screen != screenBrowseDetail {
		t.Fatalf("screen=%v, want browse detail", got.screen)
	}
}

func TestListRowsUsesTerminalHeight(t *testing.T) {
	m := New(testConfig(t))
	m.height = 18
	if got := m.listRows(16); got != 6 {
		t.Fatalf("listRows=%d, want 6", got)
	}
}
