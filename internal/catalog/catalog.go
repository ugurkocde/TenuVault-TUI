// Package catalog is the single source of truth for which Intune policy types
// TenuVault backs up and restores, and the Microsoft Graph endpoints for each.
//
// Categories and folder names match the TenuVault portal runbook
// (TenuVault-Website/templates/runbook-script.ps1) so backups produced by the
// TUI are cross-compatible with the portal.
package catalog

// PolicyType describes one backup category and how to read/write it via Graph.
type PolicyType struct {
	// Key is a stable identifier used in flags and config.
	Key string
	// Friendly is the human label shown in the TUI.
	Friendly string
	// Category is the backup subfolder name (TenuVault-compatible).
	Category string
	// Version is the Graph API version: "v1.0" or "beta".
	Version string
	// ListPath is the collection endpoint (without the version prefix).
	ListPath string
	// Expand, when set, is the $expand value needed to capture full detail.
	Expand string
	// RestorePath is where a restored item is POSTed (usually == ListPath).
	RestorePath string
	// NameField is the JSON property holding the display name (for filenames).
	NameField string
	// Verified is true once the endpoint has been confirmed live via Lokka.
	Verified bool
}

// All returns the full policy-type registry.
func All() []PolicyType {
	return []PolicyType{
		{
			Key: "deviceConfigurations", Friendly: "Device configurations",
			Category: "DeviceConfigurations", Version: "v1.0",
			ListPath:  "/deviceManagement/deviceConfigurations",
			NameField: "displayName", Verified: true,
		},
		{
			Key: "compliancePolicies", Friendly: "Compliance policies",
			Category: "CompliancePolicies", Version: "v1.0",
			ListPath:  "/deviceManagement/deviceCompliancePolicies",
			NameField: "displayName", Verified: true,
		},
		{
			Key: "configurationPolicies", Friendly: "Settings catalog",
			Category: "ConfigurationPolicies", Version: "beta",
			ListPath: "/deviceManagement/configurationPolicies", Expand: "settings",
			NameField: "name", Verified: true,
		},
		{
			Key: "groupPolicyConfigurations", Friendly: "Administrative templates",
			Category: "GroupPolicyConfigurations", Version: "beta",
			ListPath:  "/deviceManagement/groupPolicyConfigurations",
			NameField: "displayName", Verified: false,
		},
		{
			Key: "deviceManagementScripts", Friendly: "Device scripts",
			Category: "DeviceManagementScripts", Version: "beta",
			ListPath:  "/deviceManagement/deviceManagementScripts",
			NameField: "displayName", Verified: true,
		},
		{
			Key: "mobileApps", Friendly: "Mobile apps",
			Category: "MobileApps", Version: "v1.0",
			ListPath:  "/deviceAppManagement/mobileApps",
			NameField: "displayName", Verified: false,
		},
		{
			Key: "iosAppProtection", Friendly: "App protection (iOS)",
			Category: "AppProtectionPolicies", Version: "v1.0",
			ListPath:  "/deviceAppManagement/iosManagedAppProtections",
			NameField: "displayName", Verified: false,
		},
		{
			Key: "androidAppProtection", Friendly: "App protection (Android)",
			Category: "AppProtectionPolicies", Version: "v1.0",
			ListPath:  "/deviceAppManagement/androidManagedAppProtections",
			NameField: "displayName", Verified: false,
		},
		{
			Key: "conditionalAccess", Friendly: "Conditional access",
			Category: "ConditionalAccessPolicies", Version: "v1.0",
			ListPath:  "/identity/conditionalAccess/policies",
			NameField: "displayName", Verified: false,
		},
	}
}

// ByKey returns the policy type with the given key, or false.
func ByKey(key string) (PolicyType, bool) {
	for _, p := range All() {
		if p.Key == key {
			return p, true
		}
	}
	return PolicyType{}, false
}

// RestoreEndpoint returns the path to POST a restored item to.
func (p PolicyType) RestoreEndpoint() string {
	if p.RestorePath != "" {
		return p.RestorePath
	}
	return p.ListPath
}
