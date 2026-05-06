# Roadmap

実装フェーズの順序と各フェーズの責務を中央集約する。**「実機 LAN 環境での実データ採取を、フロントエンド/パーサ実装より前に行う」** 方針(採取先行)を採用しているため、フェーズ番号は通例の「ゲートウェイ → SPA → パーサ」順ではないことに注意。

> Status: **Draft v0.1.3**(実装フェーズ #1-#6 + 採取セッション + Field Test 準備(#35/#36)完了、★ Field Test α 自走分実行中)

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

[Field Test 準備]
  ✅ #36 docs/field-test-log.md 骨格                                         (PR #69)
  ✅ #35 tools/fieldtest/{tcp-emitter, ws-recorder, soak-monitor}            (PR #71)
  ⌛ #70 Windows 自走 smoke/soak harness                                     (PR #72 レビュー済み、マージ待ち)
  ⏳ #73 ★ Field Test α-1 自走分実行 + docs/field-test-log.md α-1 セッション追記
  ⏳ ★   Field Test α 人手分(iOS Safari Speech / Sleep-Wake / 物理 USB / SmartScreen / mDNS)現地セッション

[実装フェーズ 続き]
  ⏳ #7  replay モード(タイミング制御 realtime/fast/instant の正式化)
  ⏳ #8  設定 WebUI(/admin)
  ⏳ ★   Field Test β(Sleep/Wake + WiFi drop + Soak 1h、現地)
  ⏳ #9  リリース自動化(v0.1.0、ZIP 配布)

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

### Field Test 準備  ✅(PR #69 / #71、PR #72 マージ待ち)
- ✅ #36 `docs/field-test-log.md` フォーマット骨格(PR #69)
- ✅ #35 `tools/fieldtest/{tcp-emitter, ws-recorder, soak-monitor.ps1}`(PR #71、独立 Go module)
- ⌛ #70 `scripts/fieldtest-{smoke,replay-roundtrip,zip-shape,usb-pathshift,soak,runall}.ps1`(PR #72、Smoke / Replay round-trip / ZIP shape / USB pathshift / Soak の 5 シナリオを Windows 単体で自走実行)

### ★ Field Test α  ⏳(自走分: #73 着手前、人手分: 現地セッション待ち)
- **自走分**(#73): `scripts\fieldtest-runall.ps1` を Windows エージェントが実行 → `docs/field-test-log.md` α-1 セッション追記
- **人手分**(現地): iOS Safari Speech / Sleep-Wake / 物理 USB / SmartScreen / mDNS — `docs/test-strategy.md` §6.1 参照
- α 完了基準: 自走分 PASS + 人手分 4/5 以上 PASS

### #7 replay モード(正式仕様化)
- `--replay` のタイミング制御(realtime / fast / instant)の `config.json` 反映を完成
- 採取データを使った回帰テスト基盤(現状は手元のみ、CI への組み込みは別 Issue)

### #8 設定 WebUI(`/admin`)
- サーバ側設定(`config.json`)の編集 UI
- クライアント側設定(`localStorage`)とは厳格に分離(`docs/architecture.md` §3.5)

### ★ Field Test β(Sleep/Wake + WiFi drop + Soak 1h)
- #8 完了後、現地で実施。`scripts\fieldtest-soak.ps1 -DurationMin 60` をベースに、人手で Sleep/Wake と WiFi drop を投入

### #9 リリース自動化(v0.1.0)
- `release.yml` の整備、ZIP 配布(R7 で USB ポータブル前提を反映)
- `packaging/README.txt`(#37)同梱

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

### 進行中・未解決

| 議題 | Issue | 解消フェーズ | 備考 |
|---|---|---|---|
| Windows 自走 smoke/soak harness | #70 | Field Test α 前 | PR #72 レビュー済み・マージ待ち |
| 初回 Field Test α-1 自走分実行 + docs 追記 | #73 | Field Test α | PR #72 マージ後に Windows エージェント着手 |
| 上流接続状態の UI 表現(WS テキスト frame) | #28 | サーバ側未実装 | クライアント側は `/healthz` polling で代替済み(PR #63) |
| 時刻同期(`GET_TIME` 利用)の取り扱い | #30 | 採取後再評価 | 採取データから不要そう(別 PR で判断) |
| `packaging/README.txt` ひな型 | #37 | #9 リリース前 | 未着手 |
| CRC 再計算(anonymize) | #47 | パーサ CRC 検証導入後 | blocked(現状 broken CRC 許容) |
| `real.Source` Close 後 Read の契約違反 | #40 | 任意 | 現状実害なし |
| `real.Source.buf` 4096 vs 仕様 10240 | #41 | 任意 | 案 A 推奨(10240 に揃える) |

---

## 5. 改訂履歴
- v0.1.3 (2026-05-06): §2 全体マップを実装フェーズ #1-#6 + Field Test 準備(#35/#36)完了に更新。§3 #2 / #3 を ✅ + 実装内容に書き換え、Field Test 準備 / α / β / 続フェーズの粒度を分け直す。§4 を「解決済み / 未解決」に分類し直し、PR #51-#72 の実績を反映。
- v0.1.2 (2026-05-04): §2 全体マップに進行状態(✅ / ⌛ / ⏳)を表示。§4 を「R6 で起票済み」に更新し、派生 Issue 全 12 件 + PR #39 追従 Issue 2 件を表に整理。
- v0.1.1 (2026-05-04): §3 #1(gateway-recorder MVP)を ✅ 実装完了に更新。§4 ロガーを `uber-go/zap` + `lumberjack.v2` で確定(#34 クローズ)。
- v0.1 (2026-05-04): 初版。採取先行ロードマップを確定。
