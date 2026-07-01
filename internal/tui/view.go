package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/ugurkocde/TenuVault-TUI/internal/config"
	"github.com/ugurkocde/TenuVault-TUI/internal/diff"
)

func (m model) width0() int {
	if m.width < 30 {
		return 80
	}
	return m.width
}

func (m model) render() string {
	w := m.width0()
	if m.hits != nil {
		*m.hits = uiHits{}
	}
	var body, hints string

	switch m.screen {
	case screenAuth:
		body, hints = m.viewAuth(w)
	case screenAuthForm:
		body, hints = m.viewAuthForm(w), "enter sign in · tab / ↑↓ move · esc back"
	case screenConnecting:
		body, hints = m.viewConnecting(), "esc cancel"
	case screenDashboard:
		body, hints = m.viewDashboard(w), "b backup · l browse · d compare · r restore · y sync · t tenants · q quit"
	case screenBackupSelect:
		body, hints = m.viewBackupSelect(w), "space toggle · a all · / filter · enter start · esc back"
	case screenProgress:
		body, hints = m.viewProgress(w)
	case screenBrowse:
		body, hints = m.viewBrowse(w)
	case screenBrowseDetail:
		body, hints = m.viewBrowseDetail(w), "↑/↓ move · enter open · r restore · esc back"
	case screenCategoryPolicies:
		body, hints = m.viewCategoryPolicies(w), "↑/↓ move · / filter · enter view JSON · esc back"
	case screenPolicyView:
		body, hints = m.viewPolicyView(w), "↑/↓ scroll · esc back"
	case screenSettings:
		body, hints = m.viewSettings(w), "↑/↓ move · space toggle · ←/→ adjust · esc back"
	case screenConnections:
		body, hints = m.viewConnections(w), "↑/↓ move · enter use · a add · x remove · esc back"
	case screenSyncSource:
		body, hints = m.viewSyncSource(w), "↑/↓ move · enter pick · esc back"
	case screenSyncSelect:
		body, hints = m.viewSyncSelect(w), "space whole type · enter pick policies · n next · esc back"
	case screenSyncPolicies:
		body, hints = m.viewSyncPolicies(w), "space toggle · a all · / filter · esc back"
	case screenSyncTarget:
		body, hints = m.viewSyncTarget(w), "↑/↓ move · enter pick target · esc back"
	case screenSyncNaming:
		body, hints = m.viewSyncNaming(w), "↑/↓ move · enter choose · esc back"
	case screenSyncConfirm:
		body, hints = m.viewSyncConfirm(w), "y create · n cancel"
	case screenSyncResults:
		body, hints = m.viewSyncResults(w), "enter dashboard"
	case screenDiffResult:
		body, hints = m.viewDiffResult(w), "↑/↓ scroll · esc back"
	case screenRestorePick:
		body, hints = m.viewRestorePick(w), "space toggle · a all · / filter · enter review · esc back"
	case screenRestoreConfirm:
		body, hints = m.viewRestoreConfirm(w), "y confirm · n cancel"
	case screenRestoreResult:
		body, hints = m.viewRestoreResult(w), "enter dashboard"
	case screenError:
		body, hints = m.viewError(w), "enter continue"
	}

	if m.hits != nil {
		m.finalizeHits()
	}

	if m.filterActive {
		hints = "type to filter · backspace delete · enter keep · esc clear"
	}

	header := m.header(w)
	footer := m.th.footer.Render(hints)
	if m.showHelp {
		footer = m.th.footer.Render("global: ↑/↓ move · enter select · esc back · q quit · ctrl+c force quit · ? help · mouse: click & scroll")
	}
	return header + "\n" + divider(w) + "\n\n" + body + "\n\n" + m.statusLine() + divider(w) + "\n" + footer
}

