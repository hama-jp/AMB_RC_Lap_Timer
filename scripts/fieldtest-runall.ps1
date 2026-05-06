<#
.SYNOPSIS
    Run every Field Test α harness sequentially and emit a markdown
    summary block ready to paste into docs\field-test-log.md.

.DESCRIPTION
    Invokes, in order:
      1. fieldtest-zip-shape.ps1      (~30 s, optional -SkipBuild)
      2. fieldtest-smoke.ps1          (~1 min)
      3. fieldtest-replay-roundtrip.ps1 (~15 s)
      4. fieldtest-usb-pathshift.ps1  (~10 s)
      5. fieldtest-soak.ps1           (default 60 min; pass -SoakDurationMin)

    Each script is responsible for printing exactly one markdown row to
    stdout. This script captures that row plus the script's exit code and
    prints a summary block at the end.

    Exit code: non-zero if any sub-script failed. The summary still prints.

    Note: the human-only scenarios (iOS Safari Speech / Sleep-Wake / real
    WiFi drop / physical USB / SmartScreen) are NOT covered here — see
    docs\test-strategy.md §6.2 and the "未実施(人手要)" rows in
    docs\field-test-log.md.

.PARAMETER SoakDurationMin
    Pass-through to fieldtest-soak.ps1 (-DurationMin). Default 60.

.PARAMETER SkipBuild
    Forwarded to fieldtest-zip-shape.ps1; reuses the existing dist/.

.PARAMETER SkipSoak
    Skip the long soak step. Useful when iterating on the harness itself.
#>
[CmdletBinding()]
param(
    [int]$SoakDurationMin = 60,
    [switch]$SkipBuild,
    [switch]$SkipSoak
)

$ErrorActionPreference = 'Continue' # we want to keep going even on a sub-failure

$results = @()
$overallExit = 0

function Invoke-Sub {
    param(
        [Parameter(Mandatory)] [string]$Path,
        [hashtable]$Params = @{}
    )
    $argSummary = ($Params.GetEnumerator() | ForEach-Object { "-$($_.Key) $($_.Value)" }) -join ' '
    Write-Host ""
    Write-Host "============================================================" -ForegroundColor DarkCyan
    Write-Host " > $Path $argSummary" -ForegroundColor DarkCyan
    Write-Host "============================================================" -ForegroundColor DarkCyan
    # Invoke directly (not via a fresh powershell.exe) so Write-Host messages
    # surface in the parent terminal AND Write-Output ends up in $output.
    # Spawning a child powershell.exe and using -File swallows Write-Host on
    # PS 5.1 (it bypasses to the child's console which is not piped back).
    # Use hashtable splat so [switch] parameters bind correctly (an array
    # splat passes "-SkipBuild" positionally and trips parameter binding).
    $output = & $Path @Params *>&1
    $code = $LASTEXITCODE
    $rowLine = $null
    foreach ($line in @($output)) {
        $s = [string]$line
        # Echo so the operator sees what each sub-script actually did.
        Write-Host $s
        if ($s -match '^\|\s.*\|\s.*\|\s.*\|\s*$') { $rowLine = $s }
    }
    if (-not $rowLine) {
        $rowLine = "| $(Split-Path $Path -Leaf) | ❌ | (no markdown row emitted) |"
    }
    return [pscustomobject]@{
        Path = $Path
        ExitCode = $code
        Row = $rowLine
        Output = $output
    }
}

$smoke      = Invoke-Sub -Path (Join-Path $PSScriptRoot 'fieldtest-smoke.ps1')
$results += $smoke
if ($smoke.ExitCode -ne 0) { $overallExit = $smoke.ExitCode }

$replay     = Invoke-Sub -Path (Join-Path $PSScriptRoot 'fieldtest-replay-roundtrip.ps1')
$results += $replay
if ($replay.ExitCode -ne 0) { $overallExit = $replay.ExitCode }

$zipParams = @{}
if ($SkipBuild) { $zipParams['SkipBuild'] = $true }
$zip        = Invoke-Sub -Path (Join-Path $PSScriptRoot 'fieldtest-zip-shape.ps1') -Params $zipParams
$results += $zip
if ($zip.ExitCode -ne 0) { $overallExit = $zip.ExitCode }

$usb        = Invoke-Sub -Path (Join-Path $PSScriptRoot 'fieldtest-usb-pathshift.ps1')
$results += $usb
if ($usb.ExitCode -ne 0) { $overallExit = $usb.ExitCode }

if (-not $SkipSoak) {
    $soak   = Invoke-Sub -Path (Join-Path $PSScriptRoot 'fieldtest-soak.ps1') -Params @{DurationMin = $SoakDurationMin}
    $results += $soak
    if ($soak.ExitCode -ne 0) { $overallExit = $soak.ExitCode }
} else {
    Write-Host "==> [runall] soak skipped (-SkipSoak)" -ForegroundColor Yellow
    $results += [pscustomobject]@{
        Path = '(soak)'
        ExitCode = 0
        Row = "| Soak | ⏭ | skipped via -SkipSoak |"
        Output = @()
    }
}

Write-Host ""
Write-Host "============================================================" -ForegroundColor DarkCyan
Write-Host " Field Test α — runall summary" -ForegroundColor DarkCyan
Write-Host "============================================================" -ForegroundColor DarkCyan
Write-Output ""
Write-Output "| Scenario | Result | Detail |"
Write-Output "|---|---|---|"
foreach ($r in $results) {
    Write-Output $r.Row
}
Write-Output ""

if ($overallExit -ne 0) {
    Write-Host "==> [runall] one or more scenarios failed (exit $overallExit)" -ForegroundColor Red
} else {
    Write-Host "==> [runall] all scenarios passed" -ForegroundColor Green
}

exit $overallExit
