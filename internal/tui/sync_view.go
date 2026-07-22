package tui

import (
	"fmt"
	"strings"
)

func (m model) viewConnections(w int) string {
	var b strings.Builder
	b.WriteString(m.pageHeading("Dashboard / Tenants", "Manage source and target Microsoft Entra connections.") + "\n\n")
	rows := m.connectionRows()
	if len(rows) == 0 {
		b.WriteString(m.th.dim.Render("No tenants yet.") + "\n")
	}
	start, end := window(m.connCursor, len(rows)+1, m.listRows(8))
	for i := start; i < min(end, len(rows)); i++ {
		r := rows[i]
		var state string
		switch {
		case r.liveIdx == m.sourceIdx && r.liveIdx >= 0:
			state = "● SOURCE"
		case r.liveIdx >= 0:
			state = "● CONNECTED"
		default:
			state = "○ NOT CONNECTED"
		}
		detail := r.tenantID
		if r.liveIdx >= 0 && m.conns[r.liveIdx].Tenant.DefaultDomain != "" {
			detail = m.conns[r.liveIdx].Tenant.DefaultDomain
		}
		m.recordHit(&b, i)
		b.WriteString(m.actionRow(emptyDash(r.label), state, w, i == m.connCursor) + "\n")
		if !m.isShort() {
			b.WriteString("    " + m.th.cardLabel.Render(detail+" · "+friendlyAuthMethod(r.cc.AuthMethod)) + "\n")
		}
	}
	if end > len(rows) {
		m.recordHit(&b, len(rows))
		b.WriteString(m.actionRow("+  Add another tenant", m.keyBadge("a"), w, m.connCursor == len(rows)) + "\n")
	}
	if sb := m.scrollbar(start, end, len(rows)+1); sb != "" {
		b.WriteString(" " + sb + "\n")
	}
	b.WriteString("\n" + m.th.panel.Width(min(w-2, 88)).Render(m.th.cardLabel.Render("ⓘ Tokens are kept in memory. Connection metadata contains no credentials.")) + "\n")
	if m.connRemove >= 0 && m.connRemove < len(rows) {
		b.WriteString("\n" + m.th.warn.Render("Remove "+rows[m.connRemove].label+"?") + "  " +
			m.th.danger.Render("[ y ] remove") + "  " + m.th.dim.Render("[ n ] cancel") + "\n")
	}
	return b.String()
}

func (m model) syncCrumb(title string, step int) string {
	labels := []string{"Source", "Categories", "Policies", "Target", "Naming", "Review"}
	parts := make([]string, 0, len(labels))
	for i, label := range labels {
		n := i + 1
		item := fmt.Sprintf("%d %s", n, label)
		switch {
		case n < step:
			item = m.th.success.Render("✔ " + item)
		case n == step:
			item = m.th.selected.Render(item)
		default:
			item = m.th.cardLabel.Render(item)
		}
		parts = append(parts, item)
	}
	crumb := m.th.crumb.Render("Sync / "+title) + "  " + m.th.cardLabel.Render(fmt.Sprintf("step %d of 6", step))
	if m.contentWidth() < wideWidth {
		return crumb
	}
	return crumb + "\n" + strings.Join(parts, "   ")
}

func (m model) viewSyncSource(w int) string {
	var b strings.Builder
	b.WriteString(m.syncCrumb("source", 1) + "\n\n")
	b.WriteString(m.pageHeading("Choose a source", "Copy policies from a connected tenant or saved backup.") + "\n\n")
	start, end := window(m.syncSourceCursor, len(m.conns)+1, m.listRows(10))
	for i := start; i < min(end, len(m.conns)); i++ {
		c := m.conns[i]
		m.recordHit(&b, i)
		b.WriteString(m.actionRow(c.DisplayLabel(), c.Tenant.DefaultDomain, w, i == m.syncSourceCursor) + "\n")
	}
	i := len(m.conns)
	if end > i {
		m.recordHit(&b, i)
		b.WriteString(m.actionRow("Saved backup", "local snapshot", w, m.syncSourceCursor == i) + "\n")
	}
	if sb := m.scrollbar(start, end, len(m.conns)+1); sb != "" {
		b.WriteString(" " + sb + "\n")
	}
	return b.String()
}