// statusLine renders the transient status message above the footer, or "".
func (m model) statusLine() string {
	if m.status == "" {
		return ""
	}
	st := m.th.success
	switch m.statusKind {
	case "warn":
		st = m.th.warn
	case "err":
		st = m.th.danger
	}
	return st.Render(m.status) + "\n"
}

func (m model) header(w int) string {
	left := m.th.logo.Render("⬡ TenuVault") + "  " + m.th.crumb.Render("backup & restore")
	var right string
	if m.connected {
		dot := lipgloss.NewStyle().Foreground(colGreen).Render("●")
		label := m.sourceTenant().DisplayName
		if c := m.sourceConn(); c != nil {
			label = c.DisplayLabel()
		}
		// Tenant names are admin-controlled and unbounded; keep the header row
		// from overflowing narrow terminals.
		maxLabel := w / 3
		if maxLabel < 12 {
			maxLabel = 12
		}
		label = trunc(label, maxLabel)
		suffix := ""
		if len(m.conns) > 1 {
			suffix = m.th.crumb.Render(fmt.Sprintf("  +%d", len(m.conns)-1))
		}
		right = fmt.Sprintf("%s source: %s%s  %s",
			dot,
			m.th.normal.Render(label),
			suffix,
			m.th.crumb.Render(time.Now().Format("15:04")))
	} else {
		right = m.th.crumb.Render("not connected")
	}
	return spread(left, right, w)
}

func (m model) viewAuth(w int) (string, string) {
	var b strings.Builder
	b.WriteString(m.th.title.Render("Connect to Microsoft Graph") + "\n")
	b.WriteString(m.th.dim.Render("Choose how to sign in to your tenant.") + "\n\n")
	for i, o := range m.authOptions {
		cur := "  "
		label := m.th.normal.Render(o.label)
		if i == m.authCursor {
			cur = m.th.selected.Render("▸ ")
			label = m.th.selected.Render(o.label)
		}
		m.recordHit(&b, i)
		b.WriteString(cur + label + "\n")
		b.WriteString("    " + m.th.dim.Render(o.desc) + "\n")
	}
	return b.String(), "↑/↓ move · enter connect · q quit"
}

func (m model) viewConnecting() string {
	sp := m.th.accent.Render(spinnerFrames[m.frame%len(spinnerFrames)])
	var b strings.Builder
	b.WriteString(sp + " " + m.th.title.Render("Signing in…") + "\n\n")
	if m.cfg.AuthMethod == config.AuthInteractive {
		b.WriteString(m.th.dim.Render("Opening your browser to complete sign-in.") + "\n")
	} else {
		b.WriteString(m.th.dim.Render("Authenticating with the app registration…") + "\n")
	}
	return b.String()
}

func (m model) viewDashboard(w int) string {
	cardW := (w-3)/2 - 2
	if cardW < 20 {
		cardW = 20
	}
	tenant := m.th.panel.Width(cardW).Render(strings.Join([]string{
		m.th.panelLbl.Render("Tenant"),
		m.th.normal.Bold(true).Render(emptyDash(m.sourceTenant().DisplayName)),
		m.th.dim.Render(emptyDash(m.sourceTenant().DefaultDomain)),
		m.th.cardLabel.Render(fmt.Sprintf("%d verified domains", m.sourceTenant().DomainCount)),
	}, "\n"))

	var lastLines []string
	lastLines = append(lastLines, lipgloss.NewStyle().Foreground(colGreen).Bold(true).Render("Last backup"))
	if m.lastBackup != nil {
		lastLines = append(lastLines,
			m.th.normal.Bold(true).Render(m.lastBackup.Meta.BackupDate),
			m.th.dim.Render(fmt.Sprintf("%d policies · %d categories", m.lastBackup.Total(), len(m.lastBackup.Meta.ItemCounts))),
			m.th.success.Render("✔ "+emptyDash(m.lastBackup.Meta.Status)),
		)
	} else {
		lastLines = append(lastLines, m.th.dim.Render("none yet"), "", m.th.cardLabel.Render("run a backup to begin"))
	}
	last := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colGreen).Padding(0, 1).Width(cardW).Render(strings.Join(lastLines, "\n"))

	cards := lipgloss.JoinHorizontal(lipgloss.Top, tenant, " ", last)

	menu := []struct{ label, key string }{
		{"Back up now", "b"},
		{"Browse backups", "l"},
		{"Compare / drift", "d"},
		{"Restore policies", "r"},
		{"Sync to another tenant", "y"},
		{"Tenants", "t"},
		{"Settings", "s"},
	}
	var b strings.Builder
	b.WriteString(cards + "\n\n")
	b.WriteString(m.th.cardLabel.Render("Quick actions") + "\n")
	for i, it := range menu {
		marker := m.th.dim.Render("▸")
		label := m.th.normal.Render(it.label)
		if i == m.dashCursor {
			marker = m.th.selected.Render("▸")
			label = m.th.selected.Render(it.label)
		}
		row := marker + " " + label
		m.recordHit(&b, i)
		b.WriteString(spread(row, m.th.key.Render(it.key), w) + "\n")
	}
	return b.String()
}

