package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/ugurkocde/TenuVault-TUI/internal/config"
	"github.com/ugurkocde/TenuVault-TUI/internal/diff"
)

func (m model) width0() int {
	if m.width == 0 {
		return 80 // no WindowSizeMsg yet
	}
	if m.width < 1 {
		return 1
	}
	return m.width
}

// listRows bounds long views to the terminal height. The fixed allowance covers
// the shared header/footer, page heading, status line, and surrounding panels.
func (m model) listRows(preferred int) int {
	if m.height <= 0 {
		return preferred
	}
	available := m.height - 18
	if available < 2 {
		return 2
	}
	return min(preferred, available)
}

func (m model) render() string {
	if m.hits != nil {
		*m.hits = uiHits{}
	}
	if m.tooSmall() {
		return m.viewTooSmall()
	}
	terminalW := m.width0()
	w := m.contentWidth()
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
		body, hints = m.viewBrowseDetail(w), "↑/↓ move · enter open · r restore · v verify · esc back"
	case screenCategoryPolicies:
		body, hints = m.viewCategoryPolicies(w), "↑/↓ move · / filter · s sort · enter view JSON · esc back"
	case screenPolicyView:
		body, hints = m.viewPolicyView(w), "↑/↓ scroll · esc back"
	case screenSettings:
		body, hints = m.viewSettings(w), "↑/↓ move · enter select · ←/→ adjust · esc back"
	case screenConnections:
		body, hints = m.viewConnections(w), "↑/↓ move · enter use · a add · x remove · esc back"
		if m.connRemove >= 0 {
			hints = "y remove · n cancel"
		}
	case screenSyncSource:
		body, hints = m.viewSyncSource(w), "↑/↓ move · enter pick · esc back"
	case screenSyncSelect:
		body, hints = m.viewSyncSelect(w), "space whole type · enter pick policies · n next · esc back"
	case screenSyncPolicies:
		body, hints = m.viewSyncPolicies(w), "space toggle · a all · / filter · s sort · esc back"
	case screenSyncTarget:
		body, hints = m.viewSyncTarget(w), "↑/↓ move · enter pick target · esc back"
	case screenSyncNaming:
		body, hints = m.viewSyncNaming(w), "↑/↓ move · enter choose · esc back"
	case screenSyncConfirm:
		body, hints = m.viewSyncConfirm(w), "y create · n cancel"
	case screenSyncResults:
		body, hints = m.viewSyncResults(w), "enter dashboard · e export"
	case screenDiffResult:
		body, hints = m.viewDiffResult(w), "↑/↓ scroll · e export · esc back"
	case screenRestorePick:
		body, hints = m.viewRestorePick(w), "space toggle · a all · / filter · s sort · enter review · esc back"
	case screenRestoreConfirm:
		body, hints = m.viewRestoreConfirm(w), "y confirm · n cancel"
	case screenRestoreResult:
		body, hints = m.viewRestoreResult(w), "enter dashboard · e export"
	case screenError:
		body, hints = m.viewError(w), "enter continue"
	}

	if m.filterActive {
		hints = "type to filter · backspace delete · enter keep · esc clear"
	}
	if m.screen == screenSettings && m.settingsEditing {
		hints = "type path · enter save · esc cancel"
	}
	if m.showHelp {
		*m.hits = uiHits{}
		body = m.helpView(w, hints)
		hints = "? close help · esc back · ctrl+c force quit"
	} else if !m.filterActive && !m.settingsEditing && m.screen != screenAuthForm {
		hints += " · ? help"
	}

	header := m.header(w)
	footer := m.renderFooter(hints, w)
	content := header + "\n" + divider(w) + "\n\n" + body + "\n\n" + m.statusLine() + divider(w) + "\n" + footer
	if m.hits != nil {
		xOffset := 0
		if terminalW > w {
			xOffset = (terminalW - w) / 2
		}
		m.finalizeHits(strings.Count(content, "\n"), xOffset)
	}
	if terminalW > w {
		return lipgloss.NewStyle().Width(terminalW).Align(lipgloss.Center).Render(content)
	}
	return content
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
	left := m.th.logo.Render("⬡ TenuVault")
	if w >= 72 {
		left += "  " + m.th.crumb.Render("Intune backup & recovery")
	}
	var right string
	if m.connected {
		dot := lipgloss.NewStyle().Foreground(colGreen).Render("●")
		label := m.sourceTenant().DisplayName
		if c := m.sourceConn(); c != nil {
			label = c.DisplayLabel()
		}
		// Tenant names are admin-controlled and unbounded; keep the header row
		// from overflowing narrow terminals.
		maxLabel := w / 4
		if maxLabel < 12 {
			maxLabel = 12
		}
		label = trunc(label, maxLabel)
		suffix := ""
		if len(m.conns) > 1 {
			suffix = m.th.crumb.Render(fmt.Sprintf("  +%d", len(m.conns)-1))
		}
		if w >= 72 {
			right = fmt.Sprintf("%s %s  %s%s", dot, m.th.success.Render("Connected"), m.th.normal.Render(label), suffix)
		} else {
			right = dot + " " + m.th.success.Render("Connected")
		}
	} else {
		right = m.th.crumb.Render("not connected")
	}
	return spread(left, right, w)
}

