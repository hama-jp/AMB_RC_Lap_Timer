<#
.SYNOPSIS
    Field Test α — release ZIP shape check (Issue #70).

.DESCRIPTION
    Sanity-checks the bundle that scripts\build.ps1 emits.

    Required files (hard fail if missing — the operator can't run the
    gateway without them):
      - gateway.exe
      - config.example.json   (bundled by build.ps1; gateway bootstraps
                               config.json from this on first launch)
      - README.txt            (operator manual, bundled by build.ps1
                               from packaging/README.txt — Issue #37)

    Strongly preferred (warn if missing):
      - config.json   (only present after first launch; informational only)

    Size sanity check: 2 MB ≤ size ≤ 100 MB. Lower means the SPA didn't
    embed; higher suggests a stray asset that shouldn't ship.

.PARAMETER SkipBuild
    Re-use the existing dist\AMB_RC_Lap_Timer\ instead of running
    scripts\build.ps1. Useful when the harness is being iterated.

.OUTPUTS
    Markdown row on stdout. Exit code 0 only when required files are
    present and gateway.exe falls in the size range; warnings keep
    exit 0 but raise status to ⚠.
#>
[CmdletBinding()]
param(
    [switch]$SkipBuild
)

$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'fieldtest-common.ps1')

$exitCode = 0
$status = '❌'
$detail = 'aborted'

try {
    $repoRoot = Get-RepoRoot
    $distDir = Join-Path $repoRoot 'dist\AMB_RC_Lap_Timer'

    if (-not $SkipBuild) {
        Write-Host "==> [zip-shape] running scripts\build.ps1" -ForegroundColor Cyan
        & (Join-Path $PSScriptRoot 'build.ps1')
        if ($LASTEXITCODE -ne 0) { throw "build.ps1 failed (exit $LASTEXITCODE)" }
    }

    if (-not (Test-Path $distDir)) {
        throw "dist directory missing: $distDir (run scripts\build.ps1)"
    }

    $required = @('gateway.exe', 'config.example.json', 'README.txt')
    $optional = @('config.json')

    $missingRequired = @()
    foreach ($f in $required) {
        if (-not (Test-Path (Join-Path $distDir $f))) {
            $missingRequired += $f
        }
    }
    if ($missingRequired.Count -gt 0) {
        throw "required files missing: $($missingRequired -join ', ')"
    }

    $missingOptional = @()
    foreach ($f in $optional) {
        if (-not (Test-Path (Join-Path $distDir $f))) {
            $missingOptional += $f
        }
    }

    $exePath = Join-Path $distDir 'gateway.exe'
    $exeBytes = (Get-Item $exePath).Length
    $exeMb = [math]::Round($exeBytes / 1MB, 2)
    $sizeOk = ($exeBytes -ge (2 * 1MB)) -and ($exeBytes -le (100 * 1MB))

    if (-not $sizeOk) {
        throw "gateway.exe size $exeMb MB outside expected 2-100 MB range"
    }

    if ($missingOptional.Count -gt 0) {
        $status = '⚠'
        $detail = "size=${exeMb}MB; missing optional: $($missingOptional -join ', ')"
    } else {
        $status = '✅'
        $detail = "size=${exeMb}MB, files=ok"
    }
} catch {
    $status = '❌'
    $detail = $_.Exception.Message -replace '[\r\n]+', ' '
    $exitCode = 1
    Write-Error $_
}

Write-Output (Format-MarkdownRow -Label 'ZIP shape' -Status $status -Detail $detail)
exit $exitCode
