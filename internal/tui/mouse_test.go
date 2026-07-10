package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/ugurkocde/TenuVault-TUI/internal/catalog"
	"github.com/ugurkocde/TenuVault-TUI/internal/config"
)

// TestMouseClickMapsToItem renders the backup picker, then clicks the row
// recorded for a specific item and asserts the cursor lands on that exact item
// and the click toggles it — validating the body-row offset and hit recording.
func TestMouseClickMapsToItem(t *testing.T) {
	m := New(config.Default())
	m.width, m.height = 100, 40
	m.screen = screenBackupSelect
	for _, pt := range catalog.All() {
		m.cats = append(m.cats, catSel{pt: pt, sel: pt.Verified})
	}
	_ = m.render() // populates m.hits

	if len(m.hits.rows) == 0 {
		t.Fatal("no clickable rows recorded")
	}
	// Click the third recorded row.
	target := m.hits.rows[2]
	before := m.cats[target.index].sel

	out, _, ok := m.handleMouse(tea.MouseClickMsg{Y: target.y, Button: tea.MouseLeft})
	if !ok {
		t.Fatal("mouse not handled")
	}
	m2 := out.(model)
	if m2.catCursor != target.index {
		t.Errorf("cursor = %d, want %d (click row %d mapped wrong)", m2.catCursor, target.index, target.y)
	}
	if m2.cats[target.index].sel == before {
		t.Error("left-click did not toggle the item on a toggle screen")
	}
}

func TestMouseWheelMovesCursor(t *testing.T) {
	m := New(config.Default())
	m.width, m.height = 100, 40
	m.screen = screenBackupSelect
	for _, pt := range catalog.All() {
		m.cats = append(m.cats, catSel{pt: pt, sel: pt.Verified})
	}
	_ = m.render()

	out, _, _ := m.handleMouse(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	if out.(model).catCursor != 1 {
		t.Errorf("wheel down: cursor = %d, want 1", out.(model).catCursor)
	}
}

func TestBrowseClickSelectsWithoutOpening(t *testing.T) {
	m := seededModel()
	m.screen = screenBrowse
	m.browseCursor = 0
	second := m.backups[0]
	second.Folder = "backup-2026-06-25-020000"
	second.Meta.BackupDate = "2026-06-25-020000"
	m.backups = append(m.backups, second)
	_ = m.render()

	var target hitRow
	for _, row := range m.hits.rows {
		if row.index == 1 {
			target = row
			break
		}
	}
	out, _, _ := m.handleMouse(tea.MouseClickMsg{X: 4, Y: target.y, Button: tea.MouseLeft})
	got := out.(model)
	if got.browseCursor != 1 || got.screen != screenBrowse {
		t.Fatalf("cursor=%d screen=%v, want selected preview without navigation", got.browseCursor, got.screen)
	}
}

func TestFooterBackIsClickable(t *testing.T) {
	m := seededModel()
	m.screen = screenSettings
	_ = m.render()
	var back hitAction
	for _, action := range m.hits.actions {
		if action.key == "esc" {
			back = action
			break
		}
	}
	if back.key == "" {
		t.Fatal("footer did not register a clickable esc action")
	}
	out, _, _ := m.handleMouse(tea.MouseClickMsg{X: back.x0, Y: back.y, Button: tea.MouseLeft})
	if got := out.(model).screen; got != screenDashboard {
		t.Fatalf("screen=%v after clicking Back, want dashboard", got)
	}
}

func TestFilterPromptIsClickable(t *testing.T) {
	m := New(config.Default())
	m.width, m.height = 100, 40
	m.screen = screenBackupSelect
	for _, pt := range catalog.All() {
		m.cats = append(m.cats, catSel{pt: pt, sel: pt.Verified})
	}
	_ = m.render()
	var filter hitAction
	for _, action := range m.hits.actions {
		if action.key == "/" {
			filter = action
			break
		}
	}
	if filter.key == "" {
		t.Fatal("filter prompt did not register a click action")
	}
	out, _, _ := m.handleMouse(tea.MouseClickMsg{X: filter.x0, Y: filter.y, Button: tea.MouseLeft})
	if !out.(model).filterActive {
		t.Fatal("clicking the filter prompt did not activate filter input")
	}
}

func TestAuthFormFieldsAreClickable(t *testing.T) {
	m := seededModel()
	m.screen = screenAuthForm
	m.authFormFields = []formField{{label: "Tenant ID"}, {label: "Client ID"}, {label: "Client secret", masked: true}}
	_ = m.render()
	var target hitRow
	for _, row := range m.hits.rows {
		if row.index == 2 {
			target = row
			break
		}
	}
	out, _, _ := m.handleMouse(tea.MouseClickMsg{X: 4, Y: target.y, Button: tea.MouseLeft})
	got := out.(model)
	if got.authFormCursor != 2 || got.screen != screenAuthForm {
		t.Fatalf("cursor=%d screen=%v, want third field focused", got.authFormCursor, got.screen)
	}
}

func TestStaleSyncLoadIgnored(t *testing.T) {
	m := New(config.Default())
	m.syncGen = 5
	m.syncTypes = []syncType{{pt: catalog.PolicyType{Key: "deviceConfigurations", Category: "DeviceConfigurations"}}}
	// A result tagged with an older generation must be dropped.
	m.applySyncPolicies(syncPoliciesLoadedMsg{typeKey: "deviceConfigurations", gen: 4, policies: []syncPol{{name: "x"}}})
	if m.syncTypes[0].loaded {
		t.Error("stale-generation result should have been ignored")
	}
	// Current generation applies.
	m.applySyncPolicies(syncPoliciesLoadedMsg{typeKey: "deviceConfigurations", gen: 5, policies: []syncPol{{name: "x"}}})
	if !m.syncTypes[0].loaded || len(m.syncTypes[0].policies) != 1 {
		t.Error("current-generation result should have applied")
	}
}
