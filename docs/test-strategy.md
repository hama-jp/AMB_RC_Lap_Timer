# Test Strategy

本書は実機 AMB デコーダーが手元にない状態でも開発・回帰検証を回せるテスト戦略を定義する。前提となる構成は `docs/architecture.md`、プロトコル仕様は `docs/protocol-p3.md`、CI 構成は `docs/ci-cd.md` を参照。

> Status: **Draft v0.1.3**

---

## 1. 方針

### 1.1 ピラミッド
```
                ┌────────────────┐
                │   Manual E2E    │  実機 AMB に繋いだ最終検証(会場で)
                ├────────────────┤
                │   Field Test     │  実 LAN(実機なし) — Win/iOS/WiFi/長時間
                ├────────────────┤
                │   Integration    │  --mock + WS クライアントの結合(in-process)
                ├────────────────┤
                │       Unit         │  パーサ / hub / source など
                └────────────────┘
```

- **下が厚く、上は薄く**。本プロジェクトは個人練習用途で、UI を完全自動化する必要はない。
- **プロトコル境界(P3 パーサ)は最厚**にユニットテストを置く。ここの正しさが全ての前提。
- **Field Test** は実 LAN ✕ 実機なしで OS/ブラウザ/ネットワーク差を拾う。**CI には載せず現地で手動実施**(§6 参照)。
- **実機 E2E は手順書化された手動チェックリストで運用**。CI からは切り離す(§7 参照)。

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
- 採取生データ(特に第三者トランスポンダーを含むもの)の取り扱いは §7.4.1 を参照。**生 `.bin` のリポジトリ直接コミットは原則禁止**、匿名化を経由する。

---

## 6. Field Test(実 LAN 検証)

**目的**: Unit / Integration が「Linux + Go/JS 単体」で完結するのに対し、Field Test は **実 Windows ✕ 実 WiFi ✕ 実スマホ ✕ 長時間** の組み合わせでだけ出るバグを拾う。実機 AMB は不要(Mock TCP で代替)。

> **CI には載せない**。実 LAN ハードウェア構成を CI に持ち込むコストに見合わない。**現地で操作員が手動実施**する。`docs/roadmap.md` の ★ Field Test α/β がその実施ポイント。

### 6.1 この層だけが拾えるバグの例
- iOS Safari の WebSocket が画面ロック → 復帰で死ぬ
- iOS Safari `SpeechSynthesis` のユーザー操作要件(初回タップ前は無音)
- mDNS `*.local` 解決の OS 差(Android は弱い)
- Windows Defender Firewall の初回ダイアログで全停止
- Windows のスリープ復帰で TCP / WS が両方再接続できるか
- 1〜2 時間運用時のメモリ / ハンドル / ゴルーチン漏れ
- 同時接続(スマホ + タブレット)で fan-out が崩れない
- WiFi 切断 → 復帰での再接続バックオフの実挙動

### 6.2 シナリオ集

各シナリオは **30 分以内に完了** する粒度を基本(Soak のみ別)。実施日・実施者・観察結果を `docs/field-test-log.md`(将来作成)に追記する。

| シナリオ | 想定時間 | 内容 |
|---|---|---|
| **Smoke** | 5 分 | ゲートウェイ起動 → スマホで UI 開く → Mock の PASSING を受信 → 画面更新 → 音声発話 |
| **Multi-client** | 10 分 | スマホ + タブレットを同時接続。両方で同じ表示と音声が来る。一方だけ切っても他方が継続 |
| **Sleep/Wake** | 5 分 | スマホ画面ロック → 30 秒〜2 分待機 → ロック解除 → WS 自動再接続、表示復元、音声再開 |
| **WiFi drop** | 5 分 | ルータ電源 OFF/ON、または機内モード ON/OFF。ゲートウェイとクライアント両側で自動再接続 |
| **Soak** | 1〜8 時間 | Mock を回し続けてメモリ / ハンドル / 接続クライアント数 / ログサイズの推移を確認。漏れ無し |
| **Firewall fresh** | 5 分 | クリーン Windows での初回起動。Defender Firewall 許可ダイアログ通過、その後接続 |
| **mDNS** | 5 分 | `http://<host>.local/` でアクセス可能か(iOS / Android / Win それぞれ) |
| **USB 起動** | 5 分 | クリーン Windows に USB を挿し、ZIP を展開、`gateway.exe` ダブルクリックで起動できる(SmartScreen → 詳細情報 → 実行 + Firewall 許可) |
| **USB 抜き挿し** | 5 分 | 起動中に USB を抜く → ログ書込みエラー警告のみで停止しない / 上流 TCP・WS fan-out は継続 / 再挿入後も停止しない |
| **FAT32 配置** | 30 分 | FAT32 フォーマットの USB に展開し、ログ・records が `max_size_mb` でローテーションされる(4GB 上限に到達しない) |

