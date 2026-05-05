# Incident: timing.csv 採取失敗 + Ctrl-C 効かない (2026-05-05)

## TL;DR

最初のフィールド試験(2026-05-05 11:35 JST、F: ドライブで実行)で、
**2 つのバグが連鎖**して以下の状態になった:

1. **Ctrl-C を押してもゲートウェイが止まらなかった** —
   `source/real/real.go` の `conn.Read` がブロッキング syscall で、
   `signal.NotifyContext` 経由の ctx cancellation を見ていなかった。
   走行があるとデータが流れているので一見止まりそうに見えるが、
   readLoop 内で ctx を確認するタイミングが無く、無限に Read が
   返り続けた。
2. **timing.csv が採取できず、ヘッダ 1 行だけになった** —
   `recorder.Recorder.Write()` が `csv.Writer` のバッファを
   write ごとに flush しておらず、(1) で Ctrl-C が効かない結果
   ユーザが taskkill で強制終了する → `Recorder.Close()` が走らない →
   バッファ内のデータ行が全部捨てられる、というルート。

採取された `session-2026-05-05.bin` は **2102 バイト = 58 フレーム**
あり、内訳は **PASSING 15 件(2 トランスポンダー)・STATUS 42 件**
ほか。**走行ログとしては有効**(PASSING_NUMBER 0xE2〜0xF1 連番、欠番なし)。
失われたのは timing.csv の per-chunk タイミングのみ。

修正:
- `fix(recorder): timing.csv を書き込みごとに flush` (本ブランチ最初のコミット)
- `fix(source/real): conn.Read を ctx cancellation で中断する` (本ブランチ後続コミット)

## タイムライン (JST)

| 時刻 | 出来事 |
|------|------|
| 11:35:57.483 | gateway starting |
| 11:35:57.554 | source: real (TCP) — 192.168.1.21:5403 |
| 11:35:57.582 | recording (bin と timing.csv をオープン) |
| 11:35:57.582 | dialing upstream |
| 11:35:57.590 | upstream connected |
| (走行中) | PASSING 15 件 + STATUS 42 件を bin に書き込み(timing.csv はバッファに溜まる) |
| (停止試行) | ユーザが Ctrl-C を押すが効かず |
| (不明) | 最終的に taskkill / 強制終了 → `Recorder.Close()` 不発 |

`logs/gateway.log` に **`shutdown requested` の行が無い** ことから、
プロセスは Ctrl-C 経由の正常停止に至らなかったと確認。
**ユーザの当事者証言: 「コマンドプロンプトで Ctrl-C を押しても止まらなかった」。**

## 根本原因 (1): Ctrl-C が効かない

`gateway/internal/source/real/real.go` の修正前のコード:

```go
n, err := s.conn.Read(s.buf)  // ← ブロッキング syscall。ctx を見ない
```

Go の `signal.NotifyContext(os.Interrupt, ...)` は Ctrl-C で ctx を
cancel するが、`net.Conn.Read` 自体は context を受け付けないため、
読み込み中に ctx が cancel されても気づかない。

挙動:

- **走行ガンガン(データが流れる)** → conn.Read が次々返るので、
  readLoop は毎ループ Read → return out, nil をループ。ctx.Done()
  をチェックするタイミングが無い。Ctrl-C 押下後もループは止まらない。
- **走行なし(silent)** → conn.Read が無期限ブロック。Ctrl-C 押下後も
  syscall が起きないので ctx.Err() を伝える経路がない。

どちらでも結果は同じ:「Ctrl-C で止まらない」。

## 根本原因 (2): timing.csv のバッファ flush 漏れ

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

(1) があるので Ctrl-C で止められず、ユーザは taskkill で強制終了する。
すると `defer rec.Close()` が走らず (2) を踏む、という連鎖。

## 修正内容

### Fix 1: recorder の per-write flush

`gateway/internal/recorder/recorder.go`:

- `Recorder.Write()` で 1 行ごとに `r.timingW.Flush()` + `Error()`
  チェックを追加。flush 失敗も `failures` カウンタに反映。