func (m model) viewAuth(w int) (string, string) {
	var b strings.Builder
	b.WriteString(m.pageHeading("Connect to Microsoft Graph", "Choose how to sign in to your tenant.") + "\n\n")
	for i, o := range m.authOptions {
		m.recordHit(&b, i)
		b.WriteString(m.actionRow(o.label, m.keyBadge(fmt.Sprintf("%d", i+1)), w, i == m.authCursor) + "\n")
		if !m.isShort() {
			b.WriteString("    " + m.th.dim.Render(truncWidth(o.desc, w-4)) + "\n")
		}
	}
	return b.String(), "↑/↓ move · enter connect · q quit"
}

func (m model) viewConnecting() string {
	sp := m.th.accent.Render(spinnerFrames[m.frame%len(spinnerFrames)])
	var b strings.Builder
	b.WriteString(m.pageHeading("Signing in…", "Connecting securely to Microsoft Graph.") + "\n\n")
	if m.cfg.AuthMethod == config.AuthInteractive {
		b.WriteString(sp + " " + m.th.normal.Render("Opening your browser to complete sign-in.") + "\n")
	} else {
		b.WriteString(sp + " " + m.th.normal.Render("Authenticating with the app registration…") + "\n")
	}
	return b.String()
}

func (m model) viewDashboard(w int) string {
	var b strings.Builder
	b.WriteString(m.pageHeading("Dashboard", "Your tenant is protected and ready.") + "\n\n")

	tenantLines := []string{
		m.th.normal.Bold(true).Render(emptyDash(m.sourceTenant().DisplayName)),
		m.th.dim.Render(emptyDash(m.sourceTenant().DefaultDomain)),
		m.th.cardLabel.Render(fmt.Sprintf("%d verified domains", m.sourceTenant().DomainCount)),
	}
	var lastLines []string
	if m.lastBackup != nil {
		lastLines = append(lastLines,
			m.th.normal.Bold(true).Render(formatBackupDate(m.lastBackup.Meta.BackupDate)),
			m.th.dim.Render(fmt.Sprintf("%d policies · %d %s", m.lastBackup.Total(), len(m.lastBackup.Meta.ItemCounts), plural(len(m.lastBackup.Meta.ItemCounts), "category", "categories"))),
			m.th.success.Render("✔ "+friendlyBackupStatus(m.lastBackup.Meta.Status)),
		)
	} else {
		lastLines = append(lastLines, m.th.dim.Render("No backups yet"), m.th.cardLabel.Render("Run a backup to begin"), "")
	}
	if m.isWide(w) && !m.isShort() {
		cardW := (w - 1) / 2
		tenant := m.infoPanel("Tenant", tenantLines, cardW-2, false)
		last := m.infoPanel("Latest backup", lastLines, cardW-2, true)
		b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, tenant, " ", last) + "\n\n")
	} else {
		b.WriteString(m.sectionLabel("Tenant") + "  " + tenantLines[0] + "\n")
		b.WriteString(m.sectionLabel("Latest backup") + "  " + lastLines[0])
		if len(lastLines) > 1 && w >= 72 {
			b.WriteString("  " + lastLines[1])
		}
		b.WriteString("\n\n")
	}

	menu := []struct{ label, key string }{
		{"Back up now", "b"},
		{"Browse backups", "l"},
		{"Compare / drift", "d"},
		{"Restore policies", "r"},
		{"Sync to another tenant", "y"},
		{"Tenants", "t"},
		{"Settings", "s"},
	}
	b.WriteString(m.sectionLabel("Quick actions") + "\n")
	for i, it := range menu {
		m.recordHit(&b, i)
		b.WriteString(m.actionRow(it.label, m.keyBadge(it.key), w, i == m.dashCursor) + "\n")
	}
	return b.String()
}

