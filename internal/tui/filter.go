package tui

import (
	"fmt"
	"sort"
	"strings"
	"unicode"
)

// The "/" filter narrows long list screens (backup select, restore pick,
// category policies, sync policies) by a case-insensitive substring match on
// the item name. While the filter input is active, typed runes extend the
// query and backspace deletes; enter keeps the query and returns to
// navigation; esc clears it. List cursors always hold original item indexes,
// never positions in the filtered view, so toggling and mouse hit-testing
// stay correct while a filter is applied.

// resetFilter clears the "/" filter and the sort mode when entering a
// filterable or sortable list screen.
func (m *model) resetFilter() {
	m.filterActive = false
	m.filterQuery = ""
	m.listSort = sortDefault
}

// sortMode orders a list screen's visible items. "s" cycles it.
type sortMode int

const (
	sortDefault sortMode = iota // API/disk order (newest-first for backups)
	sortName                    // case-insensitive name ascending
)

// cycleSort advances to the next sort mode.
func cycleSort(mode sortMode) sortMode {
	return (mode + 1) % 2
}

// sortVisible returns visible (original indexes) reordered by mode. The
// backing slices are never reordered: cursors, selections, and mouse hits all
// key off original indexes, so sorting composes with filtering for free.
func sortVisible(visible []int, mode sortMode, name func(int) string) []int {
	if mode == sortDefault {
		return visible
	}
	out := make([]int, len(visible))
	copy(out, visible)
	sort.SliceStable(out, func(a, b int) bool {
		return strings.ToLower(name(out[a])) < strings.ToLower(name(out[b]))
	})
	return out
}

// filterInputKey handles one keypress while the filter input is active and
// reports whether the key was consumed. Multi-rune keys (up, down, ...) are
// not consumed, so the list can still be navigated while typing.
func (m *model) filterInputKey(key string) bool {
	switch key {
	case "enter":
		m.filterActive = false
	case "esc":
		m.filterActive = false
		m.filterQuery = ""
	case "backspace":
		if r := []rune(m.filterQuery); len(r) > 0 {
			m.filterQuery = string(r[:len(r)-1])
		}
	case " ", "space":
		m.filterQuery += " "
	default:
		r := []rune(key)
		if len(r) != 1 || unicode.IsControl(r[0]) {
			return false
		}
		m.filterQuery += key
	}
	return true
}

// filterIndexes returns the original indexes of items whose name contains
// query, case-insensitively. An empty query matches everything.
func filterIndexes(total int, name func(int) string, query string) []int {
	q := strings.ToLower(query)
	idx := make([]int, 0, total)
	for i := 0; i < total; i++ {
		if q == "" || strings.Contains(strings.ToLower(name(i)), q) {
			idx = append(idx, i)
		}
	}
	return idx
}

// filteredPos returns cur's position within visible, or 0 when absent.
func filteredPos(cur int, visible []int) int {
	for p, oi := range visible {
		if oi == cur {
			return p
		}
	}
	return 0
}

// snapFiltered keeps cur when it is still visible, otherwise moves it to the
// first visible item. With nothing visible it returns cur unchanged.
func snapFiltered(cur int, visible []int) int {
	for _, oi := range visible {
		if oi == cur {
			return cur
		}
	}
	if len(visible) > 0 {
		return visible[0]
	}
	return cur
}

// stepFiltered moves cur (an original index) delta steps within visible and
// returns the resulting original index, clamped to the visible range. A cursor
// that is not visible snaps to the first visible item.
func stepFiltered(cur, delta int, visible []int) int {
	if len(visible) == 0 {
		return cur
	}
	pos := -1
	for p, oi := range visible {
		if oi == cur {
			pos = p
			break
		}
	}
	if pos < 0 {
		return visible[0]
	}
	pos += delta
	if pos < 0 {
		pos = 0
	}
	if pos >= len(visible) {
		pos = len(visible) - 1
	}
	return visible[pos]
}

// idxVisible reports whether original index i is in visible.
func idxVisible(visible []int, i int) bool {
	for _, oi := range visible {
		if oi == i {
			return true
		}
	}
	return false
}

// catsVisible returns the visible original indexes of the backup categories.
func (m model) catsVisible() []int {
	return filterIndexes(len(m.cats), func(i int) string { return m.cats[i].pt.Friendly }, m.filterQuery)
}

// restoreVisible returns the visible original indexes of the restore items.
func (m model) restoreVisible() []int {
	name := func(i int) string { return m.restoreItems[i].item.Name }
	return sortVisible(filterIndexes(len(m.restoreItems), name, m.filterQuery), m.listSort, name)
}

// catPoliciesVisible returns the visible original indexes of the browse
// category's policy files.
func (m model) catPoliciesVisible() []int {
	name := func(i int) string { return m.catPolicies[i].Name }
	return sortVisible(filterIndexes(len(m.catPolicies), name, m.filterQuery), m.listSort, name)
}

// syncPolVisible returns the visible original indexes of a sync type's
// policies.
func syncPolVisible(pols []syncPol, query string, mode sortMode) []int {
	name := func(i int) string { return pols[i].name }
	return sortVisible(filterIndexes(len(pols), name, query), mode, name)
}

// browseVisible returns the display order of the backups list: store.List's
// newest-first order by default, or folder name ascending under sortName.
func (m model) browseVisible() []int {
	idx := make([]int, len(m.backups))
	for i := range idx {
		idx[i] = i
	}
	return sortVisible(idx, m.listSort, func(i int) string { return m.backups[i].Folder })
}

// filterLine renders the "/" filter input with a match count, or "" when no
// filter is in play.
func (m model) filterLine(matches, total int) string {
	if !m.filterActive && m.filterQuery == "" {
		return ""
	}
	prompt := m.th.accent.Render("/" + m.filterQuery)
	if m.filterActive {
		prompt += m.th.accent.Render("▌")
	}
	line := prompt + m.th.cardLabel.Render(fmt.Sprintf("  %d of %d", matches, total))
	if matches == 0 && m.filterQuery != "" {
		line += "  " + m.th.warn.Render(fmt.Sprintf("no matches for %q (esc to clear)", m.filterQuery))
	}
	return line + "\n"
}

// sortTag renders the active sort mode indicator for a list crumb, or "".
func (m model) sortTag() string {
	if m.listSort == sortName {
		return "  " + m.th.cardLabel.Render("sort: name")
	}
	return ""
}
