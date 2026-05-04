# Test Strategy

本書は実機 AMB デコーダーが手元にない状態でも開発・回帰検証を回せるテスト戦略を定義する。前提となる構成は `docs/architecture.md`、プロトコル仕様は `docs/protocol-p3.md`、CI 構成は `docs/ci-cd.md` を参照。

> Status: **Draft v0.1**

---

## 1. 方針

### 1.1 ピラミッド
```
                ┌──────────────┐
                │  Manual E2E   │  実機 AMB に繋いだ最終検証
                ├──────────────┤
                │  Integration  │  --mock + WS クライアントの結合
                ├──────────────┤
                │      Unit       │  パーサ / hub / source など
                └──────────────┘
```

- **下が厚く、上は薄く**。本プロジェクトは個人練習用途で、UI を完全自動化する必要はない。
- **プロトコル境界(P3 パーサ)は最厚**にユニットテストを置く。ここの正しさが全ての前提。
- **実機 E2E は手順書化された手動チェックリストで運用**。CI からは切り離す。

### 1.2 価値の優先順位
1. **誤読(parse)を出さない**: ラップタイム算出に直結。最優先で固める。
2. **接続が落ちたら戻る**: ゲートウェイの再接続/WS fan-out が壊れない。
3. **音声読み上げが期待のタイミングで鳴る**: UI 観点。半自動でよい。

---

## 2. フィクスチャ

### 2.1 配置と命名
- **gateway 側**: `gateway/testdata/<name>.bin`(生バイト列、`gateway.exe --record` 出力相当)
- **web 側**: `web/tests/fixtures/<name>.bin`(同形式)
  - 重要なフィクスチャは **両側で同じものを参照**できるよう、原本は `gateway/testdata/` に置き、`web/tests/fixtures/` からシンボリックリンク or ビルド時コピーで取得する(後続 PR で具体化)
- 命名規則: `<scenario>__<note>.bin`
  - 例: `single-passing__healthy.bin`, `double-passing__within-1s.bin`, `truncated-frame__short.bin`, `escape__0x8d-in-value.bin`

### 2.2 期待値の表現
プロトコルのパース結果は **YAML/JSON のサイドカー**で表現する(同名 `.expected.json` を併置)。
- 例: `gateway/testdata/single-passing__healthy.bin` ↔ `gateway/testdata/single-passing__healthy.expected.json`
- 期待値ファイルには TOR 名・抽出された Field の連想配列を記載
- これにより gateway / web 両方のテストから同じ期待値を参照できる

```json
// 期待値の例(イメージ)
{
  "records": [
    {
      "tor": "PASSING",
      "fields": {
        "TRANSPONDER": "0x12345678",
        "RTC_TIME": "0x00...",
        "STRENGTH": 64
      }
    }
  ]
}
```

> v0.1 では JSON で簡易表現とする。整合性チェックで揺れが出たら、TOR ごとに正規化したフォーマットへ移行する(別 Issue)。

### 2.3 フィクスチャの種別
| 種別 | 内容 | 取得方法 |
|------|------|----------|
| 合成(synthetic) | 仕様書から手で組んだ理想ケース、エスケープ境界、CRC 不一致など | テスト内で組み立てる or 小さな Go ツールで生成して `.bin` 化 |
| 録画(captured) | 実機 `--record` で取った生バイト列 | 実機接続フェーズで採取 |
| リプレイ(replay) | 上記をシナリオ化したもの | Mock TCP の入力にも流用 |

初期段階は **合成フィクスチャのみで先行**。実機録画は後続フェーズで `gateway/testdata/captured/` 配下に追加。

---

## 3. Web(TypeScript)側

### 3.1 ツール
- ランナー: **Vitest**
- DOM が必要なケース: `jsdom` 環境
- React のコンポーネントテストは **必要に応じて** `@testing-library/react`(初期は薄く)
- フォーマッタ: **Prettier**
- Lint: **ESLint**(`@typescript-eslint`)

### 3.2 ユニットテスト

#### `protocol/`(P3 パーサ)— 厚く書く
- **フレーミング層**: バイトストリーム → レコード分割
  - 1 フレーム / 連続フレーム / 跨ぎフレーム / 不完全フレーム / `0x8F 0x8E` 連結
- **エスケープ層**: `0x8D` を伴うパターン(`0x8D 0xAE` → `0x8E` 等)、エスケープ未終端
- **ヘッダ層**: little-endian 読み取り(2 バイト値の正常系・エッジ)
- **TLV 層**: Length が `0x10` 以上のケースを必ず入れる(参考実装の 10 進誤読バグを踏まないため)
- **TOR ディスパッチ**: 既知 TOR / 未知 TOR / `ERROR` 応答
- **CRC**: 検証スキップ時の挙動(初期方針)、将来 CRC 検証導入時の差し替えやすさ