func (m model) viewBackupSelect(w int) string {
	var b strings.Builder
	visible := m.catsVisible()

	selected := 0
	for _, c := range m.cats {
		if c.sel {
			selected++
		}
	}
	b.WriteString(m.pageHeading("Dashboard / Back up now", "Choose the Intune configuration you want to protect.") + "\n")
	b.WriteString(m.th.accent.Render(fmt.Sprintf("%d of %d selected", selected, len(m.cats))) + "\n")
	if m.filterActive || m.filterQuery != "" {
		b.WriteString(m.filterLine(len(visible), len(m.cats)))
	} else if m.isShort() {
		m.recordBodyAction(strings.Count(b.String(), "\n"), 1, w, "/")
		b.WriteString(m.th.cardLabel.Render("/  Filter categories") + "\n")
	} else {
		filter := m.th.panel.Width(w - 2).Render(m.th.cardLabel.Render("/  Filter categories"))
		m.recordBodyAction(strings.Count(b.String(), "\n"), len(strings.Split(filter, "\n")), w, "/")
		b.WriteString(filter + "\n")
	}

	// Window the long list around the cursor; print group headers inline.
	start, end := window(filteredPos(m.catCursor, visible), len(visible), m.listRows(12))
	lastGroup := ""
	for vp := start; vp < end; vp++ {
		i := visible[vp]
		c := m.cats[i]
		if c.pt.Group != lastGroup {
			if lastGroup != "" {
				b.WriteString("\n")
			}
			b.WriteString(m.sectionLabel(c.pt.Group) + "\n")
			lastGroup = c.pt.Group
		}
		tag := ""
		if !c.pt.RestoreSupported {
			tag = "backup-only"
		} else if !c.pt.Verified {
			tag = "unverified"
		} else {
			tag = "verified"
		}
		m.recordHit(&b, i)
		b.WriteString(m.checkRow(c.sel, c.pt.Friendly, tag, w, i == m.catCursor) + "\n")
	}
	if sb := m.scrollbar(start, end, len(visible)); sb != "" {
		b.WriteString("  " + sb + "\n")
	}
	b.WriteString("\n" + m.th.cardLabel.Render("Destination  ") + m.th.accent.Render(compactPath(m.cfg.BackupRoot)))
	return b.String()
}

func (m model) viewProgress(w int) (string, string) {
	var b strings.Builder
	b.WriteString(m.pageHeading(m.progTitle, "Tenant: "+m.sourceTenant().DisplayName) + "\n\n")
	b.WriteString(m.sectionLabel("Progress") + "\n")
	limit := min(len(m.progOrder), m.listRows(12))
	start := max(0, len(m.progOrder)-limit)
	if start > 0 {
		b.WriteString(m.th.cardLabel.Render(fmt.Sprintf("… %d earlier categories", start)) + "\n")
	}
	for _, cat := range m.progOrder[start:] {
		var icon string
		switch m.progEvents[cat] {
		case "done":
			icon = m.th.success.Render("✔")
		case "err":
			icon = m.th.danger.Render("✗")
		default:
			icon = m.th.inProg.Render(spinnerFrames[m.frame%len(spinnerFrames)])
		}
		b.WriteString("  " + icon + "  " + m.th.normal.Render(m.progFriendly[cat]) + "\n")
	}
	b.WriteString("\n")
	if !m.progDone {
		frac := 0.0
		if m.progTot > 0 {
			frac = float64(m.progCur) / float64(m.progTot)
		}
		b.WriteString(m.th.inProg.Render(emptyDash(m.progActive)) + "\n")
		b.WriteString(progBar(frac, min(56, w-12)) + fmt.Sprintf("  %d/%d", m.progCur, m.progTot) + "\n")
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
			b.WriteString("    " + m.th.danger.Render("✗ "+friendlyCategory(c.Category)) + " " + m.th.dim.Render(trunc(c.Error, 60)) + "\n")
		} else if c.Warnings > 0 {
			b.WriteString("    " + m.th.warn.Render(fmt.Sprintf("⚠ %s — %d saved, %d incomplete", friendlyCategory(c.Category), c.Count, c.Warnings)) + "\n")
		}
	}
	return b.String(), "enter continue"
}

