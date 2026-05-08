<#
.SYNOPSIS
  Build the gateway with the SPA bundled in.

.DESCRIPTION
  1. Build the SPA in `web/` (npm ci + npm run build).
  2. Copy `web/dist/*` into `gateway/internal/webassets/dist/` so go:embed
     picks it up.
  3. Build `gateway.exe` with -ldflags="-X main.version=<sha-or-tag>".

  Skip steps with -SkipWeb / -SkipCopy if the inputs are already in place.

.PARAMETER Version
  Override the embedded version string. Defaults to "dev-<short sha>".

.PARAMETER OutDir
  Output directory for gateway.exe. Defaults to "<repo>/dist/AMB_RC_Lap_Timer/".

.NOTES
  docs/architecture.md §4.1 — this script is the canonical local build path.
  CI replicates the same steps in `.github/workflows/ci.yml`.
#>
[CmdletBinding()]
param(
    [string]$Version,
    [string]$OutDir,
    [switch]$SkipWeb,
    [switch]$SkipCopy
)

$ErrorActionPreference = 'Stop'

# Repo root = parent dir of this script.
$repoRoot = Resolve-Path (Join-Path $PSScriptRoot '..')
$webDir = Join-Path $repoRoot 'web'
$gatewayDir = Join-Path $repoRoot 'gateway'
$embedDir = Join-Path $gatewayDir 'internal/webassets/dist'
$webDistDir = Join-Path $webDir 'dist'

if (-not $OutDir) {
    $OutDir = Join-Path $repoRoot 'dist/AMB_RC_Lap_Timer'
}

if (-not $Version) {
    try {
        $sha = (& git -C $repoRoot rev-parse --short HEAD).Trim()
        $Version = "dev-$sha"
    } catch {
        $Version = 'dev'
    }
}

Write-Host "==> Building AMB RC Lap Timer (version: $Version)" -ForegroundColor Cyan

# ── 1. SPA build ───────────────────────────────────────────────────────────
if (-not $SkipWeb) {
    Write-Host "==> [web] npm ci"   -ForegroundColor Cyan
    Push-Location $webDir
    try {
        npm ci
        if ($LASTEXITCODE -ne 0) { throw "npm ci failed (exit $LASTEXITCODE)" }
        Write-Host "==> [web] npm run build" -ForegroundColor Cyan
        npm run build
        if ($LASTEXITCODE -ne 0) { throw "npm run build failed (exit $LASTEXITCODE)" }
    } finally {
        Pop-Location
    }
} else {
    Write-Host "==> [web] skipped" -ForegroundColor Yellow
}

# ── 2. Copy web/dist/* → gateway/internal/webassets/dist/ ─────────────────
if (-not $SkipCopy) {
    if (-not (Test-Path $webDistDir)) {
        throw "web/dist does not exist; run without -SkipWeb first"
    }
    Write-Host "==> [embed] copy web/dist/* → gateway/internal/webassets/dist/" -ForegroundColor Cyan
    # Wipe the embed dir except .gitkeep (which `go:embed all:dist` needs to keep
    # the directory non-empty across clean checkouts).
    Get-ChildItem -Path $embedDir -Force -Exclude '.gitkeep' | Remove-Item -Recurse -Force
    Copy-Item -Path (Join-Path $webDistDir '*') -Destination $embedDir -Recurse -Force
} else {
    Write-Host "==> [embed] skipped" -ForegroundColor Yellow
}

# ── 3. Gateway build ──────────────────────────────────────────────────────
Write-Host "==> [gateway] go build" -ForegroundColor Cyan
if (-not (Test-Path $OutDir)) {
    New-Item -Path $OutDir -ItemType Directory -Force | Out-Null
}
$exePath = Join-Path $OutDir 'gateway.exe'

Push-Location $gatewayDir
try {
    $ldflags = "-s -w -X main.version=$Version"
    & go build -trimpath -ldflags $ldflags -o $exePath ./cmd/gateway
    if ($LASTEXITCODE -ne 0) { throw "go build failed (exit $LASTEXITCODE)" }
} finally {
    Pop-Location
}

# ── 4. Bundle operator-facing files next to the EXE ──────────────────────
# Operators run gateway.exe directly from this folder, so the gateway needs
# config.example.json next to it for first-launch bootstrap (main.go
# ensureConfigFile), and the README is the printed user manual.
$bundleSources = @(
    @{ Src = (Join-Path $gatewayDir 'config.example.json');     Dst = (Join-Path $OutDir 'config.example.json') }
    @{ Src = (Join-Path $repoRoot 'packaging\README.txt');       Dst = (Join-Path $OutDir 'README.txt')           }
    @{ Src = (Join-Path $repoRoot 'packaging\setup-firewall.bat'); Dst = (Join-Path $OutDir 'setup-firewall.bat') }
)
foreach ($b in $bundleSources) {
    if (-not (Test-Path $b.Src)) {
        throw "bundle source missing: $($b.Src)"
    }
    Copy-Item -LiteralPath $b.Src -Destination $b.Dst -Force
    Write-Host "==> [bundle] $(Split-Path $b.Dst -Leaf)" -ForegroundColor Cyan
}

$size = '{0:N2} MB' -f ((Get-Item $exePath).Length / 1MB)
Write-Host "==> done: $exePath ($size, version=$Version)" -ForegroundColor Green
