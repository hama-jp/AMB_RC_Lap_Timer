# Architecture

本書はリポジトリ構成、配信トポロジ、ビルドフロー、ローカル開発手順を定義する。実装着手前の合意ドキュメントであり、以降の実装 PR は本書に従う。前提となる設計判断は `docs/gateway-technical-decision.md`、プロトコル仕様は `docs/protocol-p3.md` を参照。

> Status: **Draft v0.1.8**

---

## 1. 全体俯瞰

```
+-----------------+        +-----------------------+        +---------------------+
|  AMB Decoder    |  TCP   |   Gateway (Go EXE)    |  HTTP  |   Browser (SPA)     |
|  (LAN内)        | <----> |  - TCP client         | <----> |  - P3 parser (TS)   |
|  port 5403      |        |  - WebSocket fan-out  |   WS   |  - UI / Voice (TS)  |
+-----------------+        |  - SPA static (embed) |        +---------------------+
                           |  - /admin /healthz    |
                           |  - mock/replay/record |
                           +-----------------------+
                                   |
                                   v
                           +-----------------------+
                           |  config.json / logs/  |
                           +-----------------------+
```

- ゲートウェイは TCP↔WS の **薄い byte pipe** に徹する。プロトコル解釈はブラウザ(TS)で実施。
- 同一プロセス・同一ポートで `/`(SPA)と `/ws` を提供することで Mixed Content 問題を回避。
- 配布物は `gateway.exe` + `config.json` の 2 点のみ。

---

## 2. リポジトリ構成

```
.
├── README.md
├── LICENSE
├── docs/
│   ├── architecture.md                    # 本書
│   ├── development-workflow.md
│   ├── gateway-technical-decision.md
│   ├── protocol-p3.md
│   ├── ci-cd.md                          # (後続 PR で追加)
│   └── test-strategy.md                  # (後続 PR で追加)
├── .github/
│   ├── ISSUE_TEMPLATE/
│   ├── pull_request_template.md
│   └── workflows/                        # (後続 PR で追加)
├── gateway/                              # Go モジュール(別エージェント担当範囲)
│   ├── go.mod                            # module: github.com/hama-jp/AMB_RC_Lap_Timer/gateway
│   ├── go.sum
│   ├── cmd/
│   │   └── gateway/
│   │       └── main.go                   # エントリポイント
│   ├── internal/
│   │   ├── config/                       # config.json 読み書き、デフォルト値
│   │   ├── source/                       # 受信ソース抽象(real / mock / replay / record)
│   │   ├── upstream/                     # AMB へのTCPクライアント、再接続制御
│   │   ├── hub/                          # WS fan-out (broadcast hub)
│   │   ├── httpsrv/                      # HTTPサーバ、ルーティング、/admin /healthz /logs
│   │   ├── webassets/                    # //go:embed の口(配下に dist/ を置く)
│   │   │   └── dist/                     # web ビルド成果物(.gitignore 推奨)
│   │   └── logging/                      # ログとローテーション
│   └── testdata/                         # 録画したフレーム(.bin)等
├── web/                                  # SPA(別エージェント担当範囲)
│   ├── package.json
│   ├── vite.config.ts
│   ├── tsconfig.json
│   ├── index.html
│   ├── src/
│   │   ├── main.tsx
│   │   ├── app/                          # ルーティング/レイアウト
│   │   ├── features/
│   │   │   ├── settings/                 # 対象ポンダー、IP/ポート、音量等
│   │   │   ├── laps/                     # ラップ表示
│   │   │   └── speech/                   # 音声読み上げ
│   │   ├── protocol/                     # P3 パーサ(byte pipe → 構造化イベント)
│   │   ├── transport/                    # WebSocket クライアント、再接続
│   │   └── shared/                       # 共通型/ユーティリティ
│   ├── public/
│   └── tests/
├── scripts/                              # 補助スクリプト(必要時)
└── tools/                                # 録画/再生ツール等(必要時)
```

### 設計の意図
- **`gateway/` と `web/` は同一リポジトリだが、独立した依存ツリーを持つ**。Go 側は `go.mod` を `gateway/` 直下に置き、ルートに置かない。これにより別エージェントが並行作業しやすい。
- **TS の P3 パーサは `web/src/protocol/` に隔離**。アプリ機能(`features/*`)から切り離して、単体テスト容易性とリプレイ容易性を確保する。
- **embed の口を `gateway/internal/webassets/`** に固定。ビルドフローはここに `dist/` を吐き出すだけ。`go:embed` の宣言が場所依存になるため、移動禁止。

