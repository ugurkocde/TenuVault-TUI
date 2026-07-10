//go:build tui_demo

// Command tuidemo runs a fictional, write-disabled TenuVault session for
// documentation screenshots and recordings.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/ugurkocde/TenuVault-TUI/internal/tui"
)

func main() {
	var screen string
	var flow bool
	flag.StringVar(&screen, "screen", "dashboard", "dashboard, backup, browse, compare, restore, sync, tenants, or settings")
	flag.BoolVar(&flow, "first-backup", false, "run the safe first-backup documentation flow")
	flag.Parse()

	if _, err := tui.DemoProgram(screen, flow).Run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
