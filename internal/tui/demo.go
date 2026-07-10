//go:build tui_demo

package tui

import (
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/ugurkocde/TenuVault-TUI/internal/catalog"
	"github.com/ugurkocde/TenuVault-TUI/internal/config"
	"github.com/ugurkocde/TenuVault-TUI/internal/connection"
	"github.com/ugurkocde/TenuVault-TUI/internal/diff"
	"github.com/ugurkocde/TenuVault-TUI/internal/graph"
	"github.com/ugurkocde/TenuVault-TUI/internal/restore"
	"github.com/ugurkocde/TenuVault-TUI/internal/store"
	"github.com/ugurkocde/TenuVault-TUI/internal/syncer"
)

// DemoProgram runs the real TUI renderer with fictional data for screenshots
// and recordings. The build tag keeps this documentation harness out of release
// binaries, and the wrapper intercepts writes so it can never call Graph.
func DemoProgram(screenName string, firstBackupFlow bool) *tea.Program {
	d := demoModel{model: demoSeed(), firstBackupFlow: firstBackupFlow}
	if firstBackupFlow {
		d.lastBackup = nil
	}
	d.setScreen(screenName)
	return tea.NewProgram(d)
}

type demoModel struct {
	model
	firstBackupFlow bool
}

type demoProgressMsg int

func demoProgress(step int) tea.Cmd {
	return tea.Tick(420*time.Millisecond, func(time.Time) tea.Msg { return demoProgressMsg(step) })
}

func (d demoModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if step, ok := msg.(demoProgressMsg); ok {
		return d.advanceBackup(int(step))
	}
	if key, ok := msg.(tea.KeyPressMsg); ok {
		switch {
		case d.firstBackupFlow && d.screen == screenBackupSelect && key.String() == "enter":
			d.startDemoBackup()
			return d, demoProgress(1)
		case d.firstBackupFlow && d.screen == screenProgress && d.progDone && key.String() == "enter":
			b := demoBackup("backup-2026-07-10-143200", "2026-07-10-143200", 1204, "Completed")
			d.lastBackup = &b
			d.status, d.statusKind = "Backup complete · 1,204 policies saved", "ok"
			d.goTo(screenDashboard)
			return d, nil
		}
	}
	next, cmd := d.model.Update(msg)
	d.model = next.(model)
	return d, cmd
}

func (d *demoModel) startDemoBackup() {
	d.progTitle = "Backing up"
	d.progOrder = []string{
		"DeviceConfigurations",
		"ConfigurationPolicies",
		"CompliancePolicies",
		"Intents",
		"DeviceManagementScripts",
		"ConditionalAccessPolicies",
	}
	d.progFriendly = map[string]string{
		"DeviceConfigurations":      "Device configurations",
		"ConfigurationPolicies":     "Settings catalog",
		"CompliancePolicies":        "Compliance policies",
		"Intents":                   "Endpoint security / baselines",
		"DeviceManagementScripts":   "Windows scripts",
		"ConditionalAccessPolicies": "Conditional access",
	}
	d.progEvents = map[string]string{}
	for _, key := range d.progOrder {
		d.progEvents[key] = "running"
	}
	d.progActive = d.progFriendly[d.progOrder[0]]
	d.progCur, d.progTot = 0, len(d.progOrder)
	d.progDone, d.progErr, d.progResult = false, nil, ""
	d.progStatus = ""
	d.goTo(screenProgress)
}

func (d demoModel) advanceBackup(step int) (tea.Model, tea.Cmd) {
	if step > 0 && step <= len(d.progOrder) {
		d.progEvents[d.progOrder[step-1]] = "done"
	}
	d.progCur = min(step, len(d.progOrder))
	if step < len(d.progOrder) {
		d.progActive = d.progFriendly[d.progOrder[step]]
		return d, demoProgress(step + 1)
	}
	d.progDone = true
	d.progStatus = "Completed"
	d.progResult = "1,204 policies saved"
	d.progActive = ""
	return d, nil
}

func (d *demoModel) setScreen(name string) {
	switch name {
	case "backup":
		d.screen = screenBackupSelect
	case "browse":
		d.screen = screenBrowse
		d.browseTitle = "Browse backups"
	case "compare":
		d.screen = screenDiffResult
	case "restore":
		d.screen = screenRestorePick
	case "sync":
		d.screen = screenSyncSelect
	case "tenants":
		d.screen = screenConnections
	case "settings":
		d.screen = screenSettings
	default:
		d.screen = screenDashboard
	}
}

