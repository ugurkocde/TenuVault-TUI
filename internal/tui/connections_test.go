package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/ugurkocde/TenuVault-TUI/internal/config"
	"github.com/ugurkocde/TenuVault-TUI/internal/connection"
	"github.com/ugurkocde/TenuVault-TUI/internal/graph"
	"github.com/ugurkocde/TenuVault-TUI/internal/store"
	"github.com/ugurkocde/TenuVault-TUI/internal/syncer"
)

// storeBackupForTest returns an empty on-disk backup rooted in a temp dir.
func storeBackupForTest(t *testing.T) store.Backup {
	t.Helper()
	return store.Backup{Folder: "backup-test", Path: t.TempDir()}
}

// TestRemoveTenant removes the selected tenant from both the live list and the
// persisted config. TENUVAULT_CONFIG_DIR isolates the write from the real file.
func TestRemoveTenant(t *testing.T) {
	t.Setenv("TENUVAULT_CONFIG_DIR", t.TempDir())

	m := New(config.Default())
	m.screen = screenConnections
	m.connected = true
	m.conns = []connection.Connection{
		{Label: "Lab", Cfg: config.Config{TenantID: "aaaa-1111"}, Tenant: graph.Tenant{ID: "aaaa-1111"}},
		{Label: "Prod", Cfg: config.Config{TenantID: "bbbb-2222"}, Tenant: graph.Tenant{ID: "bbbb-2222"}},
	}
	m.sourceIdx = 0
	m.cfg.Connections = []config.ConnConfig{
		{Label: "Lab", TenantID: "aaaa-1111"},
		{Label: "Prod", TenantID: "bbbb-2222"},
	}
	m.connCursor = 0

	out, _ := m.keyConnections("x") // arms the confirmation, removes nothing yet
	mid := out.(model)
	if len(mid.conns) != 2 {
		t.Fatalf("x alone removed a connection; want a pending confirmation")
	}
	out, _ = mid.keyConnections("y")
	m2 := out.(model)

	if len(m2.cfg.Connections) != 1 || m2.cfg.Connections[0].TenantID != "bbbb-2222" {
		t.Errorf("persisted connections = %+v, want only Prod", m2.cfg.Connections)
	}
	if len(m2.conns) != 1 || m2.conns[0].Tenant.ID != "bbbb-2222" {
		t.Errorf("live connections = %d, want 1 (Prod)", len(m2.conns))
	}
	if m2.sourceTenant().ID != "bbbb-2222" {
		t.Errorf("source after removing the source tenant = %q, want Prod", m2.sourceTenant().ID)
	}
}

// TestRemoveTenantCancelled arms a removal and cancels it; nothing may change.
func TestRemoveTenantCancelled(t *testing.T) {
	t.Setenv("TENUVAULT_CONFIG_DIR", t.TempDir())

	m := New(config.Default())
	m.screen = screenConnections
	m.connected = true
	m.conns = []connection.Connection{
		{Label: "Lab", Cfg: config.Config{TenantID: "aaaa-1111"}, Tenant: graph.Tenant{ID: "aaaa-1111"}},
	}
	m.cfg.Connections = []config.ConnConfig{{Label: "Lab", TenantID: "aaaa-1111"}}
	m.connCursor = 0

	for _, cancel := range []string{"n", "esc", "down"} {
		out, _ := m.keyConnections("x")
		out, _ = out.(model).keyConnections(cancel)
		m2 := out.(model)
		if len(m2.conns) != 1 || len(m2.cfg.Connections) != 1 {
			t.Errorf("cancel with %q still removed the connection", cancel)
		}
		if m2.connRemove != -1 {
			t.Errorf("cancel with %q left the confirmation pending", cancel)
		}
	}
}