func (m model) viewBackupSelect(w int) string {
	var b strings.Builder
	visible := m.catsVisible()
	pos := 0
	if len(visible) > 0 {
		pos = filteredPos(m.catCursor, visible) + 1
	}
	b.WriteString(m.th.crumb.Render("Dashboard › Back up now") + "  " +
		m.th.cardLabel.Render(fmt.Sprintf("(%d/%d)", pos, len(visible))) + "\n\n")
	b.WriteString(m.filterLine(len(visible), len(m.cats)))

	selected := 0
	for _, c := range m.cats {
		if c.sel {
			selected++
		}
	}

	if len(visible) == 0 {
		b.WriteString(m.th.dim.Render("No categories match the filter.") + "\n")
	}
	// Window the long list around the cursor; print group headers inline.
	start, end := window(filteredPos(m.catCursor, visible), len(visible), 16)
	lastGroup := ""
	for vp := start; vp < end; vp++ {
		i := visible[vp]
		c := m.cats[i]
		if c.pt.Group != lastGroup {
			if lastGroup != "" {
				b.WriteString("\n")
			}
			b.WriteString(m.th.cardLabel.Render(c.pt.Group) + "\n")
			lastGroup = c.pt.Group
		}
		box := m.th.dim.Render("[ ]")
		if c.sel {
			box = m.th.success.Render("[x]")
		}
		label := m.th.normal.Render(c.pt.Friendly)
		if i == m.catCursor {
			label = m.th.selected.Render(c.pt.Friendly)
		}
		tag := ""
		if !c.pt.RestoreSupported {
			tag = "  " + m.th.cardLabel.Render("· backup-only")
		} else if !c.pt.Verified {
			tag = "  " + m.th.cardLabel.Render("· unverified")
		}
		m.recordHit(&b, i)
		b.WriteString("  " + box + " " + label + tag + "\n")
	}
	if sb := m.scrollbar(start, end, len(visible)); sb != "" {
		b.WriteString("  " + sb + "\n")
	}
	b.WriteString("\n" + m.th.accent.Render(fmt.Sprintf("%d of %d selected", selected, len(m.cats))))
	b.WriteString("  " + m.th.cardLabel.Render("→ "+m.cfg.BackupRoot))
	return b.String()
}

