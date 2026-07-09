# TenuVault-TUI PowerShell module.
#
# Thin launcher for the bundled tenuvault.exe (64-bit Windows). The terminal UI
# and every headless subcommand live in the Go binary; this module only locates
# and executes it, passing all arguments straight through so behaviour matches
# the standalone release exactly.

Set-StrictMode -Version Latest

$script:BinaryPath = Join-Path $PSScriptRoot 'bin/tenuvault.exe'

function Invoke-TenuVault {
    <#
    .SYNOPSIS
        Launch TenuVault, the terminal UI for backing up and restoring Microsoft
        Intune via Microsoft Graph.

    .DESCRIPTION
        Runs the bundled tenuvault.exe. With no arguments it opens the interactive
        terminal UI. Any arguments are forwarded verbatim to the binary, so the
        headless subcommands (backup, restore, compare, verify, sync) and flags
        (--tenant, --backup-root, --version) work exactly as documented for the
        standalone release. The binary's exit code is left in $LASTEXITCODE.

    .EXAMPLE
        Invoke-TenuVault
        Opens the interactive terminal UI.

    .EXAMPLE
        tenuvault --version
        Prints the bundled binary version.

    .EXAMPLE
        tenuvault backup --tenant contoso.onmicrosoft.com
        Runs a headless backup.

    .LINK
        https://github.com/ugurkocde/TenuVault-TUI
    #>
    [CmdletBinding()]
    param(
        [Parameter(ValueFromRemainingArguments = $true)]
        [string[]] $Arguments
    )

    # Windows only. Windows PowerShell 5.1 (major < 6) always implies Windows;
    # PowerShell 7+ exposes $IsWindows. The -or short-circuits, so $IsWindows is
    # never referenced under StrictMode on 5.1 where it does not exist.
    $onWindows = ($PSVersionTable.PSVersion.Major -lt 6) -or $IsWindows
    if (-not $onWindows) {
        throw "TenuVault-TUI ships a Windows binary. On macOS run 'brew install ugurkocde/UgurLabs/tenuvault'; on Linux download the .deb/.rpm from https://github.com/ugurkocde/TenuVault-TUI/releases/latest"
    }

    # 64-bit only. Windows on ARM64 runs the amd64 binary via x64 emulation, so
    # this check only rejects genuine 32-bit Windows.
    if (-not [Environment]::Is64BitOperatingSystem) {
        throw "TenuVault requires 64-bit Windows."
    }

    if (-not (Test-Path -LiteralPath $script:BinaryPath)) {
        throw "Bundled binary not found at '$script:BinaryPath'. Reinstall the module: Install-Module TenuVault-TUI -Force"
    }

    # The interactive TUI needs a real console. Guard the no-argument (TUI) case
    # against hosts that can't render a full-screen app or that redirect the
    # console. Headless subcommands run fine anywhere, so only guard TUI mode.
    $isTuiMode = ($null -eq $Arguments) -or ($Arguments.Count -eq 0)
    if ($isTuiMode) {
        if ($Host.Name -eq 'Windows PowerShell ISE Host') {
            throw "The TenuVault terminal UI cannot run inside the PowerShell ISE. Use Windows Terminal, pwsh, or a console window, or run a headless subcommand (e.g. 'tenuvault backup')."
        }
        if ([Console]::IsInputRedirected -or [Console]::IsOutputRedirected) {
            Write-Warning "Standard input/output is redirected; the interactive UI may not render correctly. Run in an interactive terminal, or use a headless subcommand."
        }
    }

    # Call operator inherits the real console (required by the TUI) and sets
    # $LASTEXITCODE from the binary. Do not call 'exit' here: this function runs
    # in the caller's session and 'exit' would close their shell.
    if ($isTuiMode) {
        & $script:BinaryPath
    }
    else {
        & $script:BinaryPath @Arguments
    }
}

Set-Alias -Name tenuvault -Value Invoke-TenuVault

Export-ModuleMember -Function 'Invoke-TenuVault' -Alias 'tenuvault'