// TestRemoveEarlierTenantKeepsSource removes a tenant listed before the active
// source; the source index must shift left so it still points at the same
// tenant.
func TestRemoveEarlierTenantKeepsSource(t *testing.T) {
	t.Setenv("TENUVAULT_CONFIG_DIR", t.TempDir())

	m := New(config.Default())
	m.screen = screenConnections
	m.connected = true
	m.conns = []connection.Connection{
		{Label: "Lab", Cfg: config.Config{TenantID: "aaaa-1111"}, Tenant: graph.Tenant{ID: "aaaa-1111"}},
		{Label: "Stage", Cfg: config.Config{TenantID: "bbbb-2222"}, Tenant: graph.Tenant{ID: "bbbb-2222"}},
		{Label: "Prod", Cfg: config.Config{TenantID: "cccc-3333"}, Tenant: graph.Tenant{ID: "cccc-3333"}},
	}
	m.sourceIdx = 2 // Prod is the active source
	m.cfg.Connections = []config.ConnConfig{
		{Label: "Lab", TenantID: "aaaa-1111"},
		{Label: "Stage", TenantID: "bbbb-2222"},
		{Label: "Prod", TenantID: "cccc-3333"},
	}
	m.connCursor = 0 // remove Lab

	out, _ := m.keyConnections("x")
	out, _ = out.(model).keyConnections("y")
	m2 := out.(model)

	if m2.sourceTenant().ID != "cccc-3333" {
		t.Errorf("source after removing an earlier tenant = %q, want Prod (cccc-3333)", m2.sourceTenant().ID)
	}
}

// TestRemoveShiftsSyncIndexes removes connections around the chosen sync
// source/target: earlier removals shift the indexes, removing the chosen one
// invalidates it.
func TestRemoveShiftsSyncIndexes(t *testing.T) {
	t.Setenv("TENUVAULT_CONFIG_DIR", t.TempDir())

	base := New(config.Default())
	base.screen = screenConnections
	base.connected = true
	base.conns = []connection.Connection{
		{Label: "A", Cfg: config.Config{TenantID: "aaaa-1111"}, Tenant: graph.Tenant{ID: "aaaa-1111"}},
		{Label: "B", Cfg: config.Config{TenantID: "bbbb-2222"}, Tenant: graph.Tenant{ID: "bbbb-2222"}},
		{Label: "C", Cfg: config.Config{TenantID: "cccc-3333"}, Tenant: graph.Tenant{ID: "cccc-3333"}},
	}
	base.cfg.Connections = []config.ConnConfig{
		{Label: "A", TenantID: "aaaa-1111"},
		{Label: "B", TenantID: "bbbb-2222"},
		{Label: "C", TenantID: "cccc-3333"},
	}
	base.syncSourceConn = 2 // C
	base.syncTargetConn = 1 // B

	// Removing A shifts both indexes left; they still point at C and B.
	m := base
	m.connCursor = 0
	out, _ := m.keyConnections("x")
	out, _ = out.(model).keyConnections("y")
	m2 := out.(model)
	if m2.syncSourceConn != 1 || m2.conns[m2.syncSourceConn].Label != "C" {
		t.Errorf("syncSourceConn = %d, want 1 (C)", m2.syncSourceConn)
	}
	if m2.syncTargetConn != 0 || m2.conns[m2.syncTargetConn].Label != "B" {
		t.Errorf("syncTargetConn = %d, want 0 (B)", m2.syncTargetConn)
	}

	// Removing C (the chosen sync source) invalidates the source index.
	m = base
	m.connCursor = 2
	out, _ = m.keyConnections("x")
	out, _ = out.(model).keyConnections("y")
	m2 = out.(model)
	if m2.syncSourceConn != -1 {
		t.Errorf("syncSourceConn = %d after removing the chosen source, want -1", m2.syncSourceConn)
	}
	if m2.syncTargetConn != 1 {
		t.Errorf("syncTargetConn = %d, want 1 (B unchanged)", m2.syncTargetConn)
	}
}

