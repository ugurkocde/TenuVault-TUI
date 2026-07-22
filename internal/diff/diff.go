// Package diff compares two TenuVault backups and reports drift between them,
// mirroring the portal's detect-drifts logic: added/removed/modified policies
// per category with a severity classification.
package diff

import (
	"encoding/json"
	"reflect"
	"sort"

	"github.com/ugurkocde/TenuVault-TUI/internal/jsonutil"
	"github.com/ugurkocde/TenuVault-TUI/internal/store"
)

// ChangeType classifies a drift.
type ChangeType string

const (
	Added    ChangeType = "added"
	Removed  ChangeType = "removed"
	Modified ChangeType = "modified"
)

// Severity ranks the impact of a change.
type Severity string

const (
	Critical Severity = "critical"
	Warning  Severity = "warning"
	Info     Severity = "info"
)

// Change is a single difference between two backups.
type Change struct {
	Category string
	Name     string
	Type     ChangeType
	Severity Severity
}

// ignoreKeys are fields that shouldn't count as a meaningful change.
var ignoreKeys = map[string]bool{
	"@odata.context":       true,
	"@odata.type":          true,
	"id":                   true,
	"version":              true,
	"lastModifiedDateTime": true,
	"createdDateTime":      true,
}

// criticalCategories are security-sensitive; deletions here are critical.
var criticalCategories = map[string]bool{
	"CompliancePolicies":        true,
	"ConditionalAccessPolicies": true,
}

// Compare returns the changes going from older to newer.
func Compare(older, newer store.Backup) ([]Change, error) {
	oldCats, err := older.Categories()
	if err != nil {
		return nil, err
	}
	newCats, err := newer.Categories()
	if err != nil {
		return nil, err
	}
	cats := union(oldCats, newCats)
	oldSet, newSet := stringSet(oldCats), stringSet(newCats)

	var changes []Change
	for _, cat := range cats {
		oldFiles := map[string]entry{}
		if oldSet[cat] {
			oldFiles, err = indexPolicies(older, cat)
			if err != nil {
				return nil, err
			}
		}
		newFiles := map[string]entry{}
		if newSet[cat] {
			newFiles, err = indexPolicies(newer, cat)
			if err != nil {
				return nil, err
			}
		}

		for key, ne := range newFiles {
			oe, existed := oldFiles[key]
			if !existed {
				changes = append(changes, Change{cat, ne.name, Added, severity(cat, Added)})
				continue
			}
			equal, err := equalPolicy(oe.path, ne.path)
			if err != nil {
				return nil, err
			}
			if !equal {
				// Report under the newer name so a rename reads naturally.
				changes = append(changes, Change{cat, ne.name, Modified, severity(cat, Modified)})
			}
		}
		for key, oe := range oldFiles {
			if _, ok := newFiles[key]; !ok {
				changes = append(changes, Change{cat, oe.name, Removed, severity(cat, Removed)})
			}
		}
	}
	sort.Slice(changes, func(i, j int) bool {
		if changes[i].Category != changes[j].Category {
			return changes[i].Category < changes[j].Category
		}
		if changes[i].Name != changes[j].Name {
			return changes[i].Name < changes[j].Name
		}
		return changes[i].Type < changes[j].Type
	})
	return changes, nil
}

// entry is one backed-up policy file, keyed for pairing.
type entry struct {
	name string // file name minus .json, used for display
	path string
}

// indexPolicies keys a category's files by the policy's Graph id, so the two
// sides of a compare pair the same tenant object even when its display name
// changed or duplicate names shuffled their " (2)" file suffixes between
// runs. Files without a readable id (or with a colliding one) fall back to
// the file name, matching the previous behavior.
func indexPolicies(b store.Backup, cat string) (map[string]entry, error) {
	out := map[string]entry{}
	files, err := b.Policies(cat)
	if err != nil {
		return nil, err
	}
	for _, f := range files {
		key := "name:" + f.Name
		if id := policyID(f.Path); id != "" {
			if _, taken := out["id:"+id]; !taken {
				key = "id:" + id
			}
		}
		out[key] = entry{name: f.Name, path: f.Path}
	}
	return out, nil
}

// policyID returns the top-level "id" of a policy file, or "" if unreadable.
func policyID(path string) string {
	raw, err := store.Read(path)
	if err != nil {
		return ""
	}
	var m struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(raw, &m) != nil {
		return ""
	}
	return m.ID
}

func stringSet(values []string) map[string]bool {
	out := make(map[string]bool, len(values))
	for _, value := range values {
		out[value] = true
	}
	return out
}

func equalPolicy(pathA, pathB string) (bool, error) {
	rawA, errA := store.Read(pathA)
	rawB, errB := store.Read(pathB)
	if errA != nil {
		return false, errA
	}
	if errB != nil {
		return false, errB
	}
	a, err1 := normalizePolicy(rawA)
	b, err2 := normalizePolicy(rawB)
	if err1 != nil {
		return false, err1
	}
	if err2 != nil {
		return false, err2
	}
	return reflect.DeepEqual(a, b), nil
}

// normalizePolicy strips bookkeeping fields plus every OData annotation
// (settings@odata.context and friends), so backups taken by different
// TenuVault versions never register annotation churn as drift.
func normalizePolicy(raw json.RawMessage) (any, error) {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, err
	}
	return jsonutil.StripKeysFunc(v, func(k string) bool {
		return ignoreKeys[k] || jsonutil.IsODataAnnotation(k)
	}), nil
}

func severity(cat string, t ChangeType) Severity {
	if t == Removed && criticalCategories[cat] {
		return Critical
	}
	switch t {
	case Removed, Modified:
		return Warning
	default:
		return Info
	}
}

func union(a, b []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range append(append([]string{}, a...), b...) {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
