// Package tui implements the TenuVault terminal UI with Bubble Tea.
package tui

import (
	"context"
	"encoding/json"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/ugurkocde/TenuVault-TUI/internal/backup"
	"github.com/ugurkocde/TenuVault-TUI/internal/catalog"
	"github.com/ugurkocde/TenuVault-TUI/internal/config"
	"github.com/ugurkocde/TenuVault-TUI/internal/connection"
	"github.com/ugurkocde/TenuVault-TUI/internal/diff"
	"github.com/ugurkocde/TenuVault-TUI/internal/graph"
	"github.com/ugurkocde/TenuVault-TUI/internal/restore"
	"github.com/ugurkocde/TenuVault-TUI/internal/store"
	"github.com/ugurkocde/TenuVault-TUI/internal/syncer"
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
	screenCategoryPolicies
	screenPolicyView
	screenDiffResult
	screenRestorePick
	screenRestoreConfirm
	screenRestoreResult
	screenSettings
	screenConnections
	screenSyncSource
	screenSyncSelect
	screenSyncPolicies
	screenSyncTarget
	screenSyncNaming
	screenSyncConfirm
	screenSyncResults
	screenError
)

// browseMode controls what selecting a backup does.
type browseMode int

const (
	modeView browseMode = iota
	modeRestore
	modeDiffA
	modeDiffB
	modeSyncBackup
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

	// connections
	conns      []connection.Connection
	sourceIdx  int
	connected  bool
	deviceCode string
	addingConn bool
	connCursor int

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
	progStatus     string
	progCats       []backup.CategoryResult
	progErr        error
	restoreRunning bool

	// browse
	backups      []store.Backup
	browseCursor int
	browseMode   browseMode
	browseTitle  string

	// detail
	detail       *store.Backup
	detailCats   []catCount
	detailCursor int
	catName      string
	catPolicies  []store.PolicyFile
	policyCursor int
	policyLines  []string
	policyScroll int

	// settings
	settingsCursor int

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

	// sync
	syncFromBackup   bool
	syncSourceConn   int
	syncTargetConn   int
	syncSourceLabel  string
	syncTypes        []syncType
	syncTypeCursor   int
	syncActiveType   int
	syncPolCursor    int
	syncNamePrefix   string
	syncNameCursor   int
	syncSourceCursor int
	syncTargetCursor int
	syncItems        []syncer.Item
	syncRunning      bool
	syncMode         bool
	syncCur          int
	syncTot          int
	syncResults      []syncer.Result
}

// syncType is a restore-supported policy type in the live sync browser, with
// its policies loaded lazily from the source tenant.
type syncType struct {
	pt         catalog.PolicyType
	loaded     bool
	loading    bool
	pendingAll bool
	policies   []syncPol
}

type syncPol struct {
	name string
	id   string
	raw  json.RawMessage
	sel  bool
}

// New builds the root model.
func New(cfg config.Config) model {
	ctx, cancel := context.WithCancel(context.Background())
	start := screenAuth
	// If real tenants were saved from a previous session, open the tenant selector
	// so the user can reconnect one instead of starting from a blank sign-in.
	for _, cc := range cfg.Connections {
		if !placeholderTenant(cc.TenantID) {
			start = screenConnections
			break
		}
	}
	return model{
		cfg:    cfg,
		ctx:    ctx,
		cancel: cancel,
		ch:     make(chan tea.Msg, 64),
		th:     newTheme(),
		screen: start,
		authOptions: []authOption{
			{"Interactive sign-in", "Opens your browser. No app registration needed.", config.AuthInteractive},
			{"Device code", "Headless: enter a code at microsoft.com/devicelogin.", config.AuthDeviceCode},
		},
		progEvents:   map[string]string{},
		progFriendly: map[string]string{},
	}
}

// sourceConn returns the active source connection, or nil.
func (m model) sourceConn() *connection.Connection {
	if m.sourceIdx >= 0 && m.sourceIdx < len(m.conns) {
		return &m.conns[m.sourceIdx]
	}
	return nil
}

// sourceClient returns the active source tenant's Graph client, or nil.
func (m model) sourceClient() *graph.Client {
	if c := m.sourceConn(); c != nil {
		return c.Client
	}
	return nil
}

// sourceTenant returns the active source tenant.
func (m model) sourceTenant() graph.Tenant {
	if c := m.sourceConn(); c != nil {
		return c.Tenant
	}
	return graph.Tenant{}
}

// syncSourceClient returns the Graph client for the tenant chosen as the sync
// source (distinct from the dashboard's active source), or nil for a backup
// source.
func (m model) syncSourceClient() *graph.Client {
	if !m.syncFromBackup && m.syncSourceConn >= 0 && m.syncSourceConn < len(m.conns) {
		return m.conns[m.syncSourceConn].Client
	}
	return nil
}

// placeholderTenant reports whether a tenant id is a sign-in placeholder rather
// than a real, addressable tenant.
func placeholderTenant(id string) bool {
	switch id {
	case "", "organizations", "common", "consumers":
		return true
	}
	return false
}

// persistConnections saves connection metadata (no tokens) to config. It merges
// remembered tenants with the currently-live ones by real tenant id, so
// reconnecting one tenant never drops the others, and drops stale placeholders.
func (m *model) persistConnections() {
	byID := map[string]config.ConnConfig{}
	var order []string
	add := func(cc config.ConnConfig) {
		if placeholderTenant(cc.TenantID) {
			return
		}
		if _, ok := byID[cc.TenantID]; !ok {
			order = append(order, cc.TenantID)
		}
		byID[cc.TenantID] = cc
	}
	for _, cc := range m.cfg.Connections {
		add(cc)
	}
	for _, c := range m.conns {
		add(c.AsConfig())
	}
	var ccs []config.ConnConfig
	for _, id := range order {
		ccs = append(ccs, byID[id])
	}
	m.cfg.Connections = ccs
	_ = config.Save(m.cfg)
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
