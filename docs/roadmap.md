# Roadmap

実装フェーズの順序と各フェーズの責務を中央集約する。**「実機 LAN 環境での実データ採取を、フロントエンド/パーサ実装より前に行う」** 方針(採取先行)を採用しているため、フェーズ番号は通例の「ゲートウェイ → SPA → パーサ」順ではないことに注意。

> Status: **Draft v0.1.5**(実装フェーズ #1-#8 + α-1 自走分完了、★ Field Test β-1 自宅 dry-run / β-2 現地疎通 を残すのみ)

---

## 1. 設計の前提

採取先行の根拠:

- `docs/protocol-p3.md` §9 のオープン項目(Frame Length 範囲、CRC 計算範囲、`RTC_TIME` 単位、Flags 解釈、HITS/STRENGTH 値域、Version 組合わせ)は **実フレーム 1 セッション分あれば即時確定**するものが多い
- パーサを「想像 + 既知バグ回避」で書くと手戻りが大きい。**実データ駆動**で実装すれば、実フレームを fixture にしたテーブル駆動テストで一気に固められる
- 採取セッション自体が `gateway-recorder MVP` の実 LAN 動作確認を兼ねる(2 度手間にならない)

トレードオフ:
- 実機 AMB へのアクセス機会が必要(ユーザーの走行スケジュールに依存)
- 採取セッション前の `gateway-recorder MVP` を「現地に持って行ける品質」まで仕上げる必要がある

---

## 2. 全体マップ

```
[準備フェーズ] ✅ 完了
  ✅ R0  ハンドオフ・プロトコル(マルチエージェント運用)
  ✅ R1  README 同期
  ✅ R2  protocol-p3 用語明確化
  ✅ R3  architecture 設定境界
  ✅ R4  ロードマップ採取先行へ更新                                          ← 本書
  ✅ R5  test-strategy に Field Test 節 + 現地採取手順
  ✅ R7  ポータブル配布(USB)前提を反映
  ✅ R6  バックログ Issue 起票(#26-#37)

[実装フェーズ]
  ✅ #1  gateway-recorder MVP                                               (PR #39)
  ✅ ★   実 LAN 現地データ採取セッション                                      (2026-05-05 / PR #46 #49)
  ✅ #2  P3 パーサ TS 実装                                                  (PR #51)
  ✅ #3  gateway-full(WS fan-out + go:embed + /healthz + --replay)         (PR #53)
  ✅ #4  SPA 骨格                                                           (PR #60-#64)
  ✅ #5  ラップ計算と表示                                                    (PR #66)
  ✅ #6  音声読み上げ + iOS Safari unlock                                    (PR #68)

[Field Test 準備]                                                        ✅ 完了
  ✅ #36 docs/field-test-log.md 骨格                                         (PR #69)
  ✅ #35 tools/fieldtest/{tcp-emitter, ws-recorder, soak-monitor}            (PR #71)
  ✅ #70 Windows 自走 smoke/soak harness                                     (PR #72)
  ✅ #37 packaging/README.txt + build.ps1 bundle                             (PR #78)
  ✅ #73 ★ Field Test α-1 自走分実行 + docs/field-test-log.md 追記            (PR #75)
  (★ α 人手分は廃止 — β-1 自宅 dry-run に統合、`docs/test-strategy.md` §6.3 v0.1.6)

[実装フェーズ 続き]
  ✅ #7  replay モード(realtime / fast=10x / instant + --replay-speed CLI)  (PR #77)
  ✅ #8  設定 WebUI(/admin)+ 認証 + 監査ログ                              (PR #88 #92 #93 #94)
  ⏳ ★   Field Test β-1(自宅 dry-run、AMB 以外を全部)
  ⏳ ★   Field Test β-2(現地、AMB 疎通確認のみ)
  ⏳ #9  リリース自動化(v0.1.0、ZIP 配布)

[フォローアップ]
  ✅ #40 real.Source Close 後 Read の契約違反                                (PR #80)
  ⏳ #41 real.Source.buf サイズ表記揺れ
  ⏳ #28 上流 TCP 状態 WS テキストフレーム(サーバ側未実装、クライアントは /healthz polling で代替済み)
  ⏳ #47 CRC 再計算(blocked: protocol-p3 §9 #2 確定 + パーサ CRC 検証導入後)

[継続フェーズ]
  実機検証で派生 Issue を消化、機能拡張(将来)
```

---

## 3. 各フェーズの責務

### #1 gateway-recorder MVP  ✅ 実装完了(gateway-recorder PR で実現)
**目的**: 現地で実データを採取できる最小バイナリを用意する。

**スコープ**:
- TCP クライアント: 上流 AMB へ接続、自動再接続(指数バックオフ + jitter)
- `--record <file>`: 受信した生バイト列をファイルへ保存(同名 `.timing.csv` でタイムスタンプ分離)
- `--mock`: オフライン開発用の内蔵 Mock TCP(シナリオは最小限)
- `config.json` 読み込み(`upstream.host` / `port` / `reconnect.*` / `logging.*`)
- ログ出力(ファイル + 標準出力、ローテーション)
- Ctrl+C で record ファイルを安全にフラッシュして終了

**スコープ外**(意図的に小さく):
- WebSocket fan-out
- SPA の同梱配信(`go:embed`)
- `/admin` 設定 WebUI
- `/healthz` ヘルス公開
- `--replay`(録画ファイルから再生する側)

**成果物**:
- `gateway/` Go モジュール(`go.mod` / `cmd/gateway/main.go` / `internal/{config,upstream,source,recorder,logging}`)
- `gateway.exe`(Win amd64、Go 1.20.14 ビルド)
- 最小 `ci.yml`(`gateway` ジョブ)
- 確定: ロガー = `uber-go/zap` + `lumberjack.v2`(#34 クローズ)
- 確定: `.timing.csv` 形式 = `offset_ms,length_bytes` 2 列(test-strategy §11 #3 解消)

**現地持ち込み品質の閾値**:
- [x] `gateway.exe --upstream <ip:port> --record out.bin` で起動
- [x] TCP 接続成功時/切断時/再接続時のログが追える
- [x] 30 分連続で TCP を握り続けてもクラッシュしない(Mock で確認、実機は採取セッション)
- [x] Ctrl+C で `out.bin` がフラッシュされて閉じる
- [x] `out.bin` の冒頭/末尾を `xxd` 等で見てフレーム形に見える(sanity check、Mock で確認)
- [x] `--mock` でローカルでも起動だけ確認できる

**先に読むべき docs**: `gateway-technical-decision.md` / `architecture.md` §2-§3 / `test-strategy.md` §5

---

### ★ 実 LAN 現地データ採取セッション
**目的**: 実機 AMB に接続し、TS パーサ実装の根拠となる実フレームを採取する。

**作業内容**(現地、人間の作業):
1. ゲートウェイ PC を AMB 同 LAN に接続
2. `gateway.exe --upstream <amb-ip>:5403 --record records/session-<date>.bin` で起動
3. 30〜60 分走行(自分のトランスポンダーを使用)
4. Ctrl+C で停止、`records/` 一式を持ち帰り

**成果物**:
- `gateway/testdata/captured/session-<date>.bin` と `.timing.csv`(リポジトリにコミット)
- 観察メモ:
  - Frame Length の値と実フレーム長の対比
  - 同一ポンダー連続 PASSING の `RTC_TIME` 差分(現実のラップ秒との対比)
  - HITS / STRENGTH / Flags の出現値域
  - Version レコードの実値
- これらを反映した `docs/protocol-p3.md` の更新 PR(別 Issue 化)

**先に読むべき docs**: `test-strategy.md`(現地採取手順節、R5 で追加予定)

---

### #2 P3 パーサ TS 実装  ✅(PR #51)
- `web/src/protocol/` 一式(framing / escape / header / TLV / decoder / records)実装済み
- 採取フィクスチャ `gateway/testdata/captured/session-2026-05-05.bin` + `.expected.json` で 15 PASSING を fixture 駆動で検証
- `ci.yml` の `web` ジョブ(typecheck + Vitest + lint + format)稼働中
- `.expected.json` スキーマは `docs/test-strategy.md` §11.1 として正式化済み

---

### #3 gateway-full(WS fan-out + go:embed + /healthz + --replay)  ✅(PR #53)
- `internal/hub/` WS fan-out(ring buffer 64、`max_clients` 100、`ErrTooManyClients` → close 1013)
- `internal/httpsrv/` `/`, `/assets/*`, `/ws`, `/healthz`(`upstream`/`clients`/`uptime_sec`/`version`)
- `internal/webassets/` `go:embed` で SPA 同梱
- `--replay <file>` モード(`.timing.csv` のタイミング再生、無ければ instant)
- WS バックプレッシャ方針 = 古いフレーム破棄 + 警告ログ で確定(#27)
- `/admin` 設定 WebUI は #8 に分離

---

### #4 SPA 骨格
**目的**: ブラウザ側のアプリケーション基盤を整える。

**状態**: ✅ 完了(#55〜#59)。React SPA、WS クライアント、localStorage 設定、状態バナー、PASSING フィルタリスト、`/healthz` version 表示まで結合済み。

**スコープ**:
- React + TS + Tailwind(採用済み: #4-A / #55、状態管理は標準 hooks のみ)
- `web/src/transport/`: WebSocket クライアント、再接続バックオフ
- レイアウト最低限(ヘッダ/状態表示/設定リンク)
- Vite 開発サーバ + ゲートウェイ間の WS プロキシ

---

### #5 ラップ計算と表示
**状態**: ✅ 完了(#65)。対象トランスポンダーの連続 PASSING `RTC_TIME` 差分から Lap 秒を算出し、LapList にミリ秒精度で表示する。

- パーサ → PASSING 抽出 → 同一ポンダーの `RTC_TIME` 差分でラップ算出
- ポンダー別ラップ履歴の表示

### #6 音声読み上げ
**状態**: ✅ 完了(#67)。`PassingEntry.lapTimeUs` を Web Speech API (`SpeechSynthesis`) で発話し、iOS Safari のユーザー操作要件(#26)も unlock overlay で対応する。

- Web Speech API (`SpeechSynthesis`) 統合
- iOS Safari のユーザー操作要件への対応(#26)

### Field Test 準備  ✅(完了)
- ✅ #36 `docs/field-test-log.md` フォーマット骨格(PR #69)
- ✅ #35 `tools/fieldtest/{tcp-emitter, ws-recorder, soak-monitor.ps1}`(PR #71、独立 Go module)
- ✅ #70 `scripts/fieldtest-{smoke,replay-roundtrip,zip-shape,usb-pathshift,soak,runall}.ps1`(PR #72)
- ✅ #37 `packaging/README.txt` + `scripts/build.ps1` の bundle 化(PR #78、操作員向けマニュアルが build 一発で揃う)

### ★ Field Test α
- ✅ **自走分**(#73 / PR #75): `scripts\fieldtest-runall.ps1 -SoakDurationMin 10 -SkipBuild` を Windows エージェントが実行、5 シナリオすべて PASS(fan-out CV=0%、ws_mb +2.3%、handles +1.0%、reconnects=0)
- 人手分は **β-1 に統合**(`docs/test-strategy.md` §6.3 v0.1.6)。会場ネットワーク制約と「自分のノート PC 持ち込み」運用で SmartScreen / Firewall fresh が事実上スキップになり、α / β の必須シナリオの差が縮んだため

### #7 replay モード(正式化)  ✅(PR #77)
- `replay.SpeedFast` を `realtime × 10`(`fastSpeedupFactor=10` 私有定数)で独立実装
- `--replay-speed <realtime|fast|instant>` CLI フラグ追加(precedence: CLI > `config.json:replay.speed` > 既定値 `realtime`)
- 不正値で fatal exit、`--mock` / `--record` / live と同居時は警告のみで起動継続
- 可変倍率や採取データ CI 回帰は別 Issue(必要時)

### #8 設定 WebUI(`/admin`)  ✅(PR #82-/#83-/#84-/#90 統合完了)
- サーバ側 API + 認証(one-time passphrase + HttpOnly cookie + 5 秒 cooldown)+ 監査ログ + フロント UI(HashRouter)を一式実装
- クライアント側設定(localStorage)とは厳格に分離(`docs/architecture.md` §3.5)
- バリデーション / atomic rename / 反映タイミング分類(immediate / next-reconnect / next-start / restart)を Go と TS で同期

### ★ Field Test β-1(自宅 dry-run、AMB 以外)
- 自分のノート PC + 自分のスマホ + 自宅 WiFi で 9 シナリオを潰す
- Soak 1h は `scripts\fieldtest-soak.ps1 -DurationMin 60` で自走可(Windows エージェントに移譲可能)
- 詳細テンプレートは `docs/field-test-log.md` §2.1
- 完了基準: Smoke / Speech / Multi-client / Sleep-Wake / WiFi drop / Soak 1h / `/admin` E2E / USB 起動 / USB 抜き挿し が ✅ または ⚠

### ★ Field Test β-2(現地、AMB 疎通のみ)
- 会場ノート PC を WiFi 経由で AMB と同 LAN に接続
- `ping <AMB IP>` + `gateway --upstream <AMB>` で実フレーム到達確認、自分のトランスポンダーで 1 周走行

### #9 リリース自動化(v0.1.0)
- `release.yml` の整備、ZIP 配布。`packaging/README.txt` + `config.example.json` の bundle は #37 で完了済み(残るは ZIP 化 + GitHub Releases 連携のみ)

---

## 4. 派生 Issue

R6 で起票したバックログ + 各 PR レビューで派生したフォローアップ。**解決済みは打消し線**で残し、何で確定したかを括弧書き。

### 解決済み(クローズ)

| 議題 | Issue | 確定内容 |
|---|---|---|
| ~~ロガー(Go 1.20、`slog` 不可)~~ | ✅ #34 | `uber-go/zap` + `lumberjack.v2`(PR #39) |
| ~~UI フレームワーク~~ | ✅ #32 | React + TS + Tailwind + 標準 hooks(PR #55-) |
| ~~iOS Safari Speech のユーザー操作要件~~ | ✅ #26 / #67 | 起動画面の unlock overlay(PR #68) |
| ~~WebSocket ライブラリ(Go)~~ | ✅ #33 | `nhooyr.io/websocket@v1.8.17`(PR #53) |
| ~~クライアント許容数の上限~~ | ✅ #31 | `max_clients` 100 / 目安 10、`ErrTooManyClients` → close 1013(PR #53) |
| ~~WS バックプレッシャ方針~~ | ✅ #27 | drop-oldest ring buffer 64、`client_buffer_len` 設定可(PR #53) |
| ~~WS クライアント再接続 UX~~ | ✅ #29 | 指数バックオフ + バナー(PR #60-#63) |
| ~~`docs/field-test-log.md` フォーマット~~ | ✅ #36 | 1 セッション 1 セクション、`✅/⚠/❌`(PR #69) |
| ~~`tools/fieldtest/*` の実装~~ | ✅ #35 | tcp-emitter / ws-recorder / soak-monitor(PR #71、独立 Go module) |
| ~~Windows 自走 smoke/soak harness~~ | ✅ #70 | `scripts/fieldtest-*.ps1` 6 本(PR #72) |
| ~~初回 Field Test α-1 自走分~~ | ✅ #73 | 5 シナリオ all PASS(PR #75) |
| ~~`packaging/README.txt` ひな型~~ | ✅ #37 | 現地ガイド + `build.ps1` bundle(PR #78) |
| ~~replay モード正式化~~ | ✅ #76 | `fast = realtime × 10` + `--replay-speed` CLI(PR #77) |
| ~~`real.Source` Close 後 Read 契約~~ | ✅ #40 | watcher goroutine race fix(PR #80) |

### 進行中・未解決

| 議題 | Issue | 解消フェーズ | 備考 |
|---|---|---|---|
| 設定 WebUI(/admin)設計詰め | #79 | #8 着手前 | §1-§6 ユーザー判断中 |
| 上流接続状態の UI 表現(WS テキスト frame) | #28 | #8 と同時または別途 | クライアント側は `/healthz` polling で代替済み(PR #63) |
| 時刻同期(`GET_TIME` 利用)の取り扱い | #30 | 採取後再評価 | 採取データから不要そう(別 PR で判断) |
| CRC 再計算(anonymize) | #47 | パーサ CRC 検証導入後 | blocked(現状 broken CRC 許容) |
| `real.Source.buf` 4096 vs 仕様 10240 | #41 | 任意 | 案 A 推奨(10240 に揃える) |

---

## 5. 改訂履歴
- v0.1.5 (2026-05-06): §2 全体マップ・§3 を更新。#8 admin 系(PR #88 / #92 / #93 / #94)完了を反映、α 人手分を β-1 自宅 dry-run に統合し、β を「β-1 自宅(AMB 以外)」+「β-2 現地(AMB 疎通のみ)」の 2 段階に分割。`docs/test-strategy.md` v0.1.6 と `docs/field-test-log.md` v0.1.2 と整合。
- v0.1.4 (2026-05-06): §2 / §3 を Field Test α-1(#73 / PR #75)、replay 正式化(#7 / PR #77)、packaging README(#37 / PR #78)、real.Source 契約 fix(#40 / PR #80)の完了で更新。§4 解決済みを #70 / #73 / #37 / #76 / #40 まで取り込み、進行中表からは Windows 自走 / α-1 / packaging / Close-after-Read を移動。新規開発の焦点は #8(Issue #79 で設計詰め中)+ Field Test α 人手分(現地)に絞られる。
- v0.1.3 (2026-05-06): §2 全体マップを実装フェーズ #1-#6 + Field Test 準備(#35/#36)完了に更新。§3 #2 / #3 を ✅ + 実装内容に書き換え、Field Test 準備 / α / β / 続フェーズの粒度を分け直す。§4 を「解決済み / 未解決」に分類し直し、PR #51-#72 の実績を反映。
- v0.1.2 (2026-05-04): §2 全体マップに進行状態(✅ / ⌛ / ⏳)を表示。§4 を「R6 で起票済み」に更新し、派生 Issue 全 12 件 + PR #39 追従 Issue 2 件を表に整理。
- v0.1.1 (2026-05-04): §3 #1(gateway-recorder MVP)を ✅ 実装完了に更新。§4 ロガーを `uber-go/zap` + `lumberjack.v2` で確定(#34 クローズ)。
- v0.1 (2026-05-04): 初版。採取先行ロードマップを確定。
