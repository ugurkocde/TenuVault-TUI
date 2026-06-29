// Package catalog is the single source of truth for which Intune policy types
// TenuVault backs up and restores, and the Microsoft Graph endpoints for each.
//
// Every endpoint here was verified live against a tenant via the Lokka MCP /
// msgraph skill. Folder names for the original core set match the TenuVault
// portal runbook for cross-compatibility; newer types use clear unique names.
package catalog

// SubResource describes a nested collection that must be fetched separately and
// embedded into the saved policy JSON (e.g. admin-template definitionValues,
// security-baseline intent settings).
type SubResource struct {
	// Suffix is appended to the item path: /{ListPath}/{id}/{Suffix}.
	Suffix string
	// Expand is the $expand applied to the sub-resource fetch.
	Expand string
	// EmbedKey is the JSON property the fetched value array is stored under.
	EmbedKey string
}

// PolicyType describes one backup category and how to read/write it via Graph.
type PolicyType struct {
	Key      string // stable identifier used in flags/config
	Friendly string // label shown in the TUI
	Category string // backup subfolder name
	Group    string // UI grouping header
	Version  string // "v1.0" or "beta"
	ListPath string // collection endpoint (no version prefix)

	NameField string // JSON property holding the display name (for filenames)

	DetailByID bool         // fetch /{ListPath}/{id} per item for full content
	Expand     string       // $expand applied to the per-item detail fetch
	Sub        *SubResource // optional nested collection to embed

	// CreateMode selects the restore/create strategy: "" = simple POST,
	// "groupPolicy" = config + per-setting definitionValues, "intent" = template
	// createInstance with settingsDelta.
	CreateMode string

	RestoreSupported bool // false = backup-only (complex/upload/singleton)
	Verified         bool // confirmed live via Lokka
}

// RestoreEndpoint returns the path a restored item is POSTed to.
func (p PolicyType) RestoreEndpoint() string { return p.ListPath }