func (m model) viewProgress(w int) (string, string) {
	var b strings.Builder
	b.WriteString(m.th.title.Render(m.progTitle) + "  " + m.th.crumb.Render(m.sourceTenant().DisplayName) + "\n\n")
	for _, cat := range m.progOrder {
		var icon string
		switch m.progEvents[cat] {
		case "done":
			icon = m.th.success.Render("✔")
		case "err":
			icon = m.th.danger.Render("✗")
		default:
			icon = m.th.inProg.Render(spinnerFrames[m.frame%len(spinnerFrames)])
		}
		b.WriteString("  " + icon + " " + m.th.normal.Render(m.progFriendly[cat]) + "\n")
	}
	b.WriteString("\n")
	if !m.progDone {
		frac := 0.0
		if m.progTot > 0 {
			frac = float64(m.progCur) / float64(m.progTot)
		}
		b.WriteString("  " + m.th.inProg.Render(emptyDash(m.progActive)) + "\n")
		b.WriteString("  " + progBar(frac, min(40, w-20)) + fmt.Sprintf("  %d/%d", m.progCur, m.progTot) + "\n")
		return b.String(), "x / esc cancel backup · ctrl+c quit"
	}
	if m.progErr != nil {
		b.WriteString("  " + m.th.danger.Render("Backup failed: "+m.progErr.Error()) + "\n")
		return b.String(), "enter continue"
	}
	switch m.progStatus {
	case "Failed":
		b.WriteString("  " + m.th.danger.Render("✗ Backup failed · "+m.progResult) + "\n")
	case "CompletedWithWarnings":
		b.WriteString("  " + m.th.warn.Render("⚠ Completed with warnings · "+m.progResult) + "\n")
	case "Cancelled":
		b.WriteString("  " + m.th.warn.Render("⚠ Backup cancelled · kept "+m.progResult) + "\n")
	default:
		b.WriteString("  " + m.th.success.Render("✔ Backup complete · "+m.progResult) + "\n")
	}
	for _, c := range m.progCats {
		if c.Failed {
			b.WriteString("    " + m.th.danger.Render("✗ "+c.Category) + " " + m.th.dim.Render(trunc(c.Error, 60)) + "\n")
		} else if c.Warnings > 0 {
			b.WriteString("    " + m.th.warn.Render(fmt.Sprintf("⚠ %s — %d saved, %d incomplete", c.Category, c.Count, c.Warnings)) + "\n")
		}
	}
	return b.String(), "enter continue"
}

func (m model) viewBrowse(w int) (string, string) {
	var b strings.Builder
	b.WriteString(m.th.crumb.Render(m.browseTitle) + "\n")
	if m.diffA != nil && m.browseMode == modeDiffB {
		b.WriteString(m.th.dim.Render("baseline: "+m.diffA.Folder) + "\n")
	}
	b.WriteString("\n")
	if len(m.backups) == 0 {
		b.WriteString(m.th.dim.Render("No backups yet. Run a backup first."))
		return b.String(), "esc back"
	}
	start, end := window(m.browseCursor, len(m.backups), 12)
	for i := start; i < end; i++ {
		bk := m.backups[i]
		marker := "  "
		name := m.th.normal.Render(bk.Folder)
		if i == m.browseCursor {
			marker = m.th.selected.Render("▸ ")
			name = m.th.selected.Render(bk.Folder)
		}
		meta := m.th.dim.Render(fmt.Sprintf("  %d policies · %s", bk.Total(), emptyDash(bk.Meta.Status)))
		m.recordHit(&b, i)
		b.WriteString(marker + name + meta + "\n")
	}
	if sb := m.scrollbar(start, end, len(m.backups)); sb != "" {
		b.WriteString("  " + sb + "\n")
	}
	return b.String(), "↑/↓ move · enter select · esc back"
}

func (m model) viewBrowseDetail(w int) string {
	var b strings.Builder
	if m.detail == nil {
		return ""
	}
	b.WriteString(m.th.crumb.Render("Browse › "+m.detail.Folder) + "\n\n")
	b.WriteString(m.th.dim.Render(fmt.Sprintf("date %s · duration %s · status %s",
		emptyDash(m.detail.Meta.BackupDate), emptyDash(m.detail.Meta.Duration), emptyDash(m.detail.Meta.Status))) + "\n")
	if m.detail.Meta.TenantName != "" {
		b.WriteString(m.th.dim.Render("tenant "+m.detail.Meta.TenantName) + "\n")
	}
	b.WriteString("\n")
	for i, c := range m.detailCats {
		label := m.th.normal.Render(c.name)
		marker := "  "
		if i == m.detailCursor {
			label = m.th.selected.Render(c.name)
			marker = m.th.selected.Render("▸ ")
		}
		m.recordHit(&b, i)
		b.WriteString(spread(marker+label, m.th.accent.Render(fmt.Sprintf("%d", c.count)), w/2) + "\n")
	}
	return b.String()
}

