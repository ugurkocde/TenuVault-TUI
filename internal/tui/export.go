package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	tea "charm.land/bubbletea/v2"
)

// The result screens (compare, restore, sync) are view-only; "e" exports the
// data behind them as a timestamped JSON report in the backup root so drift
// and failure details can be audited or fed into other tooling.

type exportDoneMsg struct {
	path string
	err  error
}

// exportReport writes payload as pretty JSON into the backup root and reports
// the final path (or the failure) back to the model.
func exportReport(root, kind string, payload any) tea.Cmd {
	return func() tea.Msg {
		if err := os.MkdirAll(root, 0o755); err != nil {
			return exportDoneMsg{err: err}
		}
		data, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			return exportDoneMsg{err: err}
		}
		path := filepath.Join(root, fmt.Sprintf("tenuvault-%s-%s.json", kind, time.Now().Format("2006-01-02-150405")))
		if err := os.WriteFile(path, data, 0o644); err != nil {
			return exportDoneMsg{err: err}
		}
		return exportDoneMsg{path: path}
	}
}

// diffReportDoc is the exported shape of a compare run.
type diffReportDoc struct {
	Kind        string             `json:"kind"`
	Baseline    string             `json:"baseline"`
	Target      string             `json:"target"`
	GeneratedAt string             `json:"generatedAt"`
	Changes     []diffReportChange `json:"changes"`
}

type diffReportChange struct {
	Category string `json:"category"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	Severity string `json:"severity"`
}

func (m model) diffReport() diffReportDoc {
	older, newer := orderBackups(m.diffA, m.diffB)
	changes := make([]diffReportChange, 0, len(m.diffChanges))
	for _, c := range m.diffChanges {
		changes = append(changes, diffReportChange{
			Category: c.Category,
			Name:     c.Name,
			Type:     string(c.Type),
			Severity: string(c.Severity),
		})
	}
	return diffReportDoc{
		Kind:        "diff",
		Baseline:    older.Folder,
		Target:      newer.Folder,
		GeneratedAt: time.Now().Format(time.RFC3339),
		Changes:     changes,
	}
}

// resultReportDoc is the exported shape of a restore or sync run.
type resultReportDoc struct {
	Kind        string             `json:"kind"`
	Target      string             `json:"target,omitempty"`
	NamePrefix  string             `json:"namePrefix,omitempty"`
	GeneratedAt string             `json:"generatedAt"`
	Results     []resultReportItem `json:"results"`
}

type resultReportItem struct {
	Category       string `json:"category"`
	Name           string `json:"name"`
	NewID          string `json:"newId,omitempty"`
	Error          string `json:"error,omitempty"`
	PartialContent bool   `json:"partialContent,omitempty"`
}

func (m model) restoreReport() resultReportDoc {
	results := make([]resultReportItem, 0, len(m.restoreResults))
	for _, r := range m.restoreResults {
		item := resultReportItem{Category: r.Item.Category, Name: r.Item.Name, NewID: r.NewID}
		if r.Err != nil {
			item.Error = r.Err.Error()
		}
		results = append(results, item)
	}
	return resultReportDoc{
		Kind:        "restore",
		Target:      m.sourceTenant().DisplayName,
		GeneratedAt: time.Now().Format(time.RFC3339),
		Results:     results,
	}
}

func (m model) syncReport() resultReportDoc {
	target := ""
	if m.syncTargetConn >= 0 && m.syncTargetConn < len(m.conns) {
		target = m.conns[m.syncTargetConn].DisplayLabel()
	}
	results := make([]resultReportItem, 0, len(m.syncResults))
	for _, r := range m.syncResults {
		item := resultReportItem{Category: r.Item.Category, Name: r.Item.Name, NewID: r.NewID, PartialContent: r.Warn}
		if r.Err != nil {
			item.Error = r.Err.Error()
		}
		results = append(results, item)
	}
	return resultReportDoc{
		Kind:        "sync",
		Target:      target,
		NamePrefix:  m.syncNamePrefix,
		GeneratedAt: time.Now().Format(time.RFC3339),
		Results:     results,
	}
}
