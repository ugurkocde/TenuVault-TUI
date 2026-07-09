# TenuVault-TUI for PowerShell

This PowerShell Gallery module bundles the 64-bit Windows `tenuvault.exe` and
exports `Invoke-TenuVault` plus its `tenuvault` alias. It is a thin launcher:
every argument and exit code comes from the same binary published in the GitHub
release.

## Install or update

```powershell
Install-Module TenuVault-TUI -Scope CurrentUser

# For an existing Install-Module installation:
Update-Module TenuVault-TUI
```

If you used `Install-PSResource`, update with
`Update-PSResource TenuVault-TUI` instead. Open a new PowerShell session after
updating so the new module path and bundled executable are loaded, then verify:

```powershell
Get-InstalledModule TenuVault-TUI       # PowerShellGet
# Get-InstalledPSResource TenuVault-TUI # PSResourceGet
tenuvault --version
```

If the executable is missing or the installation is damaged:

```powershell
Install-Module TenuVault-TUI -Scope CurrentUser -Force
```

`Update-Module` does not update a manually downloaded ZIP.

## Run

```powershell
tenuvault                                      # interactive terminal UI
tenuvault --version                            # bundled binary version
tenuvault backup --out C:\Backups --assignments
tenuvault compare C:\Backups\backup-old C:\Backups\backup-new
tenuvault verify C:\Backups\backup-new
```

All binary commands and flags (`backup`, `restore`, `compare`, `verify`, and
`sync`) are forwarded unchanged. The binary's exit status remains available in
`$LASTEXITCODE`.

## Requirements and behavior

- 64-bit Windows; Windows ARM64 uses x64 emulation.
- Windows PowerShell 5.1 or PowerShell 7+.
- Run the interactive UI in Windows Terminal, `pwsh`, or a console window. The
  PowerShell ISE can run headless subcommands but cannot render the TUI.
- Authentication, permissions, command examples, safety guarantees, and backup
  format are documented in the
  [project README](https://github.com/ugurkocde/TenuVault-TUI#readme).
- The module stores no credentials and adds no PowerShell implementation of the
  backup engine; it only launches the bundled executable.
