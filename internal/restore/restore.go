// Package restore writes backed-up policies back to Microsoft Graph as new
// policies. The cleaning, routing, and create logic lives in policyops; restore
// is a thin wrapper that reads from a backup folder and prefixes "[Restored]".
package restore

import (
	"context"

	"github.com/ugurkocde/TenuVault-TUI/internal/graph"
	"github.com/ugurkocde/TenuVault-TUI/internal/syncer"
)

// RestorePrefix is prepended to restored policy names so they are obvious and
// never collide with the originals.
const RestorePrefix = "[Restored] "

// Item is one policy selected for restore.
type Item struct {
	Category string
	Name     string
	Path     string
}

// Result reports the outcome of restoring one item.
type Result struct {
	Item  Item
	NewID string
	Err   error
}

// Restore creates each item in Graph and returns per-item results in input
// order. It delegates to the syncer's bounded worker pool (a restore is a
// backup-sourced sync into the same tenant with the restore prefix).
func Restore(ctx context.Context, c graph.API, items []Item) []Result {
	sitems := make([]syncer.Item, len(items))
	for i, it := range items {
		sitems[i] = syncer.Item{Category: it.Category, Name: it.Name, Path: it.Path}
	}
	sres := syncer.Run(ctx, c, nil, sitems, RestorePrefix, nil)
	results := make([]Result, len(sres))
	for i, r := range sres {
		results[i] = Result{Item: items[i], NewID: r.NewID, Err: r.Err}
	}
	return results
}
