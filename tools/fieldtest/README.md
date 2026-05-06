# Field Test Tools

Helpers for the **Field Test α / β** scenarios in
[`docs/test-strategy.md`](../../docs/test-strategy.md) §6.2 / §6.3.
They are **not on CI** — operators run them by hand on real Windows boxes
over real WiFi, with no actual AMB decoder required.

| Tool | Purpose |
|------|---------|
| [`tcp-emitter/`](tcp-emitter/) | Mock AMB-side TCP server. Pushes wire-encoded P3 PASSING / STATUS frames to the gateway under test. |
| [`ws-recorder/`](ws-recorder/) | Headless WebSocket client for `/ws`. Logs received bytes, inter-frame latency and disconnect counts to CSV / JSONL. |
| [`soak-monitor.ps1`](soak-monitor.ps1) | PowerShell sampler for the gateway EXE: WorkingSet, PrivateMemory, handles, threads, ESTABLISHED TCP connections. Drives the 1–8 h Soak scenario. |

The Go tools form a single module (`tools/fieldtest/go.mod`) with two
binary directories. The shared `internal/frame` package re-implements
just enough of the gateway's `internal/p3frame` package to build valid
PASSING / STATUS frames; it does not import the gateway module.

## Prerequisites

- Go 1.20.x (matches `gateway/go.mod`)
- PowerShell 5.1+ (Windows) for `soak-monitor.ps1`
- The gateway EXE for end-to-end runs

## tcp-emitter

```pwsh
cd tools\fieldtest
go run ./tcp-emitter --port 5403 --interval-ms 1500 --ponders 1,2,3
```

Flags:

| Flag | Default | Notes |
|------|---------|-------|
| `--port`           | `5403`     | TCP listen port; AMB default is 5403. |
| `--interval-ms`    | `1500`     | Mean inter-frame interval. |
| `--jitter-ratio`   | `0.5`      | Uniform jitter `±ratio` of the interval. Must be `[0, 1)`. |
| `--ponders`        | `1,2,3`    | Comma-separated transponder IDs. Decimal or `0x..`. |
| `--decoder-id`     | `0x00041D17` | DECODER_ID value (matches the captured fixture). |
| `--status-every`   | `30`       | Inject one STATUS frame every N frames. `0` disables. |
| `--seed`           | `0`        | PRNG seed. `0` = time-based (per-client mixed with the connection id). |
| `--config`         | (none)     | Optional JSON file overlaid on top of flag defaults. |

Each accepted TCP connection runs in its own goroutine with an
independent counter. When the gateway disconnects (e.g., reconnect test),
the next connection starts a fresh stream.

Point the gateway at the emitter:

```pwsh
.\gateway.exe --upstream 127.0.0.1:5403
```

## ws-recorder

```pwsh
cd tools\fieldtest
go run ./ws-recorder --url ws://localhost:8080/ws --out recorder.csv --duration-sec 600
```

Flags:

| Flag | Default | Notes |
|------|---------|-------|
| `--url`                  | `ws://localhost:8080/ws` | Gateway `/ws` URL. |
| `--out`                  | `recorder.csv`           | Output path. Truncated on start. |
| `--format`               | `csv`                    | `csv` or `jsonl`. |
| `--duration-sec`         | `0`                      | Exit after N s. `0` = run until SIGINT. |
| `--reconnect-initial-ms` | `500`                    | Initial reconnect backoff. |
| `--reconnect-max-ms`     | `30000`                  | Cap on reconnect backoff. |
| `--quiet`                | `false`                  | Suppress per-frame stderr logging (rows are still written). |

CSV columns: `timestamp, event, frame_index, bytes, ms_since_prev, note`.

`event` is one of:

- `connect` — successful WebSocket handshake; `note` is the URL.
- `frame` — binary message received; `bytes` is the payload size.
- `disconnect` — connection dropped; `note` is the underlying error.

Backoff resets to the initial value once a connection that received at
least one frame disconnects, so a single connection that drops after a
long uptime does not stall the next reconnect.

For Multi-client / Soak runs launch several instances in parallel, each
with its own `--out` file:

```pwsh
Start-Process pwsh -ArgumentList '-Command', "cd tools\fieldtest; go run ./ws-recorder --url ws://localhost:8080/ws --out recorder-1.csv"
Start-Process pwsh -ArgumentList '-Command', "cd tools\fieldtest; go run ./ws-recorder --url ws://localhost:8080/ws --out recorder-2.csv"
```

## soak-monitor.ps1

```pwsh
.\tools\fieldtest\soak-monitor.ps1 -ProcessName gateway -IntervalSec 30 -OutFile soak.csv
```

Parameters:

| Parameter | Default | Notes |
|-----------|---------|-------|
| `-ProcessName` | `gateway` | Without the `.exe` suffix. |
| `-IntervalSec` | `30`      | Sampling cadence. |
| `-OutFile`     | `soak-monitor.csv` | Appended to (header written only on creation). |
| `-DurationSec` | `0`       | `0` = run until Ctrl+C. |

CSV columns:
`timestamp, process_name, pid, ws_mb, private_mb, handles, threads, cpu_seconds, established, listen, error`.

If the process is missing at sample time, a row with an `error` column
populated is written so the gap is visible.

## End-to-end smoke

In three terminals:

```pwsh
# A. mock decoder
cd tools\fieldtest
go run ./tcp-emitter --port 5403 --interval-ms 1500

# B. gateway pointed at the mock
.\gateway.exe --listen :8080
# (config.json or env to set upstream 127.0.0.1:5403)

# C. recorder
cd tools\fieldtest
go run ./ws-recorder --url ws://localhost:8080/ws --out recorder.csv --duration-sec 60
```

After ~60 s, `recorder.csv` should contain one `connect` row and roughly
40 `frame` rows.

## Scenario mapping

How the tools map to `docs/test-strategy.md` §6.2:

| Scenario | tcp-emitter | ws-recorder | soak-monitor |
|---|---|---|---|
| Smoke | yes | optional | no |
| Multi-client | yes | run N instances | no |
| Sleep/Wake | yes | yes | optional |
| WiFi drop | yes | yes | optional |
| Soak (1–8 h) | yes (long-running) | optional | yes |
| Firewall fresh | yes | optional | no |