---

## 3. ランタイム仕様

### 3.1 HTTP/WS エンドポイント

すべて同一ポート(既定 `8080`)に集約。LAN 内専用、TLS なし。

| パス       | メソッド | 用途 |
|------------|----------|------|
| `/`        | GET      | SPA 配信(`index.html` ほか static) |
| `/assets/*`| GET      | SPA の静的アセット |
| `/ws`      | WS       | デコーダーから受信した素のバイト列を fan-out。**JSON 化はしない**(byte pipe) |
| `/admin`   | GET      | **#3 ではスタブ**(プレースホルダー HTML を返す)。本実装(設定編集 WebUI、`config.json` 反映 + 反映通知)は #8 |
| `/healthz` | GET      | `{"upstream":"connected","clients":2,"uptime_sec":1234,"version":"<v>"}` 形式の JSON。`upstream` は `connecting`/`connected`/`mock`/`replay`/`finished` のいずれか |
| `/logs`    | GET      | 直近の log を ndjson もしくはプレーンで返す(認証なし、LAN 専用前提) |

### 3.2 WebSocket メッセージ
- 方向: gateway → client が大半。client → gateway は将来用途(設定通知など)に予約。
- フレーミング: **WebSocket バイナリフレーム**で素の P3 バイト列を送る。1 メッセージ = 1 P3 レコードとは限らない(複数レコードや断片もありうる)ため、クライアント側でリングバッファリングして `0x8F 0x8E` で区切る(`docs/protocol-p3.md` §2.1)。
- テキストフレームは制御メッセージ用に予約(将来、JSON で `{type:"upstream-down"}` 等を流す可能性)。
- 接続時のハンドシェイクは標準の WebSocket(サブプロトコル指定なし)。

### 3.3 起動オプション

```
gateway.exe [--config <path>] [--mock | --replay <file> | --record <file>] [--listen <addr>]
```

| オプション | 既定値           | 説明 |
|------------|------------------|------|
| `--config` | `./config.json`  | 設定ファイル |
| `--mock`   | (off)            | 受信ソースを内蔵 Mock TCP サーバ相当に切替 |
| `--replay` | (なし)           | `.bin` の録画ファイルを再生(等速/早送り/即時は config 側) |
| `--record` | (なし)           | 受信した生バイト列をファイルへ記録(デバッグ/再生フィクスチャ用) |
| `--listen` | `:8080`          | HTTP/WS の bind |

### 3.4 `config.json` の項目(初期案)

```json
{
  "listen": ":8080",
  "upstream": {
    "host": "192.168.1.21",
    "port": 5403,
    "reconnect": {
      "initial_ms": 1000,
      "max_ms": 30000,
      "jitter_ratio": 0.2
    }
  },
  "logging": {
    "dir": "./logs",
    "max_size_mb": 5,
    "max_backups": 5
  },
  "replay": {
    "speed": "realtime"   // "realtime" | "fast" | "instant"
  },
  "records": {
    "dir": "./records"
  },
  "server": {
    "max_clients": 100,        // safety cap (Issue #31; target ≈ 10)
    "client_buffer_len": 64    // per-client ring depth (Issue #27)
  }
}
```

> **パス解決**: `logging.dir` / `records.dir` などの相対パスは **EXE が置かれているディレクトリ基準**で解決する(cwd 依存にしない)。詳細は §4.4 ポータブル運用を参照。

設定変更は `/admin` 経由が一次手段。`config.json` 直編集は再起動が必要なケースとして許容するが推奨ではない。

### 3.5 設定の責務境界(サーバ側 / クライアント側)

設定は **保管場所と編集経路で 2 層に明確に分ける**。実装担当はこの境界を越えないこと。

```
┌─────────────────────────────────────────┬──────────────────────────────────────────┐
│  サーバ側設定                              │  クライアント側設定                          │
│  (ゲートウェイ EXE が保持)                  │  (ブラウザが保持)                            │
│                                          │                                          │
│  保管: config.json                        │  保管: localStorage                       │
│  編集: /admin の WebUI(一次手段)            │  編集: SPA 内の設定画面                     │
│  共有: 全クライアントに共通                  │  共有: なし。端末ごとに独立                    │
│  寿命: ゲートウェイ再起動を跨ぐ                │  寿命: ブラウザのストレージ保持期間             │
└─────────────────────────────────────────┴──────────────────────────────────────────┘
```

