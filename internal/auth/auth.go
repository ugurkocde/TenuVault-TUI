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

// GraphScope requests all delegated/application permissions already consented
// for the client. With the well-known Graph PowerShell app this covers the
// DeviceManagement.ReadWrite scopes needed for backup and restore.
var GraphScope = []string{"https://graph.microsoft.com/.default"}

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
