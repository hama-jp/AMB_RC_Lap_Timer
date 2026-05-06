<#
.SYNOPSIS
    Field Test α Smoke harness (Issue #70).

.DESCRIPTION
    Exercises gateway --mock end-to-end with three concurrent ws-recorder
    clients, then runs gateway/cmd/analyze on the captured byte stream so a
    single markdown row summarises the run. Aimed at < 2 minutes wall clock.

    Steps (matches Issue #70 spec, with adjustments noted in the PR #71 review
    comment):
      1. Build / locate gateway.exe + ws-recorder
      2. Start gateway --mock --listen :<free-port>
      3. Wait for /healthz to report upstream=mock
      4. Spawn three ws-recorder clients with --duration-sec; one captures
         raw bytes via --raw-out so analyze can introspect the stream
      5. Wait for all three to exit on their own (clean shutdown event)
      6. Stop the gateway
      7. analyze the captured bin: PASSING count, undocumented TOR count
      8. Print a markdown row to stdout

    Note vs the original Issue #70 wording: gateway's --record is mutually
    exclusive with --mock (gateway/cmd/gateway/main.go validateSourceFlags),
    so the byte capture is moved to the WS side via --raw-out. The byte
    pipe guarantees these are equivalent.

.PARAMETER DurationSec
    Per-client recording window. Default 30 seconds → ~20 frames per client.

.PARAMETER LeaveArtifacts
    Keep the run directory (CSV + raw bin) on success. Useful for debugging.

.OUTPUTS
    Markdown row on stdout. Exit code 0 on PASS, non-zero otherwise.
#>
[CmdletBinding()]
param(
    [int]$DurationSec = 30,
    [switch]$LeaveArtifacts
)

$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'fieldtest-common.ps1')

$exitCode = 0
$status = '❌'
$detail = 'aborted'

$runDir = Get-RunDir -Scenario 'smoke'
Write-Host "==> [smoke] runDir=$runDir" -ForegroundColor Cyan

