package policyops

import (
	"encoding/json"
	"testing"
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
	if version != "v1.0" || endpoint != "/deviceManagement/deviceConfigurations" {
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

func TestBackupOnlyRejected(t *testing.T) {
	raw := []byte(`{"id":"1","displayName":"Template"}`)
	if _, _, _, err := PrepareCreate("GroupPolicyConfigurations", raw, ""); err == nil {
		t.Error("group policy configurations should be rejected as backup-only")
	}
}

func TestRouteAppProtectionByODataType(t *testing.T) {
	ios := []byte(`{"@odata.type":"#microsoft.graph.iosManagedAppProtection","displayName":"x"}`)
	pt, ok := routeByType("AppProtectionPolicies", ios)
	if !ok || pt.Key != "iosAppProtection" {
		t.Errorf("ios routing failed: %+v ok=%v", pt, ok)
	}
}
