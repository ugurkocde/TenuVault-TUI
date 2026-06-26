// Package backup enumerates and writes Intune policies to a TenuVault-format
// backup folder. It preserves verbatim Graph JSON (only dropping @odata.context)
// so policies restore cleanly, and writes a metadata.json manifest.
package backup

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/ugurkocde/TenuVault-TUI/internal/catalog"
	"github.com/ugurkocde/TenuVault-TUI/internal/graph"
	"github.com/ugurkocde/TenuVault-TUI/internal/jsonutil"
	"github.com/ugurkocde/TenuVault-TUI/internal/store"
)

// RunbookVersion is recorded in the manifest for portal cross-compatibility.
const RunbookVersion = "tui-1.0.0"

// Event reports backup progress for one category, suitable for a TUI.
type Event struct {
	Category string
	Friendly string
	Current  int
	Total    int
	Done     bool
	Err      error
}

// Result summarizes a completed backup.
type Result struct {
	Folder     string
	Path       string
	ItemCounts map[string]int
}

// Run backs up the selected policy types into root, emitting progress events.
func Run(ctx context.Context, c *graph.Client, types []catalog.PolicyType, root string, tenant graph.Tenant, progress func(Event)) (Result, error) {
	start := time.Now()
	folder := "backup-" + start.Format("2006-01-02-150405")
	path := filepath.Join(root, folder)
	if err := os.MkdirAll(path, 0o755); err != nil {
		return Result{}, fmt.Errorf("create backup folder: %w", err)
	}

	counts := map[string]int{}
	warnings := 0
	emit := func(e Event) {
		if progress != nil {
			progress(e)
		}
	}

	for _, pt := range types {
		items, err := c.ListAll(ctx, pt.Version, pt.ListPath, nil)
		if err != nil {
			emit(Event{Category: pt.Category, Friendly: pt.Friendly, Err: err, Done: true})
			continue
		}
		catDir := filepath.Join(path, pt.Category)
		if err := os.MkdirAll(catDir, 0o755); err != nil {
			emit(Event{Category: pt.Category, Friendly: pt.Friendly, Err: err, Done: true})
			continue
		}
		total := len(items)
		for i, item := range items {
			if err := ctx.Err(); err != nil {
				return Result{}, err
			}
			full := item
			if pt.Expand != "" {
				if id := itemID(item); id != "" {
					q := url.Values{"$expand": {pt.Expand}}
					detail, err := c.Get(ctx, pt.Version, pt.ListPath+"/"+id, q)
					if err != nil {
						// Expanded detail (e.g. settings catalog settings) is the
						// substance of the policy; if it can't be fetched, record
						// a warning instead of silently writing a hollow item.
						warnings++
						emit(Event{Category: pt.Category, Friendly: pt.Friendly, Err: err})
					} else {
						full = detail
					}
				}
			}
			name := jsonutil.DisplayName(full, pt.NameField)
			if err := writePolicy(catDir, name, full); err != nil {
				emit(Event{Category: pt.Category, Friendly: pt.Friendly, Err: err})
				continue
			}
			counts[pt.Category]++
			emit(Event{Category: pt.Category, Friendly: pt.Friendly, Current: i + 1, Total: total})
		}
		emit(Event{Category: pt.Category, Friendly: pt.Friendly, Current: total, Total: total, Done: true})
	}

	end := time.Now()
	status := "Success"
	if warnings > 0 {
		status = "CompletedWithWarnings"
	}
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
		TenantID:        tenant.ID,
		TenantName:      tenant.DisplayName,
	}
	if err := writeMetadata(path, meta); err != nil {
		return Result{}, err
	}
	return Result{Folder: folder, Path: path, ItemCounts: counts}, nil
}

// noiseKeys are API artifacts dropped before writing (item still restores).
var noiseKeys = map[string]bool{"@odata.context": true}

func writePolicy(dir, name string, raw json.RawMessage) error {
	cleaned, err := jsonutil.Normalize(raw, noiseKeys)
	if err != nil {
		cleaned = raw // fall back to verbatim
	}
	data, err := json.MarshalIndent(cleaned, "", "  ")
	if err != nil {
		return err
	}
	file := filepath.Join(dir, jsonutil.SanitizeFilename(name)+".json")
	file = uniquePath(file)
	return os.WriteFile(file, data, 0o644)
}

func writeMetadata(dir string, m store.Metadata) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "metadata.json"), data, 0o644)
}

func itemID(raw json.RawMessage) string {
	var m struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(raw, &m)
	return m.ID
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
