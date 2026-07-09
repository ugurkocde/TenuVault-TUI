#
# Module manifest for TenuVault-TUI.
#
# ModuleVersion is a placeholder; the release pipeline (scripts/build-psmodule.ps1)
# stamps it from the git tag before publishing to the PowerShell Gallery.
#
@{
    RootModule           = 'TenuVault-TUI.psm1'
    ModuleVersion        = '0.0.0'
    GUID                 = '5b3a2f9c-8d41-4e7a-bf62-1c9d0e4a7f38'
    Author               = 'Ugur Koc'
    CompanyName          = 'UgurLabs'
    Copyright            = '(c) 2026 Ugur Koc. MIT License.'
    Description          = 'Terminal UI for backing up, restoring, comparing, and syncing Microsoft Intune configuration via Microsoft Graph. Bundles the tenuvault.exe binary (64-bit Windows) and launches it; the same terminal UI and headless subcommands as the standalone release.'

    PowerShellVersion    = '5.1'
    CompatiblePSEditions = @('Desktop', 'Core')

    # Note: ProcessorArchitecture is intentionally left unset. Setting it to
    # Amd64 makes Import-Module hard-refuse on ARM64 Windows, but the amd64
    # binary runs there fine under x64 emulation. The launcher enforces 64-bit
    # (rejecting only genuine 32-bit Windows) at runtime instead.

    FunctionsToExport    = @('Invoke-TenuVault')
    CmdletsToExport      = @()
    VariablesToExport    = @()
    AliasesToExport      = @('tenuvault')

    FileList             = @(
        'TenuVault-TUI.psm1',
        'TenuVault-TUI.psd1',
        'README.md',
        'bin/tenuvault.exe'
    )

    PrivateData = @{
        PSData = @{
            Tags         = @(
                'Intune', 'MicrosoftGraph', 'Microsoft365', 'Entra', 'MEM',
                'Backup', 'Restore', 'TUI', 'Windows',
                'PSEdition_Desktop', 'PSEdition_Core'
            )
            LicenseUri   = 'https://github.com/ugurkocde/TenuVault-TUI/blob/main/LICENSE'
            ProjectUri   = 'https://github.com/ugurkocde/TenuVault-TUI'
            ReleaseNotes = 'Release notes: https://github.com/ugurkocde/TenuVault-TUI/releases'
            RequireLicenseAcceptance = $false
        }
    }
}
