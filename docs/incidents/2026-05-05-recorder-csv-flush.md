# Incident: timing.csv 採取失敗 (2026-05-05)

## TL;DR

最初のフィールド試験(2026-05-05 11:35 JST、F: ドライブで実行)で、
`session-2026-05-05bin` には実 AMB の TCP バイト列が **2102 バイト**
記録できたが、対の `session-2026-05-05bin.timing.csv` は **ヘッダ 1 行
のみで採取済みのチャンク行が全て失われた**。

bin だけはあるので「フレーム解析(#2)の素材」としては使えるが、
受信タイミングの cadence を再現できる timing は残っていない。

修正は `fix(recorder): flush timing.csv per write` で取り込み済み
(`gateway/internal/recorder/recorder.go`)。

## タイムライン (JST)

| 時刻 | 出来事 |
|------|------|
| 11:35:57.483 | gateway starting |
| 11:35:57.554 | source: real (TCP) — 192.168.1.21:5403 |
| 11:35:57.582 | recording (bin と timing.csv をオープン) |
| 11:35:57.582 | dialing upstream |
| 11:35:57.590 | upstream connected |
| (不明) | ゲートウェイがウィンドウ ✕ ボタンか taskkill で強制終了 |

`logs/gateway.log` に **`shutdown requested` の行が無い** ことから、
プロセスは Ctrl+C 経由ではなく、SIGINT を受けないまま終了したと
判定。具体的にはコンソールウィンドウの ✕ クリック、またはコンソール
を持たないプロセスへの taskkill。

## 根本原因

`recorder.Recorder.Write()` が `csv.Writer.Write()` を呼んだ後に
`Flush()` を呼んでいなかった。`csv.Writer` は内部に bufio バッファ
(既定 4KB)を持ち、`Recorder.Close()` のとき初めて flush される
設計だった。

そのため:

- **bin 側**(`os.File.Write` 直書き) → カーネル page cache に積まれ
  ており、プロセスが SIGKILL されても OS が eventually 書き戻す。
  今回も 2102 バイト残った。
- **timing.csv 側**(`csv.Writer` 経由) → ユーザランドの bufio
  バッファに溜まったまま、`Recorder.Close()` が走らずに失われた。

つまり「2 つの出力先のうち片方だけがバッファされていた」ことが
非対称な失敗の原因。

## 修正内容

コミット: `fix(recorder): flush timing.csv per write` (branch
`fix/recorder-csv-flush`)

- `Recorder.Write()` で 1 行ごとに `r.timingW.Flush()` + `Error()`
  チェックを追加。flush 失敗も `failures` カウンタに反映。
- 回帰テスト `TestRecorder_Write_RowsPersistedBeforeClose`:
  `New()` してから 2 回 Write し、**Close を呼ばずに** 直接ファイルを
  読んでヘッダ + 2 行が永続化されているかを検証。修正前のコードでは
  ヘッダ 1 行しか読めず fail する。

## 失われたデータの扱い

- `dist/AMB_RC_Lap_Timer/records/session-2026-05-05.bin`
  (2102 バイト) — 残っている。フレーム解析の素材として有効。
- `dist/AMB_RC_Lap_Timer/records/session-2026-05-05.bin.timing.csv`
  (ヘッダのみ) — 復元不可能。**信用しないこと。**
- 同ディレクトリに `INCIDENT-2026-05-05.md` を置き、これらのファイル
  を後から触る人(エージェント含む)が timing.csv を有効データだと
  誤認しないようにしてある。

## 教訓・運用上の注意

1. **採取セッションの停止は必ず Ctrl+C**(README.txt §採取セッション
   に既出だが、今回 ✕ クリックで止めてしまったので追記強化を検討)。
2. **片側だけバッファされる出力ペアは要注意。** 以後 recorder で
   ファイルを 2 本以上書く場合、フラッシュ戦略を最初に決めること。
3. **採取が成功している証拠は事後確認だけでなくセッション中にも欲しい。**
   `/healthz` または WebSocket fan-out (#3) が無いと、現場で「録れて
   いる」を確認する手段が `timing.csv` の tail しかない。本インシデントは
   その tail が空のまま気づけなかった。

## 関連

- `gateway/internal/recorder/recorder.go` — 修正本体
- `gateway/internal/recorder/recorder_test.go` — 回帰テスト
- `dist/AMB_RC_Lap_Timer/records/INCIDENT-2026-05-05.md` — 採取物への注意書き
- `docs/architecture.md` §4.4.3 (Fail-soft I/O) — 関連方針
- `docs/roadmap.md` §3 #3 — `/healthz` / WS fan-out が来れば再発時に
  早期検知できる
