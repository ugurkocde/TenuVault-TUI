package tui

import (
	"strings"
	"testing"

	"github.com/ugurkocde/TenuVault-TUI/internal/catalog"
	"github.com/ugurkocde/TenuVault-TUI/internal/config"
	"github.com/ugurkocde/TenuVault-TUI/internal/connection"
	"github.com/ugurkocde/TenuVault-TUI/internal/diff"
	"github.com/ugurkocde/TenuVault-TUI/internal/graph"
	"github.com/ugurkocde/TenuVault-TUI/internal/restore"
	"github.com/ugurkocde/TenuVault-TUI/internal/store"
	"github.com/ugurkocde/TenuVault-TUI/internal/syncer"
)

// seededModel returns a model with realistic state for every screen.
func seededModel() model {
	m := New(config.Default())
	m.width, m.height = 100, 40
	m.connected = true
	m.conns = []connection.Connection{
		{Label: "Ugur Koc Lab", Tenant: graph.Tenant{DisplayName: "Ugur Koc Lab", DefaultDomain: "sl6ll.onmicrosoft.com", DomainCount: 4}},
		{Label: "Contoso Prod", Tenant: graph.Tenant{DisplayName: "Contoso Prod", DefaultDomain: "contoso.onmicrosoft.com", DomainCount: 1}},
	}
	m.sourceIdx = 0
	m.cfg.Connections = []config.ConnConfig{{Label: "Ugur Koc Lab", TenantID: "ffc1", AuthMethod: config.AuthInteractive}}
	m.syncTypes = buildSyncTypes()
	m.syncSourceLabel = "Ugur Koc Lab"
	m.syncItems = []syncer.Item{{Category: "DeviceConfigurations", Name: "BitLocker"}}
	m.syncTargetConn = 1
	m.syncResults = []syncer.Result{{Item: syncer.Item{Name: "BitLocker"}, NewID: "9f2a"}}
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
	return m
}

// TestRenderAllScreens ensures every screen renders without panicking and
// produces output at normal and narrow widths, with realistic state seeded in.
func TestRenderAllScreens(t *testing.T) {
	for _, w := range []int{40, 60, 100} {
		m := seededModel()
		m.width = w
		for s := screenAuth; s <= screenError; s++ {
			m.screen = s
			out := m.render()
			if strings.TrimSpace(out) == "" {
				t.Errorf("screen %d rendered empty at width %d", s, w)
			}
		}
	}
}

// TestRenderNarrowFooterFits ensures the footer hint line never exceeds the
// render width, even on the screens with the longest hints.
func TestRenderNarrowFooterFits(t *testing.T) {
	m := seededModel()
	m.width = 40
	for _, s := range []screen{screenDashboard, screenBackupSelect, screenRestorePick, screenConnections} {
		m.screen = s
		out := m.render()
		lines := strings.Split(out, "\n")
		footer := lines[len(lines)-1]
		if got := visibleWidth(footer); got > 40 {
			t.Errorf("screen %d footer is %d cols wide, want <= 40: %q", s, got, footer)
		}
	}
}

// visibleWidth measures a rendered line ignoring ANSI escapes.
func visibleWidth(line string) int {
	w := 0
	inEsc := false
	for _, r := range line {
		switch {
		case inEsc:
			if r == 'm' {
				inEsc = false
			}
		case r == '\x1b':
			inEsc = true
		default:
			w++
		}
	}
	return w
}

// TestRenderSyncBreadcrumbs pins the wizard step indicator on each sync screen
// and its absence outside the sync flow.
func TestRenderSyncBreadcrumbs(t *testing.T) {
	m := seededModel()
	steps := map[screen]string{
		screenSyncSource:  "step 1 of 6",
		screenSyncSelect:  "step 2 of 6",
		screenSyncTarget:  "step 4 of 6",
		screenSyncNaming:  "step 5 of 6",
		screenSyncConfirm: "step 6 of 6",
	}
	for s, want := range steps {
		m.screen = s
		if out := m.render(); !strings.Contains(out, want) {
			t.Errorf("screen %d missing %q", s, want)
		}
	}

	m.screen = screenSyncPolicies
	m.syncActiveType = 0
	m.syncTypes[0].loaded = true
	if out := m.render(); !strings.Contains(out, "step 3 of 6") {
		t.Error("sync policies screen missing step 3 of 6")
	}

	m.screen = screenRestorePick
	m.syncMode = true
	if out := m.render(); !strings.Contains(out, "step 3 of 6") {
		t.Error("restore pick in sync mode missing step 3 of 6")
	}
	m.syncMode = false
	if out := m.render(); strings.Contains(out, "step 3 of 6") {
		t.Error("restore pick outside sync mode shows a step indicator")
	}
	m.screen = screenSyncResults
	if out := m.render(); strings.Contains(out, "of 6") {
		t.Error("sync results screen shows a step indicator; it is not a wizard step")
	}
}

// TestRenderBrowseLoading ensures the browse screen shows the scan indicator
// instead of stale rows while a load is in flight, and ignores enter.
func TestRenderBrowseLoading(t *testing.T) {
	m := seededModel()
	m.screen = screenBrowse
	m.browseLoading = true

	out := m.render()
	if !strings.Contains(out, "scanning backups") {
		t.Error("loading browse screen missing the scan indicator")
	}
	if strings.Contains(out, "backup-2026-06-26-020000") {
		t.Error("loading browse screen rendered stale backup rows")
	}

	got, _ := m.keyBrowse("enter")
	if got.(model).screen != screenBrowse {
		t.Error("enter during a browse load left the screen")
	}

	upd, _ := m.Update(backupsLoadedMsg(m.backups))
	m2 := upd.(model)
	if m2.browseLoading {
		t.Error("backupsLoadedMsg did not clear browseLoading")
	}
	if out := m2.render(); !strings.Contains(out, "backup-2026-06-26-020000") {
		t.Error("loaded browse screen missing the backup row")
	}
}