**境界の決定基準**:
1. **誰の関心事か**: 全クライアント共通(=サーバ)か、端末ごと(=クライアント)か
2. **誰がアクセス権を持つべきか**: ネットワーク管理者(=サーバ)か、エンドユーザー(=クライアント)か
3. **どちらに保管されるのが自然か**: 上流接続情報はサーバ、走行者個人の好みはクライアント

#### 3.5.1 振り分け表

| 設定項目 | 配置 | 保管 | 備考 |
|---|---|---|---|
| 上流 AMB の IP / ポート | サーバ | `config.json` (`upstream.host` / `upstream.port`) | 全端末に共通、現場で 1 度設定 |
| 再接続パラメータ | サーバ | `config.json` (`upstream.reconnect.*`) | 運用者が触る項目 |
| HTTP/WS リスニングアドレス | サーバ | `config.json` (`listen`) | 起動時に固定 |
| ログ設定 | サーバ | `config.json` (`logging.*`) | 運用者の関心事 |
| Replay 速度 | サーバ | `config.json` (`replay.speed`) | サーバ側で再生制御 |
| **対象トランスポンダー番号** | クライアント | `localStorage` | **走行者ごと**に違う、端末ローカル |
| 読み上げ ON/OFF | クライアント | `localStorage` | 個人の好み |
| 読み上げ音量 | クライアント | `localStorage` | 端末スピーカに依存 |
| 表示単位(ms/s) | クライアント | `localStorage` | 個人の好み |
| UI テーマ(ライト/ダーク) | クライアント | `localStorage` | 端末/環境光に依存 |
| 直近の表示状態(展開/折り畳み 等) | クライアント | `localStorage` | UI 復元用 |

#### 3.5.2 `/admin` の責務範囲

`/admin` は **サーバ側設定の編集だけ**を扱う。クライアント側設定(対象ポンダー、音量、読み上げ設定)を `/admin` で扱ってはいけない。クライアント側設定は SPA 内の設定画面でのみ編集する。

これにより:
- 同 LAN 内の他人が `/admin` を開いても、**自分のスマホの読み上げや対象ポンダーは書き換えられない**(端末ローカルなので物理的に到達不能)
- ゲートウェイの設定変更で **他端末の表示や音声設定が巻き戻る事故が起きない**

#### 3.5.3 `localStorage` のキー命名規約

衝突回避と可読性のため、すべて以下のプレフィックスを付ける:

```
amb-rc:setting:transponder        → 対象トランスポンダー番号(string)
amb-rc:setting:speech.enabled     → 読み上げ ON/OFF(boolean)
amb-rc:setting:speech.volume      → 読み上げ音量(number, 0.0〜1.0)
amb-rc:setting:display.unit       → 表示単位("ms" | "s")
amb-rc:setting:ui.theme           → UI テーマ("light" | "dark" | "auto")
amb-rc:state:lap.collapsed        → ラップ表示の折り畳み状態(boolean)
```

**`amb-rc:setting:*` は永続的な設定**、**`amb-rc:state:*` はセッション復元用の一時状態**として区別する(後者は将来 `sessionStorage` 移行も検討可)。スキーマの正式版は SPA 骨格 PR で確定する。

---

## 4. ビルドフロー

```
+--------------+     +-----------------+     +----------------+     +-------------+
|  web/ (TS)   | --> | npm run build   | --> | dist/ を       | --> | go build    |
|  Vite + TS   |     | -> dist/        |     | gateway側へ配置 |     | -> EXE 単体 |
+--------------+     +-----------------+     +----------------+     +-------------+
                                                    |
                                                    v
                                  gateway/internal/webassets/dist/
                                  (go:embed の対象)
```

### 4.1 手順(リリース時)
1. `cd web && npm ci && npm run build`
2. ビルド成果物 `web/dist/` を `gateway/internal/webassets/dist/` へコピー(`Makefile` か `scripts/build.ps1` で吸収)
3. `cd gateway && go build -trimpath -ldflags="-s -w -X main.version=${TAG}" -o ../dist/AMB_RC_Lap_Timer/gateway.exe ./cmd/gateway`
4. ZIP 化: `dist/AMB_RC_Lap_Timer/` 配下に `gateway.exe` / `config.example.json` / `README.txt` / 空の `logs/` `records/` を配置 → `AMB_RC_Lap_Timer-vX.Y.Z.zip` にアーカイブ
5. **配布物: `AMB_RC_Lap_Timer-vX.Y.Z.zip` 単一ファイル**(USB に展開して使う、§4.4 参照)