func (m model) viewBrowse(w int) (string, string) {
	var b strings.Builder
	title := m.browseTitle
	if title == "" {
		title = "Browse backups"
	}
	b.WriteString(m.pageHeading("Dashboard / "+title, "Review, verify, or restore a saved tenant snapshot.") + m.sortTag() + "\n")
	if m.diffA != nil && m.browseMode == modeDiffB {
		b.WriteString(m.th.dim.Render("baseline: "+m.diffA.Folder) + "\n")
	}
	b.WriteString("\n")
	if m.browseLoading {
		b.WriteString(m.th.inProg.Render(spinnerFrames[m.frame%len(spinnerFrames)] + " scanning backups…"))
		return b.String(), "esc back"
	}
	if m.diffLoading {
		b.WriteString(m.th.inProg.Render(spinnerFrames[m.frame%len(spinnerFrames)] + " comparing backups…"))
		return b.String(), "esc continue in background"
	}
	if len(m.backups) == 0 {
		b.WriteString(m.th.dim.Render("No backups yet. Run a backup first."))
		return b.String(), "esc back"
	}
	visible := m.browseVisible()
	start, end := window(filteredPos(m.browseCursor, visible), len(visible), m.listRows(10))
	if m.isWide(w) {
		leftW := w / 2
		rightW := w - leftW - 1
		var list strings.Builder
		list.WriteString(m.sectionLabel(fmt.Sprintf("Backups  %d", len(visible))) + "\n")
		base := strings.Count(b.String(), "\n") + 1 // bordered panel top edge
		for vp := start; vp < end; vp++ {
			i := visible[vp]
			bk := m.backups[i]
			m.recordHitAt(base+strings.Count(list.String(), "\n"), i)
			label := formatBackupDate(bk.Meta.BackupDate)
			right := fmt.Sprintf("%d · %s", bk.Total(), shortBackupStatus(bk.Meta.Status))
			// The panel consumes four columns for its border and padding. Keep
			// one additional column for actionRow's leading focus indicator.
			list.WriteString(m.actionRow(label, right, leftW-5, i == m.browseCursor) + "\n")
		}
		if sb := m.scrollbar(start, end, len(visible)); sb != "" {
			list.WriteString(" " + sb)
		}
		left := m.th.panel.Width(leftW - 2).Render(list.String())
		selected := m.backups[m.browseCursor]
		details := []string{
			m.th.normal.Bold(true).Render(emptyDash(selected.Meta.TenantName)),
			m.th.normal.Render(formatBackupDate(selected.Meta.BackupDate)),
			m.th.cardLabel.Render(selected.Folder),
			m.th.dim.Render("Duration    " + emptyDash(selected.Meta.Duration)),
			m.th.dim.Render(fmt.Sprintf("Policies    %d", selected.Total())),
			m.th.dim.Render(fmt.Sprintf("Categories  %d", len(selected.Meta.ItemCounts))),
			m.th.success.Render("✔ " + friendlyBackupStatus(selected.Meta.Status)),
			m.th.cardLabel.Render("Press V to verify integrity"),
		}
		right := m.infoPanel("Backup details", details, rightW-2, false)
		b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, left, " ", right))
	} else {
		b.WriteString(m.sectionLabel(fmt.Sprintf("Backups  %d", len(visible))) + "\n")
		for vp := start; vp < end; vp++ {
			i := visible[vp]
			bk := m.backups[i]
			m.recordHit(&b, i)
			b.WriteString(m.actionRow(formatBackupDate(bk.Meta.BackupDate), fmt.Sprintf("%d · %s", bk.Total(), shortBackupStatus(bk.Meta.Status)), w, i == m.browseCursor) + "\n")
		}
	}
	return b.String(), "↑/↓ move · s sort · enter select · esc back"
}

