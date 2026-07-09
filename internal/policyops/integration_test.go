package policyops

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
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

func TestCreateGroupPolicyRollsBackWhenSettingsFail(t *testing.T) {
	raw := []byte(`{"displayName":"ADM Test","definitionValues":[{"enabled":true,"definition":{"id":"def-1"}}]}`)
	f := &graphtest.Fake{PostFunc: func(_ string, path string, _ json.RawMessage) (json.RawMessage, error) {
		if strings.HasSuffix(path, "/updateDefinitionValues") {
			return nil, errors.New("settings rejected")
		}
		return json.RawMessage(`{"id":"created-1"}`), nil
	}}

	id, err := Create(context.Background(), f, "GroupPolicyConfigurations", raw, "")
	if err == nil || id != "" {
		t.Fatalf("id=%q err=%v, want rolled-back error with no live id", id, err)
	}
	if len(f.Deletes) != 1 || !strings.HasSuffix(f.Deletes[0], "/groupPolicyConfigurations/created-1") {
		t.Fatalf("rollback deletes = %v", f.Deletes)
	}
}

func TestCreateGroupPolicyReportsFailedRollback(t *testing.T) {
	raw := []byte(`{"displayName":"ADM Test","definitionValues":[{"enabled":true,"definition":{"id":"def-1"}}]}`)
	f := &graphtest.Fake{
		PostFunc: func(_ string, path string, _ json.RawMessage) (json.RawMessage, error) {
			if strings.HasSuffix(path, "/updateDefinitionValues") {
				return nil, errors.New("settings rejected")
			}
			return json.RawMessage(`{"id":"created-1"}`), nil
		},
		DeleteFunc: func(context.Context, string, string) error { return errors.New("delete denied") },
	}

	id, err := Create(context.Background(), f, "GroupPolicyConfigurations", raw, "")
	var partial *PartialCreateError
	if id != "created-1" || !errors.As(err, &partial) || partial.PolicyID != id {
		t.Fatalf("id=%q err=%#v, want typed partial-create error", id, err)
	}
}

func TestCreateGroupPolicyRollbackSurvivesCancelledOperationContext(t *testing.T) {
	raw := []byte(`{"displayName":"ADM Test","definitionValues":[{"enabled":true,"definition":{"id":"def-1"}}]}`)
	var rollbackCtxErr error
	f := &graphtest.Fake{
		PostFunc: func(_ string, path string, _ json.RawMessage) (json.RawMessage, error) {
			if strings.HasSuffix(path, "/updateDefinitionValues") {
				return nil, context.Canceled
			}
			return json.RawMessage(`{"id":"created-1"}`), nil
		},
		DeleteFunc: func(ctx context.Context, _, _ string) error {
			rollbackCtxErr = ctx.Err()
			return nil
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if id, err := Create(ctx, f, "GroupPolicyConfigurations", raw, ""); err == nil || id != "" {
		t.Fatalf("id=%q err=%v", id, err)
	}
	if rollbackCtxErr != nil || len(f.Deletes) != 1 {
		t.Fatalf("rollback ctx err=%v deletes=%v", rollbackCtxErr, f.Deletes)
	}
}

func TestCreateGroupPolicyRejectsMissingCreatedIDBeforeSettingsWrite(t *testing.T) {
	raw := []byte(`{"displayName":"ADM Test","definitionValues":[{"enabled":true,"definition":{"id":"def-1"}}]}`)
	f := &graphtest.Fake{PostFunc: func(_ string, _ string, _ json.RawMessage) (json.RawMessage, error) {
		return json.RawMessage(`{}`), nil
	}}
	if _, err := Create(context.Background(), f, "GroupPolicyConfigurations", raw, ""); err == nil {
		t.Fatal("missing created id should fail")
	}
	if len(f.Posts) != 1 || len(f.Deletes) != 0 {
		t.Fatalf("posts=%d deletes=%v", len(f.Posts), f.Deletes)
	}
}

func TestCreateGroupPolicyRejectsInvalidDefinitionBeforeCreating(t *testing.T) {
	raw := []byte(`{"displayName":"ADM Test","definitionValues":[{"enabled":true,"definition":{}}]}`)
	f := &graphtest.Fake{}
	if _, err := Create(context.Background(), f, "GroupPolicyConfigurations", raw, ""); err == nil {
		t.Fatal("missing definition id should fail")
	}
	if len(f.Posts) != 0 {
		t.Fatalf("invalid policy should not be created, posts=%d", len(f.Posts))
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

func TestFetchFullWarnsWhenAssignmentsFail(t *testing.T) {
	pt, ok := catalog.ByKey("deviceConfigurations")
	if !ok {
		t.Fatal("deviceConfigurations missing from catalog")
	}
	f := &graphtest.Fake{ListFunc: func(_, path string, _ url.Values) ([]json.RawMessage, error) {
		if strings.HasSuffix(path, "/assignments") {
			return nil, errors.New("forbidden")
		}
		return nil, nil
	}}
	_, warn := FetchFull(context.Background(), f, pt, []byte(`{"id":"cfg-1","displayName":"x"}`), true)
	if !warn {
		t.Fatal("assignment failure must mark the policy incomplete")
	}
}

func TestFetchFullWarnsWhenGroupPolicyPresentationsFail(t *testing.T) {
	pt, ok := catalog.ByKey("groupPolicyConfigurations")
	if !ok {
		t.Fatal("groupPolicyConfigurations missing from catalog")
	}
	f := &graphtest.Fake{ListFunc: func(_, path string, _ url.Values) ([]json.RawMessage, error) {
		if strings.HasSuffix(path, "/definitionValues") {
			return []json.RawMessage{json.RawMessage(`{"id":"dv-1","definition":{"id":"def-1"}}`)}, nil
		}
		if strings.HasSuffix(path, "/presentationValues") {
			return nil, errors.New("presentation fetch failed")
		}
		return nil, nil
	}}
	_, warn := FetchFull(context.Background(), f, pt, []byte(`{"id":"cfg-1","displayName":"x"}`), false)
	if !warn {
		t.Fatal("presentation failure must mark the policy incomplete")
	}
}