### 4.2 `.gitignore` 方針
- `web/node_modules/` / `web/dist/` をコミットしない
- `gateway/internal/webassets/dist/` は **ビルド成果物のみ配置されるディレクトリ**として、`.gitkeep` だけ追跡し中身は ignore
  - これにより `go:embed` の対象パスは常に存在しつつ、リポジトリは肥大化しない
- `dist/` (リポジトリルート、リリースアーティファクト置き場)も ignore

### 4.3 OS / アーキテクチャ
- 第一ターゲット: **Windows 10/11 amd64**(8.1 はベストエフォート)
- ビルドは Linux/macOS からクロスコンパイル可能(`GOOS=windows GOARCH=amd64 go build ...`)
- 開発時は各自の OS で動作すること(Go と Node があれば動く)

### 4.4 ポータブル運用(USB 配布)

現地 Windows PC に配布する手段は **USB メモリへの ZIP 展開**を一次手段とする。インストール・管理者権限・ネットワーク導入を一切要求しない。

#### 4.4.1 配布物のディレクトリレイアウト

ZIP を展開すると以下の構成になる(USB ルート直下に置く想定):

```
USB:\AMB_RC_Lap_Timer\
├── gateway.exe              # 実行ファイル(単一バイナリ、SPA も同梱)
├── config.json              # 編集して使う設定(初回起動時に config.example.json から複製可)
├── config.example.json      # 設定のひな型
├── README.txt               # 現地手順(起動 / Firewall 許可 / SmartScreen 通過)
├── logs\                    # ローテーション出力先(EXE 起動時に自動作成)
│   └── .gitkeep
└── records\                 # --record の出力先(同上)
    └── .gitkeep
```

#### 4.4.2 パス解決ルール

`config.json` 内の相対パス(`logging.dir` / `records.dir` 等)は **すべて EXE が置かれているディレクトリを基準**に解決する。

```go
exe, _ := os.Executable()
baseDir := filepath.Dir(exe)
// "./logs" → filepath.Join(baseDir, "logs")
```

これにより:
- USB ルート直下でもサブフォルダでも、PC のデスクトップ上に展開しても **同じ挙動**
- cwd 依存しないため、`gateway.exe` をダブルクリックで起動しても期待通りに動作

絶対パス(`C:\foo\bar`)が指定された場合はそのまま使用する。

#### 4.4.3 取り外し耐性(I/O fail-soft)

USB 上で動作中に媒体が抜かれることを想定し、以下の方針で書込み I/O を **fail-soft** にする。

| 操作 | 失敗時の挙動 |
|---|---|
| `logs/*.log` への追記 | ログ書込み失敗を内部カウンタに記録し、標準出力に警告。**ゲートウェイ自体は停止しない** |
| `records/*.bin` への追記(--record 中) | 同上。`--record` セッションは継続(再挿入時に自動で書込み再開する保証はせず、その時点で記録は終了したものと扱う) |
| `config.json` の読込み | 起動時のみ。再読込みは `/admin` 経由でメモリ上の値を更新するに留め、`config.json` 直書込みは原子的(temp ファイル → rename)とする |

メモリ上の状態は維持されるため、**WS クライアントへの fan-out / 上流 TCP 接続は USB 抜き挿しの影響を受けない**。

#### 4.4.4 FAT32 制約への配慮

