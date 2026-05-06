# scripts/

Local automation that does **not** run on CI (Issue #70 explicitly forbids
adding CI jobs for the field-test harness — those scenarios need real
Windows hosts).

| Script | Purpose |
|---|---|
| [`build.ps1`](build.ps1) | Canonical local build (SPA + go embed + gateway.exe). Used by all field-test scripts. |
| [`fieldtest-common.ps1`](fieldtest-common.ps1) | Helpers shared by every `fieldtest-*` script. Dot-source from siblings, not invokable on its own. |
| [`fieldtest-smoke.ps1`](fieldtest-smoke.ps1) | ~1 min: gateway --mock × 3 ws-recorder fan-out, analyze raw bytes, emit markdown row. |
| [`fieldtest-replay-roundtrip.ps1`](fieldtest-replay-roundtrip.ps1) | ~15 s: byte-exact compare of replayed `.bin` ↔ ws-recorder `--raw-out`. Guards the byte-pipe promise. |
| [`fieldtest-zip-shape.ps1`](fieldtest-zip-shape.ps1) | Verifies `dist\AMB_RC_Lap_Timer\` shape + size after `build.ps1`. |
| [`fieldtest-usb-pathshift.ps1`](fieldtest-usb-pathshift.ps1) | `subst Z:` then run gateway from the new drive letter; checks `os.Executable()`-relative path resolution. |
| [`fieldtest-soak.ps1`](fieldtest-soak.ps1) | Default 60 min: WorkingSet / handle drift + reconnect count, judged via 5-min head/tail averages. |
| [`fieldtest-runall.ps1`](fieldtest-runall.ps1) | Sequences the four short scripts + soak, prints a markdown summary block ready for `docs\field-test-log.md`. |

## Requirements

- **PowerShell 5.1+** (Windows 8.1 baseline). Scripts deliberately avoid
  `?:` / `??` / `?.` so they run on stock Windows without PowerShell 7.
- **Go 1.20.x** on `PATH` (matches `gateway/go.mod`).
- **Node 20 LTS** + npm if `build.ps1` will be invoked.

## Quick start

```pwsh
# 1. Build once.
.\scripts\build.ps1

# 2. Cheap smoke checks.
.\scripts\fieldtest-zip-shape.ps1 -SkipBuild
.\scripts\fieldtest-smoke.ps1
.\scripts\fieldtest-replay-roundtrip.ps1
.\scripts\fieldtest-usb-pathshift.ps1

# 3. Full pass when you have the time.
.\scripts\fieldtest-runall.ps1                        # ~75 min (60 m soak + ~15 m other)
.\scripts\fieldtest-runall.ps1 -SoakDurationMin 10 -SkipBuild  # ~15 min smoke of the harness itself
```

Each script writes its run artifacts to
`dist\fieldtest-runs\<scenario>-<timestamp>\` and prints a single markdown
row to stdout. `fieldtest-runall.ps1` collects those rows into a
3-column table.

## Out of scope

The harness covers what a Windows agent can self-drive. The following
scenarios still need a human / physical hardware and are tracked as
"未実施(人手要)" in `docs\field-test-log.md`:

- iOS Safari `SpeechSynthesis` initial-tap unlock and lap announcement timing
- Sleep / Wake (lock screen, OS sleep)
- Real WiFi drop → reconnect
- Physical USB unplug / re-plug
- SmartScreen "詳細情報 → 実行" walkthrough on a fresh Windows install
- mDNS `*.local` resolution differences across iOS / Android / Windows

These are exercised at the in-person Field Test α / β sessions described
in [`docs/test-strategy.md`](../docs/test-strategy.md) §6.