func demoSeed() model {
	cfg := config.Default()
	cfg.BackupRoot = "~/TenuVault/backups"
	cfg.IncludeAssignments = true
	cfg.RetentionDays = 30
	m := New(cfg)
	m.connected = true
	m.conns = []connection.Connection{
		{Label: "Contoso Lab", Tenant: graph.Tenant{ID: "demo-source", DisplayName: "Contoso Lab", DefaultDomain: "contoso.onmicrosoft.com", DomainCount: 4}},
		{Label: "Fabrikam Test", Tenant: graph.Tenant{ID: "demo-target", DisplayName: "Fabrikam Test", DefaultDomain: "fabrikam.onmicrosoft.com", DomainCount: 2}},
	}
	m.sourceIdx = 0
	m.cfg.Connections = []config.ConnConfig{
		{Label: "Contoso Lab", TenantID: "demo-source", AuthMethod: config.AuthInteractive},
		{Label: "Fabrikam Test", TenantID: "demo-target", AuthMethod: config.AuthCertificate},
	}

	latest := demoBackup("backup-2026-07-10-143200", "2026-07-10-143200", 1204, "Completed")
	previous := demoBackup("backup-2026-07-09-020000", "2026-07-09-020000", 1198, "CompletedWithWarnings")
	older := demoBackup("backup-2026-07-08-020000", "2026-07-08-020000", 1191, "Completed")
	m.lastBackup = &latest
	m.backups = []store.Backup{latest, previous, older}
	m.detail = &latest
	m.detailCats = []catCount{
		{"DeviceConfigurations", 318},
		{"ConfigurationPolicies", 264},
		{"CompliancePolicies", 86},
		{"Intents", 142},
		{"DeviceManagementScripts", 64},
		{"ConditionalAccessPolicies", 45},
	}
	m.diffA, m.diffB = &previous, &latest
	m.diffChanges = []diff.Change{
		{Category: "Intents", Name: "Windows Defender Baseline", Type: diff.Modified, Severity: diff.Warning},
		{Category: "ConfigurationPolicies", Name: "Windows 11 Security Settings", Type: diff.Modified, Severity: diff.Warning},
		{Category: "CompliancePolicies", Name: "Corporate Windows Compliance", Type: diff.Added, Severity: diff.Info},
		{Category: "DeviceConfigurations", Name: "Legacy BitLocker Policy", Type: diff.Removed, Severity: diff.Warning},
		{Category: "ConditionalAccessPolicies", Name: "Require MFA for Admins", Type: diff.Modified, Severity: diff.Warning},
	}

	for _, pt := range catalog.All() {
		m.cats = append(m.cats, catSel{pt: pt, sel: pt.Verified})
	}
	m.restoreBackup = &latest
	m.restoreItems = []restoreSel{
		{item: restore.Item{Category: "Intents", Name: "Windows Defender Baseline"}, sel: true},
		{item: restore.Item{Category: "CompliancePolicies", Name: "Corporate Windows Compliance"}, sel: true},
		{item: restore.Item{Category: "ConfigurationPolicies", Name: "Windows 11 Security Settings"}},
		{item: restore.Item{Category: "ConditionalAccessPolicies", Name: "Require MFA for Admins"}, sel: true},
		{item: restore.Item{Category: "DeviceConfigurations", Name: "Shared Kiosk Configuration"}},
	}

	m.syncSourceConn = 0
	m.syncTargetConn = 1
	m.syncSourceLabel = "Contoso Lab"
	m.syncTypes = buildSyncTypes()
	for i := range m.syncTypes {
		m.syncTypes[i].loaded = true
		total := 8 + i%5
		for j := 0; j < total; j++ {
			m.syncTypes[i].policies = append(m.syncTypes[i].policies, syncPol{name: fmt.Sprintf("Demo policy %d", j+1), sel: i < 2})
		}
	}
	m.syncItems = []syncer.Item{{Category: "Intents", Name: "Windows Defender Baseline"}}
	return m
}

func demoBackup(folder, date string, total int, status string) store.Backup {
	counts := map[string]int{
		"DeviceConfigurations":      318,
		"ConfigurationPolicies":     264,
		"CompliancePolicies":        86,
		"Intents":                   142,
		"DeviceManagementScripts":   64,
		"ConditionalAccessPolicies": 45,
	}
	current := 0
	for _, count := range counts {
		current += count
	}
	counts["Other"] = max(0, total-current)
	return store.Backup{
		Folder: folder,
		Path:   "/demo/" + folder,
		Meta: store.Metadata{
			BackupDate: date,
			Duration:   "01:42",
			Status:     status,
			TenantID:   "demo-source",
			TenantName: "Contoso Lab",
			ItemCounts: counts,
		},
	}
}
