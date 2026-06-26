package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/ugurkocde/TenuVault-TUI/internal/backup"
	"github.com/ugurkocde/TenuVault-TUI/internal/catalog"
	"github.com/ugurkocde/TenuVault-TUI/internal/config"
	"github.com/ugurkocde/TenuVault-TUI/internal/connection"
	"github.com/ugurkocde/TenuVault-TUI/internal/restore"
	"github.com/ugurkocde/TenuVault-TUI/internal/store"
)

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case tickMsg:
		m.frame++
		return m, tick()

	case deviceCodeMsg:
		m.deviceCode = string(msg)
		return m, listen(m.ctx, m.ch)

	case connectedMsg:
		conn := connection.Connection{Label: msg.tenant.DisplayName, Cfg: msg.cfg, Tenant: msg.tenant, Client: msg.client}
		idx := -1
		for i := range m.conns {
			if m.conns[i].Tenant.ID == conn.Tenant.ID {
				idx = i
			}
		}
		if idx >= 0 {
			m.conns[idx] = conn
			m.sourceIdx = idx
		} else {
			m.conns = append(m.conns, conn)
			m.sourceIdx = len(m.conns) - 1
		}
		m.connected = true
		m.persistConnections()
		if m.addingConn {
			m.addingConn = false
			m.goTo(screenConnections)
		} else {
			m.goTo(screenDashboard)
		}
		return m, loadBackups(m.cfg.BackupRoot)

	case errMsg:
		m.err = msg.err
		m.goTo(screenError)
		return m, nil

	case backupsLoadedMsg:
		m.backups = []store.Backup(msg)
		if len(m.backups) > 0 {
			b := m.backups[0]
			m.lastBackup = &b
		}
		return m, nil

	case backupEventMsg:
		m.applyBackupEvent(msg)
		return m, listen(m.ctx, m.ch)

	case backupDoneMsg:
		m.progDone = true
		if msg.err != nil {
			m.progErr = msg.err
		} else {
			total := 0
			for _, c := range msg.res.ItemCounts {
				total += c
			}
			m.progResult = fmt.Sprintf("%d policies in %s", total, msg.res.Folder)
			m.progStatus = msg.res.Status
			m.progCats = msg.res.Categories
		}
		var cmds []tea.Cmd
		cmds = append(cmds, loadBackups(m.cfg.BackupRoot))
		if m.cfg.RetentionDays > 0 {
			cmds = append(cmds, cleanupBackups(m.cfg.BackupRoot, m.cfg.RetentionDays))
		}
		return m, tea.Batch(cmds...)

	case restoreDoneMsg:
		m.restoreResults = msg.results
		m.restoreRunning = false
		m.goTo(screenRestoreResult)
		return m, loadBackups(m.cfg.BackupRoot)

	case diffDoneMsg:
		m.diffChanges = msg.changes
		m.diffScroll = 0
		if msg.err != nil {
			m.err = msg.err
			m.goTo(screenError)
			return m, nil
		}
		m.goTo(screenDiffResult)
		return m, nil

	case syncPoliciesLoadedMsg:
		m.applySyncPolicies(msg)
		return m, nil

	case syncEventMsg:
		m.syncCur, m.syncTot = msg.Current, msg.Total
		return m, listen(m.ctx, m.ch)

	case syncDoneMsg:
		m.syncResults = msg.results
		m.syncRunning = false
		m.goTo(screenSyncResults)
		return m, nil

	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m *model) applyBackupEvent(e backupEventMsg) {
	if _, seen := m.progEvents[e.Category]; !seen {
		m.progOrder = append(m.progOrder, e.Category)
	}
	m.progFriendly[e.Category] = e.Friendly
	m.progActive = e.Friendly
	m.progCur, m.progTot = e.Current, e.Total
	switch {
	case e.Err != nil:
		m.progEvents[e.Category] = "err"
	case e.Done:
		m.progEvents[e.Category] = "done"
	default:
		m.progEvents[e.Category] = "running"
	}
}

