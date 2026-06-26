package tui

import (
	"context"
	"errors"

	tea "charm.land/bubbletea/v2"

	"github.com/ugurkocde/TenuVault-TUI/internal/auth"
	"github.com/ugurkocde/TenuVault-TUI/internal/backup"
	"github.com/ugurkocde/TenuVault-TUI/internal/catalog"
	"github.com/ugurkocde/TenuVault-TUI/internal/config"
	"github.com/ugurkocde/TenuVault-TUI/internal/diff"
	"github.com/ugurkocde/TenuVault-TUI/internal/graph"
	"github.com/ugurkocde/TenuVault-TUI/internal/jsonutil"
	"github.com/ugurkocde/TenuVault-TUI/internal/policyops"
	"github.com/ugurkocde/TenuVault-TUI/internal/restore"
	"github.com/ugurkocde/TenuVault-TUI/internal/store"
	"github.com/ugurkocde/TenuVault-TUI/internal/syncer"
)

// Async message types.
type (
	deviceCodeMsg string
	connectedMsg  struct {
		client *graph.Client
		tenant graph.Tenant
		cfg    config.Config
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
			client := graph.New(cred, auth.ScopesFor(cfg.AuthMethod))
			tenant, err := client.Organization(ctx)
			if err != nil {
				send(ctx, ch, errMsg{err})
				return
			}
			send(ctx, ch, connectedMsg{client: client, tenant: tenant, cfg: cfg})
		}()
		return nil
	}
}

// runBackup streams progress events and a final done message through ch.
func runBackup(ctx context.Context, c *graph.Client, types []catalog.PolicyType, opts backup.Options, ch chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		go func() {
			res, err := backup.Run(ctx, c, types, opts, func(e backup.Event) {
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

// syncPoliciesLoadedMsg carries the policies fetched for one type in the live
// source browser.
type syncPoliciesLoadedMsg struct {
	typeKey  string
	gen      int
	policies []syncPol
	err      error
}

type (
	syncEventMsg syncer.Event
	syncDoneMsg  struct{ results []syncer.Result }
)

var errNoSource = errors.New("no source tenant connected")

// loadSyncType lists a type's policies from the source tenant (lazy). gen is the
// sync generation the request was issued under, used to drop stale results.
func loadSyncType(ctx context.Context, c *graph.Client, pt catalog.PolicyType, gen int) tea.Cmd {
	return func() tea.Msg {
		if c == nil {
			return syncPoliciesLoadedMsg{typeKey: pt.Key, gen: gen, err: errNoSource}
		}
		items, err := c.ListAll(ctx, pt.Version, pt.ListPath, nil)
		if err != nil {
			return syncPoliciesLoadedMsg{typeKey: pt.Key, gen: gen, err: err}
		}
		pols := make([]syncPol, 0, len(items))
		for _, raw := range items {
			pols = append(pols, syncPol{
				name: jsonutil.DisplayName(raw, pt.NameField),
				id:   policyops.IDOf(raw),
				raw:  raw,
			})
		}
		return syncPoliciesLoadedMsg{typeKey: pt.Key, gen: gen, policies: pols}
	}
}

// runSync copies items into the target tenant, streaming progress.
func runSync(ctx context.Context, target, source *graph.Client, items []syncer.Item, prefix string, ch chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		go func() {
			results := syncer.Run(ctx, target, source, items, prefix, func(e syncer.Event) {
				send(ctx, ch, syncEventMsg(e))
			})
			send(ctx, ch, syncDoneMsg{results: results})
		}()
		return nil
	}
}

// cleanupBackups deletes backups older than the retention window.
func cleanupBackups(root string, days int) tea.Cmd {
	return func() tea.Msg {
		_, _ = store.Cleanup(root, days)
		return nil
	}
}

// runDiff compares two backups.
func runDiff(older, newer store.Backup) tea.Cmd {
	return func() tea.Msg {
		changes, err := diff.Compare(older, newer)
		return diffDoneMsg{changes: changes, err: err}
	}
}
