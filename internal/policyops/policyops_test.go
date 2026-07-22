package policyops

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/ugurkocde/TenuVault-TUI/internal/catalog"
)

func TestPrepareCreateCleansAndPrefixes(t *testing.T) {
	raw := []byte(`{
		"@odata.type":"#microsoft.graph.windows10GeneralConfiguration",
		"@odata.context":"ctx",
		"id":"abc","version":3,
		"createdDateTime":"2020","lastModifiedDateTime":"2021",
		"displayName":"BitLocker","assignments":[{"x":1}]
	}`)
	version, endpoint, body, err := PrepareCreate("DeviceConfigurations", raw, "[Restored] ")
	if err != nil {
		t.Fatal(err)
	}
	if version != "beta" || endpoint != "/deviceManagement/deviceConfigurations" {
		t.Errorf("route = %s %s", version, endpoint)
	}
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"id", "version", "createdDateTime", "lastModifiedDateTime", "assignments", "@odata.context"} {
		if _, ok := m[k]; ok {
			t.Errorf("read-only field %q not stripped", k)
		}
	}
	if m["displayName"] != "[Restored] BitLocker" {
		t.Errorf("displayName = %v", m["displayName"])
	}
	if m["@odata.type"] != "#microsoft.graph.windows10GeneralConfiguration" {
		t.Error("@odata.type must be preserved for create routing")
	}
}

func TestPrepareCreateKeepsNameWhenNoPrefix(t *testing.T) {
	raw := []byte(`{"@odata.type":"#microsoft.graph.windows10GeneralConfiguration","id":"1","displayName":"Keep Me"}`)
	_, _, body, err := PrepareCreate("DeviceConfigurations", raw, "")
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	_ = json.Unmarshal(body, &m)
	if m["displayName"] != "Keep Me" {
		t.Errorf("name should be unchanged, got %v", m["displayName"])
	}
}

func TestPrepareConditionalAccessDisabled(t *testing.T) {
	raw := []byte(`{"id":"1","displayName":"CA","state":"enabled"}`)
	_, _, body, err := PrepareCreate("ConditionalAccessPolicies", raw, "")
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	_ = json.Unmarshal(body, &m)
	if m["state"] != "disabled" {
		t.Errorf("CA state = %v, want disabled", m["state"])
	}
}

func TestUnknownCategoryRejected(t *testing.T) {
	raw := []byte(`{"id":"1","displayName":"x"}`)
	if _, _, _, err := PrepareCreate("NoSuchCategory", raw, ""); err == nil {
		t.Error("unknown category should be rejected")
	}
}

func TestRouteAppProtectionByODataType(t *testing.T) {
	ios := []byte(`{"@odata.type":"#microsoft.graph.iosManagedAppProtection","displayName":"x"}`)
	pt, ok := routeByType("AppProtectionPolicies", ios)
	if !ok || pt.Key != "iosAppProtection" {
		t.Errorf("ios routing failed: %+v ok=%v", pt, ok)
	}
}

func TestBuildDefinitionValue(t *testing.T) {
	dv := map[string]any{
		"enabled":    true,
		"id":         "dv-id",
		"definition": map[string]any{"id": "def-guid", "displayName": "Some setting"},
		"presentationValues": []any{
			map[string]any{
				"@odata.type":  "#microsoft.graph.groupPolicyPresentationValueText",
				"value":        "hello",
				"id":           "pv-id",
				"presentation": map[string]any{"id": "pres-guid"},
			},
		},
	}
	body := buildDefinitionValue(dv)
	if body == nil {
		t.Fatal("nil body")
	}
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatal(err)
	}
	if m["definition@odata.bind"] != graphBeta+"/deviceManagement/groupPolicyDefinitions('def-guid')" {
		t.Errorf("definition bind = %v", m["definition@odata.bind"])
	}
	if m["enabled"] != true {
		t.Error("enabled not carried")
	}
	pvs := m["presentationValues"].([]any)
	pv := pvs[0].(map[string]any)
	if pv["value"] != "hello" {
		t.Errorf("value = %v", pv["value"])
	}
	if pv["presentation@odata.bind"] != graphBeta+"/deviceManagement/groupPolicyDefinitions('def-guid')/presentations('pres-guid')" {
		t.Errorf("presentation bind = %v", pv["presentation@odata.bind"])
	}
	if _, ok := pv["presentation"]; ok {
		t.Error("nested presentation object should be removed")
	}
	if _, ok := pv["id"]; ok {
		t.Error("presentation value id should be removed")
	}
}

func TestCreateIntentRequiresTemplate(t *testing.T) {
	raw := []byte(`{"id":"1","displayName":"x","settings":[]}`)
	if _, err := createIntent(context.TODO(), nil, catalog.PolicyType{Version: "beta"}, raw, ""); err == nil {
		t.Error("expected error when templateId is missing")
	}
}

// Graph rejects compliance policy creates whose rules carry no block action
// ("must have one and only one block scheduled action"). Old backups captured
// scheduledActionsForRule without the nested scheduledActionConfigurations, so
// creates must inject the portal-default block action for them.
func TestPrepareCreateComplianceInjectsDefaultBlockAction(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{"rule without configurations", `{"@odata.type":"#microsoft.graph.windows10CompliancePolicy","displayName":"C","scheduledActionsForRule":[{"ruleName":null}]}`},
		{"no rules at all", `{"@odata.type":"#microsoft.graph.windows10CompliancePolicy","displayName":"C"}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, _, body, err := PrepareCreate("CompliancePolicies", []byte(c.raw), "")
			if err != nil {
				t.Fatal(err)
			}
			var m map[string]any
			if err := json.Unmarshal(body, &m); err != nil {
				t.Fatal(err)
			}
			rules, _ := m["scheduledActionsForRule"].([]any)
			if len(rules) != 1 {
				t.Fatalf("rules = %v, want exactly one", rules)
			}
			cfgs, _ := rules[0].(map[string]any)["scheduledActionConfigurations"].([]any)
			if len(cfgs) != 1 || cfgs[0].(map[string]any)["actionType"] != "block" {
				t.Fatalf("configurations = %v, want one default block action", cfgs)
			}
		})
	}
}

func TestPrepareCreateComplianceKeepsExistingBlockAction(t *testing.T) {
	raw := []byte(`{"@odata.type":"#microsoft.graph.windows10CompliancePolicy","displayName":"C",
		"scheduledActionsForRule":[{"ruleName":null,"scheduledActionConfigurations":[
			{"actionType":"block","gracePeriodHours":12,"notificationTemplateId":"11111111-1111-1111-1111-111111111111"}]}]}`)
	_, _, body, err := PrepareCreate("CompliancePolicies", raw, "")
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatal(err)
	}
	rules := m["scheduledActionsForRule"].([]any)
	cfgs := rules[0].(map[string]any)["scheduledActionConfigurations"].([]any)
	if len(cfgs) != 1 {
		t.Fatalf("configurations = %v, want the captured action only", cfgs)
	}
	got := cfgs[0].(map[string]any)
	if got["gracePeriodHours"] != float64(12) {
		t.Errorf("gracePeriodHours = %v, want the captured 12", got["gracePeriodHours"])
	}
}