func (m model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if key == "ctrl+c" {
		m.cancel()
		return m, tea.Quit
	}
	if key == "?" {
		m.showHelp = !m.showHelp
		return m, nil
	}

	switch m.screen {
	case screenAuth:
		return m.keyAuth(key)
	case screenConnecting:
		if key == "esc" {
			m.goTo(screenAuth)
		}
		return m, nil
	case screenDashboard:
		return m.keyDashboard(key)
	case screenBackupSelect:
		return m.keyBackupSelect(key)
	case screenProgress:
		if m.progDone && (key == "enter" || key == "esc") {
			m.goTo(screenDashboard)
		}
		return m, nil
	case screenBrowse:
		return m.keyBrowse(key)
	case screenBrowseDetail:
		return m.keyBrowseDetail(key)
	case screenCategoryPolicies:
		return m.keyCategoryPolicies(key)
	case screenPolicyView:
		return m.keyPolicyView(key)
	case screenSettings:
		return m.keySettings(key)
	case screenConnections:
		return m.keyConnections(key)
	case screenSyncSource:
		return m.keySyncSource(key)
	case screenSyncSelect:
		return m.keySyncSelect(key)
	case screenSyncPolicies:
		return m.keySyncPolicies(key)
	case screenSyncTarget:
		return m.keySyncTarget(key)
	case screenSyncNaming:
		return m.keySyncNaming(key)
	case screenSyncConfirm:
		return m.keySyncConfirm(key)
	case screenSyncResults:
		if key == "enter" || key == "esc" || key == "q" {
			m.goTo(screenDashboard)
		}
		return m, nil
	case screenDiffResult:
		return m.keyDiffResult(key)
	case screenRestorePick:
		return m.keyRestorePick(key)
	case screenRestoreConfirm:
		return m.keyRestoreConfirm(key)
	case screenRestoreResult:
		if key == "enter" || key == "esc" || key == "q" {
			m.goTo(screenDashboard)
		}
		return m, nil
	case screenError:
		if key == "enter" || key == "esc" {
			m.err = nil
			m.goTo(screenDashboard)
			if !m.connected {
				m.goTo(screenAuth)
			}
		}
		return m, nil
	}
	return m, nil
}

func (m model) keyAuth(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "up", "k":
		if m.authCursor > 0 {
			m.authCursor--
		}
	case "down", "j":
		if m.authCursor < len(m.authOptions)-1 {
			m.authCursor++
		}
	case "q":
		return m, tea.Quit
	case "enter":
		m.cfg.AuthMethod = m.authOptions[m.authCursor].method
		m.deviceCode = ""
		m.goTo(screenConnecting)
		return m, tea.Batch(connect(m.ctx, m.cfg, m.ch), listen(m.ctx, m.ch))
	}
	return m, nil
}

func (m model) keyDashboard(key string) (tea.Model, tea.Cmd) {
	const items = 7
	switch key {
	case "up", "k":
		if m.dashCursor > 0 {
			m.dashCursor--
		}
	case "down", "j":
		if m.dashCursor < items-1 {
			m.dashCursor++
		}
	case "q":
		return m, tea.Quit
	case "b":
		return m.startBackupSelect()
	case "l":
		return m.openBrowse(modeView, "Browse backups")
	case "d":
		return m.openBrowse(modeDiffA, "Compare · pick baseline (older)")
	case "r":
		return m.openBrowse(modeRestore, "Restore · pick a backup")
	case "y":
		return m.startSync()
	case "t":
		m.connCursor = 0
		m.goTo(screenConnections)
	case "s":
		m.settingsCursor = 0
		m.goTo(screenSettings)
	case "enter":
		switch m.dashCursor {
		case 0:
			return m.startBackupSelect()
		case 1:
			return m.openBrowse(modeView, "Browse backups")
		case 2:
			return m.openBrowse(modeDiffA, "Compare · pick baseline (older)")
		case 3:
			return m.openBrowse(modeRestore, "Restore · pick a backup")
		case 4:
			return m.startSync()
		case 5:
			m.connCursor = 0
			m.goTo(screenConnections)
		case 6:
			m.settingsCursor = 0
			m.goTo(screenSettings)
		}
	}
	return m, nil
}

func (m model) startBackupSelect() (tea.Model, tea.Cmd) {
	m.cats = nil
	for _, pt := range catalog.All() {
		m.cats = append(m.cats, catSel{pt: pt, sel: pt.Verified})
	}
	m.catCursor = 0
	m.goTo(screenBackupSelect)
	return m, nil
}

func (m model) openBrowse(mode browseMode, title string) (tea.Model, tea.Cmd) {
	m.browseMode = mode
	m.browseTitle = title
	m.browseCursor = 0
	m.diffA, m.diffB = nil, nil
	m.goTo(screenBrowse)
	return m, loadBackups(m.cfg.BackupRoot)
}

