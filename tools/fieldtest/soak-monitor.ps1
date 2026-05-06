<#
.SYNOPSIS
    Periodically samples a gateway process and writes resource usage to CSV.

.DESCRIPTION
    Soak-test helper for the AMB RC Lap Timer gateway. Records memory,
    handle count, thread count and the number of ESTABLISHED TCP
    connections owned by the target process so memory leaks / fd leaks
    surface during multi-hour runs (docs/test-strategy.md §6.2 Soak).

    Output columns:
        timestamp           ISO 8601 UTC
        process_name        target process name
        pid                 process id at sample time
        ws_mb               WorkingSet64 in MiB
        private_mb          PrivateMemorySize64 in MiB
        handles             handle count
        threads             thread count
        cpu_seconds         total CPU time consumed since process start
        established         ESTABLISHED TCP connections owned by pid
        listen              LISTEN TCP sockets owned by pid

    The script keeps running until Ctrl+C or until -DurationSec elapses.
    On error (process not found, transient WMI failure), a row with
    error="..." is still written so gaps are visible in the output.

.PARAMETER ProcessName
    Process to sample (without .exe suffix). Default: gateway

.PARAMETER IntervalSec
    Sampling cadence in seconds. Default: 30

.PARAMETER OutFile
    Output CSV path. Default: soak-monitor.csv (relative to cwd)

.PARAMETER DurationSec
    Stop after this many seconds. 0 means run until Ctrl+C. Default: 0

.EXAMPLE
    .\soak-monitor.ps1 -ProcessName gateway -IntervalSec 30 -OutFile soak.csv

.EXAMPLE
    .\soak-monitor.ps1 -ProcessName gateway -IntervalSec 60 -DurationSec 3600
#>
[CmdletBinding()]
param(
    [string]$ProcessName = 'gateway',
    [int]$IntervalSec = 30,
    [string]$OutFile = 'soak-monitor.csv',
    [int]$DurationSec = 0
)

$ErrorActionPreference = 'Stop'

if ($IntervalSec -le 0) {
    throw "IntervalSec must be > 0"
}

$header = 'timestamp,process_name,pid,ws_mb,private_mb,handles,threads,cpu_seconds,established,listen,error'
if (-not (Test-Path -LiteralPath $OutFile)) {
    Set-Content -LiteralPath $OutFile -Value $header -Encoding UTF8
}

$start = Get-Date
$row = 0

Write-Host "soak-monitor: process=$ProcessName interval=${IntervalSec}s out=$OutFile duration=${DurationSec}s"

while ($true) {
    $now = Get-Date
    $elapsed = ($now - $start).TotalSeconds
    if ($DurationSec -gt 0 -and $elapsed -ge $DurationSec) {
        break
    }

    $iso = $now.ToUniversalTime().ToString('yyyy-MM-ddTHH:mm:ss.fffZ')
    $errMsg = ''
    $procPid = ''
    $wsMb = ''
    $privateMb = ''
    $handles = ''
    $threads = ''
    $cpu = ''
    $established = ''
    $listen = ''

    try {
        $proc = Get-Process -Name $ProcessName -ErrorAction Stop | Select-Object -First 1
        $procPid = $proc.Id
        $wsMb = [math]::Round($proc.WorkingSet64 / 1MB, 2)
        $privateMb = [math]::Round($proc.PrivateMemorySize64 / 1MB, 2)
        $handles = $proc.HandleCount
        $threads = $proc.Threads.Count
        $cpu = [math]::Round($proc.CPU, 2)

        try {
            $conns = Get-NetTCPConnection -OwningProcess $procPid -ErrorAction Stop
            $established = ($conns | Where-Object { $_.State -eq 'Established' }).Count
            $listen = ($conns | Where-Object { $_.State -eq 'Listen' }).Count
        } catch {
            $established = '?'
            $listen = '?'
        }
    } catch {
        $errMsg = ($_.Exception.Message -replace '"', '""' -replace "[`r`n]+", ' ')
    }

    $line = '{0},{1},{2},{3},{4},{5},{6},{7},{8},{9},"{10}"' -f `
        $iso, $ProcessName, $procPid, $wsMb, $privateMb, $handles, $threads, $cpu, $established, $listen, $errMsg
    Add-Content -LiteralPath $OutFile -Value $line -Encoding UTF8

    $row++
    if (($row % 10) -eq 1) {
        Write-Host ("[{0}] pid={1} ws={2}MB priv={3}MB handles={4} threads={5} estab={6} err={7}" -f `
            $iso, $procPid, $wsMb, $privateMb, $handles, $threads, $established, $errMsg)
    }

    Start-Sleep -Seconds $IntervalSec
}

Write-Host "soak-monitor: stopped after $row samples"
