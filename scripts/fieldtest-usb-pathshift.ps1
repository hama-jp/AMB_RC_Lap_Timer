<#
.SYNOPSIS
    Field Test α — verify EXE-relative path resolution under a different
    drive letter (Issue #70 §5).

.DESCRIPTION
    Maps `subst Z: <repo>\dist\AMB_RC_Lap_Timer`, runs the gateway from
    Z:, and asserts that:
      - /healthz responds (HTTP works at the new path)
      - Z:\logs\ is created (relative path resolution per
        docs/architecture.md §4.4.2 — `os.Executable()` based)

    This is the closest we can get to the "USB plugged in at random
    drive letter" Field Test scenario without an actual USB stick.

    Cleanup: subst Z: /D is run regardless of test outcome.

.PARAMETER DriveLetter
    Drive letter to mount onto. Default Z. Will fail if already in use.

.OUTPUTS
    Markdown row on stdout. Exit code 0 on success.
#>
[CmdletBinding()]
param(
    [string]$DriveLetter = 'Z',
    [switch]$LeaveArtifacts
)

$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'fieldtest-common.ps1')

$exitCode = 0
$status = '❌'
$detail = 'aborted'

if ($DriveLetter -notmatch '^[A-Za-z]$') {
    throw "DriveLetter must be a single letter, got '$DriveLetter'"
}
$drive = "$($DriveLetter.ToUpper()):"

$gatewayProc = $null
$substApplied = $false
try {
    $repoRoot = Get-RepoRoot
    $distDir = Join-Path $repoRoot 'dist\AMB_RC_Lap_Timer'
    if (-not (Test-Path (Join-Path $distDir 'gateway.exe'))) {
        throw "gateway.exe missing under $distDir — run scripts\build.ps1 first"
    }

    if (Test-Path "${drive}\") {
        throw "$drive already in use; pick a different -DriveLetter"
    }

    Write-Host "==> [usb-pathshift] subst $drive $distDir" -ForegroundColor Cyan
    & subst $drive $distDir
    if ($LASTEXITCODE -ne 0) { throw "subst failed (exit $LASTEXITCODE)" }
    $substApplied = $true

    $exeOnDrive = Join-Path "${drive}\" 'gateway.exe'
    $port = Get-FreeTcpPort

    $gatewayProc = Start-Process -FilePath $exeOnDrive `
        -ArgumentList @('--mock', '--listen', ":$port") `
        -PassThru -WindowStyle Hidden

    $health = Wait-HealthzReady -Port $port -TimeoutSec 15
    Write-Host "==> [usb-pathshift] /healthz upstream=$($health.upstream)" -ForegroundColor Cyan

    $logsDir = Join-Path "${drive}\" 'logs'
    if (-not (Test-Path $logsDir)) {
        throw "expected $logsDir to be created relative to the EXE; not found (path resolution bug)"
    }

    Stop-GatewayProcess -Process $gatewayProc
    $gatewayProc = $null

    $status = '✅'
    $detail = "drive=$drive, healthz=ok, logs_created=true"
} catch {
    $status = '❌'
    $detail = $_.Exception.Message -replace '[\r\n]+', ' '
    $exitCode = 1
    Write-Error $_
} finally {
    if ($null -ne $gatewayProc) { Stop-GatewayProcess -Process $gatewayProc }
    if ($substApplied) {
        & subst $drive /D 2>$null | Out-Null
    }
}

Write-Output (Format-MarkdownRow -Label 'USB pathshift' -Status $status -Detail $detail)
exit $exitCode