func (m model) keyBackupSelect(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "up", "k":
		if m.catCursor > 0 {
			m.catCursor--
		}
	case "down", "j":
		if m.catCursor < len(m.cats)-1 {
			m.catCursor++
		}
	case " ", "space":
		m.cats[m.catCursor].sel = !m.cats[m.catCursor].sel
	case "a":
		all := true
		for _, c := range m.cats {
			if !c.sel {
				all = false
				break
			}
		}
		for i := range m.cats {
			m.cats[i].sel = !all
		}
	case "esc":
		m.goTo(screenDashboard)
	case "q":
		m.goTo(screenDashboard)
	case "enter":
		types := m.selectedTypes()
		if len(types) == 0 {
			return m, nil
		}
		m.progTitle = "Backing up"
		m.progEvents = map[string]string{}
		m.progFriendly = map[string]string{}
		m.progOrder = nil
		m.progActive, m.progCur, m.progTot = "", 0, 0
		m.progDone, m.progErr, m.progResult = false, nil, ""
		m.progStatus, m.progCats = "", nil
		m.goTo(screenProgress)
		opts := backup.Options{Root: m.cfg.BackupRoot, Tenant: m.sourceTenant(), IncludeAssignments: m.cfg.IncludeAssignments}
		return m, tea.Batch(runBackup(m.ctx, m.sourceClient(), types, opts, m.ch), listen(m.ctx, m.ch))
	}
	return m, nil
}

func (m model) keyBrowse(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "up", "k":
		if m.browseCursor > 0 {
			m.browseCursor--
		}
	case "down", "j":
		if m.browseCursor < len(m.backups)-1 {
			m.browseCursor++
		}
	case "esc", "q":
		m.goTo(screenDashboard)
	case "enter":
		if len(m.backups) == 0 {
			return m, nil
		}
		b := m.backups[m.browseCursor]
		switch m.browseMode {
		case modeView:
			m.detail = &b
			m.detailCats = scanCategories(b)
			m.goTo(screenBrowseDetail)
		case modeRestore, modeSyncBackup:
			m.restoreBackup = &b
			m.restoreItems = buildRestoreItems(b)
			m.restoreCursor, m.restoreScroll = 0, 0
			m.syncMode = m.browseMode == modeSyncBackup
			m.goTo(screenRestorePick)
		case modeDiffA:
			a := b
			m.diffA = &a
			m.browseMode = modeDiffB
			m.browseTitle = "Compare · pick target (newer)"
		case modeDiffB:
			t := b
			m.diffB = &t
			older, newer := orderBackups(m.diffA, m.diffB)
			return m, runDiff(older, newer)
		}
	}
	return m, nil
}

func (m model) keyBrowseDetail(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "up", "k":
		if m.detailCursor > 0 {
			m.detailCursor--
		}
	case "down", "j":
		if m.detailCursor < len(m.detailCats)-1 {
			m.detailCursor++
		}
	case "enter":
		if m.detail != nil && len(m.detailCats) > 0 {
			m.catName = m.detailCats[m.detailCursor].name
			m.catPolicies, _ = m.detail.Policies(m.catName)
			m.policyCursor = 0
			m.goTo(screenCategoryPolicies)
		}
	case "esc":
		m.goTo(screenBrowse)
	case "q":
		m.goTo(screenDashboard)
	case "r":
		if m.detail != nil {
			b := *m.detail
			m.restoreBackup = &b
			m.restoreItems = buildRestoreItems(b)
			m.restoreCursor, m.restoreScroll = 0, 0
			m.goTo(screenRestorePick)
		}
	}
	return m, nil
}

func (m model) keyCategoryPolicies(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "up", "k":
		if m.policyCursor > 0 {
			m.policyCursor--
		}
	case "down", "j":
		if m.policyCursor < len(m.catPolicies)-1 {
			m.policyCursor++
		}
	case "enter":
		if len(m.catPolicies) > 0 {
			raw, err := store.Read(m.catPolicies[m.policyCursor].Path)
			if err == nil {
				m.policyLines = strings.Split(string(raw), "\n")
			} else {
				m.policyLines = []string{"could not read file: " + err.Error()}
			}
			m.policyScroll = 0
			m.goTo(screenPolicyView)
		}
	case "esc", "q":
		m.goTo(screenBrowseDetail)
	}
	return m, nil
}

func (m model) keyPolicyView(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "up", "k":
		if m.policyScroll > 0 {
			m.policyScroll--
		}
	case "down", "j":
		if m.policyScroll < len(m.policyLines)-1 {
			m.policyScroll++
		}
	case "esc", "q":
		m.goTo(screenCategoryPolicies)
	}
	return m, nil
}