func (m model) viewBrowseDetail(w int) string {
	var b strings.Builder
	if m.detail == nil {
		return ""
	}
	b.WriteString(m.pageHeading("Browse / Backup details", formatBackupDate(m.detail.Meta.BackupDate)) + "\n\n")
	meta := []string{
		m.th.normal.Bold(true).Render(emptyDash(m.detail.Meta.TenantName)),
		m.th.dim.Render("Folder      " + m.detail.Folder),
		m.th.dim.Render("Duration    " + emptyDash(m.detail.Meta.Duration)),
		m.th.dim.Render(fmt.Sprintf("Policies    %d", m.detail.Total())),
		m.th.success.Render("✔ " + friendlyBackupStatus(m.detail.Meta.Status)),
	}
	if m.verifyRunning {
		meta = append(meta, m.th.inProg.Render(spinnerFrames[m.frame%len(spinnerFrames)]+" verifying integrity…"))
	}
	b.WriteString(m.infoPanel("Backup details", meta, min(w-2, 64), false) + "\n\n")
	b.WriteString(m.sectionLabel("Categories") + "\n")
	start, end := window(m.detailCursor, len(m.detailCats), m.listRows(12))
	for i := start; i < end; i++ {
		c := m.detailCats[i]
		m.recordHit(&b, i)
		b.WriteString(m.actionRow(friendlyCategory(c.name), fmt.Sprintf("%d policies", c.count), min(w, 72), i == m.detailCursor) + "\n")
	}
	if sb := m.scrollbar(start, end, len(m.detailCats)); sb != "" {
		b.WriteString(" " + sb + "\n")
	}
	return b.String()
}

