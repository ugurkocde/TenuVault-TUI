package restore

import (
	"encoding/json"
	"testing"
)

func TestPrepareCleansAndPrefixes(t *testing.T) {
	raw := []byte(`{
		"@odata.type":"#microsoft.graph.windows10GeneralConfiguration",
		"@odata.context":"ctx",
		"id":"abc","version":3,
		"createdDateTime":"2020","lastModifiedDateTime":"2021",
		"displayName":"BitLocker","assignments":[{"x":1}]
	}`)
	version, endpoint, body, err := prepare("DeviceConfigurations", raw)
	if err != nil {
		t.Fatal(err)
	}
	if version != "v1.0" {
		t.Errorf("version = %q", version)
	}
	if endpoint != "/deviceManagement/deviceConfigurations" {
		t.Errorf("endpoint = %q", endpoint)
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
		t.Error("@odata.type must be preserved for restore routing")
	}
}

func TestPrepareConditionalAccessDisabled(t *testing.T) {
	raw := []byte(`{"id":"1","displayName":"CA","state":"enabled"}`)
	_, _, body, err := prepare("ConditionalAccessPolicies", raw)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	_ = json.Unmarshal(body, &m)
	if m["state"] != "disabled" {
		t.Errorf("CA state = %v, want disabled", m["state"])
	}
}

func TestRouteAppProtectionByODataType(t *testing.T) {
	ios := []byte(`{"@odata.type":"#microsoft.graph.iosManagedAppProtection","displayName":"x"}`)
	pt, ok := routeByType("AppProtectionPolicies", ios)
	if !ok || pt.Key != "iosAppProtection" {
		t.Errorf("ios routing failed: %+v ok=%v", pt, ok)
	}
}
