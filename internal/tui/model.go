// Package tui implements the TenuVault terminal UI with Bubble Tea.
package tui

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/ugurkocde/TenuVault-TUI/internal/catalog"
	"github.com/ugurkocde/TenuVault-TUI/internal/config"
	"github.com/ugurkocde/TenuVault-TUI/internal/diff"
	"github.com/ugurkocde/TenuVault-TUI/internal/graph"
	"github.com/ugurkocde/TenuVault-TUI/internal/restore"
	"github.com/ugurkocde/TenuVault-TUI/internal/store"
)

type screen int

const (
	screenAuth screen = iota
	screenConnecting
	screenDashboard
	screenBackupSelect
	screenProgress
	screenBrowse
	screenBrowseDetail
	screenDiffResult
	screenRestorePick
	screenRestoreConfirm
	screenRestoreResult
	screenError
)

// browseMode controls what selecting a backup does.
type browseMode int

const (
	modeView browseMode = iota
	modeRestore
	modeDiffA
	modeDiffB
)

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

type tickMsg time.Time

func tick() tea.Cmd {
	return tea.Tick(time.Second/12, func(t time.Time) tea.Msg { return tickMsg(t) })
}

type authOption struct {
	label  string
	desc   string
	method config.AuthMethod
}

type catSel struct {
	pt  catalog.PolicyType
	sel bool
}

type restoreSel struct {
	item restore.Item
	sel  bool
}

type catCount struct {
	name  string
	count int
}

type model struct {
	cfg    config.Config
	ctx    context.Context
	cancel context.CancelFunc
	ch     chan tea.Msg
	th     theme

	width, height int
	frame         int
	screen        screen
	prevScreen    screen
	err           error
	showHelp      bool

	// connection
	client     *graph.Client
	tenant     graph.Tenant
	connected  bool
	deviceCode string

	// auth screen
	authOptions []authOption
	authCursor  int

	// dashboard
	dashCursor int
	lastBackup *store.Backup

	// backup select
	cats      []catSel
	catCursor int

	// progress (backup)
	progTitle      string
	progEvents     map[string]string // category -> status icon
	progFriendly   map[string]string // category -> friendly name
	progOrder      []string
	progActive     string
	progCur        int
	progTot        int
	progDone       bool
	progResult     string
	progErr        error
	restoreRunning bool

	// browse
	backups      []store.Backup
	browseCursor int
	browseMode   browseMode
	browseTitle  string

	// detail
	detail     *store.Backup
	detailCats []catCount

	// diff
	diffA       *store.Backup
	diffB       *store.Backup
	diffChanges []diff.Change
	diffScroll  int

	// restore
	restoreBackup  *store.Backup
	restoreItems   []restoreSel
	restoreCursor  int
	restoreScroll  int
	restoreResults []restore.Result
}

// New builds the root model.
func New(cfg config.Config) model {
	ctx, cancel := context.WithCancel(context.Background())
	return model{
		cfg:    cfg,
		ctx:    ctx,
		cancel: cancel,
		ch:     make(chan tea.Msg, 64),
		th:     newTheme(),
		screen: screenAuth,
		authOptions: []authOption{
			{"Interactive sign-in", "Opens your browser. No app registration needed.", config.AuthInteractive},
			{"Device code", "Headless: enter a code at microsoft.com/devicelogin.", config.AuthDeviceCode},
		},
		progEvents:   map[string]string{},
		progFriendly: map[string]string{},
	}
}

func (m model) Init() tea.Cmd { return tick() }

// View wraps the rendered content in a full-screen, mouse-enabled view.
func (m model) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

// Program returns a configured Bubble Tea program for this model.
func Program(cfg config.Config) *tea.Program {
	return tea.NewProgram(New(cfg))
}

func (m *model) goTo(s screen) {
	m.prevScreen = m.screen
	m.screen = s
}

func (m model) selectedTypes() []catalog.PolicyType {
	var out []catalog.PolicyType
	for _, c := range m.cats {
		if c.sel {
			out = append(out, c.pt)
		}
	}
	return out
}

func (m model) selectedRestoreItems() []restore.Item {
	var out []restore.Item
	for _, r := range m.restoreItems {
		if r.sel {
			out = append(out, r.item)
		}
	}
	return out
}
