// Package restore writes backed-up policies back to Microsoft Graph. It cleans
// read-only fields, prefixes the display name with "[Restored]", forces
// Conditional Access policies to a disabled state, and routes each item to the
// correct endpoint. All writes are gated by explicit confirmation in the UI.
package restore

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ugurkocde/TenuVault-TUI/internal/catalog"
	"github.com/ugurkocde/TenuVault-TUI/internal/graph"
	"github.com/ugurkocde/TenuVault-TUI/internal/jsonutil"
	"github.com/ugurkocde/TenuVault-TUI/internal/store"
)

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

// readOnlyKeys are stripped before POSTing a restored policy.
var readOnlyKeys = map[string]bool{
	"@odata.context":       true,
	"id":                   true,
	"createdDateTime":      true,
	"lastModifiedDateTime": true,
	"version":              true,
	"assignments":          true,
	"isAssigned":           true,
	"roleScopeTagIds":      true,
	"supportsScopeTags":    true,
}

// Plan builds restore items from a selection of "Category/Name" entries.
func Plan(b store.Backup, selection []Item) []Item { return selection }

// Restore POSTs each item to Graph and returns per-item results.
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
		version, endpoint, body, err := prepare(it.Category, raw)
		if err != nil {
			res.Err = err
			results = append(results, res)
			continue
		}
		created, err := c.Post(ctx, version, endpoint, body)
		if err != nil {
			res.Err = err
			results = append(results, res)
			continue
		}
		res.NewID = idOf(created)
		results = append(results, res)
	}
	return results
}

// prepare cleans a policy and resolves its restore endpoint.
func prepare(category string, raw json.RawMessage) (version, endpoint string, body json.RawMessage, err error) {
	pt, ok := routeByType(category, raw)
	if !ok {
		return "", "", nil, fmt.Errorf("no restore route for category %q", category)
	}
	cleaned := jsonutil.StripKeys(mustMap(raw), readOnlyKeys)
	m, _ := cleaned.(map[string]any)
	if m == nil {
		m = map[string]any{}
	}
	// Prefix the display name so restores are obvious and never overwrite.
	for _, field := range []string{"displayName", "name"} {
		if v, ok := m[field].(string); ok && v != "" && !strings.HasPrefix(v, "[Restored]") {
			m[field] = "[Restored] " + v
			break
		}
	}
	// Conditional Access: restore disabled so nothing locks admins out.
	if category == "ConditionalAccessPolicies" {
		m["state"] = "disabled"
	}
	out, err := json.Marshal(m)
	if err != nil {
		return "", "", nil, err
	}
	return pt.Version, pt.RestoreEndpoint(), out, nil
}

// routeByType resolves the policy type for a category, disambiguating app
// protection by @odata.type.
func routeByType(category string, raw json.RawMessage) (catalog.PolicyType, bool) {
	odataType := strings.ToLower(odataTypeOf(raw))
	var match catalog.PolicyType
	found := false
	for _, pt := range catalog.All() {
		if pt.Category != category {
			continue
		}
		if category == "AppProtectionPolicies" {
			if strings.Contains(odataType, "ios") && pt.Key == "iosAppProtection" {
				return pt, true
			}
			if strings.Contains(odataType, "android") && pt.Key == "androidAppProtection" {
				return pt, true
			}
			continue
		}
		match, found = pt, true
		break
	}
	return match, found
}

func odataTypeOf(raw json.RawMessage) string {
	var m struct {
		Type string `json:"@odata.type"`
	}
	_ = json.Unmarshal(raw, &m)
	return m.Type
}

func idOf(raw json.RawMessage) string {
	var m struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(raw, &m)
	return m.ID
}

func mustMap(raw json.RawMessage) any {
	var v any
	_ = json.Unmarshal(raw, &v)
	return v
}
