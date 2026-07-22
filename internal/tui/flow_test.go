package tui

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/ugurkocde/TenuVault-TUI/internal/config"
	"github.com/ugurkocde/TenuVault-TUI/internal/connection"
	"github.com/ugurkocde/TenuVault-TUI/internal/diff"
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

// TestSyncEscStepsBackOneScreen pins the wizard's esc semantics: every mid-flow
// step goes back exactly one screen (matching the "esc back" hint), never
// straight to the dashboard with all selections discarded.
func TestSyncEscStepsBackOneScreen(t *testing.T) {
	m := New(testConfig(t))
	m.connected = true
	m.conns = []connection.Connection{
		{Tenant: graph.Tenant{ID: "tenant-1"}},
		{Tenant: graph.Tenant{ID: "tenant-2"}},
	}
	m.syncTypes = buildSyncTypes()

	m.screen = screenSyncSelect
	next, _ := m.keySyncSelect("esc")
	if got := next.(model); got.screen != screenSyncSource {
		t.Fatalf("categories esc: screen=%v, want sync source", got.screen)
	}

	m.screen = screenSyncTarget
	m.syncFromBackup = false
	next, _ = m.keySyncTarget("esc")
	if got := next.(model); got.screen != screenSyncSelect {
		t.Fatalf("target esc (live): screen=%v, want categories", got.screen)
	}

	m.syncFromBackup = true
	next, _ = m.keySyncTarget("esc")
	if got := next.(model); got.screen != screenRestorePick {
		t.Fatalf("target esc (backup): screen=%v, want backup item picker", got.screen)
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

func TestStaleDiffDoesNotOverwriteNewerComparison(t *testing.T) {
	m := New(testConfig(t))
	m.screen = screenBrowse
	m.diffGen = 2
	m.diffLoading = true
	m.diffChanges = []diff.Change{{Name: "newer"}}

	next, _ := m.Update(diffDoneMsg{gen: 1, changes: []diff.Change{{Name: "stale"}}})
	got := next.(model)
	if !got.diffLoading || len(got.diffChanges) != 1 || got.diffChanges[0].Name != "newer" {
		t.Fatalf("loading=%v changes=%+v", got.diffLoading, got.diffChanges)
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
	m.height = 24
	if got := m.listRows(16); got != 6 {
		t.Fatalf("listRows=%d, want 6", got)
	}
}

func TestQOnlyQuitsFromTopLevelScreens(t *testing.T) {
	m := New(testConfig(t))
	m.connected = true
	m.screen = screenSettings
	next, cmd := m.handleKey("q")
	if cmd != nil || next.(model).screen != screenSettings {
		t.Fatal("q should not quit or navigate from a nested screen")
	}

	m.screen = screenDashboard
	_, cmd = m.handleKey("q")
	if cmd == nil {
		t.Fatal("q should quit from the dashboard")
	}
}
