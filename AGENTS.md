# AGENTS.md

## Cursor Cloud specific instructions

TenuVault TUI is a single Go binary (module `github.com/ugurkocde/TenuVault-TUI`)
that backs up/restores/compares/syncs Microsoft Intune config via Microsoft
Graph, built on the Charm stack (Bubble Tea, Lip Gloss). There is one product
and no auxiliary services (no database, queue, or web server) — everything runs
in-process.

### Go toolchain (important, non-obvious)
- The system `go` on `PATH` is an older release (1.22.x). `go.mod` pins
  `go 1.26.1`, so any `go` command run **inside the repo** auto-downloads and
  uses toolchain 1.26.1 transparently. Don't fight this; just run `go ...` from
  the repo root.
- Building tools with `go install ... @version` happens **outside** the module,
  so it uses the system Go unless you force the toolchain. To build a tool with
  1.26.1, prefix with `GOTOOLCHAIN=go1.26.1` (this is required for
  `golangci-lint`, which refuses to load this repo's `.golangci.yml` unless it
  was itself built with Go >= 1.26 — see the CI workflow comment).

### Lint / vet / format / test / build (standard commands)
Standard dev commands live in `README.md` (Development) and `.github/workflows/ci.yml`:
- Format check: `test -z "$(gofmt -l .)"`
- Vet: `go vet ./...`
- Lint: `golangci-lint run ./...` (config `.golangci.yml`, v2)
- Tests: `go test ./...` / `go test -race ./...`
- Build all release targets: loop over `GOOS/GOARCH` with `CGO_ENABLED=0 go build ./...`

`golangci-lint` is installed to `$(go env GOPATH)/bin`, which is **not** on
`PATH` by default — invoke it as `$(go env GOPATH)/bin/golangci-lint` or add
that dir to `PATH` for the session.

### Running the app
- `go run .` (or build with `go build -o tenuvault .`) launches the TUI. It
  needs a real terminal (alt-screen + mouse); it won't run attached to a
  non-TTY pipe.
- First screen is sign-in method selection. Reaching the dashboard, backup,
  restore, compare, and tenant-sync features requires a **real Microsoft Entra
  tenant**: either interactive browser sign-in or app-registration credentials
  (`AZURE_TENANT_ID`, `AZURE_CLIENT_ID`, `AZURE_CLIENT_SECRET` or a cert). There
  is no offline/mock mode in the binary — the Graph base URL is hard-coded.
- Headless automation: `go run . backup --out ./backups` and
  `go run . restore --backup <dir>` also require those `AZURE_*` env vars.
- The core backup/restore/sync engines are exercised offline (no tenant) by the
  integration tests using the in-memory fake in `internal/graphtest`; run
  `go test ./internal/...` to validate engine behavior without credentials.

### PowerShell (Pester / PSScriptAnalyzer)
- PowerShell 7 (`pwsh`, currently 7.6.3) is installed from Microsoft's apt
  repository (PMC) and lives at `/usr/bin/pwsh`. Verify with:
  `pwsh -NoLogo -NoProfile -Command '$PSVersionTable.PSVersion.ToString()'`.
- The `Pester` and `PSScriptAnalyzer` modules are installed for the current user
  (`~/.local/share/powershell/Modules`). Run tests with
  `pwsh -NoLogo -NoProfile -Command "Invoke-Pester -Path <dir>"` and lint scripts
  with `Invoke-ScriptAnalyzer -Path <file-or-dir>`.
- These are installed in the VM snapshot. The startup update script also
  re-installs them (guarded/idempotent/fail-safe) only if `pwsh` or a module is
  missing, so it's a no-op on a normal start. If `Install-Module` ever prompts
  about the untrusted PSGallery, run
  `Set-PSRepository -Name PSGallery -InstallationPolicy Trusted` first.
