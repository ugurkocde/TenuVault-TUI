// Package config loads and persists TenuVault TUI settings.
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// WellKnownClientID is the Microsoft Graph PowerShell public client. It carries
// broad pre-consented delegated permissions, so interactive and device-code
// sign-in work with no app registration. Mirrors MgGraphCommunity's default.
const WellKnownClientID = "14d82eec-204b-4c2f-b7e8-296a70dab67e"

// AuthMethod selects how the TUI authenticates to Microsoft Graph.
type AuthMethod string

const (
	AuthInteractive AuthMethod = "interactive"
	AuthDeviceCode  AuthMethod = "devicecode"
	AuthSecret      AuthMethod = "secret"
	AuthCertificate AuthMethod = "certificate"
)

// Config is the persisted user configuration.
type Config struct {
	TenantID   string     `json:"tenantId"`
	ClientID   string     `json:"clientId"`
	AuthMethod AuthMethod `json:"authMethod"`
	BackupRoot string     `json:"backupRoot"`

	// Behavior toggles.
	IncludeAssignments bool `json:"includeAssignments"`
	RetentionDays      int  `json:"retentionDays"`

	// Connections are the tenants the user has added (metadata only — no tokens
	// or secrets are ever persisted).
	Connections []ConnConfig `json:"connections,omitempty"`

	// Optional unattended credentials. Secrets are never persisted (see Save).
	ClientSecret        string `json:"-"`
	CertificatePath     string `json:"certificatePath,omitempty"`
	CertificatePassword string `json:"-"`
}

// ConnConfig is the persisted metadata for one tenant connection.
type ConnConfig struct {
	Label      string     `json:"label"`
	TenantID   string     `json:"tenantId"`
	ClientID   string     `json:"clientId"`
	AuthMethod AuthMethod `json:"authMethod"`
	CertPath   string     `json:"certPath,omitempty"`
}

// ToConnConfig captures a config as a reusable connection entry.
func (c Config) ToConnConfig(label string) ConnConfig {
	return ConnConfig{Label: label, TenantID: c.TenantID, ClientID: c.ClientID, AuthMethod: c.AuthMethod, CertPath: c.CertificatePath}
}

// Apply returns a copy of the base config with this connection's fields set,
// ready to authenticate. Secrets still come from the environment.
func (cc ConnConfig) Apply(base Config) Config {
	base.TenantID = cc.TenantID
	base.ClientID = cc.ClientID
	base.AuthMethod = cc.AuthMethod
	base.CertificatePath = cc.CertPath
	return base
}

// Default returns a config seeded with sensible defaults.
func Default() Config {
	return Config{
		TenantID:   firstNonEmpty(os.Getenv("AZURE_TENANT_ID"), "organizations"),
		ClientID:   firstNonEmpty(os.Getenv("AZURE_CLIENT_ID"), WellKnownClientID),
		AuthMethod: AuthInteractive,
		BackupRoot: defaultBackupRoot(),
	}
}

// Path returns the on-disk config location. TENUVAULT_CONFIG_DIR overrides the
// directory (used to relocate config and to isolate tests from the real file).
func Path() string {
	if d := os.Getenv("TENUVAULT_CONFIG_DIR"); d != "" {
		return filepath.Join(d, "config.json")
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "tenuvault", "config.json")
}

// Load reads the config from disk, falling back to defaults and overlaying env.
func Load() Config {
	c := Default()
	data, err := os.ReadFile(Path())
	if err == nil {
		_ = json.Unmarshal(data, &c)
	}
	// Env always wins for credentials so secrets never need to touch disk.
	if v := os.Getenv("AZURE_TENANT_ID"); v != "" {
		c.TenantID = v
	}
	if v := os.Getenv("AZURE_CLIENT_ID"); v != "" {
		c.ClientID = v
	}
	if v := os.Getenv("AZURE_CLIENT_SECRET"); v != "" {
		c.ClientSecret = v
		c.AuthMethod = AuthSecret
	}
	if c.ClientID == "" {
		c.ClientID = WellKnownClientID
	}
	if c.BackupRoot == "" {
		c.BackupRoot = defaultBackupRoot()
	}
	return c
}

// Save writes the config to disk, excluding the secret (kept in env only).
func Save(c Config) error {
	c.ClientSecret = ""
	if err := os.MkdirAll(filepath.Dir(Path()), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(Path(), data, 0o600)
}

func defaultBackupRoot() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "TenuVault"
	}
	return filepath.Join(home, "TenuVault")
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