func (m model) viewCategoryPolicies(w int) string {
	var b strings.Builder
	b.WriteString(m.pageHeading("Browse / "+m.catName, "Open a policy to inspect its captured JSON.") + m.sortTag() + "\n\n")
	if len(m.catPolicies) == 0 {
		b.WriteString(m.th.dim.Render("No policies in this category."))
		return b.String()
	}
	visible := m.catPoliciesVisible()
	if m.filterActive || m.filterQuery != "" {
		b.WriteString(m.filterLine(len(visible), len(m.catPolicies)))
	} else {
		m.recordBodyAction(strings.Count(b.String(), "\n"), 1, w, "/")
		b.WriteString(m.th.cardLabel.Render("/  Filter policies") + "\n")
	}
	if len(visible) == 0 {
		return b.String()
	}
	start, end := window(filteredPos(m.policyCursor, visible), len(visible), m.listRows(14))
	for vp := start; vp < end; vp++ {
		i := visible[vp]
		p := m.catPolicies[i]
		m.recordHit(&b, i)
		b.WriteString(m.actionRow(p.Name, "", w, i == m.policyCursor) + "\n")
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
	b.WriteString(m.pageHeading(m.catName+" / "+name, "Captured policy JSON · read only") + "\n\n")
	if len(m.policyLines) == 0 {
		b.WriteString(m.th.dim.Render("(empty)"))
		return b.String()
	}
	size := m.listRows(18)
	start, end := m.policyScroll, m.policyScroll+size
	if end > len(m.policyLines) {
		end = len(m.policyLines)
	}
	lineW := w - 2
	for i := start; i < end; i++ {
		b.WriteString(m.renderJSONLine(m.policyLines[i], lineW) + "\n")
	}
	b.WriteString("\n" + m.th.cardLabel.Render(fmt.Sprintf("lines %d–%d of %d", start+1, end, len(m.policyLines))))
	return b.String()
}

func (m model) viewSettings(w int) string {
	var b strings.Builder
	b.WriteString(m.pageHeading("Dashboard / Settings", "Configure backup behavior and local storage.") + "\n\n")
	var panel strings.Builder
	panel.WriteString(m.sectionLabel("Backups") + "\n")
	locVal := compactPath(m.cfg.BackupRoot)
	switch {
	case m.settingsPicking:
		locVal = "Choosing folder…"
	case m.settingsEditing:
		locVal = m.settingsInput + "▌"
	}
	settings := []struct {
		label, value string
	}{
		{"Include assignments in backups", onOff(m.cfg.IncludeAssignments)},
		{"Retention (days, 0 = keep all)", fmt.Sprintf("%d", m.cfg.RetentionDays)},
		{"Backup location", locVal},
	}
	for i, r := range settings {
		m.recordHitAt(strings.Count(b.String(), "\n")+1+strings.Count(panel.String(), "\n"), i)
		panel.WriteString(m.actionRow(r.label, r.value, min(w-4, 84), i == m.settingsCursor) + "\n")
	}
	if !m.isShort() {
		panel.WriteString("\n" + m.th.cardLabel.Render("Config  ") + m.th.dim.Render(compactPath(config.Path())))
		panel.WriteString("\n\n" + m.th.success.Render("✔ Settings are saved automatically"))
	}
	b.WriteString(m.th.panel.Width(min(w-2, 88)).Render(panel.String()))
	return b.String()
}

// renderJSONLine renders one line of captured policy JSON into a fixed-width
// cell. Uniform width matters: the outer layout centers every line, so ragged
// lines would each be re-centered individually and the indentation lost.
// Object keys get the accent tint to keep the structure scannable.
func (m model) renderJSONLine(line string, w int) string {
	line = truncWidth(line, w)
	styled := m.th.dim.Render(line)
	if q := strings.Index(line, `":`); q > 0 {
		if o := strings.Index(line, `"`); o >= 0 && o < q {
			styled = m.th.dim.Render(line[:o]) + m.th.accent.Render(line[o:q+1]) + m.th.normal.Render(line[q+1:])
		}
	}
	if pad := w - lipgloss.Width(styled); pad > 0 {
		styled += strings.Repeat(" ", pad)
	}
	return styled
}

func onOff(b bool) string {
	if b {
		return "On"
	}
	return "Off"
}

func (m model) viewDiffResult(w int) string {
	var b strings.Builder
	b.WriteString(m.pageHeading("Dashboard / Compare / Drift report", "Configuration changes between two tenant snapshots.") + "\n")
	if m.diffA != nil && m.diffB != nil {
		older, newer := orderBackups(m.diffA, m.diffB)
		b.WriteString(m.th.dim.Render(formatBackupDate(older.Meta.BackupDate)+"  →  "+formatBackupDate(newer.Meta.BackupDate)) + "\n\n")
	}
	if len(m.diffChanges) == 0 {
		b.WriteString(m.th.success.Render("No drift. The two backups are identical."))
		return b.String()
	}
	added, removed, changed := 0, 0, 0
	for _, c := range m.diffChanges {
		switch c.Type {
		case diff.Added:
			added++
		case diff.Removed:
			removed++
		default:
			changed++
		}
	}
	if m.isWide(w) {
		cardW := (w - 2) / 3
		c1 := m.th.panelWarn.Width(cardW - 2).Render(m.th.warn.Bold(true).Render(fmt.Sprintf("~  %d CHANGED", changed)))
		c2 := m.th.panelGood.Width(cardW - 2).Render(m.th.success.Bold(true).Render(fmt.Sprintf("+  %d ADDED", added)))
		c3 := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colRed).Padding(0, 1).Width(cardW - 2).Render(m.th.danger.Bold(true).Render(fmt.Sprintf("−  %d REMOVED", removed)))
		b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, c1, " ", c2, " ", c3) + "\n\n")
	} else {
		b.WriteString(m.th.warn.Render(fmt.Sprintf("%d changed", changed)) + "   " + m.th.success.Render(fmt.Sprintf("%d added", added)) + "   " + m.th.danger.Render(fmt.Sprintf("%d removed", removed)) + "\n\n")
	}
	b.WriteString(m.sectionLabel(fmt.Sprintf("%d configuration changes", len(m.diffChanges))) + "\n")
	start, end := window(m.diffScroll, len(m.diffChanges), m.listRows(12))
	for i := start; i < end; i++ {
		c := m.diffChanges[i]
		kind := strings.ToUpper(string(c.Type[:1])) + string(c.Type[1:])
		label := fmt.Sprintf("%s  %s / %s", kind, friendlyCategory(c.Category), c.Name)
		b.WriteString(" " + sevStyle(m.th, c.Severity).Render(sevBullet(c.Type)) + " " + m.th.normal.Render(truncWidth(label, w-6)) + "\n")
	}
	b.WriteString("\n" + m.th.success.Render(truncWidth("ⓘ Create-only restore available; existing tenant policies will not be overwritten.", w)))
	return b.String()
}

