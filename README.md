# TenuVault TUI

A fast, self-contained terminal UI for backing up and restoring Microsoft Intune
configuration via Microsoft Graph. Sign in, back up your policies to local
TenuVault-format folders, compare backups for drift, and restore policies back to
your tenant — all from a single binary, no PowerShell or cloud setup required.

Built with Go and the [Charm](https://charm.land) stack (Bubble Tea, Lip Gloss).

## Features

- Interactive browser sign-in (delegated; no app registration needed), or
  app-registration sign-in entered in the UI (client secret or certificate).
  Device code is intentionally not supported (phishing risk).
- Backs up 27 Intune policy types to per-policy JSON files plus a
  `metadata.json` manifest and a `backup.log`, in a layout cross-compatible with
  the TenuVault portal. Script content, settings-catalog settings, admin-template
  definition values, and security-baseline intent settings are fetched in full.
- Optional capture of assignments alongside each policy.
- Every category's outcome is surfaced: a backup reports `Success`,
  `CompletedWithWarnings`, or `Failed`, and lists exactly which categories failed
  and why (e.g. a missing Graph scope) — no silent drops.
- Browse local backups, drill into categories, inspect any policy's JSON, and
  compare any two backups for configuration drift with severity highlighting.
- Restore selected policies back to Graph with a dry-run preview, an explicit
  confirmation, and a `[Restored]` name prefix so nothing is overwritten.
  Conditional Access policies are restored disabled by default.
- Settings for assignment capture, backup retention, and sign-in method.
- Connect multiple tenants and **sync policies tenant-to-tenant** (e.g. dev to
  prod): pick whole types or individual policies from a live source tenant or a
  saved backup, choose a target, and create them there. Sync is create-only and
  **never overwrites** — it only ever creates new policies, leaving existing ones
  untouched (Conditional Access arrives disabled, assignments are not copied).

## Coverage

Configuration (device configurations, settings catalog, administrative
templates, compliance, endpoint security baselines), scripts (Windows, macOS
shell, proactive remediations, custom attributes), enrollment and updates
(Autopilot, enrollment configurations, feature/quality/driver updates), tenant
administration (scope tags, device categories, terms and conditions,
notification templates, assignment filters), apps (app configuration,
app protection for iOS/Android/Windows, Windows information protection, app
categories), and conditional access.

Administrative templates, security-baseline intents, and enrollment
configurations are backup-only (they require multi-part creates and are
excluded from restore for safety).

## Install

Download a release binary for your platform from the Releases page, or build
from source:

```sh
go install github.com/ugurkocde/TenuVault-TUI@latest
```

Or clone and build:

```sh
git clone https://github.com/ugurkocde/TenuVault-TUI.git
cd TenuVault-TUI
go build -o tenuvault .
```

## Usage

```sh
tenuvault
```

Flags:

| Flag            | Description                                    |
| --------------- | ---------------------------------------------- |
| `-tenant`       | Tenant id or domain (overrides config)         |
| `-backup-root`  | Directory to store backups (overrides config)  |
| `-version`      | Print version and exit                         |

On first run, choose a sign-in method: interactive (opens your browser) or
app registration (enter tenant id, client id, and a client secret or certificate
path in the form). After signing in, the dashboard shows your tenant and last
backup.

Keys: `b` back up, `l` browse, `d` compare, `r` restore, `y` sync, `t` tenants,
`s` settings, arrow keys / `j` `k` to move, `space` to toggle, `enter` to select,
`esc` to go back, `q` to quit, `?` for help. The mouse works too — click a list
row to select it and scroll with the wheel.

## Tenant sync

To copy policies between tenants, add each tenant under `Tenants` (`t`) — sign in
interactively or with app credentials. Then choose `Sync to another tenant`
(`y`), pick a source (a connected tenant or a saved backup), select whole types
or drill in to pick individual policies, choose a target tenant, pick whether to
keep the original name or add a prefix, and confirm. The target tenant's sign-in
needs write scopes (`DeviceManagement*.ReadWrite.All`). Tenant connections are
remembered across launches (metadata only — no tokens or secrets are stored).

## Permissions

Interactive sign-in requests the scopes below explicitly, so the
signed-in admin is prompted to consent to them in each tenant (the first tenant
and any tenant you add for sync). App-only (secret/certificate) sign-in uses
`.default` — consent those permissions on the app registration instead. The
relevant scopes are:

- `DeviceManagementConfiguration.ReadWrite.All`
- `DeviceManagementApps.ReadWrite.All`
- `DeviceManagementServiceConfig.ReadWrite.All`
- `DeviceManagementScripts.ReadWrite.All`
- `DeviceManagementRBAC.ReadWrite.All` (scope tags)
- `Policy.ReadWrite.ConditionalAccess`

If a scope is missing, the affected category is reported as failed on the backup
summary with the exact Graph error, so you can see what to consent.

## Configuration

Settings are stored at the OS config dir under `tenuvault/config.json`
(for example `~/Library/Application Support/tenuvault/config.json` on macOS).
Credentials are never written to disk — secrets are read from the environment:

| Variable                              | Purpose                              |
| ------------------------------------- | ------------------------------------ |
| `AZURE_TENANT_ID`                     | Target tenant                        |
| `AZURE_CLIENT_ID`                     | Custom app registration (optional)   |
| `AZURE_CLIENT_SECRET`                 | Client secret (enables secret auth)  |
| `AZURE_CLIENT_CERTIFICATE_PASSWORD`   | Certificate password (if encrypted)  |

## Backup format

```
backup-YYYY-MM-DD-HHMMSS/
  metadata.json
  DeviceConfigurations/<policy name>.json
  CompliancePolicies/<policy name>.json
  ConfigurationPolicies/<policy name>.json
  ...
```

Each policy file is the verbatim Graph JSON (with API noise removed) so it
restores cleanly and stays compatible with the TenuVault portal.

## Development

```sh
go test ./...
go build ./...
```

## License

MIT
