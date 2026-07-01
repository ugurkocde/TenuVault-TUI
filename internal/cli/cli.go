// Package cli implements the headless (non-TUI) subcommands so backups and
// restores can run in automation/CI. Headless runs use app-registration
// credentials (set AZURE_TENANT_ID, AZURE_CLIENT_ID, and AZURE_CLIENT_SECRET or
// a certificate) since interactive sign-in needs a browser.
package cli

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/ugurkocde/TenuVault-TUI/internal/auth"
	"github.com/ugurkocde/TenuVault-TUI/internal/backup"
	"github.com/ugurkocde/TenuVault-TUI/internal/catalog"
	"github.com/ugurkocde/TenuVault-TUI/internal/config"
	"github.com/ugurkocde/TenuVault-TUI/internal/diff"
	"github.com/ugurkocde/TenuVault-TUI/internal/graph"
	"github.com/ugurkocde/TenuVault-TUI/internal/store"
	"github.com/ugurkocde/TenuVault-TUI/internal/syncer"
)

// connect builds an authenticated Graph client and fetches the tenant.
func connect(cfg config.Config) (graph.API, graph.Tenant, error) {
	cred, err := auth.New(cfg)
	if err != nil {
		return nil, graph.Tenant{}, err
	}
	c := graph.New(cred, auth.ScopesFor(cfg.AuthMethod))
	tenant, err := c.Organization(context.Background())
	if err != nil {
		return nil, graph.Tenant{}, err
	}
	return c, tenant, nil
}

// Backup runs a headless backup. Returns a process exit code.
func Backup(args []string) int {
	fs := flag.NewFlagSet("backup", flag.ContinueOnError)
	categories := fs.String("categories", "", "comma-separated policy type keys (default: all verified)")
	out := fs.String("out", "", "backup root directory (default: config backupRoot)")
	assignments := fs.Bool("assignments", false, "include assignments in the backup")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	cfg := config.Load()
	root := cfg.BackupRoot
	if *out != "" {
		root = *out
	}
	c, tenant, err := connect(cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}

	res, err := backup.Run(context.Background(), c, selectedTypes(*categories),
		backup.Options{Root: root, Tenant: tenant, IncludeAssignments: *assignments},
		func(e backup.Event) {
			if e.Done {
				if e.Failed {
					fmt.Fprintf(os.Stderr, "  FAILED %s: %v\n", e.Category, e.Err)
				} else {
					fmt.Fprintf(os.Stderr, "  ok %s (%d)\n", e.Category, e.Total)
				}
			}
		})
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	total := 0
	for _, n := range res.ItemCounts {
		total += n
	}
	fmt.Printf("%s: %d policies in %s\n", res.Status, total, res.Path)
	if res.Status == "Failed" {
		return 1
	}
	return 0
}

// Restore runs a headless restore of a backup folder into the connected tenant.
func Restore(args []string) int {
	fs := flag.NewFlagSet("restore", flag.ContinueOnError)
	path := fs.String("backup", "", "path to a backup folder (required)")
	categories := fs.String("categories", "", "comma-separated policy type keys to restore (default: all)")
	prefix := fs.String("prefix", "[Restored] ", "name prefix for created policies (\"\" keeps original)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *path == "" {
		fmt.Fprintln(os.Stderr, "error: --backup is required")
		return 2
	}

	cfg := config.Load()
	c, _, err := connect(cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}

	items := backupItems(*path, *categories)
	if len(items) == 0 {
		fmt.Fprintln(os.Stderr, "nothing to restore")
		return 1
	}
	results := syncer.Run(context.Background(), c, nil, items, *prefix, nil)
	ok, fail := 0, 0
	for _, r := range results {
		if r.Err != nil {
			fail++
			fmt.Fprintf(os.Stderr, "  ✗ %s: %v\n", r.Item.Name, r.Err)
		} else {
			ok++
			if r.Warn {
				fmt.Fprintf(os.Stderr, "  ⚠ %s: created with partial content\n", r.Item.Name)
			}
		}
	}
	fmt.Printf("restored %d, %d failed\n", ok, fail)
	if fail > 0 {
		return 1
	}
	return 0
}

// Compare diffs two backup folders and prints the drift, one change per line.
// Exit codes: 0 no drift, 1 drift found, 2 usage/read error. Designed for CI
// drift gates: `tenuvault compare <older> <newer> && echo "no drift"`.
func Compare(args []string) int {
	fs := flag.NewFlagSet("compare", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 2 {
		fmt.Fprintln(os.Stderr, "usage: tenuvault compare <older-backup-dir> <newer-backup-dir>")
		return 2
	}
	older := store.Backup{Path: fs.Arg(0), Folder: fs.Arg(0)}
	newer := store.Backup{Path: fs.Arg(1), Folder: fs.Arg(1)}
	for _, b := range []store.Backup{older, newer} {
		if _, err := b.Categories(); err != nil {
			fmt.Fprintf(os.Stderr, "error: cannot read backup %s: %v\n", b.Path, err)
			return 2
		}
	}
	changes, err := diff.Compare(older, newer)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 2
	}
	if len(changes) == 0 {
		fmt.Println("no drift")
		return 0
	}
	for _, c := range changes {
		fmt.Printf("%-8s %-8s %s / %s\n", c.Severity, c.Type, c.Category, c.Name)
	}
	fmt.Printf("%d changes\n", len(changes))
	return 1
}

// selectedTypes resolves a comma-separated key list, defaulting to verified.
func selectedTypes(csv string) []catalog.PolicyType {
	if strings.TrimSpace(csv) == "" {
		var out []catalog.PolicyType
		for _, pt := range catalog.All() {
			if pt.Verified {
				out = append(out, pt)
			}
		}
		return out
	}
	var out []catalog.PolicyType
	for _, k := range strings.Split(csv, ",") {
		if pt, ok := catalog.ByKey(strings.TrimSpace(k)); ok {
			out = append(out, pt)
		}
	}
	return out
}

// backupItems builds restore items from a backup folder, restricted to
// restore-supported categories (and an optional key filter).
func backupItems(path, csv string) []syncer.Item {
	b := store.Backup{Path: path}
	allow := map[string]bool{}
	for _, pt := range selectedTypes(csv) {
		allow[pt.Category] = true
	}
	filtering := strings.TrimSpace(csv) != ""

	cats, _ := b.Categories()
	var items []syncer.Item
	for _, cat := range cats {
		if !catalog.CategoryRestoreSupported(cat) {
			continue
		}
		if filtering && !allow[cat] {
			continue
		}
		files, _ := b.Policies(cat)
		for _, f := range files {
			items = append(items, syncer.Item{Category: cat, Name: f.Name, Path: f.Path})
		}
	}
	return items
}
