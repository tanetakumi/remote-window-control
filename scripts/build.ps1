param([string]$GoExe = "go", [switch]$Check)
$ErrorActionPreference = "Stop"
$repo = Split-Path $PSScriptRoot -Parent
$requiredGo = (Select-String -Path "$repo/control-server/go.mod" -Pattern '^go (.+)$').Matches.Groups[1].Value
$goPath = (Get-Command $GoExe -ErrorAction Stop).Source
$goVersion = & $goPath version
if ($LASTEXITCODE -ne 0 -or $goVersion -notmatch "go$([regex]::Escape($requiredGo)) ") { throw "Expected Go $requiredGo; resolved $goPath ($goVersion)" }
Write-Host "Using $goPath ($goVersion)"
Push-Location $repo
try {
  $requiredNode = (Get-Content .node-version).Trim()
  if ((node --version) -ne "v$requiredNode") { throw "Expected Node $requiredNode" }
  Push-Location web-ui
  try {
    npm install
    if ($LASTEXITCODE -ne 0) { throw "Client dependency installation failed" }
    npm run build
    if ($LASTEXITCODE -ne 0) { throw "Client build failed" }
    if ($Check) { npm test; if ($LASTEXITCODE -ne 0) { throw "Client tests failed" } }
  } finally { Pop-Location }
  dotnet build window-capture/apps/CaptureProbe/CaptureProbe.csproj -p:EnableWindowsTargeting=true
  if ($LASTEXITCODE -ne 0) { throw "CaptureProbe build failed" }
  Push-Location control-server
  try {
    & $goPath build -o share-host.exe ./cmd/share-host
    if ($LASTEXITCODE -ne 0) { throw "Host build failed" }
    if ($Check) {
      & $goPath vet ./...
      if ($LASTEXITCODE -ne 0) { throw "Go vet failed" }
      & $goPath test ./...
      if ($LASTEXITCODE -ne 0) { throw "Go tests failed" }
    }
  } finally { Pop-Location }
} finally { Pop-Location }
