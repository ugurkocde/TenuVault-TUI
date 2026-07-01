package tui

import (
	"testing"

	"github.com/ugurkocde/TenuVault-TUI/internal/config"
	"github.com/ugurkocde/TenuVault-TUI/internal/connection"
	"github.com/ugurkocde/TenuVault-TUI/internal/graph"
	"github.com/ugurkocde/TenuVault-TUI/internal/store"
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

	out, _ := m.keyConnections("x")
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
	m2 := out.(model)

	if m2.sourceTenant().ID != "cccc-3333" {
		t.Errorf("source after removing an earlier tenant = %q, want Prod (cccc-3333)", m2.sourceTenant().ID)
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
