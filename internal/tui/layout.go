package tui

import (
	"fmt"
	"os"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/ugurkocde/TenuVault-TUI/internal/catalog"
	"github.com/ugurkocde/TenuVault-TUI/internal/config"
)

const (
	maxContentWidth = 112
	minUsableWidth  = 46
	minUsableHeight = 20
	wideWidth       = 88
)

func (m model) contentWidth() int {
	w := m.width0()
	if w > maxContentWidth {
		return maxContentWidth
	}
	return w
}

func (m model) isWide(w int) bool {
	return w >= wideWidth
}

func (m model) isShort() bool {
	return m.height > 0 && m.height <= 24
}

func (m model) tooSmall() bool {
	return m.width > 0 && (m.width < minUsableWidth || (m.height > 0 && m.height < minUsableHeight))
}

func (m model) viewTooSmall() string {
	w, h := m.width0(), m.height
	if h == 0 {
		h = minUsableHeight
	}
	title := "Terminal window is too small"
	if w < 30 {
		title = "Terminal too small"
	}
	msg := m.th.logo.Render(truncWidth("⬡ TenuVault", w)) + "\n\n" +
		m.th.title.Render(truncWidth(title, w)) + "\n" +
		m.th.dim.Render(truncWidth(fmt.Sprintf("Current: %d×%d · Minimum: %d×%d", w, h, minUsableWidth, minUsableHeight), w)) + "\n\n" +
		m.th.cardLabel.Render(truncWidth("Enlarge the window to continue.  q quit", w))
	return lipgloss.NewStyle().Width(max(1, w)).Align(lipgloss.Center).Render(msg)
}

func (m model) pageHeading(title, subtitle string) string {
	var b strings.Builder
	w := m.contentWidth()
	b.WriteString(m.th.title.Foreground(colPurple).Render(truncWidth(title, w)))
	if subtitle != "" && !m.isShort() {
		b.WriteString("\n" + m.th.dim.Render(truncWidth(subtitle, w)))
	}
	return b.String()
}

func (m model) sectionLabel(label string) string {
	return m.th.section.Render(strings.ToUpper(label))
}

// actionRow renders one terminal-native list row. The selected state uses both
// a full-row background and a bright leading bar, so focus never depends on
// colour alone.
func (m model) actionRow(label, right string, width int, selected bool) string {
	if width < 4 {
		width = 4
	}
	inner := width - 2
	marker := "▸ "
	leftMax := inner - lipgloss.Width(right) - 1
	if leftMax < 3 {
		leftMax = inner
		right = ""
	}
	left := marker + truncWidth(label, max(1, leftMax-lipgloss.Width(marker)))
	line := spread(left, right, inner)
	line = padWidth(truncWidth(line, inner), inner)
	if selected {
		return m.th.selectBar.Render(" ") + m.th.selectRow.Width(inner).Render(line)
	}
	return " " + m.th.normal.Render(line)
}

func (m model) checkRow(checked bool, label, right string, width int, selected bool) string {
	box := "[ ]"
	if checked {
		box = "[x]"
	}
	return m.actionRow(box+"  "+label, right, width, selected)
}

func (m model) triCheckRow(state, label, right string, width int, selected bool) string {
	box := "[ ]"
	switch state {
	case "all":
		box = "[x]"
	case "some":
		box = "[~]"
	}
	return m.actionRow(box+"  "+label, right, width, selected)
}

func (m model) keyBadge(key string) string {
	return m.th.key.Render("[ " + strings.ToUpper(key) + " ]")
}

func (m model) infoPanel(label string, lines []string, width int, good bool) string {
	if width < 16 {
		width = 16
	}
	var b strings.Builder
	b.WriteString(m.sectionLabel(label))
	for _, line := range lines {
		b.WriteString("\n" + line)
	}
	style := m.th.panel
	if good {
		style = m.th.panelGood
	}
	return style.Width(width).Render(b.String())
}

func (m model) renderFooter(hints string, width int) string {
	parts := strings.Split(hints, " · ")
	styled := make([]string, 0, len(parts))
	plain := make([]string, 0, len(parts))
	used := 0
	for _, part := range parts {
		fields := strings.Fields(part)
		if len(fields) == 0 {
			continue
		}
		key := fields[0]
		desc := strings.TrimSpace(strings.TrimPrefix(part, key))
		p := key
		if desc != "" {
			p += " " + desc
		}
		sep := 0
		if len(plain) > 0 {
			sep = 3
		}
		if used+sep+lipgloss.Width(p) > width {
			break
		}
		used += sep + lipgloss.Width(p)
		plain = append(plain, p)
		item := m.th.keyActive.Render(key)
		if desc != "" {
			item += " " + m.th.footer.Render(desc)
		}
		styled = append(styled, item)
	}
	if len(styled) == 0 {
		return m.th.footer.Render(truncWidth(hints, max(1, width)))
	}
	x := 0
	for _, part := range plain {
		fields := strings.Fields(part)
		if len(fields) == 0 {
			continue
		}
		key := footerKey(fields[0])
		itemWidth := lipgloss.Width(part)
		if key != "" {
			m.hits.actions = append(m.hits.actions, hitAction{y: -1, x0: x, x1: x + itemWidth, key: key})
		}
		x += itemWidth + 3
	}
	return strings.Join(styled, m.th.footer.Render("   "))
}

