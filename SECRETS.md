# Release secrets

The release pipeline (`.github/workflows/release.yml` + `.goreleaser.yaml`) runs
on a tag push (`v*`), on a **macOS runner** (needed to build the DMG/PKG). It
works with **no secrets** — it just produces unsigned binaries and skips the
Homebrew tap. Add the GitHub repository secrets below
(Settings → Secrets and variables → Actions → New repository secret) to ship
signed/notarized macOS artifacts and a Homebrew tap.

Tiers, smallest to largest:
- **5 base secrets** (`MACOS_SIGN_*`, `MACOS_NOTARY_*`) → signed + notarized
  binary tarballs.
- **+ installer cert** (`MACOS_INSTALLER_*`) → also a signed + notarized `.dmg`
  and `.pkg`.
- **+ tap token** (`HOMEBREW_TAP_TOKEN`) → also `brew install`.

## macOS code signing + notarization

Requires an Apple Developer account.

| Secret | What it is | How to get it |
| --- | --- | --- |
| `MACOS_SIGN_P12` | Base64 of your **Developer ID Application** certificate exported as `.p12` | In Keychain Access, export the "Developer ID Application: …" cert (with its private key) as `cert.p12` with a password, then `base64 -i cert.p12 \| pbcopy` |
| `MACOS_SIGN_PASSWORD` | The password you set on that `.p12` export | — |
| `MACOS_NOTARY_ISSUER_ID` | App Store Connect API **Issuer ID** (a UUID) | App Store Connect → Users and Access → Integrations → App Store Connect API; shown above the keys list |
| `MACOS_NOTARY_KEY_ID` | The API **Key ID** | Same page; the key's ID (also in the `.p8` filename) |
| `MACOS_NOTARY_KEY` | Base64 of the API key `.p8` file | Generate a key with **Developer** access, download `AuthKey_XXXX.p8` (one-time), then `base64 -i AuthKey_XXXX.p8 \| pbcopy` |

Generating the Developer ID Application certificate (if you don't have one):
Xcode → Settings → Accounts → Manage Certificates → "+" → Developer ID
Application. Then export it from Keychain Access as above.

When these are set, macOS binaries are signed with your Developer ID and
notarized by Apple; users can run them without Gatekeeper warnings.

## macOS DMG + PKG installers (optional)

To also publish a signed/notarized `.dmg` and `.pkg` for each macOS arch, add
your **Developer ID Installer** certificate (distinct from the Application cert
above — it signs the `.pkg`):

| Secret | What it is | How to get it |
| --- | --- | --- |
| `MACOS_INSTALLER_P12` | Base64 of your **Developer ID Installer** certificate exported as `.p12` | In Keychain Access, export the "Developer ID Installer: …" cert (with its private key) as `installer.p12` with a password, then `base64 -i installer.p12 \| pbcopy` |
| `MACOS_INSTALLER_PASSWORD` | The password you set on that `.p12` export | — |

When set, the release job imports both certs into a temporary keychain, builds
the `.dmg` (signed with Developer ID Application) and `.pkg` (signed with
Developer ID Installer) via `scripts/macos-package.sh`, notarizes + staples
them, and attaches them to the release. If unset, the DMG/PKG step is skipped.

## Homebrew tap (optional)

To publish a `brew install` formula:

1. Create a public repo named `homebrew-tap` under your account
   (`github.com/ugurkocde/homebrew-tap`).
2. Add a secret with a token that can push to it:

| Secret | What it is |
| --- | --- |
| `HOMEBREW_TAP_TOKEN` | A fine-grained PAT (or classic PAT with `repo` scope) that has **write** access to `ugurkocde/homebrew-tap` |

Then users install with:

```sh
brew install ugurkocde/tap/tenuvault
```

If `HOMEBREW_TAP_TOKEN` is unset, the formula step is skipped automatically.

## Windows signing (later)

Windows binaries are currently unsigned. When you have an Authenticode code
signing certificate, we'll add a `signs` step for the Windows artifacts.

## Cutting a release

```sh
git tag v1.0.0
git push origin v1.0.0
```

The `release` workflow builds, (optionally) signs/notarizes, publishes the
GitHub Release with binaries + checksums, and (optionally) updates the tap.
Validate the config locally first with `goreleaser check`.
