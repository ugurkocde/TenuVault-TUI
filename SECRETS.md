# Release secrets

The release pipeline (`.github/workflows/release.yml` + `.goreleaser.yaml`) runs
on a tag push (`v*`). It works with **no secrets** — it just produces unsigned
binaries and skips the Homebrew tap. To ship a signed macOS build and a Homebrew
formula, add the GitHub repository secrets below
(Settings → Secrets and variables → Actions → New repository secret).

The macOS signing/notarization runs on the Linux runner — no Mac needed.

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
