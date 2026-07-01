// Package backup enumerates and writes Intune policies to a TenuVault-format
// backup folder. It preserves verbatim Graph JSON (only dropping @odata.context)
// so policies restore cleanly, fetches per-item detail and nested sub-resources
// where required (script content, settings catalog settings, admin-template
// definition values, etc.), and writes a metadata.json manifest plus a
// backup.log that records the outcome of every category.
package backup

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/ugurkocde/TenuVault-TUI/internal/catalog"
	"github.com/ugurkocde/TenuVault-TUI/internal/graph"
	"github.com/ugurkocde/TenuVault-TUI/internal/jsonutil"
	"github.com/ugurkocde/TenuVault-TUI/internal/policyops"
	"github.com/ugurkocde/TenuVault-TUI/internal/store"
)

// RunbookVersion is recorded in the manifest for portal cross-compatibility.
const RunbookVersion = "tui-1.0.0"

// backupWorkers bounds per-item detail fetches within a category. The graph
// client already retries 429/503/504 with Retry-After, so this level of
// concurrency stays within Graph throttling limits.
const backupWorkers = 4

// Options configures a backup run.
type Options struct {
	Root               string
	Tenant             graph.Tenant
	IncludeAssignments bool
}

// Event reports backup progress for one category, suitable for a TUI.
type Event struct {
	Category string
	Friendly string
	Current  int
	Total    int
	Done     bool
	Failed   bool // category-level list failure
	Err      error
}

// CategoryResult is the per-category outcome recorded in the manifest/log.
type CategoryResult struct {
	Category string `json:"category"`
	Friendly string `json:"friendly"`
	Count    int    `json:"count"`
	Warnings int    `json:"warnings"`
	Failed   bool   `json:"failed"`
	Error    string `json:"error,omitempty"`
}

// Result summarizes a completed backup.
type Result struct {
	Folder     string
	Path       string
	Status     string
	ItemCounts map[string]int
	Categories []CategoryResult
}

// Run backs up the selected policy types into the configured root, emitting
// progress events. It never aborts the whole run on a single category failure;
// each category's outcome is captured in the result and the on-disk log.
// Item detail is fetched concurrently within each category; progress calls are
// serialized (never concurrent) and Current is monotonic per category. If ctx
// is cancelled mid-run, everything written so far is kept and the manifest is
// finalized with Status "Cancelled".
func Run(ctx context.Context, c graph.API, types []catalog.PolicyType, opts Options, progress func(Event)) (Result, error) {
	start := time.Now()
	folder := "backup-" + start.Format("2006-01-02-150405")
	path := filepath.Join(opts.Root, folder)
	if err := os.MkdirAll(path, 0o755); err != nil {
		return Result{}, fmt.Errorf("create backup folder: %w", err)
	}

	counts := map[string]int{}
	catResults := map[string]*CategoryResult{}
	var order []string
	emit := func(e Event) {
		if progress != nil {
			progress(e)
		}
	}
	result := func(pt catalog.PolicyType) *CategoryResult {
		if r, ok := catResults[pt.Category]; ok {
			return r
		}
		r := &CategoryResult{Category: pt.Category, Friendly: pt.Friendly}
		catResults[pt.Category] = r
		order = append(order, pt.Category)
		return r
	}

	cancelled := false
loop:
	for _, pt := range types {
		if ctx.Err() != nil {
			cancelled = true
			break
		}
		cr := result(pt)
		items, err := c.ListAll(ctx, pt.Version, pt.ListPath, nil)
		if err != nil {
			if ctx.Err() != nil {
				cancelled = true
				break
			}
			cr.Failed = true
			cr.Error = condense(err.Error())
			emit(Event{Category: pt.Category, Friendly: pt.Friendly, Err: err, Failed: true, Done: true})
			continue
		}
		catDir := filepath.Join(path, pt.Category)
		if err := os.MkdirAll(catDir, 0o755); err != nil {
			cr.Failed = true
			cr.Error = condense(err.Error())
			emit(Event{Category: pt.Category, Friendly: pt.Friendly, Err: err, Failed: true, Done: true})
			continue
		}
		// Fetch item detail with a bounded worker pool; mu serializes result
		// accounting, progress emits (Current stays monotonic), and the
		// writePolicy name-allocation+write step (uniquePath stats the
		// filesystem, so it must not race with a concurrent write).
		total := len(items)
		var mu sync.Mutex
		done := 0
		jobs := make(chan json.RawMessage)
		var wg sync.WaitGroup
		workers := backupWorkers
		if total < workers {
			workers = total
		}
		for w := 0; w < workers; w++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for item := range jobs {
					if ctx.Err() != nil {
						continue
					}
					full, warn := policyops.FetchFull(ctx, c, pt, item, opts.IncludeAssignments)
					if ctx.Err() != nil {
						continue
					}
					name := jsonutil.DisplayName(full, pt.NameField)
					mu.Lock()
					done++
					if warn {
						cr.Warnings++
						emit(Event{Category: pt.Category, Friendly: pt.Friendly, Err: fmt.Errorf("incomplete detail"), Current: done, Total: total})
					}
					if err := writePolicy(catDir, name, full); err != nil {
						// A failed disk write is data loss for this policy; record the
						// cause so the log can distinguish it from partial Graph detail.
						cr.Warnings++
						if cr.Error == "" {
							cr.Error = condense(fmt.Sprintf("write %q: %s", name, err))
						}
						emit(Event{Category: pt.Category, Friendly: pt.Friendly, Err: err, Current: done, Total: total})
						mu.Unlock()
						continue
					}
					counts[pt.Category]++
					cr.Count++
					emit(Event{Category: pt.Category, Friendly: pt.Friendly, Current: done, Total: total})
					mu.Unlock()
				}
			}()
		}
	feed:
		for _, item := range items {
			select {
			case jobs <- item:
			case <-ctx.Done():
				break feed
			}
		}
		close(jobs)
		wg.Wait()
		if ctx.Err() != nil {
			// Keep everything already written: a partial, honestly-labelled
			// backup beats an orphaned folder with no manifest.
			cancelled = true
			break loop
		}
		emit(Event{Category: pt.Category, Friendly: pt.Friendly, Current: total, Total: total, Done: true})
	}

	// Determine overall status.
	failed, warned := 0, 0
	var catList []CategoryResult
	for _, cat := range order {
		r := catResults[cat]
		catList = append(catList, *r)
		switch {
		case r.Failed:
			failed++
		case r.Warnings > 0:
			warned++
		}
	}
	status := "Success"
	switch {
	case cancelled:
		status = "Cancelled"
	case failed > 0 && failed == len(order):
		status = "Failed"
	case failed > 0 || warned > 0:
		status = "CompletedWithWarnings"
	}

	end := time.Now()
	meta := store.Metadata{
		BackupDate:      start.Format("2006-01-02-150405"),
		BackupFolder:    folder,
		StartTime:       start.Format("2006-01-02 15:04:05"),
		EndTime:         end.Format("2006-01-02 15:04:05"),
		Duration:        end.Sub(start).Truncate(time.Second).String(),
		DurationSeconds: int(end.Sub(start).Seconds()),
		ItemCounts:      counts,
		Status:          status,
		RunbookVersion:  RunbookVersion,
		BackupFormat:    "Individual policy files with Intune-compatible JSON",
		TenantID:        opts.Tenant.ID,
		TenantName:      opts.Tenant.DisplayName,
	}
	if err := writeMetadata(path, meta); err != nil {
		return Result{}, err
	}
	writeLog(path, meta, catList)

	return Result{Folder: folder, Path: path, Status: status, ItemCounts: counts, Categories: catList}, nil
}