func (m model) viewSyncSelect(w int) string {
	var b strings.Builder
	b.WriteString(m.syncCrumb("policies", 2) + "\n\n")
	b.WriteString(m.pageHeading("Choose policies to sync", "Source: "+m.syncSourceLabel) + "\n\n")
	total := 0
	for _, st := range m.syncTypes {
		total += countSelected(st.policies)
	}
	start, end := window(m.syncTypeCursor, len(m.syncTypes), m.listRows(16))
	for i := start; i < end; i++ {
		st := m.syncTypes[i]
		sel := countSelected(st.policies)
		state := "none"
		if st.loaded && sel > 0 {
			if allSelected(st.policies) {
				state = "all"
			} else {
				state = "some"
			}
		}
		right := ""
		switch {
		case st.loading:
			right = "loading…"
		case st.err != "":
			right = "load failed"
		case st.loaded && sel > 0:
			right = fmt.Sprintf("%d of %d", sel, len(st.policies))
		case st.loaded:
			right = fmt.Sprintf("0 of %d", len(st.policies))
		}
		m.recordHit(&b, i)
		b.WriteString(m.triCheckRow(state, st.pt.Friendly, right, min(w, 84), i == m.syncTypeCursor) + "\n")
	}
	if sb := m.scrollbar(start, end, len(m.syncTypes)); sb != "" {
		b.WriteString("  " + sb + "\n")
	}
	b.WriteString("\n" + m.th.accent.Render(fmt.Sprintf("%d policies selected", total)))
	if w >= 72 {
		b.WriteString("  " + m.th.success.Render("Create-only; never overwrites"))
	} else {
		b.WriteString("\n" + m.th.success.Render("Create-only; never overwrites"))
	}
	return b.String()
}