$gatewayProc = $null
try {
    $gatewayExe = Find-GatewayExe
    $tools = Build-FieldtestTools
    $port = Get-FreeTcpPort
    Write-Host "==> [smoke] gateway=$gatewayExe port=$port" -ForegroundColor Cyan

    $gwStdout = Join-Path $runDir 'gateway.stdout.log'
    $gwStderr = Join-Path $runDir 'gateway.stderr.log'
    $gatewayProc = Start-Process -FilePath $gatewayExe `
        -ArgumentList @('--mock', '--listen', ":$port") `
        -RedirectStandardOutput $gwStdout `
        -RedirectStandardError $gwStderr `
        -PassThru -WindowStyle Hidden

    $health = Wait-HealthzReady -Port $port -TimeoutSec 15
    Write-Host "==> [smoke] /healthz upstream=$($health.upstream) version=$($health.version)" -ForegroundColor Cyan

    $clients = @()
    for ($i = 1; $i -le 3; $i++) {
        $csv = Join-Path $runDir "recorder-$i.csv"
        $args = @(
            '--url',          "ws://localhost:$port/ws",
            '--out',          $csv,
            '--duration-sec', $DurationSec,
            '--quiet'
        )
        if ($i -eq 1) {
            $rawBin = Join-Path $runDir 'client-1.raw.bin'
            $args += @('--raw-out', $rawBin)
        }
        $stderrLog = Join-Path $runDir "recorder-$i.stderr.log"
        $proc = Start-Process -FilePath $tools.WsRecorder `
            -ArgumentList $args `
            -RedirectStandardError $stderrLog `
            -PassThru -WindowStyle Hidden
        $clients += [pscustomobject]@{ Index = $i; Process = $proc; Csv = $csv }
    }

    Write-Host "==> [smoke] waiting for $($clients.Count) recorders to finish (~${DurationSec}s)..." -ForegroundColor Cyan
    foreach ($c in $clients) {
        if (-not $c.Process.WaitForExit(($DurationSec + 30) * 1000)) {
            throw "recorder #$($c.Index) timed out"
        }
        if ($c.Process.ExitCode -ne 0) {
            throw "recorder #$($c.Index) exited with code $($c.Process.ExitCode)"
        }
    }

    Stop-GatewayProcess -Process $gatewayProc
    $gatewayProc = $null

    # Per-client bytes (excluding shutdown rows). Variance check below.
    $perClientBytes = @()
    foreach ($c in $clients) {
        $rows = Import-Csv -Path $c.Csv
        $bytes = 0
        $disconnects = 0
        $sawShutdown = $false
        foreach ($r in $rows) {
            switch ($r.event) {
                'frame'      { $bytes += [int]$r.bytes }
                'disconnect' { $disconnects++ }
                'shutdown'   { $sawShutdown = $true }
            }
        }
        if (-not $sawShutdown) {
            throw "recorder #$($c.Index) did not write a shutdown event (clean exit expected)"
        }
        if ($disconnects -gt 0) {
            throw "recorder #$($c.Index) recorded $disconnects unexpected disconnects"
        }
        $perClientBytes += $bytes
    }

    $totalBytes = ($perClientBytes | Measure-Object -Sum).Sum
    $mean = $totalBytes / $clients.Count
    # Coefficient-of-variation gate: with 3 clients on the same byte pipe the
    # spread should be < 5%. Higher than that suggests fan-out delivers
    # different streams (a real bug — Issue #27 backpressure could matter).
    $variance = 0.0
    foreach ($v in $perClientBytes) { $variance += [math]::Pow($v - $mean, 2) }
    $variance = $variance / $clients.Count
    $stdev = [math]::Sqrt($variance)
    $cv = if ($mean -gt 0) { $stdev / $mean } else { 1.0 }

    # Analyze the raw bytes captured by client-1.
    $rawBin = Join-Path $runDir 'client-1.raw.bin'
    $rawSize = (Get-Item $rawBin).Length
    if ($rawSize -le 0) { throw "client-1 raw-out is empty ($rawBin)" }

    $gatewayDir = Join-Path (Get-RepoRoot) 'gateway'
    Push-Location $gatewayDir
    try {
        $analyzeOutput = & go run ./cmd/analyze $rawBin 2>&1
        if ($LASTEXITCODE -ne 0) {
            throw "analyze failed: $($analyzeOutput -join "`n")"
        }
    } finally {
        Pop-Location
    }
    Set-Content -Path (Join-Path $runDir 'analyze.txt') -Value ($analyzeOutput -join "`n") -Encoding UTF8

    $passings = 0
    $unknownTors = 0
    foreach ($line in $analyzeOutput) {
        if ($line -match 'TOR\s+(0x[0-9a-fA-F]{4})\s*:\s*(\d+)\s+frames\s+(.*)$') {
            $count = [int]$Matches[2]
            $name = $Matches[3]
            if ($name -match 'PASSING') { $passings += $count }
            if ($name -match '\(undocumented\)') { $unknownTors += $count }
        }
    }
    if ($passings -le 0) { throw "analyze reports zero PASSING frames" }

    $cvPct = '{0:N1}%' -f ($cv * 100)
    $detail = "bytes=$totalBytes, clients=$($clients.Count), passings=$passings, unknown_tors=$unknownTors, fanout_cv=$cvPct"
    if ($cv -gt 0.05) {
        $status = '⚠'
        $detail = "$detail (cv > 5%, possible fan-out skew)"
    } else {
        $status = '✅'
    }
} catch {
    $status = '❌'
    $detail = $_.Exception.Message -replace '[\r\n]+', ' '
    $exitCode = 1
    Write-Error $_
} finally {
    if ($null -ne $gatewayProc) {
        Stop-GatewayProcess -Process $gatewayProc
    }
}

# Always emit the markdown row to stdout — the runall script greps for it.
Write-Output (Format-MarkdownRow -Label 'Smoke' -Status $status -Detail $detail)

if (-not $LeaveArtifacts -and $exitCode -eq 0) {
    # Keep gateway logs but drop the raw .bin which is large.
    $rawBin = Join-Path $runDir 'client-1.raw.bin'
    if (Test-Path $rawBin) { Remove-Item $rawBin -Force -ErrorAction SilentlyContinue }
}

exit $exitCode
