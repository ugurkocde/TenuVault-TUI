package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/ugurkocde/TenuVault-TUI/internal/catalog"
	"github.com/ugurkocde/TenuVault-TUI/internal/config"
	"github.com/ugurkocde/TenuVault-TUI/internal/restore"
	"github.com/ugurkocde/TenuVault-TUI/internal/store"
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

// TestSortComposesWithFilter sorts the restore picker by name while a filter
// is active: only matching items show, in name order, and toggles still hit
// the item under the cursor by original index.
func TestSortComposesWithFilter(t *testing.T) {
	m := New(config.Default())
	m.screen = screenRestorePick
	m.restoreItems = []restoreSel{
		{item: restore.Item{Category: "C", Name: "Zulu alpha"}},  // 0
		{item: restore.Item{Category: "C", Name: "Beta"}},        // 1
		{item: restore.Item{Category: "C", Name: "Accra alpha"}}, // 2
		{item: restore.Item{Category: "C", Name: "Mike alpha"}},  // 3
	}

	m = press(t, m, "/", "a", "l", "p", "h", "a", "enter", "s")
	if m.listSort != sortName {
		t.Fatalf("listSort = %v, want sortName", m.listSort)
	}
	if got, want := m.restoreVisible(), []int{2, 3, 0}; !equalInts(got, want) {
		t.Fatalf("visible = %v, want %v (filtered then name-sorted)", got, want)
	}

	// The cursor snapped into the visible set; move to the last visible item
	// (Zulu alpha, original index 0) and toggle it.
	m = press(t, m, "down", "down", " ")
	if !m.restoreItems[0].sel {
		t.Error("space did not toggle the item under the cursor (original index 0)")
	}
	if m.restoreItems[1].sel || m.restoreItems[2].sel || m.restoreItems[3].sel {
		t.Error("space toggled an item other than the cursor's")
	}

	// s cycles back to the default order without touching the filter.
	m = press(t, m, "s")
	if m.listSort != sortDefault || m.filterQuery != "alpha" {
		t.Errorf("after second s: sort %v query %q, want default order with query kept", m.listSort, m.filterQuery)
	}
}

// TestSortKeyWhileFilterInputActive ensures "s" typed during filter input goes
// into the query, not the sort mode.
func TestSortKeyWhileFilterInputActive(t *testing.T) {
	m := New(config.Default())
	m.screen = screenRestorePick
	m.restoreItems = []restoreSel{{item: restore.Item{Category: "C", Name: "Sun"}}}

	m = press(t, m, "/", "s")
	if m.listSort != sortDefault {
		t.Errorf("listSort changed while typing into the filter")
	}
	if m.filterQuery != "s" {
		t.Errorf("filterQuery = %q, want %q", m.filterQuery, "s")
	}
}

// TestSortResetsOnScreenEntry pins that entering a list screen starts unsorted.
func TestSortResetsOnScreenEntry(t *testing.T) {
	m := New(config.Default())
	m.screen = screenBrowse
	m.connected = true
	m.listSort = sortName

	out, _ := m.openBrowse(modeView, "Browse backups")
	m2 := out.(model)
	if m2.listSort != sortDefault {
		t.Error("openBrowse kept the previous sort mode")
	}
}

// TestBrowseSortToggle sorts the backups list by folder name and back.
func TestBrowseSortToggle(t *testing.T) {
	m := New(config.Default())
	m.screen = screenBrowse
	m.backups = []store.Backup{
		{Folder: "backup-2026-06-27-020000"},
		{Folder: "backup-2026-06-26-020000"},
	}

	if got := m.browseVisible(); !equalInts(got, []int{0, 1}) {
		t.Fatalf("default browse order = %v, want store order", got)
	}
	m = press(t, m, "s")
	if got := m.browseVisible(); !equalInts(got, []int{1, 0}) {
		t.Fatalf("name-sorted browse order = %v, want [1 0]", got)
	}
	// The cursor stays on the same backup (original index) across the toggle.
	if m.browseCursor != 0 {
		t.Errorf("browseCursor = %d, want 0 (same item)", m.browseCursor)
	}
}

// TestEmptyFilterHint renders the zero-match hint with the query and drops it
// once something matches.
func TestEmptyFilterHint(t *testing.T) {
	m := New(config.Default())
	m.width = 100
	m.screen = screenRestorePick
	m.restoreItems = []restoreSel{{item: restore.Item{Category: "C", Name: "Alpha"}}}

	m = press(t, m, "/", "x", "y", "z")
	out := m.render()
	if !strings.Contains(out, `no matches for "xyz" (esc to clear)`) {
		t.Errorf("zero-match render missing the hint:\n%s", out)
	}

	m = press(t, m, "esc") // clears the query while input is active
	if out := m.render(); strings.Contains(out, "no matches for") {
		t.Error("hint still rendered after esc cleared the query")
	}
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