var noiseKeys = map[string]bool{"@odata.context": true}

func writePolicy(dir, name string, raw json.RawMessage) error {
	cleaned, err := jsonutil.Normalize(raw, noiseKeys)
	if err != nil {
		cleaned = raw
	}
	data, err := json.MarshalIndent(cleaned, "", "  ")
	if err != nil {
		return err
	}
	file := uniquePath(filepath.Join(dir, jsonutil.SanitizeFilename(name)+".json"))
	return os.WriteFile(file, data, 0o644)
}

func writeMetadata(dir string, m store.Metadata) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "metadata.json"), data, 0o644)
}

// writeLog records a human-readable per-category outcome next to the backup.
func writeLog(dir string, m store.Metadata, cats []CategoryResult) {
	var b strings.Builder
	fmt.Fprintf(&b, "TenuVault backup %s\n", m.BackupFolder)
	fmt.Fprintf(&b, "tenant: %s (%s)\n", m.TenantName, m.TenantID)
	fmt.Fprintf(&b, "status: %s\n\n", m.Status)
	for _, c := range cats {
		switch {
		case c.Failed:
			fmt.Fprintf(&b, "FAILED   %-32s %s\n", c.Category, c.Error)
		case c.Warnings > 0:
			fmt.Fprintf(&b, "WARN     %-32s %d saved, %d incomplete", c.Category, c.Count, c.Warnings)
			if c.Error != "" {
				fmt.Fprintf(&b, " · %s", c.Error)
			}
			b.WriteString("\n")
		default:
			fmt.Fprintf(&b, "ok       %-32s %d saved\n", c.Category, c.Count)
		}
	}
	_ = os.WriteFile(filepath.Join(dir, "backup.log"), []byte(b.String()), 0o644)
}

func condense(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	if len(s) > 200 {
		cut := 200
		for cut > 0 && !utf8.RuneStart(s[cut]) {
			cut--
		}
		s = s[:cut] + "…"
	}
	return s
}

// uniquePath avoids clobbering when two policies share a display name.
func uniquePath(path string) string {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return path
	}
	ext := filepath.Ext(path)
	base := path[:len(path)-len(ext)]
	for i := 2; ; i++ {
		cand := fmt.Sprintf("%s (%d)%s", base, i, ext)
		if _, err := os.Stat(cand); os.IsNotExist(err) {
			return cand
		}
	}
}
