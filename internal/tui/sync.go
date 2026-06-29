package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/ugurkocde/TenuVault-TUI/internal/catalog"
	"github.com/ugurkocde/TenuVault-TUI/internal/config"
	"github.com/ugurkocde/TenuVault-TUI/internal/restore"
	"github.com/ugurkocde/TenuVault-TUI/internal/syncer"
)

const syncPrefix = "[Synced] "

// syncerItemFromRestore converts a backup-source restore selection into a sync
// item (sourced from the backup file on disk).
func syncerItemFromRestore(it restore.Item) syncer.Item {
	return syncer.Item{Category: it.Category, Name: it.Name, Path: it.Path}
}

// --- entry ---

// startSync begins the sync flow from the source picker.
func (m model) startSync() (tea.Model, tea.Cmd) {
	if !m.connected {
		m.connCursor = 0
		m.goTo(screenConnections)
		return m, nil
	}
	m.syncFromBackup = false
	m.syncMode = false
	m.syncSourceCursor = 0
	m.syncItems = nil
	m.goTo(screenSyncSource)
	return m, nil
}

// --- connections screen ---

type connRow struct {
	label    string
	tenantID string
	cc       config.ConnConfig
	liveIdx  int // -1 if not currently connected
}

func (m model) connectionRows() []connRow {
	var rows []connRow
	for _, cc := range m.cfg.Connections {
		if placeholderTenant(cc.TenantID) {
			continue
		}
		live := -1
		for i := range m.conns {
			if m.conns[i].Tenant.ID == cc.TenantID {
				live = i
			}
		}
		rows = append(rows, connRow{label: cc.Label, tenantID: cc.TenantID, cc: cc, liveIdx: live})
	}
	return rows
}

func (m model) keyConnections(key string) (tea.Model, tea.Cmd) {
	rows := m.connectionRows()
	addIdx := len(rows) // last selectable row is "add"
	switch key {
	case "up", "k":
		if m.connCursor > 0 {
			m.connCursor--
		}
	case "down", "j":
		if m.connCursor < addIdx {
			m.connCursor++
		}
	case "a":
		m.addingConn = true
		m.goTo(screenAuth)
	case "x", "d", "delete":
		if m.connCursor >= len(rows) {
			return m, nil // the "add" row, nothing to remove
		}
		row := rows[m.connCursor]
		var kept []config.ConnConfig
		for _, cc := range m.cfg.Connections {
			if cc.TenantID != row.tenantID {
				kept = append(kept, cc)
			}
		}
		m.cfg.Connections = kept
		if row.liveIdx >= 0 {
			m.conns = append(m.conns[:row.liveIdx], m.conns[row.liveIdx+1:]...)
			if m.sourceIdx >= len(m.conns) {
				m.sourceIdx = len(m.conns) - 1
			}
			if m.sourceIdx < 0 {
				m.sourceIdx = 0
			}
			m.connected = len(m.conns) > 0
		}
		m.persistConnections()
		if m.connCursor > 0 {
			m.connCursor--
		}
		m.status, m.statusKind = "Removed "+row.label, "ok"
	case "enter":
		if m.connCursor == addIdx {
			m.addingConn = true
			m.goTo(screenAuth)
			return m, nil
		}
		row := rows[m.connCursor]
		if row.liveIdx >= 0 {
			m.sourceIdx = row.liveIdx
			m.goTo(screenDashboard)
			return m, nil
		}
		// Reconnect a remembered tenant. App-registration tenants go back through
		// the form so the secret (never persisted) can be re-entered.
		cfg := row.cc.Apply(m.cfg)
		if cfg.AuthMethod == config.AuthSecret || cfg.AuthMethod == config.AuthCertificate {
			m.cfg = cfg
			return m.startAuthForm(cfg.AuthMethod)
		}
		m.goTo(screenConnecting)
		return m, tea.Batch(connect(m.ctx, cfg, m.ch), listen(m.ctx, m.ch))
	case "esc", "q":
		if m.connected {
			m.goTo(screenDashboard)
		} else if key == "q" {
			return m, tea.Quit
		}
	}
	return m, nil
}

// --- sync source picker ---

