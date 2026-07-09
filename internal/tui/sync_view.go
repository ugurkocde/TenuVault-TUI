package tui

import (
	"fmt"
	"strings"
)

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
	if m.connRemove >= 0 && m.connRemove < len(rows) {
		b.WriteString("\n" + m.th.warn.Render("Remove "+rows[m.connRemove].label+"?") + "  " +
			m.th.danger.Render("[ y ] remove") + "  " + m.th.dim.Render("[ n ] cancel") + "\n")
	}
	return b.String()
}

func (m model) syncCrumb(title string, step int) string {
	return m.th.crumb.Render("Sync › "+title) + "  " + m.th.cardLabel.Render(fmt.Sprintf("step %d of 6", step))
}

func (m model) viewSyncSource(w int) string {
	var b strings.Builder
	b.WriteString(m.syncCrumb("source", 1) + "\n\n")
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
	b.WriteString(m.syncCrumb("select", 2) + "  " + m.th.dim.Render("source: "+m.syncSourceLabel) + "\n")
	b.WriteString(m.th.cardLabel.Render("space = whole type · enter = pick policies · n = choose target") + "\n\n")
	total := 0
	start, end := window(m.syncTypeCursor, len(m.syncTypes), m.listRows(16))
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
		case st.err != "":
			right = m.th.danger.Render("load failed · space to retry · enter to pick policies")
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
	b.WriteString(m.syncCrumb(st.pt.Friendly, 3) + m.sortTag() + "\n\n")
	if st.loading {
		b.WriteString(m.th.inProg.Render(spinnerFrames[m.frame%len(spinnerFrames)] + " loading policies…"))
		return b.String()
	}
	if st.err != "" {
		b.WriteString(m.th.danger.Render("Could not load policies: "+st.err) + "\n\n")
		b.WriteString(m.th.cardLabel.Render("r retry · esc back"))
		return b.String()
	}
	if len(st.policies) == 0 {
		b.WriteString(m.th.dim.Render("No policies of this type in the source tenant."))
		return b.String()
	}
	visible := syncPolVisible(st.policies, m.filterQuery, m.listSort)
	b.WriteString(m.filterLine(len(visible), len(st.policies)))
	if len(visible) == 0 {
		return b.String()
	}
	startp, endp := window(filteredPos(m.syncPolCursor, visible), len(visible), m.listRows(14))
	for vp := startp; vp < endp; vp++ {
		i := visible[vp]
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
	if sb := m.scrollbar(startp, endp, len(visible)); sb != "" {
		b.WriteString("  " + sb + "\n")
	}
	b.WriteString("\n" + m.th.accent.Render(fmt.Sprintf("%d of %d selected", countSelected(st.policies), len(st.policies))))
	return b.String()
}

func (m model) viewSyncTarget(w int) string {
	var b strings.Builder
	b.WriteString(m.syncCrumb("target", 4) + "\n\n")
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
	b.WriteString(m.syncCrumb("naming", 5) + "\n\n")
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
	b.WriteString(m.syncCrumb("confirm", 6) + "\n\n")
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
		b.WriteString(m.th.inProg.Render(fmt.Sprintf("%s creating… %d/%d", spinnerFrames[m.frame%len(spinnerFrames)], m.syncCur, m.syncTot)) +
			"  " + m.th.cardLabel.Render("x cancel"))
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

func (m model) rowMarker(active bool, label string) (string, string) {
	if active {
		return m.th.selected.Render("▸ "), m.th.selected.Render(label)
	}
	return "  ", m.th.normal.Render(label)
}
