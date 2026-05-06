<#
.SYNOPSIS
    Field Test α replay byte-pipe round-trip (Issue #70).

.DESCRIPTION
    Asserts that bytes flowing
        captured .bin  →  gateway --replay  →  /ws  →  ws-recorder --raw-out
    are byte-identical to the input. Any drift here is a regression in the
    "byte pipe" promise (docs/architecture.md §1) — which the SPA parser
    relies on absolutely.

    Steps:
      1. Start gateway --replay <fixture> --listen :<free-port>
      2. Wait for /healthz upstream != "down"
      3. Run ws-recorder --raw-out for a short window
      4. Compare WS raw output to input bin via SHA-256 + length

    Uses gateway/testdata/captured/session-2026-05-05.bin by default. That
    fixture has no .timing.csv so replay uses "instant" mode (replay.go),
    which keeps this test fast (~5 seconds wall clock).

.PARAMETER Fixture
    Path to the input .bin. Default: gateway/testdata/captured/session-2026-05-05.bin

.PARAMETER DurationSec
    How long the recorder runs. Default 6 seconds — replay finishes well
    inside that for the bundled fixture.

.OUTPUTS
    Markdown row on stdout. Exit code 0 on byte-equal, non-zero otherwise.
#>
[CmdletBinding()]
param(
    [string]$Fixture,
    [int]$DurationSec = 8,
    [switch]$LeaveArtifacts
)

$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'fieldtest-common.ps1')

$exitCode = 0
$status = '❌'
$detail = 'aborted'

if (-not $Fixture) {
    $Fixture = Join-Path (Get-RepoRoot) 'gateway\testdata\captured\session-2026-05-05.bin'
}
if (-not (Test-Path $Fixture)) {
    throw "fixture not found: $Fixture"
}
$Fixture = (Resolve-Path $Fixture).Path

$runDir = Get-RunDir -Scenario 'replay-rt'
Write-Host "==> [replay-rt] runDir=$runDir fixture=$Fixture" -ForegroundColor Cyan

$gatewayProc = $null
try {
    $gatewayExe = Find-GatewayExe
    $tools = Build-FieldtestTools
    $port = Get-FreeTcpPort

    # Replay in instant mode (no timing.csv) dumps the entire bin in one
    # goroutine tick, so the WS broadcast can fire before the harness
    # finishes spawning ws-recorder. We side-step the race by copying the
    # fixture into the run dir and writing a timing.csv that delays the
    # first chunk by 1500ms — well past the ~200-500ms dial+upgrade window
    # that ws-recorder needs.
    $replayBin = Join-Path $runDir 'replay-input.bin'
    Copy-Item -LiteralPath $Fixture -Destination $replayBin -Force
    $inLen = (Get-Item $replayBin).Length
    $timingCsv = "$replayBin.timing.csv"
    Set-Content -LiteralPath $timingCsv -Value @("offset_ms,length_bytes", "1500,$inLen") -Encoding ascii

    $gwStdout = Join-Path $runDir 'gateway.stdout.log'
    $gwStderr = Join-Path $runDir 'gateway.stderr.log'
    $gatewayProc = Start-Process -FilePath $gatewayExe `
        -ArgumentList @('--replay', $replayBin, '--listen', ":$port") `
        -RedirectStandardOutput $gwStdout `
        -RedirectStandardError $gwStderr `
        -PassThru -WindowStyle Hidden

    Wait-HealthzReady -Port $port -TimeoutSec 15 -AcceptStates @('replay', 'finished') | Out-Null

    $rawOut = Join-Path $runDir 'rt.bin'
    $csv    = Join-Path $runDir 'rt.csv'
    $stderrLog = Join-Path $runDir 'recorder.stderr.log'
    $recorderProc = Start-Process -FilePath $tools.WsRecorder `
        -ArgumentList @(
            '--url',          "ws://localhost:$port/ws",
            '--out',          $csv,
            '--raw-out',      $rawOut,
            '--duration-sec', $DurationSec,
            '--quiet'
        ) `
        -RedirectStandardError $stderrLog `
        -PassThru -WindowStyle Hidden

    if (-not $recorderProc.WaitForExit(($DurationSec + 30) * 1000)) {
        throw "ws-recorder did not exit in time"
    }
    if ($recorderProc.ExitCode -ne 0) {
        throw "ws-recorder exited non-zero: $($recorderProc.ExitCode)"
    }

    Stop-GatewayProcess -Process $gatewayProc
    $gatewayProc = $null

    $inHash  = (Get-FileHash -Algorithm SHA256 -Path $Fixture).Hash
    $outHash = (Get-FileHash -Algorithm SHA256 -Path $rawOut).Hash
    $outLen = (Get-Item $rawOut).Length

    if ($inLen -ne $outLen) {
        throw "byte length mismatch: in=$inLen out=$outLen"
    }
    if ($inHash -ne $outHash) {
        throw "byte content mismatch: in=$inHash out=$outHash"
    }

    $status = '✅'
    $detail = "bytes_in=$inLen, bytes_out=$outLen, diff=0"
} catch {
    $status = '❌'
    $detail = $_.Exception.Message -replace '[\r\n]+', ' '
    $exitCode = 1
    Write-Error $_
} finally {
    if ($null -ne $gatewayProc) { Stop-GatewayProcess -Process $gatewayProc }
}

Write-Output (Format-MarkdownRow -Label 'Replay round-trip' -Status $status -Detail $detail)

if (-not $LeaveArtifacts -and $exitCode -eq 0) {
    Remove-Item -Recurse -Force $runDir -ErrorAction SilentlyContinue
}

exit $exitCode
