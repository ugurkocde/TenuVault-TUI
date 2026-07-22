package tui

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ncruces/zenity"
)

func TestSettingsEnterOnLocationOpensPicker(t *testing.T) {
	m := New(testConfig(t))
	m.connected = true
	m.screen = screenSettings
	m.settingsCursor = 2

	next, cmd := m.keySettings("enter")
	got := next.(model)
	if !got.settingsPicking || cmd == nil {
		t.Fatalf("picking=%v cmd=%v, want picker command issued", got.settingsPicking, cmd)
	}

	// A second enter while the dialog is open must not spawn another one.
	next, cmd = got.keySettings("enter")
	if cmd != nil {
		t.Fatal("enter while picking should be ignored")
	}
	_ = next
}

func TestDirPickedCommitsAndReloads(t *testing.T) {
	t.Setenv("TENUVAULT_CONFIG_DIR", t.TempDir())
	m := New(testConfig(t))
	m.connected = true
	m.screen = screenSettings
	m.settingsPicking = true

	newRoot := filepath.Join(t.TempDir(), "picked")
	next, cmd := m.Update(dirPickedMsg{path: newRoot})
	got := next.(model)
	if got.settingsPicking {
		t.Fatal("picking flag should be cleared")
	}
	if got.cfg.BackupRoot != newRoot {
		t.Fatalf("BackupRoot=%q, want %q", got.cfg.BackupRoot, newRoot)
	}
	if cmd == nil {
		t.Fatal("commit should reload the snapshot list")
	}
}

func TestEscWhilePickingStaysOnSettings(t *testing.T) {
	m := New(testConfig(t))
	m.connected = true
	m.screen = screenSettings
	m.settingsPicking = true

	next, _ := m.handleKey("esc")
	got := next.(model)
	if got.screen != screenSettings {
		t.Fatalf("screen=%v, want settings while the picker dialog is open", got.screen)
	}
}

func TestStalePickerErrorOffSettingsDoesNotHijack(t *testing.T) {
	m := New(testConfig(t))
	m.connected = true
	m.screen = screenDashboard
	m.settingsPicking = true

	next, _ := m.Update(dirPickedMsg{err: errors.New("no display")})
	got := next.(model)
	if got.settingsEditing || got.settingsPicking {
		t.Fatalf("editing=%v picking=%v, want stale picker result dropped off the settings screen",
			got.settingsEditing, got.settingsPicking)
	}
}

func TestDirPickedCancelKeepsOldValue(t *testing.T) {
	m := New(testConfig(t))
	m.settingsPicking = true
	oldRoot := m.cfg.BackupRoot

	next, _ := m.Update(dirPickedMsg{err: zenity.ErrCanceled})
	got := next.(model)
	if got.cfg.BackupRoot != oldRoot || got.settingsPicking || got.settingsEditing {
		t.Fatalf("cancel must change nothing, got root=%q picking=%v editing=%v",
			got.cfg.BackupRoot, got.settingsPicking, got.settingsEditing)
	}
}

func TestDirPickedErrorFallsBackToInlineEdit(t *testing.T) {
	m := New(testConfig(t))
	m.screen = screenSettings
	m.settingsPicking = true

	next, _ := m.Update(dirPickedMsg{err: errors.New("no display")})
	got := next.(model)
	if !got.settingsEditing || got.settingsInput != got.cfg.BackupRoot {
		t.Fatalf("editing=%v input=%q, want inline edit prefilled with current root",
			got.settingsEditing, got.settingsInput)
	}
}

func TestSettingsInlineEditTypesAndCommits(t *testing.T) {
	t.Setenv("TENUVAULT_CONFIG_DIR", t.TempDir())
	m := New(testConfig(t))
	m.connected = true
	m.screen = screenSettings
	m.settingsEditing = true
	m.settingsInput = ""

	// q and ? must land in the input, not quit or open help.
	for _, k := range []string{"q", "?", "x"} {
		next, _ := m.handleKey(k)
		m = next.(model)
	}
	if m.settingsInput != "q?x" {
		t.Fatalf("input=%q, want %q", m.settingsInput, "q?x")
	}

	next, _ := m.handleKey("backspace")
	m = next.(model)
	if m.settingsInput != "q?" {
		t.Fatalf("input=%q after backspace, want %q", m.settingsInput, "q?")
	}

	m.settingsInput = filepath.Join(t.TempDir(), "typed")
	next, cmd := m.handleKey("enter")
	got := next.(model)
	if got.settingsEditing || got.cfg.BackupRoot != m.settingsInput || cmd == nil {
		t.Fatalf("editing=%v root=%q, want committed %q", got.settingsEditing, got.cfg.BackupRoot, m.settingsInput)
	}
}

func TestSettingsInlineEditEscCancels(t *testing.T) {
	m := New(testConfig(t))
	m.screen = screenSettings
	m.settingsEditing = true
	m.settingsInput = "/somewhere/else"
	oldRoot := m.cfg.BackupRoot

	next, _ := m.handleKey("esc")
	got := next.(model)
	if got.settingsEditing || got.cfg.BackupRoot != oldRoot || got.screen != screenSettings {
		t.Fatalf("esc must cancel edit only, got editing=%v root=%q screen=%v",
			got.settingsEditing, got.cfg.BackupRoot, got.screen)
	}
}

func TestSettingsRendersLocationRowAndStates(t *testing.T) {
	m := seededModel()
	m.screen = screenSettings

	out := m.viewSettings(88)
	if !strings.Contains(out, "Backup location") {
		t.Fatal("settings view missing the backup location row")
	}

	m.settingsPicking = true
	if out = m.viewSettings(88); !strings.Contains(out, "Choosing") {
		t.Fatal("settings view missing the picking indicator")
	}

	m.settingsPicking = false
	m.settingsEditing = true
	m.settingsInput = "/typed/path"
	if out = m.viewSettings(88); !strings.Contains(out, "/typed/path") {
		t.Fatal("settings view missing the in-progress input")
	}
}

func TestCommitEmptyInputRevertsToDefault(t *testing.T) {
	t.Setenv("TENUVAULT_CONFIG_DIR", t.TempDir())
	t.Setenv("HOME", t.TempDir()) // keep the default ~/TenuVault out of the real home
	m := New(testConfig(t))
	m.screen = screenSettings
	m.settingsEditing = true
	m.settingsInput = "   "

	next, _ := m.handleKey("enter")
	got := next.(model)
	if got.cfg.BackupRoot == "" || got.cfg.BackupRoot == "   " {
		t.Fatalf("empty input must revert to default, got %q", got.cfg.BackupRoot)
	}
}
