// Package connection models an authenticated tenant connection. The TUI holds a
// list of these so the user can sign into multiple tenants at once and pick a
// source and target for policy sync.
package connection

import (
	"github.com/ugurkocde/TenuVault-TUI/internal/config"
	"github.com/ugurkocde/TenuVault-TUI/internal/graph"
)

// Connection is one authenticated tenant.
type Connection struct {
	Label  string
	Cfg    config.Config
	Tenant graph.Tenant
	Client *graph.Client
}

// DisplayLabel returns the label, falling back to the tenant name.
func (c Connection) DisplayLabel() string {
	if c.Label != "" {
		return c.Label
	}
	return c.Tenant.DisplayName
}

// Connected reports whether the connection has a live client.
func (c Connection) Connected() bool { return c.Client != nil }

// AsConfig captures this connection as persistable metadata.
func (c Connection) AsConfig() config.ConnConfig {
	return c.Cfg.ToConnConfig(c.DisplayLabel())
}
