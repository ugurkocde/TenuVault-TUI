// Package cli implements the headless (non-TUI) subcommands so backups and
// restores can run in automation/CI. Headless runs use app-registration
// credentials (set AZURE_TENANT_ID, AZURE_CLIENT_ID, and AZURE_CLIENT_SECRET or
// a certificate) since interactive sign-in needs a browser.
package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/ugurkocde/TenuVault-TUI/internal/auth"
	"github.com/ugurkocde/TenuVault-TUI/internal/backup"
	"github.com/ugurkocde/TenuVault-TUI/internal/catalog"
	"github.com/ugurkocde/TenuVault-TUI/internal/config"
	"github.com/ugurkocde/TenuVault-TUI/internal/diff"
	"github.com/ugurkocde/TenuVault-TUI/internal/graph"
	"github.com/ugurkocde/TenuVault-TUI/internal/jsonutil"
	"github.com/ugurkocde/TenuVault-TUI/internal/policyops"
	"github.com/ugurkocde/TenuVault-TUI/internal/store"
	"github.com/ugurkocde/TenuVault-TUI/internal/syncer"
)

// connect builds an authenticated Graph client and fetches the tenant.
func connect(ctx context.Context, cfg config.Config) (graph.API, graph.Tenant, error) {
	cred, err := auth.New(cfg)
	if err != nil {
		return nil, graph.Tenant{}, err
	}
	c := graph.New(cred, auth.ScopesFor(cfg.AuthMethod))
	tenant, err := c.Organization(ctx)
	if err != nil {
		return nil, graph.Tenant{}, err
	}
	return c, tenant, nil
}

func commandContext() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
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
	types, err := selectedTypes(*categories)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 2
	}
	ctx, cancel := commandContext()
	defer cancel()
	c, tenant, err := connect(ctx, cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}

	res, err := backup.Run(ctx, c, types,
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
	return backupExitCode(res.Status)
}

func backupExitCode(status string) int {
	switch status {
	case "Cancelled":
		return 130
	case "Failed":
		return 1
	default:
		return 0
	}
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

	items, err := backupItems(*path, *categories)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 2
	}
	if len(items) == 0 {
		fmt.Fprintln(os.Stderr, "nothing to restore")
		return 1
	}
	ctx, cancel := commandContext()
	defer cancel()
	cfg := config.Load()
	c, _, err := connect(ctx, cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}

	results := syncer.Run(ctx, c, nil, items, *prefix, nil)
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

// Verify checks a backup folder against its recorded checksums. Exit codes:
// 0 intact, 1 integrity failure, 2 usage/read error (including backups from
// versions that predate checksum support).
func Verify(args []string) int {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: tenuvault verify <backup-dir>")
		return 2
	}
	rep, err := store.Verify(fs.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 2
	}
	for _, f := range rep.Mismatched {
		fmt.Printf("MISMATCH %s\n", f)
	}
	for _, f := range rep.Missing {
		fmt.Printf("MISSING  %s\n", f)
	}
	for _, f := range rep.Extra {
		fmt.Printf("EXTRA    %s\n", f)
	}
	if !rep.Clean() {
		fmt.Printf("%d ok, %d mismatched, %d missing, %d extra\n",
			rep.OK, len(rep.Mismatched), len(rep.Missing), len(rep.Extra))
		return 1
	}
	fmt.Printf("verified %d files\n", rep.OK)
	return 0
}

