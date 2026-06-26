package tui

import (
	"testing"

	"github.com/ugurkocde/TenuVault-TUI/internal/config"
	"github.com/ugurkocde/TenuVault-TUI/internal/connection"
	"github.com/ugurkocde/TenuVault-TUI/internal/graph"
)

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
