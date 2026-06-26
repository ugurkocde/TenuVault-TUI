// Package auth builds a Microsoft Graph token credential from the configured
// flow. It reimplements the flows of the MgGraphCommunity PowerShell module
// natively in Go (interactive PKCE, device code, client secret, certificate)
// using the Azure SDK, so the TUI ships as a single self-contained binary.
package auth

import (
	"context"
	"fmt"
	"os"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"

	"github.com/ugurkocde/TenuVault-TUI/internal/config"
)

// AppScope is used for app-only (client secret / certificate) flows, where the
// granted permissions are admin-consented on the app registration itself.
var AppScope = []string{"https://graph.microsoft.com/.default"}

// DelegatedScopes are requested explicitly for interactive / device-code sign-in
// so Microsoft Entra prompts the signed-in admin to consent to the full set the
// tool needs in that tenant — backup (read), and restore/sync (create). Each is
// ReadWrite (which includes Read). Verified against Microsoft Graph docs.
var DelegatedScopes = []string{
	"https://graph.microsoft.com/DeviceManagementConfiguration.ReadWrite.All",
	"https://graph.microsoft.com/DeviceManagementApps.ReadWrite.All",
	"https://graph.microsoft.com/DeviceManagementServiceConfig.ReadWrite.All",
	"https://graph.microsoft.com/DeviceManagementScripts.ReadWrite.All",
	"https://graph.microsoft.com/DeviceManagementRBAC.ReadWrite.All",
	"https://graph.microsoft.com/Policy.ReadWrite.ConditionalAccess",
	"https://graph.microsoft.com/Organization.Read.All",
}

// ScopesFor returns the token scopes to request for the given auth method.
// Delegated flows request the explicit scope set (prompting consent per tenant);
// app-only flows use .default (permissions are consented on the app registration).
func ScopesFor(method config.AuthMethod) []string {
	switch method {
	case config.AuthSecret, config.AuthCertificate:
		return AppScope
	default:
		return DelegatedScopes
	}
}

// DeviceCodePrompt is called with the device-code instructions to show the user.
type DeviceCodePrompt func(message string)

// New constructs a token credential for the configured authentication method.
// prompt may be nil for non-device-code flows.
func New(cfg config.Config, prompt DeviceCodePrompt) (azcore.TokenCredential, error) {
	clientID := cfg.ClientID
	if clientID == "" {
		clientID = config.WellKnownClientID
	}
	tenantID := cfg.TenantID
	if tenantID == "" {
		tenantID = "organizations"
	}

	switch cfg.AuthMethod {
	case config.AuthDeviceCode:
		return azidentity.NewDeviceCodeCredential(&azidentity.DeviceCodeCredentialOptions{
			ClientID: clientID,
			TenantID: tenantID,
			UserPrompt: func(_ context.Context, m azidentity.DeviceCodeMessage) error {
				if prompt != nil {
					prompt(m.Message)
				}
				return nil
			},
		})

	case config.AuthSecret:
		secret := cfg.ClientSecret
		if secret == "" {
			secret = os.Getenv("AZURE_CLIENT_SECRET")
		}
		if secret == "" {
			return nil, fmt.Errorf("client secret auth selected but no secret provided (set AZURE_CLIENT_SECRET)")
		}
		if tenantID == "organizations" {
			return nil, fmt.Errorf("client secret auth requires a concrete tenant id")
		}
		return azidentity.NewClientSecretCredential(tenantID, clientID, secret, nil)

	case config.AuthCertificate:
		if cfg.CertificatePath == "" {
			return nil, fmt.Errorf("certificate auth selected but no certificatePath provided")
		}
		data, err := os.ReadFile(cfg.CertificatePath)
		if err != nil {
			return nil, fmt.Errorf("read certificate: %w", err)
		}
		var password []byte
		if p := os.Getenv("AZURE_CLIENT_CERTIFICATE_PASSWORD"); p != "" {
			password = []byte(p)
		}
		certs, key, err := azidentity.ParseCertificates(data, password)
		if err != nil {
			return nil, fmt.Errorf("parse certificate: %w", err)
		}
		if tenantID == "organizations" {
			return nil, fmt.Errorf("certificate auth requires a concrete tenant id")
		}
		return azidentity.NewClientCertificateCredential(tenantID, clientID, certs, key, nil)

	default: // AuthInteractive
		return azidentity.NewInteractiveBrowserCredential(&azidentity.InteractiveBrowserCredentialOptions{
			ClientID: clientID,
			TenantID: tenantID,
		})
	}
}
