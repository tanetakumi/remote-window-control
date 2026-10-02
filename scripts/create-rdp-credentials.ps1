# Run on the Windows PC, as the user who runs Share App. Only encrypted data
# is written, directly to the app's user data directory by default.
[CmdletBinding()]
param(
    [string]$OutputPath = (Join-Path $env:LOCALAPPDATA 'ShareApp\rdp-credentials.bin'),
    [System.Management.Automation.PSCredential]$Credential,
    [switch]$Force
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

if ($env:OS -ne 'Windows_NT') {
    throw 'This script requires Windows DPAPI.'
}
Add-Type -AssemblyName System.Security

$outputFile = [System.IO.Path]::GetFullPath($OutputPath)
if ([System.IO.File]::Exists($outputFile) -and -not $Force) {
    throw 'The output file already exists. Use -Force to replace it.'
}

$ownedPassword = $null
$secretPointer = [IntPtr]::Zero
$payloadBytes = $null
$plainPassword = $null
$payload = $null
$stream = $null

try {
    if ($null -eq $Credential) {
        $username = (Read-Host 'RDP username (the Windows account running Share App)').Trim()
        $ownedPassword = Read-Host 'RDP password (not the Windows Hello PIN)' -AsSecureString
        $Credential = [System.Management.Automation.PSCredential]::new($username, $ownedPassword)
    }
    if ([string]::IsNullOrWhiteSpace($Credential.UserName) -or $Credential.Password.Length -eq 0) {
        throw 'The username and password must both be non-empty.'
    }

    $secretPointer = [System.Runtime.InteropServices.Marshal]::SecureStringToBSTR($Credential.Password)
    $plainPassword = [System.Runtime.InteropServices.Marshal]::PtrToStringBSTR($secretPointer)
    $payload = [ordered]@{
        username = $Credential.UserName
        password = $plainPassword
    } | ConvertTo-Json -Compress
    $payloadBytes = [System.Text.Encoding]::UTF8.GetBytes($payload)
    if ($payloadBytes.Length -gt 32768) {
        throw 'The credential payload is too large.'
    }
    $encrypted = [System.Security.Cryptography.ProtectedData]::Protect(
        $payloadBytes,
        $null,
        [System.Security.Cryptography.DataProtectionScope]::CurrentUser
    )
    $fileMode = [System.IO.FileMode]::CreateNew
    if ($Force) { $fileMode = [System.IO.FileMode]::Create }
    [System.IO.Directory]::CreateDirectory([System.IO.Path]::GetDirectoryName($outputFile)) | Out-Null
    $stream = [System.IO.File]::Open($outputFile, $fileMode, [System.IO.FileAccess]::Write, [System.IO.FileShare]::None)
    $stream.Write($encrypted, 0, $encrypted.Length)
    $stream.Dispose()
    $stream = $null

    Write-Host "Created: $outputFile"
    Write-Host 'Restart Share App to load credentials from its user data directory.'
    Write-Host 'The file is protected for the current Windows user on this PC.'
} finally {
    if ($null -ne $stream) { $stream.Dispose() }
    if ($null -ne $payloadBytes) { [System.Array]::Clear($payloadBytes, 0, $payloadBytes.Length) }
    if ($secretPointer -ne [IntPtr]::Zero) {
        [System.Runtime.InteropServices.Marshal]::ZeroFreeBSTR($secretPointer)
    }
    if ($null -ne $ownedPassword) { $ownedPassword.Dispose() }
    $plainPassword = $null
    $payload = $null
}
