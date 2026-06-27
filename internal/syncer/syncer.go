// Package syncer copies policies into a target tenant — from a live source
// tenant or from a saved backup. It is create-only and never reads or modifies
// existing policies in the target, so it can never overwrite. All cleaning,
// routing, and the create call are delegated to policyops.
package syncer

import (
	"context"
	"encoding/json"

	"github.com/ugurkocde/TenuVault-TUI/internal/catalog"
	"github.com/ugurkocde/TenuVault-TUI/internal/graph"
	"github.com/ugurkocde/TenuVault-TUI/internal/policyops"
	"github.com/ugurkocde/TenuVault-TUI/internal/store"
)

// Item is one policy to copy. Exactly one source is set: Raw (+TypeKey) for a
// live source policy, or Path for a policy file from a saved backup.
type Item struct {
	Category string          // create-routing category
	TypeKey  string          // catalog key (live source, for full-content fetch)
	Name     string          // display name (for progress/results)
	Raw      json.RawMessage // live source list item
	Path     string          // backup source file path
}

// Result reports the outcome of copying one item.
type Result struct {
	Item  Item
	NewID string
	Err   error
}

// Event reports sync progress.
type Event struct {
	Current int
	Total   int
	Name    string
}

// Run creates each item in the target tenant. For live items, source must be
// the source tenant's client (used to fetch full policy content); it may be nil
// when every item comes from a backup. namePrefix is prepended to copied policy
// names ("" keeps the original name).
func Run(ctx context.Context, target, source graph.API, items []Item, namePrefix string, progress func(Event)) []Result {
	results := make([]Result, 0, len(items))
	total := len(items)
	for i, it := range items {
		if err := ctx.Err(); err != nil {
			return results
		}
		if progress != nil {
			progress(Event{Current: i + 1, Total: total, Name: it.Name})
		}
		res := Result{Item: it}

		raw, err := resolve(ctx, source, it)
		if err != nil {
			res.Err = err
			results = append(results, res)
			continue
		}
		res.NewID, res.Err = policyops.Create(ctx, target, it.Category, raw, namePrefix)
		results = append(results, res)
	}
	return results
}

// resolve returns the full policy JSON for an item from its source.
func resolve(ctx context.Context, source graph.API, it Item) (json.RawMessage, error) {
	if it.Path != "" {
		return store.Read(it.Path)
	}
	if pt, ok := catalog.ByKey(it.TypeKey); ok && source != nil {
		full, _ := policyops.FetchFull(ctx, source, pt, it.Raw, false)
		return full, nil
	}
	return it.Raw, nil
}