func (m model) viewCategoryPolicies(w int) string {
	var b strings.Builder
	b.WriteString(m.th.crumb.Render("Browse › "+m.catName) + "\n\n")
	if len(m.catPolicies) == 0 {
		b.WriteString(m.th.dim.Render("No policies in this category."))
		return b.String()
	}
	visible := m.catPoliciesVisible()
	b.WriteString(m.filterLine(len(visible), len(m.catPolicies)))
	if len(visible) == 0 {
		b.WriteString(m.th.dim.Render("No policies match the filter."))
		return b.String()
	}
	start, end := window(filteredPos(m.policyCursor, visible), len(visible), 14)
	for vp := start; vp < end; vp++ {
		i := visible[vp]
		p := m.catPolicies[i]
		marker := "  "
		name := m.th.normal.Render(p.Name)
		if i == m.policyCursor {
			marker = m.th.selected.Render("▸ ")
			name = m.th.selected.Render(p.Name)
		}
		m.recordHit(&b, i)
		b.WriteString(marker + name + "\n")
	}
	if sb := m.scrollbar(start, end, len(visible)); sb != "" {
		b.WriteString("  " + sb + "\n")
	}
	return b.String()
}

func (m model) viewPolicyView(w int) string {
	var b strings.Builder
	name := ""
	if len(m.catPolicies) > 0 && m.policyCursor < len(m.catPolicies) {
		name = m.catPolicies[m.policyCursor].Name
	}
	b.WriteString(m.th.crumb.Render(m.catName+" › "+name) + "\n\n")
	if len(m.policyLines) == 0 {
		b.WriteString(m.th.dim.Render("(empty)"))
		return b.String()
	}
	size := 18
	start, end := m.policyScroll, m.policyScroll+size
	if end > len(m.policyLines) {
		end = len(m.policyLines)
	}
	for i := start; i < end; i++ {
		b.WriteString(m.th.dim.Render(m.policyLines[i]) + "\n")
	}
	b.WriteString("\n" + m.th.cardLabel.Render(fmt.Sprintf("lines %d–%d of %d", start+1, end, len(m.policyLines))))
	return b.String()
}

func (m model) viewSettings(w int) string {
	var b strings.Builder
	b.WriteString(m.th.crumb.Render("Settings") + "\n\n")
	rows := []struct {
		label, value string
	}{
		{"Include assignments in backups", onOff(m.cfg.IncludeAssignments)},
		{"Retention (days, 0 = keep all)", fmt.Sprintf("%d", m.cfg.RetentionDays)},
	}
	for i, r := range rows {
		marker := "  "
		label := m.th.normal.Render(r.label)
		if i == m.settingsCursor {
			marker = m.th.selected.Render("▸ ")
			label = m.th.selected.Render(r.label)
		}
		m.recordHit(&b, i)
		b.WriteString(spread(marker+label, m.th.accent.Render(r.value), min(w, 70)) + "\n")
	}
	b.WriteString("\n" + m.th.cardLabel.Render("backups → "+m.cfg.BackupRoot))
	b.WriteString("\n" + m.th.cardLabel.Render("config  → "+config.Path()))
	return b.String()
}

func onOff(b bool) string {
	if b {
		return "on"
	}
	return "off"
}

