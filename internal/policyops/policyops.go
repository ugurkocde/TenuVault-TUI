// Package policyops holds the shared "read a full policy" and "create a policy"
// logic used by both restore (from a backup) and sync (tenant to tenant). It is
// the single place that enforces the create-only, never-overwrite contract:
// read-only fields and OData annotations are stripped, assignments are removed,
// Conditional Access is created disabled, and existing tenant objects are never
// patched or deleted. A failed multi-step create may delete only the object that
// operation just created as a compensating rollback.
package policyops

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/ugurkocde/TenuVault-TUI/internal/catalog"
	"github.com/ugurkocde/TenuVault-TUI/internal/graph"
	"github.com/ugurkocde/TenuVault-TUI/internal/jsonutil"
)

// FetchFull returns a policy with its full content: per-item detail (script
// bodies, settings-catalog settings), nested sub-resources (admin-template
// definition values, intent settings), and optionally assignments. The bool is
// true if any enrichment was incomplete.
func FetchFull(ctx context.Context, c graph.API, pt catalog.PolicyType, item json.RawMessage, includeAssignments bool) (json.RawMessage, bool) {
	full := item
	warn := false
	id := IDOf(item)

	// Administrative templates need a two-level fetch (Graph caps $expand depth
	// at 1): definitionValues with their definition, then each value's
	// presentationValues with their presentation.
	if pt.CreateMode == "groupPolicy" && id != "" {
		dvs, err := c.ListAll(ctx, pt.Version, pt.ListPath+"/"+id+"/definitionValues", url.Values{"$expand": {"definition"}})
		if err != nil {
			warn = true
		} else {
			enriched := make([]json.RawMessage, 0, len(dvs))
			for _, dv := range dvs {
				var definitionValue map[string]any
				if json.Unmarshal(dv, &definitionValue) != nil || buildDefinitionValue(definitionValue) == nil {
					warn = true
				}
				if dvID := IDOf(dv); dvID != "" {
					if pvs, perr := c.ListAll(ctx, pt.Version, pt.ListPath+"/"+id+"/definitionValues/"+dvID+"/presentationValues", url.Values{"$expand": {"presentation"}}); perr == nil {
						dv = embed(dv, "presentationValues", pvs)
					} else {
						warn = true
					}
				} else {
					warn = true
				}
				enriched = append(enriched, dv)
			}
			full = embed(full, "definitionValues", enriched)
		}
	}

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
		} else {
			warn = true
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

const graphBeta = "https://graph.microsoft.com/beta"

// route resolves the policy type for a category and enforces restore support.
func route(category string, raw json.RawMessage) (catalog.PolicyType, error) {
	pt, ok := routeByType(category, raw)
	if !ok {
		if category == "AppProtectionPolicies" {
			return pt, fmt.Errorf("unrecognized app protection type %q", odataTypeOf(raw))
		}
		return pt, fmt.Errorf("no create route for category %q", category)
	}
	if !pt.RestoreSupported {
		return pt, fmt.Errorf("%s is backup-only; create is not supported", pt.Friendly)
	}
	return pt, nil
}

// cleanForCreate strips read-only fields and OData annotations, applies the name
// prefix, and (for Conditional Access) forces a disabled state.
func cleanForCreate(raw json.RawMessage, namePrefix string, caDisable bool) map[string]any {
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
	if caDisable {
		m["state"] = "disabled"
	}
	return m
}

// ensureComplianceBlockAction guarantees the scheduled block action Graph
// requires on compliance policy creates ("Compliance policy must have one and
// only one block scheduled action"). Backups taken before
// scheduledActionConfigurations were captured hold rules without actions;
// they get the portal default (block immediately) so they stay restorable.
func ensureComplianceBlockAction(m map[string]any) {
	rules, _ := m["scheduledActionsForRule"].([]any)
	for _, rAny := range rules {
		r, ok := rAny.(map[string]any)
		if !ok {
			continue
		}
		cfgs, _ := r["scheduledActionConfigurations"].([]any)
		for _, cAny := range cfgs {
			if c, ok := cAny.(map[string]any); ok && c["actionType"] == "block" {
				return
			}
		}
	}
	block := map[string]any{
		"actionType":                "block",
		"gracePeriodHours":          0,
		"notificationTemplateId":    "00000000-0000-0000-0000-000000000000",
		"notificationMessageCCList": []any{},
	}
	if len(rules) > 0 {
		if r, ok := rules[0].(map[string]any); ok {
			cfgs, _ := r["scheduledActionConfigurations"].([]any)
			r["scheduledActionConfigurations"] = append(cfgs, block)
			return
		}
	}
	m["scheduledActionsForRule"] = []any{map[string]any{
		"ruleName":                      nil,
		"scheduledActionConfigurations": []any{block},
	}}
}

// prepareBody cleans a simple policy for create and applies per-category
// fixups that Graph demands on POST.
func prepareBody(pt catalog.PolicyType, raw json.RawMessage, namePrefix string) (json.RawMessage, error) {
	m := cleanForCreate(raw, namePrefix, pt.Category == "ConditionalAccessPolicies")
	if pt.Category == "CompliancePolicies" {
		ensureComplianceBlockAction(m)
	}
	return json.Marshal(m)
}

// PrepareCreate cleans a simple policy and resolves its create endpoint. A
// non-empty namePrefix is prepended to the display name (empty keeps it).
func PrepareCreate(category string, raw json.RawMessage, namePrefix string) (version, endpoint string, body json.RawMessage, err error) {
	pt, err := route(category, raw)
	if err != nil {
		return "", "", nil, err
	}
	out, err := prepareBody(pt, raw, namePrefix)
	if err != nil {
		return "", "", nil, err
	}
	return pt.Version, pt.RestoreEndpoint(), out, nil
}

// Create creates a policy in the target tenant, returning the new id. It
// dispatches by the type's CreateMode (simple POST, admin-template multi-part,
// or endpoint-security intent createInstance).
func Create(ctx context.Context, c graph.API, category string, raw json.RawMessage, namePrefix string) (string, error) {
	pt, err := route(category, raw)
	if err != nil {
		return "", err
	}
	switch pt.CreateMode {
	case "groupPolicy":
		return createGroupPolicy(ctx, c, pt, raw, namePrefix)
	case "intent":
		return createIntent(ctx, c, pt, raw, namePrefix)
	default:
		body, err := prepareBody(pt, raw, namePrefix)
		if err != nil {
			return "", err
		}
		created, err := c.Post(ctx, pt.Version, pt.RestoreEndpoint(), body)
		if err != nil {
			return "", err
		}
		return IDOf(created), nil
	}
}

// createGroupPolicy creates an administrative-template policy, then applies all
// captured settings in one atomic updateDefinitionValues action. Each setting is
// bound to Microsoft's global policy definition catalog (definition ids are
// tenant-independent), with its presentation values nested.
func createGroupPolicy(ctx context.Context, c graph.API, pt catalog.PolicyType, raw json.RawMessage, namePrefix string) (string, error) {
	// Pull the settings from the RAW policy first: cleanForCreate strips "id"
	// recursively, which would destroy the definition/presentation ids the
	// settings need to bind against.
	var rawMap map[string]any
	if err := json.Unmarshal(raw, &rawMap); err != nil {
		return "", err
	}
	defVals, _ := rawMap["definitionValues"].([]any)
	delete(rawMap, "definitionValues")
	configRaw, err := json.Marshal(rawMap)
	if err != nil {
		return "", err
	}

	body, err := json.Marshal(cleanForCreate(configRaw, namePrefix, false))
	if err != nil {
		return "", err
	}
	var update json.RawMessage
	if len(defVals) > 0 {
		added := make([]json.RawMessage, 0, len(defVals))
		for i, dvAny := range defVals {
			dv, ok := dvAny.(map[string]any)
			if !ok {
				return "", fmt.Errorf("definition value %d is not an object", i+1)
			}
			built := buildDefinitionValue(dv)
			if built == nil {
				return "", fmt.Errorf("definition value %d has no definition id", i+1)
			}
			added = append(added, built)
		}
		update, err = json.Marshal(map[string]any{
			"added":      added,
			"updated":    []json.RawMessage{},
			"deletedIds": []string{},
		})
		if err != nil {
			return "", err
		}
	}

	created, err := c.Post(ctx, pt.Version, pt.ListPath, body)
	if err != nil {
		return "", err
	}
	id := IDOf(created)
	if id == "" {
		return "", fmt.Errorf("created group policy response did not include an id; settings were not applied")
	}
	if len(defVals) == 0 {
		return id, nil
	}
	if _, err := c.Post(ctx, pt.Version, pt.ListPath+"/"+id+"/updateDefinitionValues", update); err != nil {
		// A cancellation may be what interrupted the settings POST. Cleanup must
		// therefore retain request values but have its own short deadline.
		rollbackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
		defer cancel()
		rollbackErr := c.Delete(rollbackCtx, pt.Version, pt.ListPath+"/"+id)
		if rollbackErr == nil {
			return "", fmt.Errorf("policy settings failed to apply; newly created policy was rolled back: %w", err)
		}
		return id, &PartialCreateError{PolicyID: id, ApplyErr: err, RollbackErr: rollbackErr}
	}
	return id, nil
}

// PartialCreateError means a multi-step create failed after Graph allocated the
// policy and the compensating delete also failed. PolicyID identifies the
// resource requiring manual cleanup.
type PartialCreateError struct {
	PolicyID    string
	ApplyErr    error
	RollbackErr error
}

func (e *PartialCreateError) Error() string {
	return fmt.Sprintf("policy %s was created without settings; apply failed: %v; rollback failed: %v",
		e.PolicyID, e.ApplyErr, e.RollbackErr)
}

func (e *PartialCreateError) Unwrap() error { return e.ApplyErr }

// buildDefinitionValue converts a captured definitionValue into an
// updateDefinitionValues "added" entry with definition/presentation @odata.bind
// references and nested presentation values.
func buildDefinitionValue(dv map[string]any) json.RawMessage {
	def, _ := dv["definition"].(map[string]any)
	defID, _ := def["id"].(string)
	if defID == "" {
		return nil
	}
	configType := "policy"
	if ct, ok := dv["configurationType"].(string); ok && ct != "" {
		configType = ct
	}
	out := map[string]any{
		"@odata.type":           "#microsoft.graph.groupPolicyDefinitionValue",
		"enabled":               dv["enabled"],
		"configurationType":     configType,
		"definition@odata.bind": graphBeta + "/deviceManagement/groupPolicyDefinitions('" + defID + "')",
	}
	if pvs, ok := dv["presentationValues"].([]any); ok && len(pvs) > 0 {
		outPvs := make([]any, 0, len(pvs))
		for _, pvAny := range pvs {
			pv, ok := pvAny.(map[string]any)
			if !ok {
				continue
			}
			pres, _ := pv["presentation"].(map[string]any)
			presID, _ := pres["id"].(string)
			np := map[string]any{}
			for k, v := range pv {
				switch k {
				case "presentation", "id", "createdDateTime", "lastModifiedDateTime":
					continue
				}
				np[k] = v
			}
			if presID != "" {
				np["presentation@odata.bind"] = graphBeta + "/deviceManagement/groupPolicyDefinitions('" + defID + "')/presentations('" + presID + "')"
			}
			outPvs = append(outPvs, np)
		}
		out["presentationValues"] = outPvs
	}
	b, err := json.Marshal(out)
	if err != nil {
		return nil
	}
	return b
}

// createIntent recreates an endpoint-security/baseline intent from its template
// via createInstance, carrying the captured settings as settingsDelta.
func createIntent(ctx context.Context, c graph.API, pt catalog.PolicyType, raw json.RawMessage, namePrefix string) (string, error) {
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return "", err
	}
	templateID, _ := m["templateId"].(string)
	if templateID == "" {
		return "", fmt.Errorf("intent has no templateId; cannot recreate")
	}
	name, _ := m["displayName"].(string)
	if namePrefix != "" && !strings.HasPrefix(name, namePrefix) {
		name = namePrefix + name
	}
	var settingsDelta []any
	if settings, ok := m["settings"].([]any); ok {
		for _, sAny := range settings {
			s, ok := sAny.(map[string]any)
			if !ok {
				continue
			}
			ns := map[string]any{}
			for k, v := range s {
				if k == "id" {
					continue
				}
				ns[k] = v
			}
			settingsDelta = append(settingsDelta, ns)
		}
	}
	body := map[string]any{"displayName": name, "settingsDelta": settingsDelta}
	if d, ok := m["description"].(string); ok {
		body["description"] = d
	}
	if r, ok := m["roleScopeTagIds"]; ok {
		body["roleScopeTagIds"] = r
	}
	out, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	created, err := c.Post(ctx, pt.Version, "/deviceManagement/templates/"+templateID+"/createInstance", out)
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

// ResolveTypeKey returns the catalog key that would be used to create raw.
// It is primarily useful for disambiguating categories shared by several
// policy types, such as iOS/Android/Windows app protection policies.
func ResolveTypeKey(category string, raw json.RawMessage) (string, bool) {
	pt, ok := routeByType(category, raw)
	return pt.Key, ok
}

// supportsAssignments defers to the catalog, the single source of truth for
// per-type capabilities.
func supportsAssignments(pt catalog.PolicyType) bool {
	return !pt.NoAssignments
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
