package policyops

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ugurkocde/TenuVault-TUI/internal/catalog"
	"github.com/ugurkocde/TenuVault-TUI/internal/graphtest"
)

func TestCreateGroupPolicyMultiPart(t *testing.T) {
	raw := []byte(`{
		"id":"old-id","displayName":"ADM Test","description":"",
		"roleScopeTagIds":["0"],
		"definitionValues":[
			{"enabled":true,"configurationType":"policy",
			 "definition":{"id":"def-1"},
			 "presentationValues":[
				{"@odata.type":"#microsoft.graph.groupPolicyPresentationValueText","value":"hi","presentation":{"id":"pres-1"}}
			 ]}
		]
	}`)
	f := &graphtest.Fake{}
	id, err := Create(context.Background(), f, "GroupPolicyConfigurations", raw, "[Restored] ")
	if err != nil {
		t.Fatal(err)
	}
	if id == "" {
		t.Fatal("no id returned")
	}
	if len(f.Posts) != 2 {
		t.Fatalf("expected 2 POSTs (config + updateDefinitionValues), got %d", len(f.Posts))
	}

	// 1) The config POST: name prefixed, no settings inline.
	var cfg map[string]any
	_ = json.Unmarshal(f.Posts[0].Body, &cfg)
	if f.Posts[0].Path != "/deviceManagement/groupPolicyConfigurations" {
		t.Errorf("config path = %q", f.Posts[0].Path)
	}
	if cfg["displayName"] != "[Restored] ADM Test" {
		t.Errorf("displayName = %v", cfg["displayName"])
	}
	if _, ok := cfg["definitionValues"]; ok {
		t.Error("definitionValues must not be inline in the config POST")
	}
	if _, ok := cfg["id"]; ok {
		t.Error("id must be stripped")
	}

	// 2) The updateDefinitionValues POST: bound settings.
	if !strings.Contains(f.Posts[1].Path, "/updateDefinitionValues") {
		t.Fatalf("second POST path = %q", f.Posts[1].Path)
	}
	var upd struct {
		Added []map[string]any `json:"added"`
	}
	if err := json.Unmarshal(f.Posts[1].Body, &upd); err != nil {
		t.Fatal(err)
	}
	if len(upd.Added) != 1 {
		t.Fatalf("added = %d, want 1", len(upd.Added))
	}
	dv := upd.Added[0]
	if b, _ := dv["definition@odata.bind"].(string); !strings.Contains(b, "groupPolicyDefinitions('def-1')") {
		t.Errorf("definition bind = %v", dv["definition@odata.bind"])
	}
	pvs, _ := dv["presentationValues"].([]any)
	if len(pvs) != 1 {
		t.Fatalf("presentationValues = %d, want 1", len(pvs))
	}
	pv := pvs[0].(map[string]any)
	if pv["value"] != "hi" {
		t.Errorf("presentation value = %v", pv["value"])
	}
	if b, _ := pv["presentation@odata.bind"].(string); !strings.Contains(b, "presentations('pres-1')") {
		t.Errorf("presentation bind = %v", pv["presentation@odata.bind"])
	}
}

func TestCreateIntentUsesCreateInstance(t *testing.T) {
	raw := []byte(`{
		"id":"old","displayName":"Baseline","description":"d",
		"templateId":"tmpl-1","roleScopeTagIds":["0"],
		"settings":[{"@odata.type":"#microsoft.graph.deviceManagementBooleanSettingInstance","id":"s1","definitionId":"def","valueJson":"true"}]
	}`)
	f := &graphtest.Fake{}
	if _, err := Create(context.Background(), f, "EndpointSecurityIntents", raw, "[Restored] "); err != nil {
		t.Fatal(err)
	}
	if len(f.Posts) != 1 {
		t.Fatalf("expected 1 POST, got %d", len(f.Posts))
	}
	if !strings.Contains(f.Posts[0].Path, "/deviceManagement/templates/tmpl-1/createInstance") {
		t.Errorf("path = %q", f.Posts[0].Path)
	}
	var body struct {
		DisplayName   string           `json:"displayName"`
		SettingsDelta []map[string]any `json:"settingsDelta"`
	}
	_ = json.Unmarshal(f.Posts[0].Body, &body)
	if body.DisplayName != "[Restored] Baseline" {
		t.Errorf("displayName = %q", body.DisplayName)
	}
	if len(body.SettingsDelta) != 1 || body.SettingsDelta[0]["definitionId"] != "def" {
		t.Errorf("settingsDelta = %+v", body.SettingsDelta)
	}
	if _, ok := body.SettingsDelta[0]["id"]; ok {
		t.Error("setting id should be stripped")
	}
}

func TestFetchFullGroupPolicyTwoLevel(t *testing.T) {
	base := "/deviceManagement/groupPolicyConfigurations/cfg-1"
	f := &graphtest.Fake{
		Lists: map[string][]json.RawMessage{
			"beta " + base + "/definitionValues": {
				[]byte(`{"id":"dv-1","enabled":true,"definition":{"id":"def-1"}}`),
			},
			"beta " + base + "/definitionValues/dv-1/presentationValues": {
				[]byte(`{"@odata.type":"#microsoft.graph.groupPolicyPresentationValueText","value":"x","presentation":{"id":"pres-1"}}`),
			},
		},
	}
	gp, ok := catalog.ByKey("groupPolicyConfigurations")
	if !ok {
		t.Fatal("groupPolicyConfigurations missing from catalog")
	}
	full, warn := FetchFull(context.Background(), f, gp, []byte(`{"id":"cfg-1","displayName":"x"}`), false)
	if warn {
		t.Error("unexpected warn")
	}
	var m map[string]any
	_ = json.Unmarshal(full, &m)
	dvs, _ := m["definitionValues"].([]any)
	if len(dvs) != 1 {
		t.Fatalf("definitionValues = %d, want 1", len(dvs))
	}
	dv := dvs[0].(map[string]any)
	pvs, _ := dv["presentationValues"].([]any)
	if len(pvs) != 1 {
		t.Errorf("presentationValues not embedded: %+v", dv)
	}
}
