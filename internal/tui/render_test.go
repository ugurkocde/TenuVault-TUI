package tui

import (
	"strings"
	"testing"

	"github.com/ugurkocde/TenuVault-TUI/internal/catalog"
	"github.com/ugurkocde/TenuVault-TUI/internal/config"
	"github.com/ugurkocde/TenuVault-TUI/internal/diff"
	"github.com/ugurkocde/TenuVault-TUI/internal/graph"
	"github.com/ugurkocde/TenuVault-TUI/internal/restore"
	"github.com/ugurkocde/TenuVault-TUI/internal/store"
)

// TestRenderAllScreens ensures every screen renders without panicking and
// produces output, with realistic state seeded in.
func TestRenderAllScreens(t *testing.T) {
	m := New(config.Default())
	m.width, m.height = 100, 40
	m.connected = true
	m.tenant = graph.Tenant{DisplayName: "Ugur Koc Lab", DefaultDomain: "sl6ll.onmicrosoft.com", DomainCount: 4}
	b := store.Backup{Folder: "backup-2026-06-26-020000", Path: "/tmp/x",
		Meta: store.Metadata{BackupDate: "2026-06-26-020000", Status: "Success", ItemCounts: map[string]int{"DeviceConfigurations": 5}}}
	m.lastBackup = &b
	m.backups = []store.Backup{b}
	m.detail = &b
	m.detailCats = []catCount{{"DeviceConfigurations", 5}}
	m.diffA, m.diffB = &b, &b
	m.diffChanges = []diff.Change{{Category: "CompliancePolicies", Name: "x", Type: diff.Removed, Severity: diff.Critical}}
	m.restoreBackup = &b
	m.restoreItems = []restoreSel{{item: restore.Item{Category: "DeviceConfigurations", Name: "p", Path: "/tmp/p"}, sel: true}}
	m.restoreResults = []restore.Result{{Item: restore.Item{Name: "p"}, NewID: "1"}}
	for _, pt := range catalog.All() {
		m.cats = append(m.cats, catSel{pt: pt, sel: pt.Verified})
	}
	m.progOrder = []string{"DeviceConfigurations"}
	m.progEvents["DeviceConfigurations"] = "running"
	m.progFriendly["DeviceConfigurations"] = "Device configurations"
	m.progActive, m.progCur, m.progTot = "Device configurations", 3, 5

	for s := screenAuth; s <= screenError; s++ {
		m.screen = s
		out := m.render()
		if strings.TrimSpace(out) == "" {
			t.Errorf("screen %d rendered empty", s)
		}
	}
}
