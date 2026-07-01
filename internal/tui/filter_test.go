package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/ugurkocde/TenuVault-TUI/internal/catalog"
	"github.com/ugurkocde/TenuVault-TUI/internal/config"
	"github.com/ugurkocde/TenuVault-TUI/internal/restore"
)

// press feeds a sequence of key strings through handleKey and returns the
// resulting model.
func press(t *testing.T, m model, keys ...string) model {
	t.Helper()
	for _, k := range keys {
		out, _ := m.handleKey(k)
		m = out.(model)
	}
	return m
}

// TestRestorePickFilter drives the "/" filter on the restore picker: typed
// runes narrow the list, the cursor and space-toggle keep original indexes,
// "a" selects only the filtered subset, enter keeps the filter, and esc
// clears it before leaving the screen.
func TestRestorePickFilter(t *testing.T) {
	m := New(config.Default())
	m.screen = screenRestorePick
	m.restoreItems = []restoreSel{
		{item: restore.Item{Category: "C", Name: "Alpha Baseline"}},
		{item: restore.Item{Category: "C", Name: "Beta Policy"}},
		{item: restore.Item{Category: "C", Name: "alphabet soup"}},
	}

	m = press(t, m, "/", "a", "l", "p", "h", "a")
	if !m.filterActive || m.filterQuery != "alpha" {
		t.Fatalf("filter state = active %v query %q, want active with %q", m.filterActive, m.filterQuery, "alpha")
	}
	if m.restoreCursor != 0 {
		t.Fatalf("cursor = %d, want 0 (first match)", m.restoreCursor)
	}

	// Enter keeps the filter and returns to navigation.
	m = press(t, m, "enter")
	if m.filterActive || m.filterQuery != "alpha" {
		t.Fatalf("after enter: active %v query %q, want inactive with %q kept", m.filterActive, m.filterQuery, "alpha")
	}
	if m.screen != screenRestorePick {
		t.Fatal("enter while filtering must not leave the screen")
	}

	// Navigation moves within matches only: down skips the filtered-out item 1
	// and lands on original index 2.
	m = press(t, m, "down")
	if m.restoreCursor != 2 {
		t.Fatalf("cursor after down = %d, want original index 2", m.restoreCursor)
	}

	// Space toggles the item at the original index, not the view position.
	m = press(t, m, " ")
	if !m.restoreItems[2].sel {
		t.Error("space did not toggle the item at original index 2")
	}
	if m.restoreItems[1].sel {
		t.Error("space toggled a filtered-out item")
	}

	// "a" toggles the filtered subset only. Item 2 is selected, item 0 is not,
	// so the first press selects both matches; item 1 stays untouched.
	m = press(t, m, "a")
	if !m.restoreItems[0].sel || !m.restoreItems[2].sel {
		t.Error("a did not select all filtered items")
	}
	if m.restoreItems[1].sel {
		t.Error("a selected an item outside the filtered subset")
	}

	// Backspace edits the query after re-entering filter mode.
	m = press(t, m, "/", "backspace")
	if m.filterQuery != "alph" {
		t.Errorf("query after backspace = %q, want %q", m.filterQuery, "alph")
	}

	// Esc while typing clears the filter and stays on the screen.
	m = press(t, m, "esc")
	if m.filterActive || m.filterQuery != "" {
		t.Errorf("esc while typing: active %v query %q, want cleared", m.filterActive, m.filterQuery)
	}
	if m.screen != screenRestorePick {
		t.Fatal("esc while typing must not leave the screen")
	}

	// With no filter set, esc leaves the screen as before.
	m = press(t, m, "esc")
	if m.screen != screenDashboard {
		t.Errorf("esc without filter: screen = %d, want dashboard", m.screen)
	}
}

// TestRestorePickFilterEscClearsBeforeLeaving checks that after committing a
// filter with enter, the first esc clears the query and the second leaves.
func TestRestorePickFilterEscClearsBeforeLeaving(t *testing.T) {
	m := New(config.Default())
	m.screen = screenRestorePick
	m.restoreItems = []restoreSel{
		{item: restore.Item{Name: "Alpha"}},
		{item: restore.Item{Name: "Beta"}},
	}
	m = press(t, m, "/", "b", "e", "enter")
	if m.filterQuery != "be" {
		t.Fatalf("query = %q, want %q", m.filterQuery, "be")
	}
	m = press(t, m, "esc")
	if m.filterQuery != "" || m.screen != screenRestorePick {
		t.Fatalf("first esc: query %q screen %d, want cleared query on same screen", m.filterQuery, m.screen)
	}
	m = press(t, m, "esc")
	if m.screen != screenDashboard {
		t.Errorf("second esc: screen = %d, want dashboard", m.screen)
	}
}