// Sync copies policies into the target tenant (authenticated via the AZURE_*
// env vars, exactly like restore - the write side always uses the default
// credentials). The source is either a saved backup (--from-backup) or a live
// tenant supplied via TENUVAULT_SOURCE_TENANT_ID, TENUVAULT_SOURCE_CLIENT_ID,
// and TENUVAULT_SOURCE_CLIENT_SECRET (or TENUVAULT_SOURCE_CERT_PATH plus
// TENUVAULT_SOURCE_CERT_PASSWORD). Exit codes: 0 ok, 1 failures, 2 usage/auth.
func Sync(args []string) int {
	fs := flag.NewFlagSet("sync", flag.ContinueOnError)
	fromBackup := fs.String("from-backup", "", "backup folder to copy from (omit to sync from a TENUVAULT_SOURCE_* live tenant)")
	categories := fs.String("categories", "", "comma-separated policy type keys (default: all restore-supported)")
	names := fs.String("names", "", "comma-separated exact display names to include (default: all)")
	prefix := fs.String("prefix", "[Synced] ", "name prefix for created policies (\"\" keeps original)")
	dryRun := fs.Bool("dry-run", false, "print what would be created without writing anything")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	srcCfg, haveLive := sourceConfigFromEnv()
	if (*fromBackup != "") == haveLive {
		fmt.Fprintln(os.Stderr, "error: provide exactly one source: --from-backup <dir>, or TENUVAULT_SOURCE_* env vars for a live tenant")
		return 2
	}
	types, err := syncTypes(*categories)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 2
	}
	ctx, cancel := commandContext()
	defer cancel()

	var (
		items          []syncer.Item
		source         graph.API
		sourceTenantID string
	)
	if *fromBackup != "" {
		items, err = backupItems(*fromBackup, *categories)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			return 2
		}
		// The manifest records which tenant the backup came from; used below to
		// refuse an accidental same-tenant duplicate without a prefix.
		if !*dryRun {
			data, readErr := os.ReadFile(filepath.Join(*fromBackup, "metadata.json"))
			if readErr != nil {
				fmt.Fprintln(os.Stderr, "error: read backup metadata:", readErr)
				return 2
			}
			var meta store.Metadata
			if err := json.Unmarshal(data, &meta); err != nil {
				fmt.Fprintln(os.Stderr, "error: decode backup metadata:", err)
				return 2
			}
			sourceTenantID = meta.TenantID
		}
	} else {
		c, tenant, err := connect(ctx, srcCfg)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error: source auth failed:", err)
			return 1
		}
		source, sourceTenantID = c, tenant.ID
		for _, pt := range types {
			raws, err := c.ListAll(ctx, pt.Version, pt.ListPath, nil)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error: list %s: %v\n", pt.Key, err)
				return 1
			}
			for _, raw := range raws {
				items = append(items, syncer.Item{Category: pt.Category, TypeKey: pt.Key, Name: jsonutil.DisplayName(raw, pt.NameField), Raw: raw})
			}
		}
	}
	items = filterByNames(items, *names)
	if len(items) == 0 {
		fmt.Fprintln(os.Stderr, "nothing to sync")
		return 1
	}

	if *dryRun {
		for _, it := range items {
			fmt.Printf("+ %s%s · %s\n", *prefix, it.Name, it.Category)
		}
		fmt.Printf("%d policies would be created (dry run, nothing written)\n", len(items))
		return 0
	}

	target, targetTenant, err := connect(ctx, config.Load())
	if err != nil {
		fmt.Fprintln(os.Stderr, "error: target auth failed:", err)
		return 1
	}
	if *prefix == "" {
		if sourceTenantID == "" {
			fmt.Fprintln(os.Stderr, "error: backup source tenant is unknown; use --prefix to avoid unverified duplicates")
			return 2
		}
		if sourceTenantID == targetTenant.ID {
			fmt.Fprintln(os.Stderr, "error: source and target are the same tenant; refusing to duplicate policies without --prefix")
			return 2
		}
	}

	results := syncer.Run(ctx, target, source, items, *prefix, nil)
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
	fmt.Printf("synced %d, %d failed\n", ok, fail)
	if fail > 0 {
		return 1
	}
	return 0
}

// sourceConfigFromEnv builds the sync-source credentials from the
// TENUVAULT_SOURCE_* environment. The bool reports whether a live source was
// configured at all.
func sourceConfigFromEnv() (config.Config, bool) {
	tenant := os.Getenv("TENUVAULT_SOURCE_TENANT_ID")
	if tenant == "" {
		return config.Config{}, false
	}
	c := config.Config{
		TenantID:            tenant,
		ClientID:            os.Getenv("TENUVAULT_SOURCE_CLIENT_ID"),
		ClientSecret:        os.Getenv("TENUVAULT_SOURCE_CLIENT_SECRET"),
		CertificatePath:     os.Getenv("TENUVAULT_SOURCE_CERT_PATH"),
		CertificatePassword: os.Getenv("TENUVAULT_SOURCE_CERT_PASSWORD"),
	}
	if c.CertificatePath != "" {
		c.AuthMethod = config.AuthCertificate
	} else {
		c.AuthMethod = config.AuthSecret
	}
	return c, true
}