→ **テーブル駆動**(配列 `cases: { name, input: bytes, expected: ... }[]`)を基本とし、フィクスチャは §2 のものを再利用。

#### `transport/`(WebSocket)
- 再接続バックオフ計算ロジック(LE 指数 + jitter)
- 切断時のリングバッファクリア / 復元動作
- `MockServer`(`vi.fn()` で `WebSocket` をスタブ)で送受を再現

#### `features/laps/`(ラップ計算)
- 同一トランスポンダーの連続 PASSING → ラップ差分
- ラップ間隔の単位仮定(`docs/protocol-p3.md` §8 オープン項目)を明示し、定数化したものを差し替え可能にする
- 異常: 1 周内に複数回検出された場合の重複除去(閾値設計は別 Issue)

#### `features/speech/`
- `SpeechSynthesis` を `vi.spyOn(window, 'speechSynthesis', ...)` でスタブし、発話キューが期待順に積まれることを検証

### 3.3 コンポーネントテスト
- 当面は **スモーク程度**(描画されるか、設定保存が `localStorage` に書かれるか)に留める。
- ビジュアルリグレッションは導入しない(個人開発で運用負担に見合わない)。

### 3.4 統合テスト
- **WS 受信 → P3 パース → ラップ表示** の通しテストを 1〜2 本だけ用意。
- フィクスチャを `MockServer` から流して、最終的な DOM 表示まで確認。

---

## 4. Gateway(Go)側

### 4.1 ツール
- 標準 `testing`
- レース検出: `go test -race`(CI で必須)
- `golangci-lint`: 採用可否は別 Issue で。当面 `go vet` + `gofmt -s` のみ強制
- アサーション: 標準のみ。必要時に `github.com/stretchr/testify` を採否(別 Issue)

### 4.2 ユニットテスト
- `internal/source/`: 各ソース(real / mock / replay / record)が同じインタフェースに従い、入出力が決定論的になることを検証
- `internal/upstream/`: TCP クライアントの再接続バックオフ、エラー分岐(`testing/iotest` で I/O 模擬)
- `internal/hub/`: 同時 WS クライアント数 N に対する fan-out 順序、`Close` 後の安全性、レース無し
- `internal/config/`: `config.json` の読み込みと既定値マージ
- `internal/httpsrv/`: ルーティング(`/`, `/ws`, `/healthz`, `/admin`, `/logs`)、`/healthz` のレスポンス JSON 形

### 4.3 統合テスト
- `cmd/gateway` をプロセスとして起動せず、`net/http/httptest` + `net.Listen("tcp", ":0")` でゲートウェイの主要処理を組み立てて起動する
- シナリオ:
  1. ローカルでダミー TCP サーバを立て、フィクスチャ `.bin` を流す
  2. ゲートウェイがそれを `/ws` に fan-out
  3. テスト側 WS クライアントが受信したバイト列がフィクスチャと等しい(byte pipe 性の検証)
- ストリーム途切れからの再接続も同様に再現可能(ダミー TCP サーバ側で `Close` → 再 `Accept`)

### 4.4 ベンチマーク(任意)
- `hub` の fan-out スループット、エスケープ処理(将来 Go 側で何か持つなら)等
- 必須ではない。性能に問題が出てから

---

## 5. 受信ソースの抽象とモード

`docs/architecture.md` §3.3 の起動オプションに対応する受信ソース実装。

| モード      | 入力                          | テストでの用途 |
|-------------|------------------------------|----------------|
| `real`      | 実機 AMB の TCP               | 実機接続検証のみ |
| `mock`      | 内蔵スクリプト(コードベタ書き or `testdata/*.bin` 再生) | gateway 統合テスト、フロント開発 |
| `replay`    | `--replay <file>` 指定の `.bin` を再生 | 回帰テスト、現場再現 |
| `record`    | 受信した生バイト列をファイル保存 | フィクスチャ採取 |

### 5.1 Replay の速度制御
`config.json` の `replay.speed`:

| 値          | 意味 |
|-------------|------|
| `realtime`  | 録画時のタイムスタンプ間隔を再現(原則これ) |
| `fast`      | 一定倍率(例 10x。既定値は実装側で固定) |
| `instant`   | 待たずに全フレームを連続送出(テスト高速化用) |