func (m model) viewRestorePick(w int) string {
	var b strings.Builder
	title := "Dashboard / Restore policies"
	if m.syncMode {
		title = "Sync / Pick policies"
	}
	subtitle := "Choose policies that will be created in " + emptyDash(m.sourceTenant().DisplayName) + "."
	crumb := m.pageHeading(title, subtitle)
	if m.syncMode {
		crumb += "  " + m.th.cardLabel.Render("step 3 of 6")
	}
	b.WriteString(crumb + m.sortTag() + "\n\n")
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
	if m.filterActive || m.filterQuery != "" {
		b.WriteString(m.filterLine(len(visible), len(m.restoreItems)))
	} else {
		m.recordBodyAction(strings.Count(b.String(), "\n"), 1, w, "/")
		b.WriteString(m.th.cardLabel.Render("/  Filter policies") + "\n")
	}
	if len(visible) == 0 {
		return b.String()
	}
	start, end := window(filteredPos(m.restoreCursor, visible), len(visible), m.listRows(12))
	for vp := start; vp < end; vp++ {
		i := visible[vp]
		r := m.restoreItems[i]
		m.recordHit(&b, i)
		b.WriteString(m.checkRow(r.sel, r.item.Name, friendlyCategory(r.item.Category), w, i == m.restoreCursor) + "\n")
	}
	if sb := m.scrollbar(start, end, len(visible)); sb != "" {
		b.WriteString("  " + sb + "\n")
	}
	b.WriteString("\n" + m.th.accent.Render(fmt.Sprintf("%d of %d selected", selected, len(m.restoreItems))))
	if w >= 72 {
		b.WriteString("  " + m.th.success.Render("Creates new policies only") + "  " + m.th.cardLabel.Render("Existing policies stay untouched"))
	} else {
		b.WriteString("  " + m.th.success.Render("Create-only") + "\n" + m.th.cardLabel.Render("Existing policies stay untouched"))
	}
	return b.String()
}

func (m model) viewRestoreConfirm(w int) string {
	var b strings.Builder
	b.WriteString(m.pageHeading("Restore / Review", "Confirm the create-only restore plan.") + "\n\n")
	items := m.selectedRestoreItems()
	b.WriteString(m.th.dim.Render(fmt.Sprintf("Policies selected: %d", len(items))) + "\n")
	b.WriteString(m.th.success.Render("Create new policies") + m.th.dim.Render(" in ") + m.th.normal.Render(truncWidth(m.sourceTenant().DisplayName, max(8, w-24))) + "\n\n")
	hasCA := false
	limit := min(len(items), m.listRows(10))
	for i := 0; i < limit; i++ {
		it := items[i]
		if it.Category == "ConditionalAccessPolicies" {
			hasCA = true
		}
		b.WriteString("  " + m.th.success.Render("+") + " " + m.th.normal.Render("[Restored] "+it.Name) +
			" " + m.th.cardLabel.Render("· "+friendlyCategory(it.Category)) + "\n")
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
	b.WriteString(m.pageHeading("Restore / Results", "Policies created in "+emptyDash(m.sourceTenant().DisplayName)+".") + "\n\n")
	ok, fail := 0, 0
	for _, r := range m.restoreResults {
		if r.Err != nil {
			fail++
		} else {
			ok++
		}
	}
	limit := min(len(m.restoreResults), m.listRows(12))
	for _, r := range m.restoreResults[:limit] {
		if r.Err != nil {
			b.WriteString("  " + m.th.danger.Render("✗") + " " + m.th.normal.Render(r.Item.Name) + "  " +
				m.th.dim.Render(trunc(r.Err.Error(), 50)) + "\n")
		} else {
			b.WriteString("  " + m.th.success.Render("✔") + " " + m.th.normal.Render("[Restored] "+r.Item.Name) + "\n")
			if r.Warn {
				b.WriteString("    " + m.th.warn.Render("⚠ created with partial content") + "\n")
			}
		}
	}
	if len(m.restoreResults) > limit {
		b.WriteString("  " + m.th.dim.Render(fmt.Sprintf("… and %d more results", len(m.restoreResults)-limit)) + "\n")
	}
	b.WriteString("\n" + m.th.accent.Render(fmt.Sprintf("%d succeeded, %d failed", ok, fail)))
	return b.String()
}

func (m model) viewError(w int) string {
	var b strings.Builder
	b.WriteString(m.pageHeading("Something went wrong", "TenuVault could not finish the requested action.") + "\n\n")
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