// syncTypes resolves the policy types eligible for sync: the requested keys
// (or every type when unset), restricted to restore-supported ones.
func syncTypes(csv string) ([]catalog.PolicyType, error) {
	var out []catalog.PolicyType
	if strings.TrimSpace(csv) == "" {
		for _, pt := range catalog.All() {
			if pt.RestoreSupported {
				out = append(out, pt)
			}
		}
		return out, nil
	}
	selected, err := selectedTypes(csv)
	if err != nil {
		return nil, err
	}
	for _, pt := range selected {
		if pt.RestoreSupported {
			out = append(out, pt)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("none of the requested categories support restore or sync")
	}
	return out, nil
}

// filterByNames keeps only items whose display name is in the comma-separated
// list (exact match); an empty list keeps everything.
func filterByNames(items []syncer.Item, csv string) []syncer.Item {
	if strings.TrimSpace(csv) == "" {
		return items
	}
	want := map[string]bool{}
	for _, n := range strings.Split(csv, ",") {
		want[strings.TrimSpace(n)] = true
	}
	var out []syncer.Item
	for _, it := range items {
		if want[it.Name] {
			out = append(out, it)
		}
	}
	return out
}

// selectedTypes resolves a comma-separated key list, defaulting to verified.
func selectedTypes(csv string) ([]catalog.PolicyType, error) {
	if strings.TrimSpace(csv) == "" {
		var out []catalog.PolicyType
		for _, pt := range catalog.All() {
			if pt.Verified {
				out = append(out, pt)
			}
		}
		return out, nil
	}
	var out []catalog.PolicyType
	var unknown []string
	seen := map[string]bool{}
	for _, k := range strings.Split(csv, ",") {
		k = strings.TrimSpace(k)
		if pt, ok := catalog.ByKey(k); ok {
			if !seen[pt.Key] {
				out = append(out, pt)
				seen[pt.Key] = true
			}
		} else {
			unknown = append(unknown, k)
		}
	}
	if len(unknown) > 0 {
		return nil, fmt.Errorf("unknown category keys: %s", strings.Join(unknown, ", "))
	}
	return out, nil
}

// backupItems builds restore items from a backup folder, restricted to
// restore-supported categories (and an optional key filter).
func backupItems(path, csv string) ([]syncer.Item, error) {
	b := store.Backup{Path: path}
	allow := map[string]bool{}
	allowKey := map[string]bool{}
	selected, err := selectedTypes(csv)
	if err != nil {
		return nil, err
	}
	for _, pt := range selected {
		allow[pt.Category] = true
		allowKey[pt.Key] = true
	}
	filtering := strings.TrimSpace(csv) != ""

	cats, err := b.Categories()
	if err != nil {
		return nil, fmt.Errorf("list backup categories: %w", err)
	}
	var items []syncer.Item
	for _, cat := range cats {
		if !catalog.CategoryRestoreSupported(cat) {
			continue
		}
		if filtering && !allow[cat] {
			continue
		}
		files, err := b.Policies(cat)
		if err != nil {
			return nil, fmt.Errorf("list policies in %s: %w", cat, err)
		}
		for _, f := range files {
			if filtering && sharedCategory(cat) {
				raw, err := store.Read(f.Path)
				if err != nil {
					return nil, fmt.Errorf("read %s: %w", f.Path, err)
				}
				key, ok := policyops.ResolveTypeKey(cat, raw)
				if !ok {
					return nil, fmt.Errorf("resolve policy type for %s", f.Path)
				}
				if !allowKey[key] {
					continue
				}
			}
			items = append(items, syncer.Item{Category: cat, Name: f.Name, Path: f.Path})
		}
	}
	return items, nil
}

func sharedCategory(category string) bool {
	n := 0
	for _, pt := range catalog.All() {
		if pt.Category == category {
			n++
		}
	}
	return n > 1
}