- 回帰テスト `TestRecorder_Write_RowsPersistedBeforeClose`:
  `New()` してから 2 回 Write し、**Close を呼ばずに** 直接ファイルを
  読んでヘッダ + 2 行が永続化されているかを検証。修正前のコードでは
  ヘッダ 1 行しか読めず fail する。

### Fix 2: source/real の ctx-cancellation ブリッジ

`gateway/internal/source/real/real.go`:

- `Read()` 内で `ctx.Done()` を監視するゴルーチンを起動し、cancel 時に
  `s.conn.Close()` を呼ぶ。これでブロッキング中の `conn.Read` が
  unblock する。
- Read が err で返ったとき `ctx.Err() != nil` ならそれを優先して返す
  ので、main の readLoop は `errors.Is(err, context.Canceled)` で
  通常の shutdown 経路に入る。
- 回帰テスト `TestRead_ContextCanceled_DuringConnRead_ReturnsCtxErr`:
  `blockingConn`(Read が Close まで永久にブロック)を注入し、
  ctx.Cancel から 2 秒以内に Read が ctx.Err() で返ることを検証。
  修正前は時間切れで fail する。

## 失われたデータの扱い

- `dist/AMB_RC_Lap_Timer/records/session-2026-05-05.bin`
  (2102 バイト, 58 フレーム, **PASSING 15 件**) — 残っている。
  フレーム解析(#2)の素材として **そのまま使える**。
- `dist/AMB_RC_Lap_Timer/records/session-2026-05-05.bin.timing.csv`
  (ヘッダのみ) — 復元不可能。**信用しないこと。**
- 同ディレクトリに `INCIDENT-2026-05-05.md` を置き、これらのファイル
  を後から触る人(エージェント含む)が timing.csv を有効データだと
  誤認しないようにしてある。

### bin の中身(参考)

`gateway/cmd/analyze/main.go`(本インシデントで作成した一回もの解析ツール)
の出力より:

```
file size: 2102 bytes
frames found: 58
TOR distribution:
  TOR 0x001c : 1 frames    (undocumented; 接続時の挨拶系と推定)
  TOR 0x0002 : 42 frames   STATUS
  TOR 0x0001 : 15 frames   PASSING ★
```

PASSING の TRANSPONDER フィールドから、**2 種類のトランスポンダー**
(0x0052998D / 0x004AE65E)が検出され、PASSING_NUMBER は連番
0xE2〜0xF1 で欠番なし。RTC_TIME も単調増加。データ整合性 OK。

## 教訓・運用上の注意

1. **シグナル処理を ctx に集約しても、ブロッキング syscall は別途
   ブリッジが必要。** 特に `net.Conn.Read` / `conn.Write`、ファイル
   I/O など。今回は ctx → conn.Close ゴルーチンで対処した。
   以後 source/* に類似コードを書くときは同じ作りにする。
2. **片側だけバッファされる出力ペアは要注意。** 以後 recorder で
   ファイルを 2 本以上書く場合、フラッシュ戦略を最初に決めること。
3. **採取が成功している証拠は事後確認だけでなくセッション中にも欲しい。**
   `/healthz` または WebSocket fan-out (#3) が無いと、現場で「録れて
   いる」を確認する手段が timing.csv の tail しかない。本インシデントは
   その tail が空のまま気づけなかった。

## 関連

- `gateway/internal/recorder/recorder.go` — Fix 1 本体
- `gateway/internal/recorder/recorder_test.go` — Fix 1 回帰テスト
- `gateway/internal/source/real/real.go` — Fix 2 本体
- `gateway/internal/source/real/real_test.go` — Fix 2 回帰テスト
- `gateway/cmd/analyze/main.go` — bin 解析用の一回ものツール
- `dist/AMB_RC_Lap_Timer/records/INCIDENT-2026-05-05.md` — 採取物への注意書き
- `docs/architecture.md` §4.4.3 (Fail-soft I/O) — 関連方針
- `docs/roadmap.md` §3 #3 — `/healthz` / WS fan-out が来れば再発時に
  早期検知できる
