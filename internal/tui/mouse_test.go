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
