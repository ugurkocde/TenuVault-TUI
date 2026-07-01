package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

// bodyBaseRow is the absolute terminal row where the screen body starts:
// header (row 0), divider (row 1), blank (row 2), body (row 3).
const bodyBaseRow = 3

// hitRow maps a rendered list row to the item index it selects.
type hitRow struct {
	y     int
	index int
}

// uiHits is the set of clickable rows recorded during the last render.
type uiHits struct {
	rows   []hitRow
	action string // key synthesized when a row is clicked
}

// recordHit registers the row about to be written to b as clickable for item
// index. It is called from View (read-only) but mutates the shared *uiHits.
func (m model) recordHit(b *strings.Builder, index int) {
	m.hits.rows = append(m.hits.rows, hitRow{y: strings.Count(b.String(), "\n"), index: index})
}

// finalizeHits offsets recorded body-relative rows to absolute screen rows and
// sets the click action for the current screen.
func (m model) finalizeHits() {
	for i := range m.hits.rows {
		m.hits.rows[i].y += bodyBaseRow
	}
	m.hits.action = clickAction(m.screen)
}

// clickAction is the key a left-click synthesizes on each screen after moving
// the cursor to the clicked item.
func clickAction(s screen) string {
	switch s {
	case screenBackupSelect, screenRestorePick, screenSyncPolicies:
		return " " // toggle
	case screenDiffResult, screenPolicyView:
		return "" // scroll-only screens; click just moves nothing
	default:
		return "enter"
	}
}

// handleMouse dispatches mouse messages: wheel scrolls the active screen, a left
// click moves the cursor to the clicked row and triggers its action.
func (m model) handleMouse(msg tea.Msg) (tea.Model, tea.Cmd, bool) {
	switch e := msg.(type) {
	case tea.MouseWheelMsg:
		switch e.Button {
		case tea.MouseWheelUp:
			mm, cmd := m.handleKey("up")
			return mm, cmd, true
		case tea.MouseWheelDown:
			mm, cmd := m.handleKey("down")
			return mm, cmd, true
		}
		return m, nil, true
	case tea.MouseClickMsg:
		if e.Button != tea.MouseLeft {
			return m, nil, true
		}
		for _, r := range m.hits.rows {
			if r.y == e.Y {
				// Commit an in-progress "/" filter so the synthesized action key is
				// handled as navigation, not typed into the query.
				m.filterActive = false
				m.setListCursor(r.index)
				if a := m.hits.action; a != "" {
					mm, cmd := m.handleKey(a)
					return mm, cmd, true
				}
				return m, nil, true
			}
		}
		return m, nil, true
	}
	return m, nil, false
}

// okOrWarn returns the status kind for a result count with the given failures.
func okOrWarn(fail int) string {
	if fail > 0 {
		return "warn"
	}
	return "ok"
}

// setListCursor points the active screen's list cursor at index.
func (m *model) setListCursor(i int) {
	switch m.screen {
	case screenAuth:
		m.authCursor = i
	case screenDashboard:
		m.dashCursor = i
	case screenBackupSelect:
		m.catCursor = i
	case screenBrowse:
		m.browseCursor = i
	case screenBrowseDetail:
		m.detailCursor = i
	case screenCategoryPolicies:
		m.policyCursor = i
	case screenRestorePick:
		m.restoreCursor = i
	case screenSettings:
		m.settingsCursor = i
	case screenConnections:
		m.connCursor = i
	case screenSyncSource:
		m.syncSourceCursor = i
	case screenSyncSelect:
		m.syncTypeCursor = i
	case screenSyncPolicies:
		m.syncPolCursor = i
	case screenSyncTarget:
		m.syncTargetCursor = i
	case screenSyncNaming:
		m.syncNameCursor = i
	}
}
