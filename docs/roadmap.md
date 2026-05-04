# Roadmap

実装フェーズの順序と各フェーズの責務を中央集約する。**「実機 LAN 環境での実データ採取を、フロントエンド/パーサ実装より前に行う」** 方針(採取先行)を採用しているため、フェーズ番号は通例の「ゲートウェイ → SPA → パーサ」順ではないことに注意。

> Status: **Draft v0.1.1**(実装フェーズ #1 完了、★ 採取セッション待ち)

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
  ✅ #1  gateway-recorder MVP                                               (PR #39 merged)
  ⌛ ★   実 LAN 現地データ採取セッション                                       ← 次のアクション
  ⏳ #2  P3 パーサ TS 実装
  ⏳ #3  gateway-full(WS fan-out + go:embed + /healthz + /admin)
  ⏳ #4  SPA 骨格
  ⏳ #5  ラップ計算と表示
  ⏳ #6  音声読み上げ
  ⏳ ★   Field Test α(Smoke + Multi-client)
  ⏳ #7  replay モード
  ⏳ #8  設定 WebUI
  ⏳ ★   Field Test β(Sleep/Wake + WiFi drop + Soak 1h)
  ⏳ #9  リリース自動化(v0.1.0)

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

### #2 P3 パーサ TS 実装
**目的**: 採取データを fixture にして、ブラウザ側 P3 パーサを実装する。

**スコープ**:
- `web/` モジュール初期化(Vite + TS、まだ UI は無くて良い)
- `web/src/protocol/`: フレーミング層、エスケープ層、ヘッダ層、TLV 層、TOR ディスパッチ
- Vitest によるテーブル駆動ユニットテスト
  - 合成フィクスチャ(エッジケース)
  - **採取フィクスチャ**(`gateway/testdata/captured/`)を `.expected.json` と組で参照

**成果物**:
- `web/src/protocol/` のパーサ実装一式
- `.expected.json` スキーマ確定(`docs/test-strategy.md` §2.2 の暫定形を正式化)
- `ci.yml` に `web` ジョブ追加(typecheck + Vitest)

**先に読むべき docs**: `protocol-p3.md` 全文 / `test-strategy.md` §3.2

---

### #3 gateway-full(WS fan-out + go:embed + /healthz + /admin)
**目的**: ゲートウェイを「ブラウザに配信できる」フル装備にする。

**スコープ**:
- `internal/hub/`: WebSocket fan-out(複数クライアント、バックプレッシャ方針は別 Issue で確定)
- `internal/httpsrv/`: ルーティング(`/`, `/assets/*`, `/ws`, `/admin`, `/healthz`, `/logs`)
- `internal/webassets/`: `go:embed` の口
- `--replay <file>` モード(`.timing.csv` のタイミングを再生)

**成果物**:
- 単一 EXE で SPA も配信可能な状態
- `/healthz` が JSON で `upstream` / `clients` / `uptime_sec` を返す
- WS バックプレッシャ方針の確定(別 Issue から決定 → 実装)

---

### #4 SPA 骨格
**目的**: ブラウザ側のアプリケーション基盤を整える。

**スコープ**:
- React + TS + Tailwind(暫定。R6 で確定)
- `web/src/transport/`: WebSocket クライアント、再接続バックオフ
- レイアウト最低限(ヘッダ/状態表示/設定リンク)
- Vite 開発サーバ + ゲートウェイ間の WS プロキシ

---

### #5 ラップ計算と表示
- パーサ → PASSING 抽出 → 同一ポンダーの `RTC_TIME` 差分でラップ算出
- ポンダー別ラップ履歴の表示

### #6 音声読み上げ
- Web Speech API (`SpeechSynthesis`) 統合
- iOS Safari のユーザー操作要件への対応(別 Issue)

### ★ Field Test α(Smoke + Multi-client)
- `docs/test-strategy.md` の Field Test 章を参照(R5 で追加)
- 30 分の現地確認で重大バグを早期検出

### #7 replay モード(正式仕様化)
- `--replay` のタイミング制御(realtime / fast / instant)を仕様確定
- 採取データを使った回帰テスト基盤

### #8 設定 WebUI(`/admin`)
- サーバ側設定(`config.json`)の編集 UI
- クライアント側設定(`localStorage`)とは厳格に分離(`docs/architecture.md` §3.5)

### ★ Field Test β(Sleep/Wake + WiFi drop + Soak 1h)
- 長時間運用に近い検証

### #9 リリース自動化(v0.1.0)
- `release.yml` の整備、ZIP 配布(R7 で USB ポータブル前提を反映)

---

## 4. 派生 Issue(R6 で起票済み)

各フェーズで決定が必要な技術選択は GitHub Issue として起票済み。Issue 番号は対応するフェーズで参照する。

| 議題 | Issue | 解消フェーズ | 暫定方針 |
|---|---|---|---|
| WebSocket ライブラリ(Go) | [#33](https://github.com/hama-jp/AMB_RC_Lap_Timer/issues/33) | #3 | `nhooyr.io/websocket` 候補 |
| ~~ロガー(Go 1.20、`slog` 不可)~~ | ✅ [#34](https://github.com/hama-jp/AMB_RC_Lap_Timer/issues/34) | **#1 で確定** | `uber-go/zap` + `lumberjack.v2`(PR #39) |
| WS バックプレッシャ方針 | [#27](https://github.com/hama-jp/AMB_RC_Lap_Timer/issues/27) | #3 | 古いフレーム破棄 + 警告ログ(暫定) |
| UI フレームワーク | [#32](https://github.com/hama-jp/AMB_RC_Lap_Timer/issues/32) | #4 | React + TS + Tailwind(暫定) |
| iOS Safari Speech のユーザー操作要件 | [#26](https://github.com/hama-jp/AMB_RC_Lap_Timer/issues/26) | #6 | 起動画面で「読み上げを許可」ボタン |
| 上流接続状態の UI 表現 | [#28](https://github.com/hama-jp/AMB_RC_Lap_Timer/issues/28) | #3-#5 | WS テキストフレームで通知 + バナー |
| WS クライアント再接続 UX | [#29](https://github.com/hama-jp/AMB_RC_Lap_Timer/issues/29) | #4 | 指数バックオフ + バナー表示 |
| クライアント許容数の上限 | [#31](https://github.com/hama-jp/AMB_RC_Lap_Timer/issues/31) | #3 | 10 を目安、100 でセーフティ切断 |
| 時刻同期(`GET_TIME` 利用)の取り扱い | [#30](https://github.com/hama-jp/AMB_RC_Lap_Timer/issues/30) | 採取後 | 必要性を採取データで判断 |
| `tools/fieldtest/*` の実装 | [#35](https://github.com/hama-jp/AMB_RC_Lap_Timer/issues/35) | Field Test α 前 | tcp-emitter / ws-recorder / soak-monitor |
| `docs/field-test-log.md` フォーマット | [#36](https://github.com/hama-jp/AMB_RC_Lap_Timer/issues/36) | Field Test α 前 | フォーマット骨格 |
| `packaging/README.txt` ひな型 | [#37](https://github.com/hama-jp/AMB_RC_Lap_Timer/issues/37) | リリース PR 前 | 現地ガイド |

PR #39 のレビューで派生した追従 Issue:
| 議題 | Issue | 解消フェーズ | 備考 |
|---|---|---|---|
| `real.Source` Close 後 Read の契約違反 | [#40](https://github.com/hama-jp/AMB_RC_Lap_Timer/issues/40) | 任意(現状実害なし) | `closed` フラグ導入 |
| `real.Source.buf` 4096 vs 仕様 10240 の表記揺れ | [#41](https://github.com/hama-jp/AMB_RC_Lap_Timer/issues/41) | 任意 | 案 A 推奨(10240 に揃える) |

---

## 5. 改訂履歴
- v0.1.2 (2026-05-04): §2 全体マップに進行状態(✅ / ⌛ / ⏳)を表示。§4 を「R6 で起票済み」に更新し、派生 Issue 全 12 件 + PR #39 追従 Issue 2 件を表に整理。
- v0.1.1 (2026-05-04): §3 #1(gateway-recorder MVP)を ✅ 実装完了に更新。§4 ロガーを `uber-go/zap` + `lumberjack.v2` で確定(#34 クローズ)。
- v0.1 (2026-05-04): 初版。採取先行ロードマップを確定。
