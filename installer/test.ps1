# Run on a disposable Windows runner; deliberately refuse an existing install.
[CmdletBinding()]
param([Parameter(Mandatory)][string]$InstallerPath)
$ErrorActionPreference = 'Stop'
$InstallerPath = (Resolve-Path $InstallerPath).Path
$app = Join-Path $env:LOCALAPPDATA 'Programs/ShareApp'
$data = Join-Path $env:LOCALAPPDATA 'ShareApp'
$registry = 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Uninstall\{9E7D147B-C72B-4B55-86C9-1D9A64379EE5}_is1'
if ((Test-Path $app) -or (Test-Path $registry) -or (Test-Path $data)) {
    throw 'Installer smoke test requires a disposable user profile without Share App or its data'
}
$logRoot = Join-Path ([IO.Path]::GetTempPath()) "share-app-installer-test-$([Guid]::NewGuid())"
New-Item -ItemType Directory -Path $logRoot, $data | Out-Null
$settings = Join-Path $data 'config.json'
$credentials = Join-Path $data 'rdp-credentials.bin'
[IO.File]::WriteAllText($settings, '{"listenAddr":"127.0.0.1:8443","captureStats":"off","fps":8,"crf":31,"maxScale":2,"scrollSensitivity":1}')
# A sentinel, never a real credential; the app is not launched in this test.
[IO.File]::WriteAllBytes($credentials, [byte[]](1, 2, 3, 4))
$settingsHash = (Get-FileHash $settings).Hash
$credentialsHash = (Get-FileHash $credentials).Hash
function Run-Installer([string]$File, [string]$LogName) {
    $log = Join-Path $logRoot $LogName
    $process = Start-Process $File -ArgumentList @('/VERYSILENT', '/SUPPRESSMSGBOXES', '/NORESTART', "/LOG=`"$log`"") -Wait -PassThru
    if ($process.ExitCode -ne 0) { throw "Installer returned $($process.ExitCode). See $log" }
}
function Assert-DataPreserved {
    if ((Get-FileHash $settings).Hash -ne $settingsHash -or (Get-FileHash $credentials).Hash -ne $credentialsHash) {
        throw 'Installer changed user settings or credentials'
    }
}
try {
    Run-Installer $InstallerPath 'install.log'
    foreach ($file in @('share-host.exe', 'ffmpeg.exe', 'CaptureProbe/CaptureProbe.exe', 'web/index.html', 'licenses/ffmpeg/COPYING.LGPLv2.1', 'unins000.exe')) {
        if (-not (Test-Path (Join-Path $app $file))) { throw "Missing installed file: $file" }
    }
    $encoderList = (& (Join-Path $app 'ffmpeg.exe') -hide_banner -encoders | Out-String)
    if ($LASTEXITCODE -ne 0 -or $encoderList -notmatch '(?m)^\s*V\S*\s+libvpx-vp9\s') { throw 'Installed FFmpeg is not usable' }
    if (-not (Test-Path $registry)) { throw 'Missing per-user uninstall registration' }
    $shortcut = Join-Path ([Environment]::GetFolderPath('Programs')) 'Share App.lnk'
    if (-not (Test-Path $shortcut)) { throw 'Missing Start Menu shortcut' }
    Assert-DataPreserved

    # Setup / Uninstall must refuse to replace running host files in silent mode.
    $mutex = [Threading.Mutex]::new($false, 'Local\ShareApp.Running.9E7D147B-C72B-4B55-86C9-1D9A64379EE5')
    try {
        $blocked = Start-Process $InstallerPath -ArgumentList @('/VERYSILENT', '/SUPPRESSMSGBOXES', '/NORESTART') -Wait -PassThru
        if ($blocked.ExitCode -eq 0) { throw 'Setup ignored the running app mutex' }
        $blockedUninstall = Start-Process (Join-Path $app 'unins000.exe') -ArgumentList @('/VERYSILENT', '/SUPPRESSMSGBOXES', '/NORESTART') -Wait -PassThru
        if ($blockedUninstall.ExitCode -eq 0) { throw 'Uninstall ignored the running app mutex' }
    } finally { $mutex.Dispose() }

    $stale = Join-Path $app 'web/stale-installer-test.js'
    [IO.File]::WriteAllText($stale, 'old generated asset')
    Run-Installer $InstallerPath 'upgrade.log'
    if (Test-Path $stale) { throw 'Upgrade left a stale generated web asset' }
    Assert-DataPreserved
    Run-Installer (Join-Path $app 'unins000.exe') 'uninstall.log'
    foreach ($file in @('share-host.exe', 'ffmpeg.exe', 'CaptureProbe/CaptureProbe.exe', 'web/index.html')) {
        if (Test-Path (Join-Path $app $file)) { throw "Uninstall left an installed file: $file" }
    }
    if ((Test-Path $registry) -or (Test-Path $shortcut)) { throw 'Uninstall left registration or shortcut' }
    Assert-DataPreserved
    Write-Host "Installer smoke test passed: per-user install, running-app guard, upgrade, uninstall and data preservation. Logs: $logRoot"
} finally {
    # Only remove test-owned sentinel data; logs remain available for failures.
    Remove-Item $data -Recurse -Force
}