func (m model) viewDiffResult(w int) string {
	var b strings.Builder
	b.WriteString(m.th.crumb.Render("Compare") + "\n")
	if m.diffA != nil && m.diffB != nil {
		older, newer := orderBackups(m.diffA, m.diffB)
		b.WriteString(m.th.dim.Render(older.Folder+"  →  "+newer.Folder) + "\n\n")
	}
	if len(m.diffChanges) == 0 {
		b.WriteString(m.th.success.Render("No drift. The two backups are identical."))
		return b.String()
	}
	b.WriteString(m.th.normal.Render(fmt.Sprintf("%d changes", len(m.diffChanges))) + "\n\n")
	start, end := window(m.diffScroll, len(m.diffChanges), 12)
	for i := start; i < end; i++ {
		c := m.diffChanges[i]
		b.WriteString("  " + sevStyle(m.th, c.Severity).Render(sevBullet(c.Type)) + " " +
			m.th.dim.Render(string(c.Type)) + " " +
			m.th.cardLabel.Render(c.Category+" /") + " " +
			m.th.normal.Render(c.Name) + "\n")
	}
	return b.String()
}

func (m model) viewRestorePick(w int) string {
	var b strings.Builder
	title := "Restore"
	if m.restoreBackup != nil {
		title = "Restore · " + m.restoreBackup.Folder
	}
	b.WriteString(m.th.crumb.Render(title) + "\n\n")
	if len(m.restoreItems) == 0 {
		b.WriteString(m.th.dim.Render("This backup has no policies."))
		return b.String()
	}
	selected := 0
	for _, r := range m.restoreItems {
		if r.sel {
			selected++
		}
	}
	visible := m.restoreVisible()
	b.WriteString(m.filterLine(len(visible), len(m.restoreItems)))
	if len(visible) == 0 {
		b.WriteString(m.th.dim.Render("No policies match the filter."))
		return b.String()
	}
	start, end := window(filteredPos(m.restoreCursor, visible), len(visible), 12)
	for vp := start; vp < end; vp++ {
		i := visible[vp]
		r := m.restoreItems[i]
		box := m.th.dim.Render("[ ]")
		if r.sel {
			box = m.th.success.Render("[x]")
		}
		name := m.th.normal.Render(r.item.Name)
		if i == m.restoreCursor {
			name = m.th.selected.Render(r.item.Name)
		}
		m.recordHit(&b, i)
		b.WriteString(box + " " + m.th.cardLabel.Render(r.item.Category) + "  " + name + "\n")
	}
	if sb := m.scrollbar(start, end, len(visible)); sb != "" {
		b.WriteString("  " + sb + "\n")
	}
	b.WriteString("\n" + m.th.accent.Render(fmt.Sprintf("%d of %d selected", selected, len(m.restoreItems))))
	return b.String()
}

func (m model) viewRestoreConfirm(w int) string {
	var b strings.Builder
	b.WriteString(m.th.crumb.Render("Restore · confirm") + "\n\n")
	items := m.selectedRestoreItems()
	b.WriteString(m.th.dim.Render(fmt.Sprintf("These %d policies will be ", len(items))) +
		m.th.success.Render("created") + m.th.dim.Render(" in ") + m.th.normal.Render(m.sourceTenant().DisplayName) + "\n\n")
	hasCA := false
	limit := len(items)
	if limit > 10 {
		limit = 10
	}
	for i := 0; i < limit; i++ {
		it := items[i]
		if it.Category == "ConditionalAccessPolicies" {
			hasCA = true
		}
		b.WriteString("  " + m.th.success.Render("+") + " " + m.th.normal.Render("[Restored] "+it.Name) +
			" " + m.th.cardLabel.Render("· "+it.Category) + "\n")
	}
	if len(items) > limit {
		b.WriteString("  " + m.th.dim.Render(fmt.Sprintf("… and %d more", len(items)-limit)) + "\n")
	}
	if hasCA {
		b.WriteString("\n" + m.th.warn.Render("⚠ Conditional access policies are restored disabled by default") + "\n")
	}
	b.WriteString("\n")
	if m.restoreRunning {
		b.WriteString(m.th.inProg.Render(spinnerFrames[m.frame%len(spinnerFrames)]+" Restoring…") + "  " +
			m.th.cardLabel.Render("x cancel") + "\n")
	} else {
		b.WriteString(lipgloss.NewStyle().Foreground(colGreen).Render("[ y ] confirm restore") + "   " +
			m.th.dim.Render("[ n ] cancel") + "\n")
		b.WriteString(m.th.inProg.Render("● writes to Microsoft Graph") + "\n")
	}
	return b.String()
}