// TestSyncPoliciesFilter exercises the same filter on the sync policy list,
// including snapping the cursor onto the first match when the current item is
// filtered out.
func TestSyncPoliciesFilter(t *testing.T) {
	m := New(config.Default())
	m.screen = screenSyncPolicies
	m.syncActiveType = 0
	m.syncTypes = []syncType{{
		pt:     catalog.PolicyType{Key: "k", Category: "C", Friendly: "Type"},
		loaded: true,
		policies: []syncPol{
			{name: "One"},
			{name: "Two"},
			{name: "Twenty"},
		},
	}}

	// Cursor starts on "One", which the query filters out, so it snaps to the
	// first match at original index 1.
	m = press(t, m, "/", "t", "w")
	if m.syncPolCursor != 1 {
		t.Fatalf("cursor = %d, want snap to original index 1", m.syncPolCursor)
	}
	m = press(t, m, "enter", "down", " ")
	if m.syncPolCursor != 2 || !m.syncTypes[0].policies[2].sel {
		t.Errorf("cursor %d sel[2] %v, want toggle at original index 2", m.syncPolCursor, m.syncTypes[0].policies[2].sel)
	}
	m = press(t, m, "a")
	pols := m.syncTypes[0].policies
	if !pols[1].sel || !pols[2].sel || pols[0].sel {
		t.Errorf("a on filtered subset: sel = %v %v %v, want false true true", pols[0].sel, pols[1].sel, pols[2].sel)
	}
	// Esc with a filter set clears it; the next esc goes back to the type list.
	m = press(t, m, "esc")
	if m.filterQuery != "" || m.screen != screenSyncPolicies {
		t.Fatalf("first esc: query %q screen %d", m.filterQuery, m.screen)
	}
	m = press(t, m, "esc")
	if m.screen != screenSyncSelect {
		t.Errorf("second esc: screen = %d, want sync select", m.screen)
	}
}

// TestFilteredRenderRecordsOriginalIndexes renders a filtered restore list and
// asserts the mouse hit rows carry original item indexes, so a click toggles
// the right item.
func TestFilteredRenderRecordsOriginalIndexes(t *testing.T) {
	m := New(config.Default())
	m.width, m.height = 100, 40
	m.screen = screenRestorePick
	m.restoreItems = []restoreSel{
		{item: restore.Item{Category: "C", Name: "Alpha"}},
		{item: restore.Item{Category: "C", Name: "Beta"}},
		{item: restore.Item{Category: "C", Name: "Alps"}},
	}
	m = press(t, m, "/", "a", "l", "enter")

	_ = m.render()
	if len(m.hits.rows) != 2 {
		t.Fatalf("recorded %d hit rows, want 2 (filtered view)", len(m.hits.rows))
	}
	if m.hits.rows[0].index != 0 || m.hits.rows[1].index != 2 {
		t.Fatalf("hit indexes = %d, %d, want original indexes 0 and 2", m.hits.rows[0].index, m.hits.rows[1].index)
	}

	// Clicking the second visible row toggles original index 2.
	out, _, ok := m.handleMouse(tea.MouseClickMsg{Y: m.hits.rows[1].y, Button: tea.MouseLeft})
	if !ok {
		t.Fatal("mouse not handled")
	}
	m2 := out.(model)
	if m2.restoreCursor != 2 || !m2.restoreItems[2].sel {
		t.Errorf("click: cursor %d sel[2] %v, want cursor 2 and toggled", m2.restoreCursor, m2.restoreItems[2].sel)
	}
	if m2.restoreItems[1].sel {
		t.Error("click toggled a filtered-out item")
	}
}

// TestBackupSelectFilter checks the optional filter on the category picker.
func TestBackupSelectFilter(t *testing.T) {
	m := New(config.Default())
	m.screen = screenBackupSelect
	for _, pt := range catalog.All() {
		m.cats = append(m.cats, catSel{pt: pt})
	}
	m = press(t, m, "/", "c", "o", "m", "p", "l", "i", "a", "enter")
	visible := m.catsVisible()
	if len(visible) == 0 || len(visible) == len(m.cats) {
		t.Fatalf("filter matched %d of %d categories, want a strict subset", len(visible), len(m.cats))
	}
	m = press(t, m, "a")
	for _, i := range visible {
		if !m.cats[i].sel {
			t.Errorf("filtered category %d not selected by a", i)
		}
	}
	sel := 0
	for _, c := range m.cats {
		if c.sel {
			sel++
		}
	}
	if sel != len(visible) {
		t.Errorf("selected %d categories, want only the %d filtered ones", sel, len(visible))
	}
}
