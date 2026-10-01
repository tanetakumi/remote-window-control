$ErrorActionPreference = "Stop"
$repo = Split-Path $PSScriptRoot -Parent
Push-Location $repo
try {
  if (-not (Test-Path control-server/bin/share-host.exe)) { throw "Build first with scripts/build.ps1" }
  & ./control-server/bin/share-host.exe
  if ($LASTEXITCODE -ne 0) { throw "Host exited with code $LASTEXITCODE" }
} finally { Pop-Location }
