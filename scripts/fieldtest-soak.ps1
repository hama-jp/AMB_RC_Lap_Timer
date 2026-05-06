<#
.SYNOPSIS
    Field Test α Soak harness (Issue #70).

.DESCRIPTION
    Runs gateway --mock for -DurationMin minutes with one ws-recorder
    consumer attached and the soak-monitor PowerShell sampler running in
    parallel. After the run, judges memory / handle drift over the first
    five vs last five minutes:

        ws_mb_delta     = avg(last 5min)  / avg(first 5min)  - 1
        handle_delta    = same, on HandleCount
        reconnect_count = ws-recorder rows where event="disconnect"

    Per Issue #70 review: ws-recorder writes a single "shutdown" row at
    clean exit; this harness counts only event="disconnect" so the soak
    PASS gate is "0 abnormal disconnects during the run".

    Pass thresholds (Issue #70 spec, root cause for the magnitudes):
      - WorkingSet drift  ≤ +20% (loose; Windows page cache is noisy)
      - Handle drift      ≤ +10% (tight; goroutine / fd leaks compound fast)
      - reconnect_count   == 0   (mock + localhost shouldn't ever drop)

    Below those: ✅. Reconnects > 0 OR ws drift between +20% and +50%: ⚠.
    Anything worse: ❌.

.PARAMETER DurationMin
    Total run time in minutes. Default 60. Use small values (e.g. 10) for
    a quick smoke of the harness itself.

.PARAMETER WindowMin
    Size of the head / tail averaging window. Default 5. Must be at most
    DurationMin / 2 — otherwise the windows overlap and the deltas are
    meaningless.

.OUTPUTS
    Markdown row on stdout. Exit code 0 on PASS, non-zero otherwise.
#>
[CmdletBinding()]
param(
    [int]$DurationMin = 60,
    [int]$WindowMin = 5,
    [switch]$LeaveArtifacts
)

$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'fieldtest-common.ps1')

if ($DurationMin -lt ($WindowMin * 2)) {
    throw "DurationMin ($DurationMin) must be >= 2 * WindowMin ($WindowMin)"
}

$exitCode = 0
$status = '❌'
$detail = 'aborted'

$runDir = Get-RunDir -Scenario 'soak'
Write-Host "==> [soak] runDir=$runDir duration=${DurationMin}m window=${WindowMin}m" -ForegroundColor Cyan

$gatewayProc = $null
$recorderProc = $null
$soakProc = $null
try {
    $gatewayExe = Find-GatewayExe
    $tools = Build-FieldtestTools
    $port = Get-FreeTcpPort
    $durationSec = $DurationMin * 60

    $gwStdout = Join-Path $runDir 'gateway.stdout.log'
    $gwStderr = Join-Path $runDir 'gateway.stderr.log'
    $gatewayProc = Start-Process -FilePath $gatewayExe `
        -ArgumentList @('--mock', '--listen', ":$port") `
        -RedirectStandardOutput $gwStdout `
        -RedirectStandardError $gwStderr `
        -PassThru -WindowStyle Hidden

    Wait-HealthzReady -Port $port -TimeoutSec 15 | Out-Null
    Write-Host "==> [soak] gateway pid=$($gatewayProc.Id) port=$port" -ForegroundColor Cyan

    $recorderCsv = Join-Path $runDir 'recorder.csv'
    $recorderStderr = Join-Path $runDir 'recorder.stderr.log'
    $recorderProc = Start-Process -FilePath $tools.WsRecorder `
        -ArgumentList @(
            '--url',          "ws://localhost:$port/ws",
            '--out',          $recorderCsv,
            '--duration-sec', $durationSec,
            '--quiet'
        ) `
        -RedirectStandardError $recorderStderr `
        -PassThru -WindowStyle Hidden

    $soakCsv = Join-Path $runDir 'soak-monitor.csv'
    $soakScript = Join-Path (Get-RepoRoot) 'tools\fieldtest\soak-monitor.ps1'
    # soak-monitor exits when -DurationSec elapses (reuses our duration).
    $soakProc = Start-Process -FilePath 'powershell.exe' `
        -ArgumentList @(
            '-NoProfile', '-ExecutionPolicy', 'Bypass',
            '-File', $soakScript,
            '-ProcessName',  ([System.IO.Path]::GetFileNameWithoutExtension($gatewayExe)),
            '-IntervalSec',  '30',
            '-OutFile',      $soakCsv,
            '-DurationSec',  $durationSec
        ) `
        -PassThru -WindowStyle Hidden

    $waitMs = ($durationSec + 60) * 1000
    Write-Host "==> [soak] waiting up to ${waitMs}ms for recorder + monitor..." -ForegroundColor Cyan
    if (-not $recorderProc.WaitForExit($waitMs)) { throw "ws-recorder did not exit in time" }
    if (-not $soakProc.WaitForExit($waitMs))     { throw "soak-monitor did not exit in time" }

    Stop-GatewayProcess -Process $gatewayProc
    $gatewayProc = $null

    if ($recorderProc.ExitCode -ne 0) {
        throw "ws-recorder exited non-zero: $($recorderProc.ExitCode)"
    }

    # ---- recorder analysis ----
    $rows = Import-Csv -Path $recorderCsv
    $disconnects = ($rows | Where-Object { $_.event -eq 'disconnect' }).Count
    $shutdowns   = ($rows | Where-Object { $_.event -eq 'shutdown'   }).Count
    if ($shutdowns -ne 1) {
        throw "expected exactly one shutdown event, got $shutdowns"
    }

    # ---- soak-monitor analysis ----
    if (-not (Test-Path $soakCsv)) { throw "soak CSV missing: $soakCsv" }
    $soakRows = Import-Csv -Path $soakCsv | Where-Object { $_.error -eq '' -or $null -eq $_.error }
    if ($soakRows.Count -lt 4) {
        throw "soak CSV has only $($soakRows.Count) rows (need >=4 for windowed avg)"
    }
    $start = [datetime]::Parse($soakRows[0].timestamp).ToUniversalTime()
    $headEnd = $start.AddMinutes($WindowMin)
    $tailStart = $start.AddMinutes($DurationMin - $WindowMin)

    $headRows = @()
    $tailRows = @()
    foreach ($r in $soakRows) {
        $ts = [datetime]::Parse($r.timestamp).ToUniversalTime()
        if ($ts -le $headEnd) { $headRows += $r }
        if ($ts -ge $tailStart) { $tailRows += $r }
    }
    if ($headRows.Count -eq 0 -or $tailRows.Count -eq 0) {
        throw "head ($($headRows.Count)) or tail ($($tailRows.Count)) window empty"
    }

    function Get-AvgFloat([array]$rows, [string]$col) {
        $vals = foreach ($r in $rows) {
            $v = $r.$col
            if ($v -ne '' -and $v -ne '?') {
                [double]$v
            }
        }
        if ($vals.Count -eq 0) { return 0.0 }
        return ($vals | Measure-Object -Average).Average
    }

    $headWs   = Get-AvgFloat $headRows 'ws_mb'
    $tailWs   = Get-AvgFloat $tailRows 'ws_mb'
    $headHnd  = Get-AvgFloat $headRows 'handles'
    $tailHnd  = Get-AvgFloat $tailRows 'handles'

    $wsDelta  = if ($headWs   -gt 0) { ($tailWs  / $headWs)  - 1 } else { 0 }
    $hndDelta = if ($headHnd  -gt 0) { ($tailHnd / $headHnd) - 1 } else { 0 }

    # Verdict
    $verdict = '✅'
    $reasons = @()
    if ($wsDelta  -gt 0.50) { $verdict = '❌'; $reasons += 'ws_mb +50%' }
    elseif ($wsDelta  -gt 0.20) { $verdict = '⚠'; $reasons += "ws_mb $('{0:N1}%' -f ($wsDelta*100))" }
    if ($hndDelta -gt 0.20) { $verdict = '❌'; $reasons += 'handles +20%' }
    elseif ($hndDelta -gt 0.10) { if ($verdict -ne '❌') { $verdict = '⚠' }; $reasons += "handles $('{0:N1}%' -f ($hndDelta*100))" }
    if ($disconnects -gt 0) { if ($verdict -ne '❌') { $verdict = '⚠' }; $reasons += "reconnects=$disconnects" }

    $detail = ('ws_mb_delta={0}, handle_delta={1}, reconnects={2}' -f `
        ('{0:+0.0;-0.0;0.0}%' -f ($wsDelta*100)),
        ('{0:+0.0;-0.0;0.0}%' -f ($hndDelta*100)),
        $disconnects)
    if ($reasons.Count -gt 0 -and $verdict -ne '✅') {
        $detail = "$detail ($(($reasons | Select-Object -Unique) -join '; '))"
    }
    $status = $verdict
    if ($verdict -eq '❌') { $exitCode = 1 }
} catch {
    $status = '❌'
    $detail = $_.Exception.Message -replace '[\r\n]+', ' '
    $exitCode = 1
    Write-Error $_
} finally {
    if ($null -ne $gatewayProc) { Stop-GatewayProcess -Process $gatewayProc }
    if ($null -ne $recorderProc -and -not $recorderProc.HasExited) {
        Stop-Process -Id $recorderProc.Id -Force -ErrorAction SilentlyContinue
    }
    if ($null -ne $soakProc -and -not $soakProc.HasExited) {
        Stop-Process -Id $soakProc.Id -Force -ErrorAction SilentlyContinue
    }
}

Write-Output (Format-MarkdownRow -Label "Soak (${DurationMin}m)" -Status $status -Detail $detail)

if (-not $LeaveArtifacts -and $exitCode -eq 0) {
    # Soak runs are big; clean up unless asked to keep them.
    Remove-Item -Recurse -Force $runDir -ErrorAction SilentlyContinue
}

exit $exitCode