func (m model) keySyncSource(key string) (tea.Model, tea.Cmd) {
	n := len(m.conns) + 1 // connections + "saved backup"
	switch key {
	case "up", "k":
		if m.syncSourceCursor > 0 {
			m.syncSourceCursor--
		}
	case "down", "j":
		if m.syncSourceCursor < n-1 {
			m.syncSourceCursor++
		}
	case "esc", "q":
		m.goTo(screenDashboard)
	case "enter":
		if m.syncSourceCursor < len(m.conns) {
			m.syncFromBackup = false
			m.syncMode = false
			m.syncSourceConn = m.syncSourceCursor
			m.syncSourceLabel = m.conns[m.syncSourceCursor].DisplayLabel()
			m.syncTypes = buildSyncTypes()
			m.syncTypeCursor = 0
			m.syncGen++ // invalidate any in-flight loads from a previous source
			m.goTo(screenSyncSelect)
			return m, nil
		}
		// Saved backup as source -> reuse the backup picker + restore-style list.
		m.syncFromBackup = true
		m.syncMode = true
		m.syncSourceLabel = "saved backup"
		return m.openBrowse(modeSyncBackup, "Sync · pick a backup")
	}
	return m, nil
}

func buildSyncTypes() []syncType {
	var out []syncType
	for _, pt := range catalog.All() {
		if pt.RestoreSupported {
			out = append(out, syncType{pt: pt})
		}
	}
	return out
}

// --- live source: type list ---

func (m model) keySyncSelect(key string) (tea.Model, tea.Cmd) {
	if len(m.syncTypes) == 0 {
		if key == "esc" || key == "q" {
			m.goTo(screenDashboard)
		}
		return m, nil
	}
	switch key {
	case "up", "k":
		if m.syncTypeCursor > 0 {
			m.syncTypeCursor--
		}
	case "down", "j":
		if m.syncTypeCursor < len(m.syncTypes)-1 {
			m.syncTypeCursor++
		}
	case " ", "space":
		st := &m.syncTypes[m.syncTypeCursor]
		if !st.loaded {
			if st.loading {
				return m, nil
			}
			st.loading = true
			st.pendingAll = true
			return m, loadSyncType(m.ctx, m.syncSourceClient(), st.pt, m.syncGen)
		}
		all := allSelected(st.policies)
		for i := range st.policies {
			st.policies[i].sel = !all
		}
	case "enter", "right", "l":
		m.syncActiveType = m.syncTypeCursor
		m.syncPolCursor = 0
		st := &m.syncTypes[m.syncActiveType]
		m.goTo(screenSyncPolicies)
		if !st.loaded && !st.loading {
			st.loading = true
			return m, loadSyncType(m.ctx, m.syncSourceClient(), st.pt, m.syncGen)
		}
	case "n", "tab":
		m.syncItems = m.gatherLiveSyncItems()
		if len(m.syncItems) == 0 {
			return m, nil
		}
		m.syncTargetCursor = 0
		m.goTo(screenSyncTarget)
	case "esc", "q":
		m.goTo(screenDashboard)
	}
	return m, nil
}

// --- live source: policy list for one type ---

func (m model) keySyncPolicies(key string) (tea.Model, tea.Cmd) {
	if m.syncActiveType < 0 || m.syncActiveType >= len(m.syncTypes) {
		m.goTo(screenSyncSelect)
		return m, nil
	}
	st := &m.syncTypes[m.syncActiveType]
	switch key {
	case "up", "k":
		if m.syncPolCursor > 0 {
			m.syncPolCursor--
		}
	case "down", "j":
		if m.syncPolCursor < len(st.policies)-1 {
			m.syncPolCursor++
		}
	case " ", "space":
		if len(st.policies) > 0 {
			st.policies[m.syncPolCursor].sel = !st.policies[m.syncPolCursor].sel
		}
	case "a":
		all := allSelected(st.policies)
		for i := range st.policies {
			st.policies[i].sel = !all
		}
	case "esc", "q":
		m.goTo(screenSyncSelect)
	}
	return m, nil
}

func (m *model) applySyncPolicies(msg syncPoliciesLoadedMsg) {
	if msg.gen != m.syncGen {
		return // stale result from a previous source selection
	}
	for i := range m.syncTypes {
		if m.syncTypes[i].pt.Key != msg.typeKey {
			continue
		}
		st := &m.syncTypes[i]
		st.loading = false
		st.loaded = true
		st.policies = msg.policies
		if st.pendingAll {
			for j := range st.policies {
				st.policies[j].sel = true
			}
			st.pendingAll = false
		}
		return
	}
}

func (m model) gatherLiveSyncItems() []syncer.Item {
	var items []syncer.Item
	for _, st := range m.syncTypes {
		for _, p := range st.policies {
			if p.sel {
				items = append(items, syncer.Item{Category: st.pt.Category, TypeKey: st.pt.Key, Name: p.name, Raw: p.raw})
			}
		}
	}
	return items
}

