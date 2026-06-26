// Package policyops holds the shared "read a full policy" and "create a policy"
// logic used by both restore (from a backup) and sync (tenant to tenant). It is
// the single place that enforces the create-only, never-overwrite contract:
// read-only fields and OData annotations are stripped, assignments are removed,
// Conditional Access is created disabled, and every write is a POST (create) —
// never a PATCH or DELETE of anything existing.
package policyops

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/ugurkocde/TenuVault-TUI/internal/catalog"
	"github.com/ugurkocde/TenuVault-TUI/internal/graph"
	"github.com/ugurkocde/TenuVault-TUI/internal/jsonutil"
)

// FetchFull returns a policy with its full content: per-item detail (script
// bodies, settings-catalog settings), nested sub-resources (admin-template
// definition values, intent settings), and optionally assignments. The bool is
// true if any enrichment was incomplete.
func FetchFull(ctx context.Context, c *graph.Client, pt catalog.PolicyType, item json.RawMessage, includeAssignments bool) (json.RawMessage, bool) {
	full := item
	warn := false
	id := IDOf(item)

	if pt.DetailByID && id != "" {
		var q url.Values
		if pt.Expand != "" {
			q = url.Values{"$expand": {pt.Expand}}
		}
		if detail, err := c.Get(ctx, pt.Version, pt.ListPath+"/"+id, q); err == nil {
			full = detail
		} else {
			warn = true
		}
	}

	if pt.Sub != nil && id != "" {
		var q url.Values
		if pt.Sub.Expand != "" {
			q = url.Values{"$expand": {pt.Sub.Expand}}
		}
		if vals, err := c.ListAll(ctx, pt.Version, pt.ListPath+"/"+id+"/"+pt.Sub.Suffix, q); err == nil {
			full = embed(full, pt.Sub.EmbedKey, vals)
		} else {
			warn = true
		}
	}

	if includeAssignments && id != "" && supportsAssignments(pt) {
		if vals, err := c.ListAll(ctx, pt.Version, pt.ListPath+"/"+id+"/assignments", nil); err == nil {
			full = embed(full, "assignments", vals)
		}
	}
	return full, warn
}

// readOnlyKeys are stripped before creating a policy. OData annotations (other
// than @odata.type) are stripped separately via jsonutil.IsODataAnnotation.
var readOnlyKeys = map[string]bool{
	"id":                   true,
	"createdDateTime":      true,
	"lastModifiedDateTime": true,
	"version":              true,
	"assignments":          true,
	"isAssigned":           true,
	"supportsScopeTags":    true,
}

// PrepareCreate cleans a policy and resolves the endpoint to create it at. A
// non-empty namePrefix is prepended to the display name (empty keeps the
// original name). Backup-only types and unroutable items return an error.
func PrepareCreate(category string, raw json.RawMessage, namePrefix string) (version, endpoint string, body json.RawMessage, err error) {
	pt, ok := routeByType(category, raw)
	if !ok {
		if category == "AppProtectionPolicies" {
			return "", "", nil, fmt.Errorf("unrecognized app protection type %q", odataTypeOf(raw))
		}
		return "", "", nil, fmt.Errorf("no create route for category %q", category)
	}
	if !pt.RestoreSupported {
		return "", "", nil, fmt.Errorf("%s is backup-only; create is not supported", pt.Friendly)
	}
	cleaned := jsonutil.StripKeysFunc(mustMap(raw), func(k string) bool {
		return readOnlyKeys[k] || jsonutil.IsODataAnnotation(k)
	})
	m, _ := cleaned.(map[string]any)
	if m == nil {
		m = map[string]any{}
	}
	if namePrefix != "" {
		for _, field := range []string{"displayName", "name"} {
			if v, ok := m[field].(string); ok && v != "" {
				if !strings.HasPrefix(v, namePrefix) {
					m[field] = namePrefix + v
				}
				break
			}
		}
	}
	// Conditional Access is created disabled so a copy never locks admins out.
	if category == "ConditionalAccessPolicies" {
		m["state"] = "disabled"
	}
	out, err := json.Marshal(m)
	if err != nil {
		return "", "", nil, err
	}
	return pt.Version, pt.RestoreEndpoint(), out, nil
}

// Create cleans and POSTs a policy to the client, returning the new id.
func Create(ctx context.Context, c *graph.Client, category string, raw json.RawMessage, namePrefix string) (string, error) {
	version, endpoint, body, err := PrepareCreate(category, raw, namePrefix)
	if err != nil {
		return "", err
	}
	created, err := c.Post(ctx, version, endpoint, body)
	if err != nil {
		return "", err
	}
	return IDOf(created), nil
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
			if strings.Contains(odataType, "windows") && pt.Key == "windowsAppProtection" {
				return pt, true
			}
			continue
		}
		match, found = pt, true
		break
	}
	return match, found
}

func supportsAssignments(pt catalog.PolicyType) bool {
	if !strings.HasPrefix(pt.ListPath, "/deviceManagement/") && !strings.HasPrefix(pt.ListPath, "/deviceAppManagement/") {
		return false
	}
	switch pt.Key {
	case "roleScopeTags", "deviceCategories", "notificationTemplates", "appCategories", "assignmentFilters":
		return false
	}
	return true
}

// embed inserts a sub-resource value array into the policy JSON under key.
func embed(raw json.RawMessage, key string, vals []json.RawMessage) json.RawMessage {
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return raw
	}
	arr := make([]any, 0, len(vals))
	for _, v := range vals {
		var x any
		if err := json.Unmarshal(v, &x); err == nil {
			arr = append(arr, x)
		}
	}
	m[key] = arr
	if out, err := json.Marshal(m); err == nil {
		return out
	}
	return raw
}

func odataTypeOf(raw json.RawMessage) string {
	var m struct {
		Type string `json:"@odata.type"`
	}
	_ = json.Unmarshal(raw, &m)
	return m.Type
}

// IDOf returns the "id" field of a policy JSON, or "".
func IDOf(raw json.RawMessage) string {
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