func (m model) keySettings(key string) (tea.Model, tea.Cmd) {
	const items = 3 // include assignments, retention days, auth method
	switch key {
	case "up", "k":
		if m.settingsCursor > 0 {
			m.settingsCursor--
		}
	case "down", "j":
		if m.settingsCursor < items-1 {
			m.settingsCursor++
		}
	case " ", "space", "enter":
		switch m.settingsCursor {
		case 0:
			m.cfg.IncludeAssignments = !m.cfg.IncludeAssignments
		case 2:
			if m.cfg.AuthMethod == config.AuthDeviceCode {
				m.cfg.AuthMethod = config.AuthInteractive
			} else {
				m.cfg.AuthMethod = config.AuthDeviceCode
			}
		}
		_ = config.Save(m.cfg)
	case "+", "=", "right", "l":
		if m.settingsCursor == 1 {
			m.cfg.RetentionDays++
			_ = config.Save(m.cfg)
		}
	case "-", "left", "h":
		if m.settingsCursor == 1 && m.cfg.RetentionDays > 0 {
			m.cfg.RetentionDays--
			_ = config.Save(m.cfg)
		}
	case "esc", "q":
		m.goTo(screenDashboard)
	}
	return m, nil
}

func (m model) keyDiffResult(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "up", "k":
		if m.diffScroll > 0 {
			m.diffScroll--
		}
	case "down", "j":
		if m.diffScroll < len(m.diffChanges)-1 {
			m.diffScroll++
		}
	case "esc", "q":
		m.goTo(screenDashboard)
	}
	return m, nil
}

func (m model) keyRestorePick(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "up", "k":
		if m.restoreCursor > 0 {
			m.restoreCursor--
		}
	case "down", "j":
		if m.restoreCursor < len(m.restoreItems)-1 {
			m.restoreCursor++
		}
	case " ", "space":
		if len(m.restoreItems) > 0 {
			m.restoreItems[m.restoreCursor].sel = !m.restoreItems[m.restoreCursor].sel
		}
	case "a":
		all := true
		for _, r := range m.restoreItems {
			if !r.sel {
				all = false
				break
			}
		}
		for i := range m.restoreItems {
			m.restoreItems[i].sel = !all
		}
	case "esc", "q":
		m.goTo(screenDashboard)
	case "enter":
		sel := m.selectedRestoreItems()
		if len(sel) == 0 {
			return m, nil
		}
		if m.syncMode {
			// Backup-as-source sync: carry the selection forward to a target.
			m.syncItems = nil
			for _, it := range sel {
				m.syncItems = append(m.syncItems, syncerItemFromRestore(it))
			}
			m.syncTargetCursor = 0
			m.goTo(screenSyncTarget)
			return m, nil
		}
		m.goTo(screenRestoreConfirm)
	}
	return m, nil
}

func (m model) keyRestoreConfirm(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "y":
		items := m.selectedRestoreItems()
		if len(items) == 0 {
			m.goTo(screenRestorePick)
			return m, nil
		}
		m.restoreRunning = true
		m.restoreResults = nil
		return m, tea.Batch(runRestore(m.ctx, m.sourceClient(), items, m.ch), listen(m.ctx, m.ch))
	case "n", "esc":
		m.goTo(screenRestorePick)
	}
	return m, nil
}

// scanCategories counts policy files per category in a backup.
func scanCategories(b store.Backup) []catCount {
	cats, _ := b.Categories()
	var out []catCount
	for _, c := range cats {
		files, _ := b.Policies(c)
		out = append(out, catCount{name: c, count: len(files)})
	}
	return out
}

// buildRestoreItems flattens restorable policies in a backup into restore items,
// skipping backup-only categories (apps, admin templates, intents, enrollment).
func buildRestoreItems(b store.Backup) []restoreSel {
	cats, _ := b.Categories()
	var out []restoreSel
	for _, c := range cats {
		if !catalog.CategoryRestoreSupported(c) {
			continue
		}
		files, _ := b.Policies(c)
		for _, f := range files {
			out = append(out, restoreSel{item: restore.Item{Category: c, Name: f.Name, Path: f.Path}})
		}
	}
	return out
}

// orderBackups returns (older, newer) by folder name. It tolerates nil inputs
// so a stray navigation can never panic.
func orderBackups(a, b *store.Backup) (store.Backup, store.Backup) {
	if a == nil && b == nil {
		return store.Backup{}, store.Backup{}
	}
	if a == nil {
		return *b, *b
	}
	if b == nil {
		return *a, *a
	}
	if a.Folder <= b.Folder {
		return *a, *b
	}
	return *b, *a
}