// --- target picker ---

// targetConns returns connection indices eligible as a sync target.
func (m model) targetConns() []int {
	var out []int
	for i := range m.conns {
		if !m.syncFromBackup && i == m.syncSourceConn {
			continue
		}
		out = append(out, i)
	}
	return out
}

func (m model) keySyncTarget(key string) (tea.Model, tea.Cmd) {
	targets := m.targetConns()
	switch key {
	case "up", "k":
		if m.syncTargetCursor > 0 {
			m.syncTargetCursor--
		}
	case "down", "j":
		if m.syncTargetCursor < len(targets)-1 {
			m.syncTargetCursor++
		}
	case "enter":
		if len(targets) == 0 {
			return m, nil
		}
		m.syncTargetConn = targets[m.syncTargetCursor]
		m.syncNameCursor = 0
		m.goTo(screenSyncNaming)
	case "esc", "q":
		m.goTo(screenDashboard)
	}
	return m, nil
}

// --- naming ---

func (m model) keySyncNaming(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "up", "k":
		if m.syncNameCursor > 0 {
			m.syncNameCursor--
		}
	case "down", "j":
		if m.syncNameCursor < 1 {
			m.syncNameCursor++
		}
	case "enter":
		if m.syncNameCursor == 1 {
			m.syncNamePrefix = syncPrefix
		} else {
			m.syncNamePrefix = ""
		}
		m.goTo(screenSyncConfirm)
	case "esc", "q":
		m.goTo(screenSyncTarget)
	}
	return m, nil
}

// --- confirm ---

func (m model) keySyncConfirm(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "y":
		if len(m.syncItems) == 0 || m.syncTargetConn < 0 || m.syncTargetConn >= len(m.conns) {
			m.goTo(screenDashboard)
			return m, nil
		}
		// Fail closed: the source client is only the chosen sync source (nil for a
		// backup source), never the dashboard's active connection.
		source := m.syncSourceClient()
		target := m.conns[m.syncTargetConn].Client
		m.syncRunning = true
		m.syncResults = nil
		m.syncCur, m.syncTot = 0, len(m.syncItems)
		return m, tea.Batch(runSync(m.ctx, target, source, m.syncItems, m.syncNamePrefix, m.ch), listen(m.ctx, m.ch))
	case "n", "esc":
		m.goTo(screenSyncNaming)
	}
	return m, nil
}

func allSelected(pols []syncPol) bool {
	if len(pols) == 0 {
		return false
	}
	for _, p := range pols {
		if !p.sel {
			return false
		}
	}
	return true
}

func countSelected(pols []syncPol) int {
	n := 0
	for _, p := range pols {
		if p.sel {
			n++
		}
	}
	return n
}

// --- views ---

func (m model) viewConnections(w int) string {
	var b strings.Builder
	b.WriteString(m.th.crumb.Render("Tenants") + "\n\n")
	rows := m.connectionRows()
	if len(rows) == 0 {
		b.WriteString(m.th.dim.Render("No tenants yet.") + "\n")
	}
	for i, r := range rows {
		marker := "  "
		label := m.th.normal.Render(emptyDash(r.label))
		if i == m.connCursor {
			marker = m.th.selected.Render("▸ ")
			label = m.th.selected.Render(emptyDash(r.label))
		}
		var state string
		switch {
		case r.liveIdx == m.sourceIdx && r.liveIdx >= 0:
			state = m.th.success.Render("● source")
		case r.liveIdx >= 0:
			state = m.th.success.Render("● connected")
		default:
			state = m.th.cardLabel.Render("○ not connected")
		}
		detail := r.tenantID
		if r.liveIdx >= 0 && m.conns[r.liveIdx].Tenant.DefaultDomain != "" {
			detail = m.conns[r.liveIdx].Tenant.DefaultDomain
		}
		meta := m.th.cardLabel.Render("  " + detail + " · " + string(r.cc.AuthMethod))
		m.recordHit(&b, i)
		b.WriteString(spread(marker+label+meta, state, w) + "\n")
	}
	addLabel := m.th.accent.Render("+ Add tenant")
	if m.connCursor == len(rows) {
		addLabel = m.th.selected.Render("▸ + Add tenant")
	} else {
		addLabel = "  " + addLabel
	}
	m.recordHit(&b, len(rows))
	b.WriteString(addLabel + "\n")
	return b.String()
}

