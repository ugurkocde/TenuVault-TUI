package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	tea "charm.land/bubbletea/v2"

	"github.com/ugurkocde/TenuVault-TUI/internal/catalog"
	"github.com/ugurkocde/TenuVault-TUI/internal/config"
	"github.com/ugurkocde/TenuVault-TUI/internal/restore"
	"github.com/ugurkocde/TenuVault-TUI/internal/store"
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
		m.connRemove = -1
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
	// A pending removal intercepts every key: y confirms, anything else cancels.
	if m.connRemove >= 0 {
		if key == "y" && m.connRemove < len(rows) {
			m.removeConnection(rows[m.connRemove])
		}
		m.connRemove = -1
		return m, nil
	}
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
		m.connRemove = m.connCursor
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
		return m, m.beginConnect(cfg)
	case "esc", "q":
		if m.connected {
			m.goTo(screenDashboard)
		} else if key == "esc" {
			m.goTo(screenAuth) // not connected: back out to a fresh sign-in
		} else {
			return m, tea.Quit
		}
	}
	return m, nil
}

// removeConnection drops a tenant from the persisted config and, when live,
// from the connected set, keeping every index that pointed past it stable.
func (m *model) removeConnection(row connRow) {
	var kept []config.ConnConfig
	for _, cc := range m.cfg.Connections {
		if cc.TenantID != row.tenantID {
			kept = append(kept, cc)
		}
	}
	m.cfg.Connections = kept
	if row.liveIdx >= 0 {
		m.conns = append(m.conns[:row.liveIdx], m.conns[row.liveIdx+1:]...)
		// Removing an earlier entry shifts everything left; follow the
		// active source so it keeps pointing at the same tenant.
		if row.liveIdx < m.sourceIdx {
			m.sourceIdx--
		}
		if m.sourceIdx >= len(m.conns) {
			m.sourceIdx = len(m.conns) - 1
		}
		if m.sourceIdx < 0 {
			m.sourceIdx = 0
		}
		// The sync source/target must follow the same tenant too; when the
		// removed connection was the chosen one, mark it invalid so the sync
		// confirm fails closed instead of writing to a different tenant.
		m.syncSourceConn = shiftIdx(m.syncSourceConn, row.liveIdx)
		m.syncTargetConn = shiftIdx(m.syncTargetConn, row.liveIdx)
		m.connected = len(m.conns) > 0
	}
	saveErr := m.persistConnections()
	if m.connCursor > 0 {
		m.connCursor--
	}
	if saveErr != nil {
		m.status, m.statusKind = "Removed tenant, but saving failed: "+saveErr.Error(), "warn"
	} else {
		m.status, m.statusKind = "Removed "+row.label, "ok"
	}
	if !m.connected {
		m.goTo(screenConnections)
	}
}

