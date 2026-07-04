// Package syncer copies policies into a target tenant — from a live source
// tenant or from a saved backup. It is create-only and never reads or modifies
// existing policies in the target, so it can never overwrite. All cleaning,
// routing, and the create call are delegated to policyops.
package syncer

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/ugurkocde/TenuVault-TUI/internal/catalog"
	"github.com/ugurkocde/TenuVault-TUI/internal/graph"
	"github.com/ugurkocde/TenuVault-TUI/internal/policyops"
	"github.com/ugurkocde/TenuVault-TUI/internal/store"
)

// syncWorkers bounds concurrent policy creates. Intune's write limits are
// tighter than its read limits, so this must not exceed the backup pool size;
// a 429/503/504 on a POST is retried with Retry-After by the graph client
// (Graph rejected the request without applying it, so retrying cannot
// double-create). Multi-step creates (e.g. admin templates, intents) run all
// their steps inside policyops.Create, so item-level parallelism never
// interleaves one item's own steps.
const syncWorkers = 4

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
	// Warn is set when the item was created but its content could not be fully
	// resolved from the source (e.g. a sub-resource fetch failed), so the copy
	// may be incomplete.
	Warn bool
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
//
// Items are created by a bounded worker pool; results come back in input
// order regardless of completion order. Progress events report the completed
// count and are never emitted concurrently. On cancellation every item still
// gets a result: items never attempted carry Err = context.Canceled.
func Run(ctx context.Context, target, source graph.API, items []Item, namePrefix string, progress func(Event)) []Result {
	total := len(items)
	results := make([]Result, total)
	started := make([]bool, total)

	var mu sync.Mutex
	done := 0
	jobs := make(chan int)
	var wg sync.WaitGroup
	workers := syncWorkers
	if total < workers {
		workers = total
	}
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				it := items[i]
				res := Result{Item: it}
				if err := ctx.Err(); err != nil {
					res.Err = err
				} else if raw, warn, err := resolve(ctx, source, it); err != nil {
					res.Err = err
				} else {
					res.Warn = warn
					res.NewID, res.Err = policyops.Create(ctx, target, it.Category, raw, namePrefix)
				}
				mu.Lock()
				results[i] = res
				started[i] = true
				done++
				if progress != nil {
					progress(Event{Current: done, Total: total, Name: it.Name})
				}
				mu.Unlock()
			}
		}()
	}
feed:
	for i := range items {
		select {
		case jobs <- i:
		case <-ctx.Done():
			break feed
		}
	}
	close(jobs)
	wg.Wait()

	// Items the feed never handed out (cancelled mid-run) are reported
	// honestly instead of silently dropped.
	for i := range results {
		if !started[i] {
			results[i] = Result{Item: items[i], Err: context.Canceled}
		}
	}
	return results
}

// resolve returns the full policy JSON for an item from its source. The bool is
// true when the source content could not be fully enriched (the copy may be
// incomplete) so callers can surface a warning while still creating the policy.
func resolve(ctx context.Context, source graph.API, it Item) (json.RawMessage, bool, error) {
	if it.Path != "" {
		raw, err := store.Read(it.Path)
		return raw, false, err
	}
	if pt, ok := catalog.ByKey(it.TypeKey); ok && source != nil {
		full, warn := policyops.FetchFull(ctx, source, pt, it.Raw, false)
		return full, warn, nil
	}
	return it.Raw, false, nil
}