func (m model) viewSyncSource(w int) string {
	var b strings.Builder
	b.WriteString(m.th.crumb.Render("Sync › source") + "\n\n")
	b.WriteString(m.th.dim.Render("Copy policies from…") + "\n\n")
	for i, c := range m.conns {
		marker, label := m.rowMarker(i == m.syncSourceCursor, c.DisplayLabel())
		m.recordHit(&b, i)
		b.WriteString(marker + label + m.th.cardLabel.Render("  "+c.Tenant.DefaultDomain) + "\n")
	}
	i := len(m.conns)
	marker, label := m.rowMarker(m.syncSourceCursor == i, "Saved backup")
	m.recordHit(&b, i)
	b.WriteString(marker + label + m.th.cardLabel.Render("  push a local backup into a tenant") + "\n")
	return b.String()
}

func (m model) viewSyncSelect(w int) string {
	var b strings.Builder
	b.WriteString(m.th.crumb.Render("Sync › select") + "  " + m.th.dim.Render("source: "+m.syncSourceLabel) + "\n")
	b.WriteString(m.th.cardLabel.Render("space = whole type · enter = pick policies · n = choose target") + "\n\n")
	total := 0
	start, end := window(m.syncTypeCursor, len(m.syncTypes), 16)
	for i := start; i < end; i++ {
		st := m.syncTypes[i]
		sel := countSelected(st.policies)
		total += sel
		box := m.th.dim.Render("[ ]")
		if st.loaded && sel > 0 {
			if allSelected(st.policies) {
				box = m.th.success.Render("[x]")
			} else {
				box = m.th.warn.Render("[~]")
			}
		}
		label := m.th.normal.Render(st.pt.Friendly)
		if i == m.syncTypeCursor {
			label = m.th.selected.Render(st.pt.Friendly)
		}
		right := ""
		switch {
		case st.loading:
			right = m.th.inProg.Render("loading…")
		case st.loaded && sel > 0:
			right = m.th.accent.Render(fmt.Sprintf("%d of %d", sel, len(st.policies)))
		case st.loaded:
			right = m.th.cardLabel.Render(fmt.Sprintf("%d", len(st.policies)))
		}
		m.recordHit(&b, i)
		b.WriteString(spread("  "+box+" "+label, right, min(w, 72)) + "\n")
	}
	if sb := m.scrollbar(start, end, len(m.syncTypes)); sb != "" {
		b.WriteString("  " + sb + "\n")
	}
	b.WriteString("\n" + m.th.accent.Render(fmt.Sprintf("%d policies selected", total)))
	return b.String()
}

func (m model) viewSyncPolicies(w int) string {
	var b strings.Builder
	if m.syncActiveType >= len(m.syncTypes) {
		return m.th.dim.Render("…")
	}
	st := m.syncTypes[m.syncActiveType]
	b.WriteString(m.th.crumb.Render("Sync › "+st.pt.Friendly) + "\n\n")
	if st.loading {
		b.WriteString(m.th.inProg.Render(spinnerFrames[m.frame%len(spinnerFrames)] + " loading policies…"))
		return b.String()
	}
	if len(st.policies) == 0 {
		b.WriteString(m.th.dim.Render("No policies of this type in the source tenant."))
		return b.String()
	}
	startp, endp := window(m.syncPolCursor, len(st.policies), 14)
	for i := startp; i < endp; i++ {
		p := st.policies[i]
		box := m.th.dim.Render("[ ]")
		if p.sel {
			box = m.th.success.Render("[x]")
		}
		name := m.th.normal.Render(p.name)
		if i == m.syncPolCursor {
			name = m.th.selected.Render(p.name)
		}
		m.recordHit(&b, i)
		b.WriteString(box + " " + name + "\n")
	}
	if sb := m.scrollbar(startp, endp, len(st.policies)); sb != "" {
		b.WriteString("  " + sb + "\n")
	}
	b.WriteString("\n" + m.th.accent.Render(fmt.Sprintf("%d of %d selected", countSelected(st.policies), len(st.policies))))
	return b.String()
}

func (m model) viewSyncTarget(w int) string {
	var b strings.Builder
	b.WriteString(m.th.crumb.Render("Sync › target") + "\n\n")
	targets := m.targetConns()
	if len(targets) == 0 {
		b.WriteString(m.th.warn.Render("No other tenant connected. Add one from Tenants first."))
		return b.String()
	}
	b.WriteString(m.th.dim.Render(fmt.Sprintf("Create %d policies in…", len(m.syncItems))) + "\n\n")
	for i, ci := range targets {
		c := m.conns[ci]
		marker, label := m.rowMarker(i == m.syncTargetCursor, c.DisplayLabel())
		m.recordHit(&b, i)
		b.WriteString(marker + label + m.th.cardLabel.Render("  "+c.Tenant.DefaultDomain) + "\n")
	}
	return b.String()
}

