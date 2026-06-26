package graph

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
)

// Tenant is a summary of the signed-in organization.
type Tenant struct {
	ID            string
	DisplayName   string
	DefaultDomain string
	DomainCount   int
}

// Organization fetches the signed-in tenant. It doubles as a token/permission
// smoke test after authentication.
func (c *Client) Organization(ctx context.Context) (Tenant, error) {
	q := url.Values{"$select": {"id,displayName,verifiedDomains"}}
	data, err := c.Get(ctx, "beta", "/organization", q)
	if err != nil {
		return Tenant{}, err
	}
	var env struct {
		Value []struct {
			ID              string `json:"id"`
			DisplayName     string `json:"displayName"`
			VerifiedDomains []struct {
				Name      string `json:"name"`
				IsDefault bool   `json:"isDefault"`
			} `json:"verifiedDomains"`
		} `json:"value"`
	}
	if err := json.Unmarshal(data, &env); err != nil {
		return Tenant{}, fmt.Errorf("decode organization: %w", err)
	}
	if len(env.Value) == 0 {
		return Tenant{}, fmt.Errorf("no organization returned")
	}
	o := env.Value[0]
	t := Tenant{ID: o.ID, DisplayName: o.DisplayName, DomainCount: len(o.VerifiedDomains)}
	for _, d := range o.VerifiedDomains {
		if d.IsDefault {
			t.DefaultDomain = d.Name
		}
	}
	return t, nil
}