func (m model) viewSyncPolicies(w int) string {
	var b strings.Builder
	if m.syncActiveType >= len(m.syncTypes) {
		return m.th.dim.Render("…")
	}
	st := m.syncTypes[m.syncActiveType]
	b.WriteString(m.syncCrumb("policies", 3) + "\n\n")
	b.WriteString(m.pageHeading(st.pt.Friendly, "Choose individual policies from this category.") + m.sortTag() + "\n\n")
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
	if m.filterActive || m.filterQuery != "" {
		b.WriteString(m.filterLine(len(visible), len(st.policies)))
	} else {
		m.recordBodyAction(strings.Count(b.String(), "\n"), 1, w, "/")
		b.WriteString(m.th.cardLabel.Render("/  Filter policies") + "\n")
	}
	if len(visible) == 0 {
		return b.String()
	}
	startp, endp := window(filteredPos(m.syncPolCursor, visible), len(visible), m.listRows(14))
	for vp := startp; vp < endp; vp++ {
		i := visible[vp]
		p := st.policies[i]
		m.recordHit(&b, i)
		b.WriteString(m.checkRow(p.sel, p.name, "", w, i == m.syncPolCursor) + "\n")
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
	b.WriteString(m.pageHeading("Choose a target tenant", fmt.Sprintf("Create %d policies in…", len(m.syncItems))) + "\n\n")
	start, end := window(m.syncTargetCursor, len(targets), m.listRows(10))
	for i := start; i < end; i++ {
		ci := targets[i]
		c := m.conns[ci]
		m.recordHit(&b, i)
		b.WriteString(m.actionRow(c.DisplayLabel(), c.Tenant.DefaultDomain, w, i == m.syncTargetCursor) + "\n")
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
		m.recordHit(&b, i)
		b.WriteString(m.actionRow(o.label, "", w, i == m.syncNameCursor) + "\n")
		b.WriteString("    " + m.th.dim.Render(truncWidth(o.desc, w-4)) + "\n")
	}
	return b.String()
}

func (m model) viewSyncConfirm(w int) string {
	var b strings.Builder
	b.WriteString(m.syncCrumb("review", 6) + "\n\n")
	b.WriteString(m.pageHeading("Review sync plan", "Confirm the policies that will be created.") + "\n\n")
	target := ""
	if m.syncTargetConn >= 0 && m.syncTargetConn < len(m.conns) {
		target = m.conns[m.syncTargetConn].DisplayLabel()
	}
	naming := "keep original name"
	if m.syncNamePrefix != "" {
		naming = "prefix " + strings.TrimSpace(m.syncNamePrefix)
	}
	summary := fmt.Sprintf("Create %d policies in %s · %s", len(m.syncItems), target, naming)
	b.WriteString(m.th.dim.Render(truncWidth(summary, w)) + "\n\n")
	limit := min(len(m.syncItems), m.listRows(10))
	for i := 0; i < limit; i++ {
		it := m.syncItems[i]
		b.WriteString("  " + m.th.success.Render("+") + " " + m.th.normal.Render(m.syncNamePrefix+it.Name) +
			" " + m.th.cardLabel.Render("· "+friendlyCategory(it.Category)) + "\n")
	}
	if len(m.syncItems) > limit {
		b.WriteString("  " + m.th.dim.Render(fmt.Sprintf("… and %d more", len(m.syncItems)-limit)) + "\n")
	}
	b.WriteString("\n" + m.th.warn.Render(truncWidth("⚠ Never overwrites — only creates new policies; existing policies are untouched", w)) + "\n")
	b.WriteString(m.th.dim.Render(truncWidth("Conditional access is created disabled · assignments and scope tags are not copied", w)) + "\n\n")
	if m.syncRunning {
		b.WriteString(m.th.inProg.Render(fmt.Sprintf("%s creating… %d/%d", spinnerFrames[m.frame%len(spinnerFrames)], m.syncCur, m.syncTot)) +
			"  " + m.th.cardLabel.Render("x cancel"))
	} else {
		b.WriteString(m.th.success.Render("[ y ] create") + "   " + m.th.dim.Render("[ n ] cancel"))
		if w >= 64 {
			b.WriteString("   " + m.th.inProg.Render("● writes to "+target))
		} else {
			b.WriteString("\n" + m.th.inProg.Render(truncWidth("● writes to "+target, w)))
		}
	}
	return b.String()
}

func (m model) viewSyncResults(w int) string {
	var b strings.Builder
	target := ""
	if m.syncTargetConn >= 0 && m.syncTargetConn < len(m.conns) {
		target = m.conns[m.syncTargetConn].DisplayLabel()
	}
	b.WriteString(m.pageHeading("Sync / Results", "Target: "+target) + "\n\n")
	ok, fail, warned := 0, 0, 0
	for _, r := range m.syncResults {
		if r.Err != nil {
			fail++
		} else {
			ok++
			if r.Warn {
				warned++
			}
		}
	}
	limit := min(len(m.syncResults), m.listRows(12))
	for _, r := range m.syncResults[:limit] {
		if r.Err != nil {
			b.WriteString("  " + m.th.danger.Render("✗") + " " + m.th.normal.Render(r.Item.Name) + "  " +
				m.th.dim.Render(trunc(r.Err.Error(), 50)) + "\n")
		} else {
			line := "  " + m.th.success.Render("✔") + " " + m.th.normal.Render(m.syncNamePrefix+r.Item.Name) +
				m.th.cardLabel.Render("  → "+trunc(r.NewID, 12))
			if r.Warn {
				line += " " + m.th.warn.Render("⚠ partial content")
			}
			b.WriteString(line + "\n")
		}
	}
	if len(m.syncResults) > limit {
		b.WriteString("  " + m.th.dim.Render(fmt.Sprintf("… and %d more results", len(m.syncResults)-limit)) + "\n")
	}
	summary := m.th.accent.Render(fmt.Sprintf("%d created, %d failed", ok, fail))
	if warned > 0 {
		summary += m.th.warn.Render(fmt.Sprintf(" · %d with partial content", warned))
	}
	b.WriteString("\n" + summary)
	if w >= 64 {
		b.WriteString(m.th.dim.Render(" · nothing existing was modified"))
	} else {
		b.WriteString("\n" + m.th.dim.Render("Nothing existing was modified"))
	}
	return b.String()
}