func footerKey(label string) string {
	switch strings.ToLower(label) {
	case "enter", "space", "esc", "q", "?", "/", "a", "b", "d", "e", "l", "n", "r", "s", "t", "v", "x", "y":
		return strings.ToLower(label)
	case "←/→":
		return "right"
	}
	return ""
}

func (m model) helpView(w int, contextual string) string {
	if m.isShort() {
		lines := []string{
			m.th.title.Render("Keyboard & mouse help"),
			m.th.keyActive.Render("↑↓ / j k") + m.th.dim.Render("   move"),
			m.th.keyActive.Render("enter") + m.th.dim.Render("      open"),
			m.th.keyActive.Render("space") + m.th.dim.Render("      toggle"),
			m.th.keyActive.Render("esc") + m.th.dim.Render("        back"),
			m.th.keyActive.Render("?") + m.th.dim.Render("          close help"),
			m.th.cardLabel.Render("Mouse: click rows or use the wheel."),
		}
		return m.th.panel.Width(min(w-2, 76)).Render(strings.Join(lines, "\n"))
	}
	lines := []string{
		m.th.title.Render("Keyboard & mouse help"),
		"",
		m.th.keyActive.Render("↑↓ / j k") + m.th.dim.Render("   move selection"),
		m.th.keyActive.Render("enter") + m.th.dim.Render("      open or continue"),
		m.th.keyActive.Render("space") + m.th.dim.Render("      toggle a checkbox"),
		m.th.keyActive.Render("/") + m.th.dim.Render("          filter the current list"),
		m.th.keyActive.Render("esc") + m.th.dim.Render("        go back"),
		m.th.keyActive.Render("?") + m.th.dim.Render("          close this help"),
		"",
		m.th.section.Render("THIS SCREEN"),
		m.th.normal.Render(strings.ReplaceAll(contextual, " · ", "   ")),
		"",
		m.th.cardLabel.Render("Mouse: click rows and use the wheel to move. Hold Shift to select terminal text."),
	}
	return m.th.panel.Width(min(w, 76)).Render(strings.Join(lines, "\n"))
}

func formatBackupDate(value string) string {
	for _, layout := range []string{
		"2006-01-02-150405.000000000",
		"2006-01-02-150405",
		"2006-01-02T15:04:05Z07:00",
		"2006-01-02 15:04:05",
	} {
		if t, err := time.Parse(layout, value); err == nil {
			return t.Format("02 Jan 2006 · 15:04")
		}
	}
	return emptyDash(value)
}

func friendlyBackupStatus(status string) string {
	switch strings.ToLower(status) {
	case "success", "completed":
		return "Completed successfully"
	case "completedwithwarnings":
		return "Completed with warnings"
	case "cancelled":
		return "Cancelled"
	case "failed":
		return "Failed"
	default:
		return emptyDash(status)
	}
}

func shortBackupStatus(status string) string {
	switch strings.ToLower(status) {
	case "success", "completed":
		return "OK"
	case "completedwithwarnings":
		return "WARN"
	default:
		return friendlyBackupStatus(status)
	}
}

func friendlyCategory(category string) string {
	for _, pt := range catalog.All() {
		if pt.Category == category {
			return pt.Friendly
		}
	}
	// Older TenuVault backups used this folder name for endpoint security
	// intents. Keep their labels friendly when they are browsed or restored.
	if category == "Intents" {
		return "Endpoint security / baselines"
	}
	return category
}

func compactPath(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	if path == home {
		return "~"
	}
	if strings.HasPrefix(path, home+string(os.PathSeparator)) {
		return "~" + strings.TrimPrefix(path, home)
	}
	return path
}

func plural(n int, singular, pluralForm string) string {
	if n == 1 {
		return singular
	}
	return pluralForm
}

func friendlyAuthMethod(method config.AuthMethod) string {
	switch method {
	case config.AuthInteractive:
		return "Interactive sign-in"
	case config.AuthSecret:
		return "Client secret"
	case config.AuthCertificate:
		return "Certificate"
	default:
		return string(method)
	}
}

func truncWidth(s string, n int) string {
	if n <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= n {
		return s
	}
	r := []rune(s)
	for len(r) > 0 && lipgloss.Width(string(r))+1 > n {
		r = r[:len(r)-1]
	}
	return string(r) + "…"
}

func padWidth(s string, width int) string {
	if gap := width - lipgloss.Width(s); gap > 0 {
		return s + strings.Repeat(" ", gap)
	}
	return s
}