### 6.3 α(早期検証)と β(リリース前)の位置づけ

`docs/roadmap.md` で 2 度実施するタイミングを明示している。

| 段階 | 実施タイミング | 必須シナリオ | 任意シナリオ |
|---|---|---|---|
| **Field Test α** | 実装フェーズ #6(音声読み上げ)完了後 | Smoke / Multi-client / Firewall fresh / **USB 起動** | mDNS |
| **Field Test β** | 実装フェーズ #8(設定 WebUI)完了後 | Sleep/Wake / WiFi drop / Soak 1h / **USB 抜き挿し** / **FAT32 配置** | Soak 8h、再 Smoke |

### 6.4 Field Test 用ツール群

`tools/fieldtest/` 配下に置く(実装は別 Issue / 別 PR、必要時に最小から作る):

| パス | 役割 |
|---|---|
| `tools/fieldtest/tcp-emitter/` | Mock TCP サーバ。実 LAN 上の別 PC か、ゲートウェイ PC 自身で起動。リアルな PASSING パターン(1〜3 秒間隔、複数ポンダー)を送出 |
| `tools/fieldtest/ws-recorder/` | ヘッドレスで `/ws` に接続し、受信バイト数 / レイテンシ / 切断回数を CSV/JSON で記録 |
| `tools/fieldtest/soak-monitor.ps1` | ゲートウェイ EXE のメモリ・ハンドル数・接続数を一定間隔で記録 |