→ 速度制御のためにフィクスチャはタイムスタンプを保持する必要がある。**`.bin` は生バイト列のみ**とし、タイムスタンプは **同名 `.timing.csv`**(`offset_ms,length_bytes`)に分離する案を初期採用(後続 PR で確定)。
- 録画: `--record` 出力時に `.bin` と `.timing.csv` の両方を吐く
- 再生: `.timing.csv` がなければ `instant` 相当で動作

### 5.2 Mock シナリオ
- 初期は **コード内に小さな PASSING 連発シナリオ**を 1 つ持つ(1.5 秒間隔で乱数強度の PASSING を流す)
- 余裕が出たら、シナリオを `testdata/scenarios/<name>.yaml` で記述してロードできるようにする(別 Issue)

### 5.3 Record の運用
- ファイル命名: `record__YYYYMMDD_HHMMSS__<note>.bin`
- 個人情報・トランスポンダー番号など機微なものは含まれないと想定するが、コミット前に確認する文化を `docs/development-workflow.md` に追記候補(別 Issue 化検討)

---

## 6. 実機 Manual E2E チェックリスト

実機接続フェーズで使う最小チェックリスト(`docs/test-strategy.md` のセクションとして維持し、実機検証で更新)。

- [ ] ゲートウェイ起動 → `/healthz` が `upstream:"connected"` を返す
- [ ] スマホブラウザで UI が表示される(LAN IP 直打ち)
- [ ] 自分のトランスポンダーで通過 → 1 秒以内に画面更新と読み上げ
- [ ] 通信切断(LAN ケーブル抜き)→ 数秒で自動再接続、UI に状態表示
- [ ] スマホスリープ復帰 → WS 再接続、表示復元
- [ ] 30 分連続走行 → メモリ・接続クライアント数が漸増しない
- [ ] 別端末(タブレット同時接続)でも同じ表示

---

## 7. 静的解析・フォーマット

| 対象 | ツール | 強制度 |
|------|--------|--------|
| Go   | `gofmt -s -d`(差分が出たら fail) | 必須 |
| Go   | `go vet ./...`                    | 必須 |
| Go   | `golangci-lint`                    | 採用検討(別 Issue) |
| TS   | `tsc --noEmit`                    | 必須 |
| TS   | `eslint .`                        | 必須 |
| TS   | `prettier --check`                | 必須 |
| MD   | リンク切れチェック                | 任意 |

これらは `docs/ci-cd.md` の `web` / `gateway` ジョブで実行する。

---

## 8. カバレッジとフレーキー

### 8.1 カバレッジ
- 当面 **強制しない**(個人開発の現実性優先)
- 計測は任意(ローカルで `go test -cover` / Vitest の `--coverage`)
- 将来、以下に到達したら方針見直し: gateway が複数モジュールに肥大化、web が 5 ルート以上

### 8.2 フレーキーテスト
- **再試行で誤魔化さない**。フレーキーが出たら即修正 or 一時 skip + Issue 起票。
- ネットワーク I/O を含むテストは `localhost` の自由ポート(`:0`)で行い、固定ポート禁止。
- タイマーが絡むテストは可能な限り **時間注入**(`clock` を引数に取る等)で決定論化。

---

## 9. テストの書き方ガイド(短く)

### 9.1 Go
- テーブル駆動を基本(`tests := []struct{ name string; ... }`)
- アサート関数は薄く自前で(初期は `t.Errorf` 直接で十分)
- ヘルパは `t.Helper()` を必ず付ける
- ファイル名は `*_test.go`、整合性検査は `func TestXxx_Yyy`

### 9.2 TypeScript
- `describe / it` でグルーピング(深い入れ子はしない)
- 1 ケース 1 アサーションを原則(複数アサートが必要な場合は分割)
- 共通フィクスチャは `web/tests/fixtures/` に置き、`vi.mock` での偽物は最小限

---

## 10. オープン項目

| # | 項目 | 解消タイミング |
|---|------|----------------|
| 1 | フィクスチャ期待値 JSON のスキーマ確定 | パーサ実装 PR |
| 2 | `gateway/testdata/` と `web/tests/fixtures/` の同期手段(symlink / コピー / ビルド) | 骨格 PR |
| 3 | `--replay` のタイミングデータ(`.timing.csv`)形式の確定 | 骨格 PR |
| 4 | `golangci-lint` / `testify` 採否 | 骨格 PR |
| 5 | Mock シナリオの YAML 化 | 機能拡張時 |
| 6 | 録画フィクスチャの取り扱い(コミット可否、サイズ閾値) | 実機接続後 |
| 7 | E2E 用にヘッドレスブラウザを導入するか(Playwright 等)。当面は不要 | 後日再検討 |

---

## 11. 改訂履歴
- v0.1 (2026-05-04): 初版。実装着手前のテスト方針合意。
