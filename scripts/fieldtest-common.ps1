<#
.SYNOPSIS
    Helpers shared by scripts\fieldtest-*.ps1.

.DESCRIPTION
    Dot-source from a sibling script:

        . (Join-Path $PSScriptRoot 'fieldtest-common.ps1')

    Provides:
      - Get-RepoRoot                 absolute path to the repo root
      - Get-RunDir                   per-run scratch directory (creates it)
      - Get-FreeTcpPort              an OS-allocated free TCP port
      - Build-FieldtestTools         compiles tcp-emitter / ws-recorder once
      - Wait-HealthzReady            polls /healthz until upstream is up or timeout
      - Stop-GatewayProcess          best-effort kill that survives missing PID
      - Format-MarkdownRow           common 3-column markdown summary row

    Targets PowerShell 5.1 (Win 8.1 baseline) per docs/test-strategy.md §6 and
    Issue #70 review notes — no `?:` / `??` / `?.` / null-conditional members.
#>

$ErrorActionPreference = 'Stop'

function Get-RepoRoot {
    return (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
}

function Get-RunDir {
    param(
        [Parameter(Mandatory)]
        [string]$Scenario
    )
    $stamp = Get-Date -Format 'yyyyMMdd-HHmmss'
    $root = Get-RepoRoot
    $dir = Join-Path $root "dist\fieldtest-runs\$Scenario-$stamp"
    New-Item -ItemType Directory -Path $dir -Force | Out-Null
    return $dir
}

function Get-FreeTcpPort {
    $listener = New-Object System.Net.Sockets.TcpListener([System.Net.IPAddress]::Loopback, 0)
    $listener.Start()
    try {
        return $listener.LocalEndpoint.Port
    } finally {
        $listener.Stop()
    }
}

function Build-FieldtestTools {
    [CmdletBinding()]
    param(
        [string]$OutDir
    )
    $root = Get-RepoRoot
    $modDir = Join-Path $root 'tools\fieldtest'
    if (-not $OutDir) {
        $OutDir = Join-Path $root 'dist\fieldtest-bin'
    }
    if (-not (Test-Path $OutDir)) {
        New-Item -ItemType Directory -Path $OutDir -Force | Out-Null
    }
    $tcpEmitter = Join-Path $OutDir 'tcp-emitter.exe'
    $wsRecorder = Join-Path $OutDir 'ws-recorder.exe'

    Push-Location $modDir
    try {
        Write-Host "==> [fieldtest] go build tcp-emitter / ws-recorder -> $OutDir" -ForegroundColor Cyan
        & go build -o $tcpEmitter ./tcp-emitter
        if ($LASTEXITCODE -ne 0) { throw "go build tcp-emitter failed (exit $LASTEXITCODE)" }
        & go build -o $wsRecorder ./ws-recorder
        if ($LASTEXITCODE -ne 0) { throw "go build ws-recorder failed (exit $LASTEXITCODE)" }
    } finally {
        Pop-Location
    }

    return [pscustomobject]@{
        TcpEmitter = $tcpEmitter
        WsRecorder = $wsRecorder
    }
}

function Wait-HealthzReady {
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)] [int]$Port,
        [int]$TimeoutSec = 15,
        [string[]]$AcceptStates = @('connected', 'mock', 'replay', 'connecting')
    )
    $url = "http://localhost:$Port/healthz"
    $deadline = (Get-Date).AddSeconds($TimeoutSec)
    while ((Get-Date) -lt $deadline) {
        try {
            $r = Invoke-RestMethod -Uri $url -Method Get -TimeoutSec 2
            if ($AcceptStates -contains $r.upstream) {
                return $r
            }
        } catch {
            # not ready yet
        }
        Start-Sleep -Milliseconds 200
    }
    throw "healthz did not reach an accepted state within ${TimeoutSec}s (last url=$url)"
}

function Stop-GatewayProcess {
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)] $Process,
        [int]$WaitSec = 5
    )
    if ($null -eq $Process) { return }
    try {
        if ($Process.HasExited) { return }
        # Stop-Process is SIGKILL on Windows; the gateway's recorder flushes
        # per-write so on-disk state is preserved (gateway/internal/recorder
        # docstring). The "Shutdown completed" log line is therefore not
        # asserted by the harness — if Issue #70 needs it, follow up with a
        # CtrlC helper using kernel32!GenerateConsoleCtrlEvent.
        Stop-Process -Id $Process.Id -Force -ErrorAction Stop
        $Process.WaitForExit($WaitSec * 1000) | Out-Null
    } catch {
        Write-Warning "Stop-GatewayProcess: $($_.Exception.Message)"
    }
}

function Format-MarkdownRow {
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)] [string]$Label,
        [Parameter(Mandatory)] [string]$Status,   # ✅ / ⚠ / ❌
        [Parameter(Mandatory)] [string]$Detail
    )
    return "| $Label | $Status | $Detail |"
}

function Find-GatewayExe {
    $root = Get-RepoRoot
    $candidates = @(
        (Join-Path $root 'dist\AMB_RC_Lap_Timer\gateway.exe'),
        (Join-Path $root 'gateway\gateway.exe')
    )
    foreach ($c in $candidates) {
        if (Test-Path $c) { return (Resolve-Path $c).Path }
    }
    throw "gateway.exe not found. Run scripts\build.ps1 first. Looked in: $($candidates -join ', ')"
}