// shiftIdx adjusts a connection index after the entry at removed is deleted:
// later entries shift left, the removed entry itself becomes invalid.
func shiftIdx(idx, removed int) int {
	switch {
	case idx == removed:
		return -1
	case idx > removed:
		return idx - 1
	default:
		return idx
	}
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
		return m.openBrowse(modeSyncBackup, "Sync · pick a backup (step 2 of 6)")
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
			m.goTo(screenSyncSource)
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
		m.resetFilter()
		m.goTo(screenSyncPolicies)
		if !st.loaded && !st.loading {
			st.loading = true
			return m, loadSyncType(m.ctx, m.syncSourceClient(), st.pt, m.syncGen)
		}
	case "n", "tab":
		m.syncItems = m.gatherLiveSyncItems()
		if len(m.syncItems) == 0 {
			m.status, m.statusKind = "Select at least one policy", "err"
			return m, nil
		}
		m.syncTargetCursor = 0
		m.goTo(screenSyncTarget)
	case "esc", "q":
		m.goTo(screenSyncSource)
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
	if m.filterActive && m.filterInputKey(key) {
		m.syncPolCursor = snapFiltered(m.syncPolCursor, syncPolVisible(st.policies, m.filterQuery, m.listSort))
		return m, nil
	}
	visible := syncPolVisible(st.policies, m.filterQuery, m.listSort)
	switch key {
	case "/":
		m.filterActive = true
	case "s":
		m.listSort = cycleSort(m.listSort)
	case "up", "k":
		m.syncPolCursor = stepFiltered(m.syncPolCursor, -1, visible)
	case "down", "j":
		m.syncPolCursor = stepFiltered(m.syncPolCursor, 1, visible)
	case " ", "space":
		if idxVisible(visible, m.syncPolCursor) {
			st.policies[m.syncPolCursor].sel = !st.policies[m.syncPolCursor].sel
		}
	case "a":
		all := true
		for _, i := range visible {
			if !st.policies[i].sel {
				all = false
				break
			}
		}
		for _, i := range visible {
			st.policies[i].sel = !all
		}
	case "r":
		if st.err != "" && !st.loading {
			st.loading = true
			st.err = ""
			return m, loadSyncType(m.ctx, m.syncSourceClient(), st.pt, m.syncGen)
		}
	case "esc":
		if m.filterQuery != "" {
			m.filterQuery = ""
			return m, nil
		}
		m.goTo(screenSyncSelect)
	case "q":
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
		if msg.err != nil {
			// Leave the type unloaded so the next open/toggle retries, and keep
			// the cause visible instead of pretending the tenant has no policies.
			st.loaded = false
			st.pendingAll = false
			st.err = trunc(msg.err.Error(), 70)
			return
		}
		st.err = ""
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
		// Step back one screen, matching the "esc back" hint: to the backup item
		// picker for a backup source, or the category list for a live source.
		if m.syncFromBackup {
			m.goTo(screenRestorePick)
		} else {
			m.goTo(screenSyncSelect)
		}
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
	if m.syncRunning {
		if key == "x" && m.runCancel != nil {
			m.runCancel()
		}
		return m, nil
	}
	switch key {
	case "y":
		if len(m.syncItems) == 0 {
			m.goTo(screenDashboard)
			return m, nil
		}
		// Fail closed when a connection chosen earlier in the wizard was removed
		// in the meantime: never write using a re-pointed index.
		if m.syncTargetConn < 0 || m.syncTargetConn >= len(m.conns) {
			m.status, m.statusKind = "Sync target is no longer connected", "err"
			m.syncTargetCursor = 0
			m.goTo(screenSyncTarget)
			return m, nil
		}
		if !m.syncFromBackup && (m.syncSourceConn < 0 || m.syncSourceConn >= len(m.conns)) {
			m.status, m.statusKind = "Sync source is no longer connected", "err"
			m.syncSourceCursor = 0
			m.goTo(screenSyncSource)
			return m, nil
		}
		if m.syncFromBackup && m.syncNamePrefix == "" {
			same, err := m.backupMatchesSyncTarget()
			if err != nil {
				m.status, m.statusKind = "Cannot verify backup tenant: "+err.Error(), "err"
				return m, nil
			}
			if same {
				m.status, m.statusKind = "Choose a name prefix before syncing a backup into its source tenant", "err"
				m.goTo(screenSyncNaming)
				return m, nil
			}
		}
		// Fail closed: the source client is only the chosen sync source (nil for a
		// backup source), never the dashboard's active connection.
		source := m.syncSourceClient()
		target := m.conns[m.syncTargetConn].Client
		m.syncRunning = true
		m.syncResults = nil
		m.syncCur, m.syncTot = 0, len(m.syncItems)
		runCtx := m.newRun()
		return m, tea.Batch(runSync(runCtx, m.ctx, target, source, m.syncItems, m.syncNamePrefix, m.ch), listen(m.ctx, m.ch))
	case "n", "esc":
		m.goTo(screenSyncNaming)
	}
	return m, nil
}

func (m model) backupMatchesSyncTarget() (bool, error) {
	if m.restoreBackup == nil || m.syncTargetConn < 0 || m.syncTargetConn >= len(m.conns) {
		return false, nil
	}
	data, err := os.ReadFile(filepath.Join(m.restoreBackup.Path, "metadata.json"))
	if err != nil {
		return false, err
	}
	var meta store.Metadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return false, err
	}
	if meta.TenantID == "" {
		return false, fmt.Errorf("backup metadata does not identify its source tenant; choose a prefix")
	}
	return meta.TenantID == m.conns[m.syncTargetConn].Tenant.ID, nil
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
