// Package diff compares two TenuVault backups and reports drift between them,
// mirroring the portal's detect-drifts logic: added/removed/modified policies
// per category with a severity classification.
package diff

import (
	"reflect"

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

	var changes []Change
	for _, cat := range cats {
		oldFiles, err := indexByName(older, cat)
		if err != nil {
			return nil, err
		}
		newFiles, err := indexByName(newer, cat)
		if err != nil {
			return nil, err
		}

		for name, np := range newFiles {
			op, existed := oldFiles[name]
			if !existed {
				changes = append(changes, Change{cat, name, Added, severity(cat, Added)})
				continue
			}
			equal, err := equalPolicy(op, np)
			if err != nil {
				return nil, err
			}
			if !equal {
				changes = append(changes, Change{cat, name, Modified, severity(cat, Modified)})
			}
		}
		for name := range oldFiles {
			if _, ok := newFiles[name]; !ok {
				changes = append(changes, Change{cat, name, Removed, severity(cat, Removed)})
			}
		}
	}
	return changes, nil
}

func indexByName(b store.Backup, cat string) (map[string]string, error) {
	out := map[string]string{}
	files, err := b.Policies(cat)
	if err != nil {
		return nil, err
	}
	for _, f := range files {
		out[f.Name] = f.Path
	}
	return out, nil
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
	a, err1 := jsonutil.Normalize(rawA, ignoreKeys)
	b, err2 := jsonutil.Normalize(rawB, ignoreKeys)
	if err1 != nil {
		return false, err1
	}
	if err2 != nil {
		return false, err2
	}
	return reflect.DeepEqual(a, b), nil
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