> ヘッドレスブラウザ(Playwright 等)は **当面導入しない**。手動チェックで補う(本書 §10 #7)。

---

## 7. 現地データ採取セッション

`docs/roadmap.md` の **「★ 実 LAN 現地データ採取セッション」** に対応する作業手順。実機 AMB が利用可能な現地で、`gateway-recorder` MVP を使って実フレームを採取し、TS パーサ実装(#2)の根拠データとする。

### 7.1 持参物

| 項目 | 内容 | 備考 |
|---|---|---|
| ゲートウェイ EXE 入り USB | 最新 `gateway.exe` + サンプル `config.json` + 空 `records/` ディレクトリ | `docs/architecture.md` のポータブル運用に従う |
| Windows ノート PC | Win 8.1 以上、AMB 同 LAN 接続用 LAN ケーブル | スリープ抑止しておく |
| LAN ケーブル / WiFi 設定 | AMB が刺さっているスイッチに繋ぐ手段 | 現地ネットワーク構成を事前確認 |
| 自分のトランスポンダー | 走行用 | ID をメモしておく |
| ストップウォッチ / 計時アプリ | `RTC_TIME` 単位の同定用 | 別端末の時計でも可 |
| ノートまたはスマホメモ | 観察記録用 | §7.3 のテンプレートを事前に開いておく |

### 7.2 当日の手順

```
1. AMB と同 LAN にノート PC を接続
2. AMB の IP / ポート (5403 既定) を確認
3. config.json を編集または環境変数で指定
4. pwsh で起動:
     gateway.exe --upstream <amb-ip>:5403 --record records\session-<date>.bin
5. ログを目視確認: 「TCP 接続成功」のメッセージが出るまで待つ
6. 自分のトランスポンダーで走行開始(30〜60 分)
   - 各周回でストップウォッチを並行で押し、ラップ秒を記録(後で RTC_TIME と突合)
   - 数周ごとにゲートウェイ側でログを確認、エラー無いか
7. 走行終了後、Ctrl+C でゲートウェイ停止
   - records\session-<date>.bin が flush されて閉じられたことを確認
8. records\ 一式を持ち帰り
```

### 7.3 観察メモのテンプレート

`docs/protocol-p3.md` §9 オープン項目を埋めるための最低限の観察項目。

```markdown
## 採取セッション: <YYYY-MM-DD> @ <場所>

### 環境
- AMB 機種: ___
- AMB IP / Port: ___
- ゲートウェイ PC: Windows ___ / Go 1.20.x で build した gateway.exe v___
- 自分のトランスポンダー番号: ___

### 走行ログ(ストップウォッチ計測)
| 周回 | ストップウォッチ秒 | 備考 |
|------|-------------------|------|
| 1    |                   |      |
| 2    |                   |      |
| ...  |                   |      |

### Frame Length 観察
- xxd 等で session-*.bin を覗き、ヘッダ Frame Length と実フレーム長が一致しているか:
  - 例: SOR=8E, Length=XX YY, ボディ + EOR まで含めた長さと一致するか

### RTC_TIME 単位の推定
- 同一ポンダーの連続 PASSING の RTC_TIME 差分を、ストップウォッチ実測値と比較:
  - 比率: ___ → 単位は ___(候補: μs / ms / 任意)

### HITS / STRENGTH の値域
- 観測された最小値・最大値・典型値:
  - HITS: min=___ max=___ typ=___
  - STRENGTH: min=___ max=___ typ=___

### Flags(ヘッダ / PASSING.FLAGS)の値
- 観測された値の集合: ___
- 文脈と推定意味: ___

### Version レコード(初回接続時)
- DECODER_TYPE / DESCRIPTION / VERSION / RELEASE / BUILD_NUMBER の生バイト:
  ___

### その他気づき
- 接続が落ちた場面 / 再接続できた / Errors:
  ___
```

### 7.4 持ち帰り後の作業

採取セッションが終わったら、まず privacy 面の処理を済ませてからリポジトリへ反映する。

#### 7.4.1 第三者 TRANSPONDER 混入時の匿名化(必須)

テスト中は採取者本人が走行できないため、採取データには **第三者のトランスポンダーが含まれる** ことが通常。AMB トランスポンダー番号は MyLaps アカウントと結びつく個人識別子に近いため、**第三者を含むデータは必ず匿名化してからコミット**する。

- 生 `.bin` は `dist/` 配下 or リポジトリ外のローカル保管のみ。**`.gitignore` で永続的にコミット禁止**(§7.4.2 参照)。
- 匿名化は `gateway/cmd/anonymize` を使い、TRANSPONDER 値を観測順に `0x00000001`, `0x00000002`, ... に再マッピング。それ以外の TLV(`PASSING_NUMBER` / `RTC_TIME` / `STRENGTH` / `HITS` / `FLAGS` / `DECODER_ID` 等)とヘッダ情報は **一切変更しない**。
- 匿名化前→後のマッピング表は **コミットしない**(repo に置かない)。`anonymize` ツールの出力は stdout のみで、ファイル化されないようになっている。クロスセッション相関が必要な場合のみ操作員が手元のローカルメモに保管する。
- byte 構造テスト・filter ロジックテスト・RTC_TIME 単位推定 はすべて anonymized データで成立する。
- CRC は再計算しない(`docs/protocol-p3.md` §6 で「初期実装は検証スキップ」方針、broken CRC は許容)。ヘッダの `FrameLength` も再計算せず、original の値をそのまま残す(escape 解除 / 再構成によりフレームの wire 長は数 byte 変化しうる)。

実行例(Windows):

```pwsh
go run .\gateway\cmd\anonymize `
    -in  "..\dist\AMB_RC_Lap_Timer\records\session-2026-05-05.bin" `
    -out "gateway\testdata\captured\session-2026-05-05.bin"
```

#### 7.4.2 `.gitignore` ルール

生採取データの誤コミットを防ぐため、リポジトリの `.gitignore` に以下を含める:

```
/gateway/testdata/captured/raw/
*-raw.bin
/dist/
```

`gateway/testdata/captured/` 直下の `.bin` は **anonymized 版のみ許可**(レビュー時に手動チェック)。生データを保持したい場合は `gateway/testdata/captured/raw/` か `*-raw.bin` のサフィックスを使う。

#### 7.4.3 配置

匿名化済み `.bin` を `gateway/testdata/captured/session-<YYYY-MM-DD>__<note>.bin` としてコミット。
今回(2026-05-05)のように timing.csv 側が破損しているセッションでは `.timing.csv` はコミットせず、`docs/incidents/` に経緯を残す。

#### 7.4.4 観察メモの追記

§7.3 テンプレートを埋めたものを `docs/captured-sessions/<YYYY-MM-DD>.md` として追加(別 PR でも OK)。**マッピング表は本ファイルにも書かない**。

#### 7.4.5 `docs/protocol-p3.md` §9 の更新

観察結果を別 PR で反映:

- 例: 「Frame Length は SOR を含む全フレーム長(エスケープ後)」「`RTC_TIME` の単位は ms」など。
- 匿名化後のデータでも、Frame Length / RTC_TIME / Header CRC など、TRANSPONDER 値以外の項目はすべて検証可能。

#### 7.4.6 派生 Issue の起票

採取データから新たに見つかった事項(未知 TOR、想定外フィールド ID 等)を Issue 化。

---

## 8. 静的解析・フォーマット

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

## 9. カバレッジとフレーキー

### 9.1 カバレッジ
- 当面 **強制しない**(個人開発の現実性優先)
- 計測は任意(ローカルで `go test -cover` / Vitest の `--coverage`)
- 将来、以下に到達したら方針見直し: gateway が複数モジュールに肥大化、web が 5 ルート以上

### 9.2 フレーキーテスト
- **再試行で誤魔化さない**。フレーキーが出たら即修正 or 一時 skip + Issue 起票。
- ネットワーク I/O を含むテストは `localhost` の自由ポート(`:0`)で行い、固定ポート禁止。
- タイマーが絡むテストは可能な限り **時間注入**(`clock` を引数に取る等)で決定論化。

---

## 10. テストの書き方ガイド(短く)

### 10.1 Go
- テーブル駆動を基本(`tests := []struct{ name string; ... }`)
- アサート関数は薄く自前で(初期は `t.Errorf` 直接で十分)
- ヘルパは `t.Helper()` を必ず付ける
- ファイル名は `*_test.go`、整合性検査は `func TestXxx_Yyy`

### 10.2 TypeScript
- `describe / it` でグルーピング(深い入れ子はしない)
- 1 ケース 1 アサーションを原則(複数アサートが必要な場合は分割)
- 共通フィクスチャは `web/tests/fixtures/` に置き、`vi.mock` での偽物は最小限

---

## 11. オープン項目

> 解消タイミングは `docs/roadmap.md` の対応フェーズに揃える。

| # | 項目 | 解消タイミング |
|---|------|----------------|
| 1 | ~~フィクスチャ期待値 JSON のスキーマ確定~~ → **確定**: `gateway/testdata/captured/<session>.expected.json` で以下の形に固定。BigInt は string で表現。`web` パーサ PR(#2)で `session-2026-05-05.expected.json` を生成・コミット済み。 |
| 2 | `gateway/testdata/` と `web/tests/fixtures/` の同期手段(symlink / コピー / ビルド) | gateway-full PR(#3) |
| 3 | ~~`--replay` のタイミングデータ(`.timing.csv`)形式の確定~~ → **確定**: ヘッダ `offset_ms,length_bytes` の 2 列 CSV、`offset_ms` は接続成功時刻からの経過 ms、1 受信チャンクにつき 1 行(gateway-recorder PR で確定) |
| 4 | ~~`golangci-lint` / `testify` 採否~~ → **不採用**(本 PR 時点)。`gofmt -s` + `go vet` + 標準 `testing` のテーブル駆動で十分。将来テストコードが拡大したら再検討(別 Issue で起票候補) |
| 5 | Mock シナリオの YAML 化 | 機能拡張時 |
| 6 | 録画フィクスチャの取り扱い(コミット可否、サイズ閾値) | 採取セッション後 |
| 7 | E2E 用にヘッドレスブラウザを導入するか(Playwright 等)。当面は不要 | 後日再検討 |
| 8 | `tools/fieldtest/*` の実装(tcp-emitter / ws-recorder / soak-monitor) | Field Test α 実施前 |
| 9 | `docs/field-test-log.md` のフォーマット(実施記録) | 初回 Field Test 直前 |

---

### 11.1 期待値 JSON スキーマ(§11 #1 解消)

```json
{
  "fixture": "gateway/testdata/captured/session-<YYYY-MM-DD>.bin",
  "frameCount": 58,
  "torDistribution": { "0x0001": 15, "0x0002": 42, "0x001C": 1 },
  "passingRecords": [
    {
      "passingNumber": 1148387,
      "transponder": 1,
      "rtcTimeUs": "1777985972473000",
      "strength": 167,
      "hits": 144,
      "flags": 0,
      "decoderId": 269591
    }
  ],
  "statusRecordCount": 42,
  "unknownTors": [{ "tor": "0x001C", "frameIndex": 0 }],
  "malformedCount": 0
}
```

- `rtcTimeUs` は uint64 → BigInt のため、JSON portability のために **string** で表現する。
- `torDistribution` のキーは `0x` + 4 桁 hex(大文字)。
- 生成は `web/scripts/dump-expected.ts`(本リポジトリの `web` パッケージ)で行う。

---

## 12. 改訂履歴
- v0.1.4 (2026-05-05): §11 #1 を解消。`gateway/testdata/captured/<session>.expected.json` のスキーマを §11.1 として正式化。BigInt は string 表現。`web` パーサ PR(#2)で session-2026-05-05.expected.json を生成・コミット。
- v0.1.3 (2026-05-04): §11 #3 / #4 を gateway-recorder PR の確定事項で更新。`.timing.csv` 形式は `offset_ms,length_bytes` 2 列で確定(#3)、`golangci-lint` / `testify` は本 PR 時点では不採用とし、必要時に別 Issue 起票で再検討(#4)。
- v0.1.2 (2026-05-04): §6.2 シナリオに **USB 起動 / USB 抜き挿し / FAT32 配置** を追加。§6.3 α/β の必須シナリオに USB 起動 / 抜き挿し / FAT32 配置を組み込み。`docs/architecture.md` §4.4(ポータブル運用)と整合。
- v0.1.1 (2026-05-04): §1.1 ピラミッドに Field Test 層を追加。§6 Field Test(実 LAN)、§7 現地データ採取セッション手順を新設。§11 にオープン項目 #8 / #9(`tools/fieldtest/`、`field-test-log.md`)を追加。
- v0.1 (2026-05-04): 初版。実装着手前のテスト方針合意。
