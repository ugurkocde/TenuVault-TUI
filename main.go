// Command tenuvault is a terminal UI for backing up and restoring Microsoft
// Intune configuration via Microsoft Graph.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/ugurkocde/TenuVault-TUI/internal/cli"
	"github.com/ugurkocde/TenuVault-TUI/internal/config"
	"github.com/ugurkocde/TenuVault-TUI/internal/tui"
)

// version is overridden at release time via -ldflags "-X main.version=...".
var version = "1.0.0"

func main() {
	// Headless subcommands for automation/CI; no args launches the TUI.
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "backup":
			os.Exit(cli.Backup(os.Args[2:]))
		case "restore":
			os.Exit(cli.Restore(os.Args[2:]))
		}
	}

	var (
		showVersion bool
		tenant      string
		backupRoot  string
	)
	flag.BoolVar(&showVersion, "version", false, "print version and exit")
	flag.StringVar(&tenant, "tenant", "", "tenant id or domain (overrides config)")
	flag.StringVar(&backupRoot, "backup-root", "", "directory to store backups (overrides config)")
	flag.Parse()

	if showVersion {
		fmt.Printf("tenuvault %s\n", version)
		return
	}

	cfg := config.Load()
	if tenant != "" {
		cfg.TenantID = tenant
	}
	if backupRoot != "" {
		cfg.BackupRoot = backupRoot
	}

	if _, err := tui.Program(cfg).Run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