func (m model) viewSyncNaming(w int) string {
	var b strings.Builder
	b.WriteString(m.th.crumb.Render("Sync › naming") + "\n\n")
	opts := []struct{ label, desc string }{
		{"Keep original name", "Target policy carries the same name as the source."},
		{"Add prefix " + strings.TrimSpace(syncPrefix), "Copied policies are prefixed so they're easy to spot."},
	}
	for i, o := range opts {
		marker, label := m.rowMarker(i == m.syncNameCursor, o.label)
		m.recordHit(&b, i)
		b.WriteString(marker + label + "\n")
		b.WriteString("    " + m.th.dim.Render(o.desc) + "\n")
	}
	return b.String()
}

func (m model) viewSyncConfirm(w int) string {
	var b strings.Builder
	b.WriteString(m.th.crumb.Render("Sync › confirm") + "\n\n")
	target := ""
	if m.syncTargetConn >= 0 && m.syncTargetConn < len(m.conns) {
		target = m.conns[m.syncTargetConn].DisplayLabel()
	}
	naming := "keep original name"
	if m.syncNamePrefix != "" {
		naming = "prefix " + strings.TrimSpace(m.syncNamePrefix)
	}
	b.WriteString(m.th.dim.Render(fmt.Sprintf("Create %d policies in ", len(m.syncItems))) +
		m.th.inProg.Render(target) + m.th.dim.Render("  ·  "+naming) + "\n\n")
	limit := len(m.syncItems)
	if limit > 10 {
		limit = 10
	}
	for i := 0; i < limit; i++ {
		it := m.syncItems[i]
		b.WriteString("  " + m.th.success.Render("+") + " " + m.th.normal.Render(m.syncNamePrefix+it.Name) +
			" " + m.th.cardLabel.Render("· "+it.Category) + "\n")
	}
	if len(m.syncItems) > limit {
		b.WriteString("  " + m.th.dim.Render(fmt.Sprintf("… and %d more", len(m.syncItems)-limit)) + "\n")
	}
	b.WriteString("\n" + m.th.warn.Render("⚠ Never overwrites — only creates new policies; existing policies are untouched") + "\n")
	b.WriteString(m.th.dim.Render("   Conditional access is created disabled · assignments are not copied") + "\n\n")
	if m.syncRunning {
		b.WriteString(m.th.inProg.Render(fmt.Sprintf("%s creating… %d/%d", spinnerFrames[m.frame%len(spinnerFrames)], m.syncCur, m.syncTot)))
	} else {
		b.WriteString(m.th.success.Render("[ y ] create") + "   " + m.th.dim.Render("[ n ] cancel") + "   " + m.th.inProg.Render("● writes to "+target))
	}
	return b.String()
}

func (m model) viewSyncResults(w int) string {
	var b strings.Builder
	target := ""
	if m.syncTargetConn >= 0 && m.syncTargetConn < len(m.conns) {
		target = m.conns[m.syncTargetConn].DisplayLabel()
	}
	b.WriteString(m.th.crumb.Render("Sync › results") + "  " + m.th.dim.Render(target) + "\n\n")
	ok, fail, warned := 0, 0, 0
	for _, r := range m.syncResults {
		if r.Err != nil {
			fail++
			b.WriteString("  " + m.th.danger.Render("✗") + " " + m.th.normal.Render(r.Item.Name) + "  " +
				m.th.dim.Render(trunc(r.Err.Error(), 50)) + "\n")
		} else {
			ok++
			line := "  " + m.th.success.Render("✔") + " " + m.th.normal.Render(m.syncNamePrefix+r.Item.Name) +
				m.th.cardLabel.Render("  → "+trunc(r.NewID, 12))
			if r.Warn {
				warned++
				line += " " + m.th.warn.Render("⚠ partial content")
			}
			b.WriteString(line + "\n")
		}
	}
	summary := m.th.accent.Render(fmt.Sprintf("%d created, %d failed", ok, fail))
	if warned > 0 {
		summary += m.th.warn.Render(fmt.Sprintf(" · %d with partial content", warned))
	}
	b.WriteString("\n" + summary + m.th.dim.Render(" · nothing existing was modified"))
	return b.String()
}

// rowMarker renders a cursor marker + label for a simple list row.
func (m model) rowMarker(active bool, label string) (string, string) {
	if active {
		return m.th.selected.Render("▸ "), m.th.selected.Render(label)
	}
	return "  ", m.th.normal.Render(label)
}
