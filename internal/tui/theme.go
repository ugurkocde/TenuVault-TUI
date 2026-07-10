package tui

import "charm.land/lipgloss/v2"

// TenuVault brand palette on a fixed dark surface.
var (
	colPurple  = lipgloss.Color("#c084fc")
	colPurple2 = lipgloss.Color("#33234f")
	colPink    = lipgloss.Color("#f472b6")
	colOrange  = lipgloss.Color("#fb923c")
	colGreen   = lipgloss.Color("#34d399")
	colYellow  = lipgloss.Color("#facc15")
	colRed     = lipgloss.Color("#f87171")
	colText    = lipgloss.Color("#ece8f5")
	colDim     = lipgloss.Color("#9b93ad")
	colMuted   = lipgloss.Color("#6f6685")
	colBorder  = lipgloss.Color("#5b3f8a")
	colLine    = lipgloss.Color("#3a3050")
)

// theme holds reusable styles.
type theme struct {
	title     lipgloss.Style
	logo      lipgloss.Style
	crumb     lipgloss.Style
	panel     lipgloss.Style
	panelGood lipgloss.Style
	panelWarn lipgloss.Style
	panelLbl  lipgloss.Style
	section   lipgloss.Style
	key       lipgloss.Style
	keyActive lipgloss.Style
	footer    lipgloss.Style
	selected  lipgloss.Style
	selectRow lipgloss.Style
	selectBar lipgloss.Style
	normal    lipgloss.Style
	dim       lipgloss.Style
	success   lipgloss.Style
	warn      lipgloss.Style
	danger    lipgloss.Style
	accent    lipgloss.Style
	inProg    lipgloss.Style
	cardLabel lipgloss.Style
}

func newTheme() theme {
	return theme{
		logo:      lipgloss.NewStyle().Foreground(colPurple).Bold(true),
		title:     lipgloss.NewStyle().Foreground(colText).Bold(true),
		crumb:     lipgloss.NewStyle().Foreground(colMuted),
		panel:     lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colBorder).Padding(0, 1),
		panelGood: lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colGreen).Padding(0, 1),
		panelWarn: lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colYellow).Padding(0, 1),
		panelLbl:  lipgloss.NewStyle().Foreground(colPurple).Bold(true),
		section:   lipgloss.NewStyle().Foreground(colDim).Bold(true),
		key:       lipgloss.NewStyle().Foreground(colDim),
		keyActive: lipgloss.NewStyle().Foreground(colPurple).Bold(true),
		footer:    lipgloss.NewStyle().Foreground(colMuted),
		selected:  lipgloss.NewStyle().Foreground(colPurple).Bold(true),
		selectRow: lipgloss.NewStyle().Background(colPurple2).Foreground(colText).Bold(true),
		selectBar: lipgloss.NewStyle().Background(colPurple).Foreground(colPurple),
		normal:    lipgloss.NewStyle().Foreground(colText),
		dim:       lipgloss.NewStyle().Foreground(colDim),
		success:   lipgloss.NewStyle().Foreground(colGreen),
		warn:      lipgloss.NewStyle().Foreground(colYellow),
		danger:    lipgloss.NewStyle().Foreground(colRed),
		accent:    lipgloss.NewStyle().Foreground(colPink),
		inProg:    lipgloss.NewStyle().Foreground(colOrange),
		cardLabel: lipgloss.NewStyle().Foreground(colDim),
	}
}