// All returns the full policy-type registry.
func All() []PolicyType {
	return []PolicyType{
		// --- Configuration & compliance ---
		{Key: "deviceConfigurations", Friendly: "Device configurations", Category: "DeviceConfigurations", Group: "Configuration", Version: "beta",
			ListPath: "/deviceManagement/deviceConfigurations", NameField: "displayName", RestoreSupported: true, Verified: true},
		{Key: "configurationPolicies", Friendly: "Settings catalog", Category: "ConfigurationPolicies", Group: "Configuration", Version: "beta",
			ListPath: "/deviceManagement/configurationPolicies", NameField: "name", DetailByID: true, Expand: "settings", RestoreSupported: true, Verified: true},
		{Key: "groupPolicyConfigurations", Friendly: "Administrative templates", Category: "GroupPolicyConfigurations", Group: "Configuration", Version: "beta",
			ListPath: "/deviceManagement/groupPolicyConfigurations", NameField: "displayName",
			CreateMode: "groupPolicy", RestoreSupported: true, Verified: true},
		{Key: "compliancePolicies", Friendly: "Compliance policies", Category: "CompliancePolicies", Group: "Configuration", Version: "beta",
			ListPath: "/deviceManagement/deviceCompliancePolicies", NameField: "displayName", DetailByID: true, Expand: "scheduledActionsForRule", RestoreSupported: true, Verified: true},
		{Key: "intents", Friendly: "Endpoint security / baselines", Category: "EndpointSecurityIntents", Group: "Configuration", Version: "beta",
			ListPath: "/deviceManagement/intents", NameField: "displayName",
			Sub:        &SubResource{Suffix: "settings", EmbedKey: "settings"},
			CreateMode: "intent", RestoreSupported: true, Verified: true},

		// --- Scripts & remediations (content only on per-item GET) ---
		{Key: "deviceManagementScripts", Friendly: "Windows scripts", Category: "DeviceManagementScripts", Group: "Scripts", Version: "beta",
			ListPath: "/deviceManagement/deviceManagementScripts", NameField: "displayName", DetailByID: true, RestoreSupported: true, Verified: true},
		{Key: "deviceShellScripts", Friendly: "macOS shell scripts", Category: "ShellScripts", Group: "Scripts", Version: "beta",
			ListPath: "/deviceManagement/deviceShellScripts", NameField: "displayName", DetailByID: true, RestoreSupported: true, Verified: true},
		{Key: "deviceHealthScripts", Friendly: "Proactive remediations", Category: "ProactiveRemediations", Group: "Scripts", Version: "beta",
			ListPath: "/deviceManagement/deviceHealthScripts", NameField: "displayName", DetailByID: true, RestoreSupported: true, Verified: true},
		{Key: "customAttributeScripts", Friendly: "macOS custom attributes", Category: "CustomAttributeScripts", Group: "Scripts", Version: "beta",
			ListPath: "/deviceManagement/deviceCustomAttributeShellScripts", NameField: "displayName", DetailByID: true, RestoreSupported: true, Verified: true},

		// --- Enrollment, Autopilot, updates ---
		{Key: "autopilotProfiles", Friendly: "Autopilot profiles", Category: "AutopilotProfiles", Group: "Enrollment & updates", Version: "beta",
			ListPath: "/deviceManagement/windowsAutopilotDeploymentProfiles", NameField: "displayName", RestoreSupported: true, Verified: true},
		{Key: "enrollmentConfigurations", Friendly: "Enrollment configurations", Category: "EnrollmentConfigurations", Group: "Enrollment & updates", Version: "beta",
			ListPath: "/deviceManagement/deviceEnrollmentConfigurations", NameField: "displayName", RestoreSupported: true, Verified: true},
		{Key: "featureUpdateProfiles", Friendly: "Feature update profiles", Category: "FeatureUpdateProfiles", Group: "Enrollment & updates", Version: "beta",
			ListPath: "/deviceManagement/windowsFeatureUpdateProfiles", NameField: "displayName", RestoreSupported: true, Verified: true},
		{Key: "qualityUpdateProfiles", Friendly: "Quality update profiles", Category: "QualityUpdateProfiles", Group: "Enrollment & updates", Version: "beta",
			ListPath: "/deviceManagement/windowsQualityUpdateProfiles", NameField: "displayName", RestoreSupported: true, Verified: true},
		{Key: "driverUpdateProfiles", Friendly: "Driver update profiles", Category: "DriverUpdateProfiles", Group: "Enrollment & updates", Version: "beta",
			ListPath: "/deviceManagement/windowsDriverUpdateProfiles", NameField: "displayName", RestoreSupported: true, Verified: true},

		// --- Tenant administration ---
		{Key: "roleScopeTags", Friendly: "Scope tags", Category: "RoleScopeTags", Group: "Tenant admin", Version: "beta",
			ListPath: "/deviceManagement/roleScopeTags", NameField: "displayName", RestoreSupported: true, Verified: true},
		{Key: "deviceCategories", Friendly: "Device categories", Category: "DeviceCategories", Group: "Tenant admin", Version: "beta",
			ListPath: "/deviceManagement/deviceCategories", NameField: "displayName", RestoreSupported: true, Verified: true},
		{Key: "termsAndConditions", Friendly: "Terms and conditions", Category: "TermsAndConditions", Group: "Tenant admin", Version: "beta",
			ListPath: "/deviceManagement/termsAndConditions", NameField: "displayName", RestoreSupported: true, Verified: true},
		{Key: "notificationTemplates", Friendly: "Notification templates", Category: "NotificationTemplates", Group: "Tenant admin", Version: "beta",
			ListPath: "/deviceManagement/notificationMessageTemplates", NameField: "displayName", RestoreSupported: true, Verified: true},
		{Key: "assignmentFilters", Friendly: "Assignment filters", Category: "AssignmentFilters", Group: "Tenant admin", Version: "beta",
			ListPath: "/deviceManagement/assignmentFilters", NameField: "displayName", RestoreSupported: true, Verified: true},

		// --- Apps & app management ---
		{Key: "appConfigDevice", Friendly: "App configuration (devices)", Category: "AppConfigurationPolicies", Group: "Apps", Version: "beta",
			ListPath: "/deviceAppManagement/mobileAppConfigurations", NameField: "displayName", RestoreSupported: true, Verified: true},
		{Key: "appConfigManaged", Friendly: "App configuration (managed apps)", Category: "ManagedAppConfigurations", Group: "Apps", Version: "beta",
			ListPath: "/deviceAppManagement/targetedManagedAppConfigurations", NameField: "displayName", RestoreSupported: true, Verified: true},
		{Key: "iosAppProtection", Friendly: "App protection (iOS)", Category: "AppProtectionPolicies", Group: "Apps", Version: "beta",
			ListPath: "/deviceAppManagement/iosManagedAppProtections", NameField: "displayName", RestoreSupported: true, Verified: true},
		{Key: "androidAppProtection", Friendly: "App protection (Android)", Category: "AppProtectionPolicies", Group: "Apps", Version: "beta",
			ListPath: "/deviceAppManagement/androidManagedAppProtections", NameField: "displayName", RestoreSupported: true, Verified: true},
		{Key: "windowsAppProtection", Friendly: "App protection (Windows)", Category: "AppProtectionPolicies", Group: "Apps", Version: "beta",
			ListPath: "/deviceAppManagement/windowsManagedAppProtections", NameField: "displayName", RestoreSupported: true, Verified: true},
		{Key: "windowsInformationProtection", Friendly: "Windows information protection", Category: "WindowsInformationProtection", Group: "Apps", Version: "beta",
			ListPath: "/deviceAppManagement/mdmWindowsInformationProtectionPolicies", NameField: "displayName", RestoreSupported: true, Verified: true},
		{Key: "appCategories", Friendly: "App categories", Category: "AppCategories", Group: "Apps", Version: "beta",
			ListPath: "/deviceAppManagement/mobileAppCategories", NameField: "displayName", RestoreSupported: true, Verified: true},

		// --- Identity ---
		{Key: "conditionalAccess", Friendly: "Conditional access", Category: "ConditionalAccessPolicies", Group: "Identity", Version: "beta",
			ListPath: "/identity/conditionalAccess/policies", NameField: "displayName", RestoreSupported: true, Verified: true},
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

// CategoryRestoreSupported reports whether any policy type in a category can be
// restored (app protection shares a category across iOS/Android/Windows).
func CategoryRestoreSupported(category string) bool {
	for _, p := range All() {
		if p.Category == category && p.RestoreSupported {
			return true
		}
	}
	return false
}

// DefaultSelection returns the keys selected by default in the backup picker
// (every verified type is selected).
func DefaultSelection() map[string]bool {
	sel := map[string]bool{}
	for _, p := range All() {
		sel[p.Key] = p.Verified
	}
	return sel
}
