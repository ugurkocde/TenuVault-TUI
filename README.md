# TenuVault TUI

A fast, self-contained terminal UI for backing up and restoring Microsoft Intune
configuration via Microsoft Graph. Sign in, back up your policies to local
TenuVault-format folders, compare backups for drift, and restore policies back to
your tenant — all from a single binary, no PowerShell or cloud setup required.

Built with Go and the [Charm](https://charm.land) stack (Bubble Tea, Lip Gloss).

## Features

- Interactive browser or device-code sign-in (delegated permissions; no app
  registration needed). Client secret and certificate flows are supported for
  unattended use.
- Backs up Intune policy types to per-policy JSON files plus a `metadata.json`
  manifest, in a layout cross-compatible with the TenuVault portal.
- Browse local backups, drill into categories, and compare any two backups for
  configuration drift with severity highlighting.
- Restore selected policies back to Graph with a dry-run preview, an explicit
  confirmation, and a `[Restored]` name prefix so nothing is overwritten.
- Conditional Access policies are restored disabled by default.

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

On first run, choose a sign-in method. Interactive sign-in opens your browser;
device code prints a code to enter at `microsoft.com/devicelogin`. After signing
in, the dashboard shows your tenant and last backup.

Keys: `b` back up, `l` browse, `d` compare, `r` restore, arrow keys / `j` `k`
to move, `space` to toggle, `enter` to select, `esc` to go back, `q` to quit,
`?` for help.

## Permissions

Backups need read access; restores need write access. With the default
well-known Microsoft Graph PowerShell client, an Intune administrator's
delegated permissions cover both. The relevant scopes are:

- `DeviceManagementConfiguration.ReadWrite.All`
- `DeviceManagementApps.ReadWrite.All`
- `DeviceManagementServiceConfig.ReadWrite.All`
- `Policy.ReadWrite.ConditionalAccess`

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
