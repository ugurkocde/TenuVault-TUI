package catalog

import "testing"

func TestRegistryInvariants(t *testing.T) {
	seen := map[string]bool{}
	for _, p := range All() {
		if p.Key == "" || p.Category == "" || p.ListPath == "" || p.NameField == "" {
			t.Errorf("incomplete entry: %+v", p)
		}
		if p.Version != "v1.0" && p.Version != "beta" {
			t.Errorf("%s: bad version %q", p.Key, p.Version)
		}
		if seen[p.Key] {
			t.Errorf("duplicate key %q", p.Key)
		}
		seen[p.Key] = true
	}
}

func TestCategoryRestoreSupported(t *testing.T) {
	// App protection shares a category; all variants are restorable.
	if !CategoryRestoreSupported("AppProtectionPolicies") {
		t.Error("AppProtectionPolicies should be restorable")
	}
	// Backup-only categories.
	for _, c := range []string{"GroupPolicyConfigurations", "EndpointSecurityIntents", "EnrollmentConfigurations"} {
		if CategoryRestoreSupported(c) {
			t.Errorf("%s should be backup-only", c)
		}
	}
	if CategoryRestoreSupported("Nonexistent") {
		t.Error("unknown category must not be restorable")
	}
}

func TestByKeyAndDefaults(t *testing.T) {
	if _, ok := ByKey("conditionalAccess"); !ok {
		t.Error("conditionalAccess missing")
	}
	sel := DefaultSelection()
	if sel["roleScopeTags"] {
		t.Error("unverified roleScopeTags should not be default-selected")
	}
	if !sel["deviceConfigurations"] {
		t.Error("deviceConfigurations should be default-selected")
	}
}
