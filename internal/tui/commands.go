package tui

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/ugurkocde/TenuVault-TUI/internal/auth"
	"github.com/ugurkocde/TenuVault-TUI/internal/backup"
	"github.com/ugurkocde/TenuVault-TUI/internal/catalog"
	"github.com/ugurkocde/TenuVault-TUI/internal/config"
	"github.com/ugurkocde/TenuVault-TUI/internal/diff"
	"github.com/ugurkocde/TenuVault-TUI/internal/graph"
	"github.com/ugurkocde/TenuVault-TUI/internal/restore"
	"github.com/ugurkocde/TenuVault-TUI/internal/store"
)

// Async message types.
type (
	deviceCodeMsg string
	connectedMsg  struct {
		client *graph.Client
		tenant graph.Tenant
	}
	errMsg         struct{ err error }
	backupEventMsg backup.Event
	backupDoneMsg  struct {
		res backup.Result
		err error
	}
	restoreDoneMsg struct{ results []restore.Result }
	diffDoneMsg    struct {
		changes []diff.Change
		err     error
	}
	backupsLoadedMsg []store.Backup
)

// listen reads the next streamed message from the channel, unblocking if the
// context is cancelled (e.g. the user quit) so the command goroutine can exit.
func listen(ctx context.Context, ch chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		select {
		case msg := <-ch:
			return msg
		case <-ctx.Done():
			return nil
		}
	}
}

// send delivers a streamed message unless the context is cancelled, so producer
// goroutines never block forever on a full channel after the UI has stopped.
func send(ctx context.Context, ch chan tea.Msg, msg tea.Msg) {
	select {
	case ch <- msg:
	case <-ctx.Done():
	}
}

// connect builds the credential and client, then fetches the tenant. Device
// code prompts are streamed through ch and shown on the connecting screen.
func connect(ctx context.Context, cfg config.Config, ch chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		go func() {
			cred, err := auth.New(cfg, func(msg string) { send(ctx, ch, deviceCodeMsg(msg)) })
			if err != nil {
				send(ctx, ch, errMsg{err})
				return
			}
			client := graph.New(cred)
			tenant, err := client.Organization(ctx)
			if err != nil {
				send(ctx, ch, errMsg{err})
				return
			}
			send(ctx, ch, connectedMsg{client: client, tenant: tenant})
		}()
		return nil
	}
}

// runBackup streams progress events and a final done message through ch.
func runBackup(ctx context.Context, c *graph.Client, types []catalog.PolicyType, root string, tenant graph.Tenant, ch chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		go func() {
			res, err := backup.Run(ctx, c, types, root, tenant, func(e backup.Event) {
				send(ctx, ch, backupEventMsg(e))
			})
			send(ctx, ch, backupDoneMsg{res: res, err: err})
		}()
		return nil
	}
}

// runRestore performs the restore and returns the results.
func runRestore(ctx context.Context, c *graph.Client, items []restore.Item, ch chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		go func() {
			results := restore.Restore(ctx, c, items)
			send(ctx, ch, restoreDoneMsg{results: results})
		}()
		return nil
	}
}

// loadBackups lists local backups.
func loadBackups(root string) tea.Cmd {
	return func() tea.Msg {
		backups, err := store.List(root)
		if err != nil {
			return errMsg{err}
		}
		return backupsLoadedMsg(backups)
	}
}

// runDiff compares two backups.
func runDiff(older, newer store.Backup) tea.Cmd {
	return func() tea.Msg {
		changes, err := diff.Compare(older, newer)
		return diffDoneMsg{changes: changes, err: err}
	}
}
