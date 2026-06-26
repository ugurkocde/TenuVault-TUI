// Package restore writes backed-up policies back to Microsoft Graph as new
// policies. The cleaning, routing, and create logic lives in policyops; restore
// is a thin wrapper that reads from a backup folder and prefixes "[Restored]".
package restore

import (
	"context"

	"github.com/ugurkocde/TenuVault-TUI/internal/graph"
	"github.com/ugurkocde/TenuVault-TUI/internal/policyops"
	"github.com/ugurkocde/TenuVault-TUI/internal/store"
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

// Plan builds restore items from a selection.
func Plan(b store.Backup, selection []Item) []Item { return selection }

// Restore creates each item in Graph and returns per-item results.
func Restore(ctx context.Context, c *graph.Client, items []Item) []Result {
	results := make([]Result, 0, len(items))
	for _, it := range items {
		res := Result{Item: it}
		raw, err := store.Read(it.Path)
		if err != nil {
			res.Err = err
			results = append(results, res)
			continue
		}
		res.NewID, res.Err = policyops.Create(ctx, c, it.Category, raw, RestorePrefix)
		results = append(results, res)
	}
	return results
}