// TestSyncConfirmRejectsStaleSource ensures a sync confirm with an invalidated
// live source refuses to run and returns to the source picker.
func TestSyncConfirmRejectsStaleSource(t *testing.T) {
	m := New(config.Default())
	m.screen = screenSyncConfirm
	m.connected = true
	m.conns = []connection.Connection{
		{Label: "Target", Cfg: config.Config{TenantID: "bbbb-2222"}, Tenant: graph.Tenant{ID: "bbbb-2222"}},
	}
	m.syncFromBackup = false
	m.syncSourceConn = -1 // chosen source was removed
	m.syncTargetConn = 0
	m.syncItems = []syncer.Item{{Category: "DeviceConfigurations", Name: "P"}}

	out, _ := m.keySyncConfirm("y")
	m2 := out.(model)

	if m2.syncRunning {
		t.Fatal("sync started with a stale source connection")
	}
	if m2.screen != screenSyncSource {
		t.Errorf("screen = %v, want screenSyncSource", m2.screen)
	}
	if m2.statusKind != "err" {
		t.Errorf("statusKind = %q, want err", m2.statusKind)
	}
}

// TestRestoreFromBrowseDetailClearsSyncMode ensures a restore started from the
// browse-detail shortcut never inherits sync routing from an abandoned sync
// flow.
func TestRestoreFromBrowseDetailClearsSyncMode(t *testing.T) {
	m := New(config.Default())
	m.screen = screenBrowseDetail
	m.syncMode = true // leftover from an abandoned backup-source sync
	b := storeBackupForTest(t)
	m.detail = &b

	out, _ := m.keyBrowseDetail("r")
	m2 := out.(model)

	if m2.syncMode {
		t.Error("syncMode still set after starting restore from browse detail")
	}
	if m2.screen != screenRestorePick {
		t.Errorf("screen = %v, want screenRestorePick", m2.screen)
	}
}

// TestMouseClickDuringRemoveConfirm: a click while a removal confirmation is
// pending dismisses the confirmation and moves the cursor, but never removes
// a tenant and never fires the clicked row's action.
func TestMouseClickDuringRemoveConfirm(t *testing.T) {
	t.Setenv("TENUVAULT_CONFIG_DIR", t.TempDir())

	m := New(config.Default())
	m.width, m.height = 100, 40
	m.screen = screenConnections
	m.connected = true
	m.conns = []connection.Connection{
		{Label: "Lab", Cfg: config.Config{TenantID: "aaaa-1111"}, Tenant: graph.Tenant{ID: "aaaa-1111"}},
		{Label: "Prod", Cfg: config.Config{TenantID: "bbbb-2222"}, Tenant: graph.Tenant{ID: "bbbb-2222"}},
	}
	m.cfg.Connections = []config.ConnConfig{
		{Label: "Lab", TenantID: "aaaa-1111"},
		{Label: "Prod", TenantID: "bbbb-2222"},
	}
	m.connCursor = 0

	for _, click := range []int{1, 0} { // a different row, then the armed row itself
		armed, _ := m.keyConnections("x") // arm removal of Lab (cursor row 0)
		mm := armed.(model)
		if mm.connRemove != 0 {
			t.Fatalf("connRemove = %d, want 0 (armed)", mm.connRemove)
		}
		_ = mm.render()
		var row hitRow
		found := false
		for _, r := range mm.hits.rows {
			if r.index == click {
				row, found = r, true
				break
			}
		}
		if !found {
			t.Fatalf("no hit recorded for row %d", click)
		}
		res, _, ok := mm.handleMouse(tea.MouseClickMsg{Y: row.y, Button: tea.MouseLeft})
		if !ok {
			t.Fatal("mouse not handled")
		}
		m2 := res.(model)
		if len(m2.conns) != 2 || len(m2.cfg.Connections) != 2 {
			t.Fatalf("click on row %d removed a tenant", click)
		}
		if m2.connRemove != -1 {
			t.Errorf("click on row %d left the confirmation pending", click)
		}
		if m2.connCursor != click {
			t.Errorf("click on row %d moved cursor to %d", click, m2.connCursor)
		}
		if m2.screen != screenConnections {
			t.Errorf("click on row %d fired the row action (screen = %v)", click, m2.screen)
		}
	}
}