func (m model) viewRestoreResult(w int) string {
	var b strings.Builder
	b.WriteString(m.th.crumb.Render("Restore · results") + "\n\n")
	ok, fail := 0, 0
	for _, r := range m.restoreResults {
		if r.Err != nil {
			fail++
			b.WriteString("  " + m.th.danger.Render("✗") + " " + m.th.normal.Render(r.Item.Name) + "  " +
				m.th.dim.Render(trunc(r.Err.Error(), 50)) + "\n")
		} else {
			ok++
			b.WriteString("  " + m.th.success.Render("✔") + " " + m.th.normal.Render("[Restored] "+r.Item.Name) + "\n")
		}
	}
	b.WriteString("\n" + m.th.accent.Render(fmt.Sprintf("%d succeeded, %d failed", ok, fail)))
	return b.String()
}

func (m model) viewError(w int) string {
	var b strings.Builder
	b.WriteString(m.th.danger.Bold(true).Render("Something went wrong") + "\n\n")
	if m.err != nil {
		b.WriteString(lipgloss.NewStyle().Foreground(colText).Width(min(w, 100)).Render(m.err.Error()))
	}
	return b.String()
}

// --- helpers ---

func spread(left, right string, width int) string {
	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + right
}

func divider(width int) string {
	return lipgloss.NewStyle().Foreground(colLine).Render(strings.Repeat("─", width))
}

func progBar(frac float64, width int) string {
	if width < 4 {
		width = 4
	}
	if frac < 0 {
		frac = 0
	}
	if frac > 1 {
		frac = 1
	}
	filled := int(frac * float64(width))
	bar := lipgloss.NewStyle().Foreground(colPurple).Render(strings.Repeat("█", filled)) +
		lipgloss.NewStyle().Foreground(colLine).Render(strings.Repeat("░", width-filled))
	return bar + "  " + m0pct(frac)
}

func m0pct(frac float64) string {
	return lipgloss.NewStyle().Foreground(colPurple).Bold(true).Render(fmt.Sprintf("%d%%", int(frac*100)))
}

// scrollbar renders a compact position indicator for a windowed list, with
// arrows that light up when there is more above or below.
func (m model) scrollbar(start, end, total int) string {
	if total == 0 || (start == 0 && end >= total) {
		return ""
	}
	up := m.th.cardLabel.Render("↑")
	if start > 0 {
		up = m.th.accent.Render("↑")
	}
	down := m.th.cardLabel.Render("↓")
	if end < total {
		down = m.th.accent.Render("↓")
	}
	return m.th.cardLabel.Render(fmt.Sprintf("%d–%d of %d ", start+1, end, total)) + up + down
}

func window(cursor, total, size int) (int, int) {
	if total <= size {
		return 0, total
	}
	start := cursor - size/2
	if start < 0 {
		start = 0
	}
	if start+size > total {
		start = total - size
	}
	return start, start + size
}

func sevBullet(t diff.ChangeType) string {
	switch t {
	case diff.Added:
		return "+"
	case diff.Removed:
		return "−"
	default:
		return "~"
	}
}

func sevStyle(th theme, s diff.Severity) lipgloss.Style {
	switch s {
	case diff.Critical:
		return th.danger
	case diff.Warning:
		return th.warn
	default:
		return th.success
	}
}

func emptyDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

func trunc(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
