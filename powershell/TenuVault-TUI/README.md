# TenuVault-TUI (PowerShell Gallery)

Install and run [TenuVault](https://github.com/ugurkocde/TenuVault-TUI) from
PowerShell on 64-bit Windows. The module bundles the `tenuvault.exe` binary and
launches it - the same terminal UI and headless subcommands as the standalone
release, nothing reimplemented in PowerShell.

## Install

```powershell
Install-Module TenuVault-TUI -Scope CurrentUser
```

## Use

```powershell
tenuvault                                   # open the terminal UI
tenuvault --version                         # print version
tenuvault backup --tenant contoso.onmicrosoft.com   # headless backup
```

`tenuvault` is an alias for `Invoke-TenuVault`. Every argument is forwarded
verbatim to the binary, so all documented subcommands (`backup`, `restore`,
`compare`, `verify`, `sync`) and flags work unchanged.

## Notes

- 64-bit Windows only. On macOS use Homebrew; on Linux use the `.deb`/`.rpm`.
  See the [project README](https://github.com/ugurkocde/TenuVault-TUI#install).
- The terminal UI needs an interactive console (Windows Terminal, `pwsh`, or a
  console window). It does not run inside the PowerShell ISE; use a headless
  subcommand there instead.