| 制約 | 対応 |
|---|---|
| 単一ファイル 4GB 上限 | ログ・records ともに **必ずローテーション**(`logging.max_size_mb` で制御。既定 5MB / max_backups 5) |
| パス長 260 文字 | `gateway.exe` の隣接配置(`AMB_RC_Lap_Timer\` 直下に成果物を集約)で対応 |
| 大文字小文字非区別 | パス比較は実装側で case-insensitive 前提 |

#### 4.4.5 SmartScreen / Defender の運用

- 署名なし EXE のため、**初回起動時に Microsoft Defender SmartScreen の警告**が出る。`README.txt` に「**詳細情報** → **実行**」の手順を明記する。
- Defender Antivirus による初回スキャンで起動が遅れる(数秒〜十数秒)。問題ではないが現地では時間に余裕を持たせる。
- コード署名は当面しない方針(別 Issue で議論)。
- Windows Defender Firewall の受信許可ダイアログも初回に出る。`README.txt` で「プライベートネットワークのみ許可」を案内する。

#### 4.4.6 起動時の自動初期化

EXE 起動時に以下を実行する(欠けていても自前で作成):

1. `os.Executable()` で baseDir を取得
2. `baseDir/logs/` / `baseDir/records/` が無ければ作成(失敗したら警告のみ)
3. `baseDir/config.json` が無ければ `baseDir/config.example.json` をコピーして使用(両方無ければデフォルト値で起動)
4. `/healthz` の `paths` フィールドで実際に解決された絶対パスを返す(現地切り分け用)

---

## 5. ローカル開発手順

### 5.1 前提
- **Go 1.20.x**(`docs/gateway-technical-decision.md` でピン留め)
- **Node.js 20 LTS**(初期想定。`web/.nvmrc` で固定する想定 — 別 PR)
- Git

### 5.2 初回セットアップ
```sh
git clone https://github.com/hama-jp/AMB_RC_Lap_Timer.git
cd AMB_RC_Lap_Timer

# web 側
cd web && npm ci && cd ..

# gateway 側
cd gateway && go mod download && cd ..
```

### 5.3 開発ループ(2 プロセス並走)

**ターミナル A — gateway を mock モードで起動**:
```sh
cd gateway
go run ./cmd/gateway --mock --listen :8080
```

**ターミナル B — web の dev server を起動**:
```sh
cd web
npm run dev   # 既定では http://localhost:5173
```

dev server から `:8080/ws` へ接続するため、`vite.config.ts` で **WebSocket プロキシ**を設定する。

```ts
// web/vite.config.ts (抜粋)
export default defineConfig({
  server: {
    proxy: {
      '/ws': { target: 'ws://localhost:8080', ws: true },
      '/healthz': 'http://localhost:8080',
      '/admin':   'http://localhost:8080',
      '/logs':    'http://localhost:8080'
    }
  }
});
```

これにより、ブラウザは `:5173` を見るだけで、WS は実体としてゲートウェイに届く。

### 5.4 統合動作確認(本番に近い構成)
```sh
# web をビルドして gateway 側へ配置
cd web && npm run build && cd ..
mkdir -p gateway/internal/webassets/dist
cp -R web/dist/* gateway/internal/webassets/dist/

# gateway を本番モードで起動
cd gateway
go run ./cmd/gateway --mock --listen :8080
# ブラウザで http://localhost:8080/ を開く
```

---

## 6. 依存とバージョン方針

### 6.1 Go 側の主要依存
- 標準ライブラリ中心(`net/http`, `net`, `embed`)
- WebSocket: `nhooyr.io/websocket` または `gorilla/websocket`(#3 で確定)
- 設定: `encoding/json`(YAML は採用しない、運用簡易さ優先)
- ロガー: **`go.uber.org/zap` + `gopkg.in/natefinch/lumberjack.v2`**(#34 / gateway-recorder PR で確定)
  - Go 1.20 固定で `log/slog`(1.21+)は使えないため
  - zap は Console / JSON エンコーダ両対応で §7 の「人間可読 + 構造化」要件を満たす
  - lumberjack は FAT32 上で安全な原子的 rename によるローテーションを提供し §4.4.4 を直接サポート

### 6.2 Web 側の主要依存
- ビルド: `vite`, `typescript`
- UI: **React + TypeScript** を採用(#4-A / #55)。SPA は同梱配信とブラウザ単体動作を優先し、標準的な React 構成で固定する。
- 状態管理: **React 標準 hooks** のみで開始する。グローバル store は必要性が出るまで導入しない。
- テスト: `vitest` + `@testing-library/react`
- スタイリング: **Tailwind CSS v3** を採用する。

### 6.3 バージョンピン留めの方針
- Go: `gateway/go.mod` に `go 1.20` を明記
- Node: `web/package.json` の `engines` および `web/.nvmrc` で固定
- npm/Go の依存は lock(`package-lock.json`, `go.sum`)を必ずコミット

---

## 7. ログ・観測性

- ログレベル: `error` / `warn` / `info` / `debug`。既定 `info`。
- 出力: ファイル(`logging.dir/gateway.log` を JSON 行で出力、ローテーション)+ 標準出力(Console エンコーダで人間可読)
- 実装: `go.uber.org/zap` + `gopkg.in/natefinch/lumberjack.v2`(§6.1)
- ローテーション: `logging.max_size_mb`(既定 5 MB)を超えたら `gateway.log.YYYY-MM-DDTHH-MM-SS.SSS` 形式の bak に rename。`logging.max_backups`(既定 5)で世代管理
- I/O fail-soft: ログ書込みエラーはプロセスを止めず標準エラーに警告のみ(§4.4.3)
- `/healthz` で簡易ヘルスを公開(#3)。詳細メトリクスは LAN 内専用なので最小限。

---

## 8. セキュリティ方針(再掲)
- **LAN 内専用**。インターネット越しに公開しない。
- TLS / 認証は実装しない(`docs/gateway-technical-decision.md` 第7節)。
- Windows Defender Firewall の受信許可は初回ガイドで案内する。
- `/admin` は LAN 信頼前提でアクセス制御なし。将来必要になったら別 Issue で議論。

---

## 9. オープン項目(後続 Issue で対応)

> 各項目の **解消タイミング**は [`docs/roadmap.md`](roadmap.md) §4 を参照。実機採取で埋まる項目(`docs/protocol-p3.md` §9)は、ロードマップの「★ 実 LAN 現地データ採取セッション」フェーズで解消する。

| # | 項目 | 担当ドキュメント / 担当 |
|---|------|------------------------|
| 1 | 採用する WebSocket ライブラリ確定(nhooyr / gorilla) | gateway-full PR(#3) |
| 2 | ~~フロントの UI フレームワーク確定(React 想定の検証含む)~~ → **React + TypeScript + Tailwind CSS v3 + 標準 hooks を採用**(SPA 骨格 #4-A / #55) |
| 3 | ~~ロガー選定(zap / 自前ラップ / その他)~~ → **`uber-go/zap` を採用**(gateway-recorder PR で確定、#34 クローズ) |
| 4 | ~~`logs/` のローテーション仕様(`lumberjack` 等)~~ → **`gopkg.in/natefinch/lumberjack.v2` を採用**(`max_size_mb=5` / `max_backups=5` 既定、JSON 行で `<dir>/gateway.log` に出力、FAT32 上で原子的 rename ローテーション) |
| 5 | ~~`--replay` の速度切替の細かい仕様~~ → 速度切替自体は replay PR(#7)で確定だが、**`.timing.csv` の形式は本 PR で確定**: ヘッダ `offset_ms,length_bytes` の 2 列 CSV、`offset_ms` は接続成功時刻からの経過 ms、1 受信チャンクにつき 1 行 |
| 6 | CI でのアーティファクト生成 | リリース自動化 PR(#9) / `docs/ci-cd.md` |
| 7 | `web/dist/` を `gateway/internal/webassets/dist/` に同期する手段(`Makefile` / PowerShell スクリプト) | gateway-full PR(#3) |

---

## 10. 改訂履歴
- v0.1.8 (2026-05-06): 音声読み上げ #6 / #67 の完了に合わせ、Web Speech API による Lap 秒発話と iOS Safari 向けユーザー操作 unlock を SPA に追加。
- v0.1.7 (2026-05-06): ラップ計算 #5 / #65 の完了に合わせ、SPA の PASSING 表示に連続 PASSING の `RTC_TIME` 差分から算出する Lap 秒列を追加。
- v0.1.6 (2026-05-06): SPA 骨格 #4-E / #59 の完了に合わせ、Web 側で WS 受信、クライアント設定、状態バナー、PASSING フィルタリスト、`/healthz` version 表示まで結合済みとする。
- v0.1.5 (2026-05-06): §6.2 を SPA 骨格 #4-A の採用判断で更新。React + TypeScript + Tailwind CSS v3 + 標準 hooks を正式採用し、§9 #2 を解消。
- v0.1.4 (2026-05-04): §6.1 / §7 / §9 を gateway-recorder PR の確定事項で更新。ロガーを `uber-go/zap` + `lumberjack.v2` に確定(§9 #3 / #4)、`.timing.csv` 形式を `offset_ms,length_bytes` の 2 列に確定(§9 #5)。
- v0.1.3 (2026-05-04): §4.4 ポータブル運用(USB 配布)を新設。配布物を ZIP に変更、`os.Executable()` ベースのパス解決、I/O fail-soft、FAT32 制約、SmartScreen / Defender 運用、起動時の自動初期化を明文化。`config.json` に `records.dir` を追加。
- v0.1.2 (2026-05-04): §9 オープン項目を採取先行ロードマップ(#1〜#9)の番号体系で参照するよう更新し、`docs/roadmap.md` への入口を追加。
- v0.1.1 (2026-05-04): §3.5 設定の責務境界を新設(サーバ側 / クライアント側の振り分け、`/admin` の責務範囲、`localStorage` キー命名規約)。
- v0.1 (2026-05-04): 初版。実装前の構成合意。
