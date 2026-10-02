[CmdletBinding()]
param(
    [string]$Version = '0.0.0-dev',
    [string]$Compiler = ''
)
$ErrorActionPreference = 'Stop'
$root = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$Version = $Version -replace '^v', ''
if ($Version -notmatch '^(\d+)\.(\d+)\.(\d+)(?:-[0-9A-Za-z.-]+)?$') {
    throw 'Version must be major.minor.patch, optionally followed by a prerelease suffix (for example v1.2.3-beta.1)'
}
$parts = @($Matches[1], $Matches[2], $Matches[3])
if (@($parts | Where-Object { [long]$_ -gt 65535 }).Count) { throw 'Version components must be at most 65535' }
$fileVersion = "$($parts -join '.').0"
$dist = Join-Path $root 'dist/share-app'
$output = Join-Path $root 'dist/releases'
$ffmpeg = Join-Path $root 'dist/ffmpeg'
$ffmpegVersion = (Get-Content (Join-Path $root 'scripts/ffmpeg/versions.env') | Where-Object { $_ -match '^FFMPEG_VERSION=' }) -replace '^FFMPEG_VERSION=', ''
$sourceName = "ffmpeg-$ffmpegVersion-share-app-sources.tar.gz"
$sourceHash = (Get-Content (Join-Path $ffmpeg 'sources.sha256') -Raw).Trim() -split '\s+'
if ($sourceHash.Count -ne 2 -or $sourceHash[1] -ne $sourceName -or
    (Get-FileHash (Join-Path $ffmpeg $sourceName) -Algorithm SHA256).Hash.ToLowerInvariant() -ne $sourceHash[0]) {
    throw 'Corresponding FFmpeg source checksum mismatch'
}
$binaryHash = (Get-Content (Join-Path $dist 'ffmpeg.sha256') -Raw).Trim() -split '\s+'
if ($binaryHash.Count -ne 2 -or $binaryHash[1] -ne 'ffmpeg.exe' -or
    (Get-FileHash (Join-Path $dist 'ffmpeg.exe') -Algorithm SHA256).Hash.ToLowerInvariant() -ne $binaryHash[0]) {
    throw 'Bundled FFmpeg checksum mismatch'
}
if (-not $Compiler) {
    $destination = Join-Path $root 'dist/tools/inno-setup'
    $Compiler = Join-Path $destination 'ISCC.exe'
    if (-not (Test-Path $Compiler)) {
        # Bootstrap the pinned compiler once; local builds and Actions share it.
        $download = Join-Path ([IO.Path]::GetTempPath()) "share-app-inno-$([Guid]::NewGuid()).exe"
        try {
            Invoke-WebRequest 'https://github.com/jrsoftware/issrc/releases/download/is-6_7_3/innosetup-6.7.3.exe' -OutFile $download
            if ((Get-FileHash $download -Algorithm SHA256).Hash.ToLowerInvariant() -ne '9c73c3bae7ed48d44112a0f48e66742c00090bdb5bef71d9d3c056c66e97b732') {
                throw 'Inno Setup download checksum mismatch'
            }
            $process = Start-Process $download -ArgumentList @('/VERYSILENT', '/SUPPRESSMSGBOXES', '/NORESTART', '/CURRENTUSER', "/DIR=`"$destination`"") -Wait -PassThru
            if ($process.ExitCode -ne 0) { throw "Inno Setup installation failed: $($process.ExitCode)" }
        } finally {
            Remove-Item $download -Force -ErrorAction SilentlyContinue
        }
    }
}
foreach ($inputFile in @((Join-Path $ffmpeg $sourceName), $Compiler, (Join-Path $dist 'share-host.exe'),
    (Join-Path $dist 'CaptureProbe/CaptureProbe.exe'), (Join-Path $dist 'web/index.html'),
    (Join-Path $dist 'LICENSE'), (Join-Path $dist 'scripts/create-rdp-credentials.ps1'))) {
    if (-not (Test-Path $inputFile -PathType Leaf)) { throw "Missing packaging input: $inputFile" }
}
if (Test-Path $output) { Remove-Item $output -Recurse -Force }
New-Item -ItemType Directory -Path $output -Force | Out-Null
& $Compiler "/DAppVersion=$Version" "/DAppFileVersion=$fileVersion" "/DDistDir=$dist" "/DOutputDir=$output" (Join-Path $root 'installer/share-app.iss')
if ($LASTEXITCODE -ne 0) { throw "Inno Setup compilation failed: $LASTEXITCODE" }
$installerName = "ShareApp-$Version-Setup.exe"
if (-not (Test-Path (Join-Path $output $installerName))) { throw 'Installer was not created' }
$zipName = "ShareApp-$Version-windows-x64.zip"
[IO.Compression.ZipFile]::CreateFromDirectory($dist, (Join-Path $output $zipName))
Copy-Item (Join-Path $ffmpeg $sourceName) $output
$checksums = foreach ($name in @($installerName, $zipName, $sourceName)) {
    "$((Get-FileHash (Join-Path $output $name) -Algorithm SHA256).Hash.ToLowerInvariant())  $name"
}
[IO.File]::WriteAllLines((Join-Path $output 'SHA256SUMS.txt'), $checksums, [Text.UTF8Encoding]::new($false))
$notes = @"
Share App $Version for Windows 11 x64.

- $installerName installs for the current user into %LOCALAPPDATA%\Programs\ShareApp without requesting administrator privileges.
- $zipName can be extracted and run without installation.
- Both include a standalone FFmpeg (LGPL-2.1-or-later) and libvpx (BSD). No separate FFmpeg installation is needed.
- $sourceName contains the corresponding FFmpeg/libvpx sources and build instructions; keep it available alongside the binaries.
- SHA256SUMS.txt lists SHA-256 checksums for all three downloads.

Settings, logs, and RDP credentials remain in %LOCALAPPDATA%\ShareApp and are preserved on upgrade and uninstall. Exit the app from the notification area before upgrading or uninstalling.

See the README for setup and configuration. Before publishing this draft, confirm launch and VP9 streaming on Windows 11.
"@
[IO.File]::WriteAllText((Join-Path $output 'RELEASE-NOTES.md'), $notes, [Text.UTF8Encoding]::new($false))
Write-Host "Installer, ZIP, sources and checksums ready: $output"
