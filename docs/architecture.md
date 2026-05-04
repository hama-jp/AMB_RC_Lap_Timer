# Architecture

本書はリポジトリ構成、配信トポロジ、ビルドフロー、ローカル開発手順を定義する。実装着手前の合意ドキュメントであり、以降の実装 PR は本書に従う。前提となる設計判断は `docs/gateway-technical-decision.md`、プロトコル仕様は `docs/protocol-p3.md` を参照。

> Status: **Draft v0.1**

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
| `/admin`   | GET/POST | 設定 WebUI(後続)。設定を変更すると `config.json` に反映 + 反映通知 |
| `/healthz` | GET      | `{"upstream":"connected","clients":2,"uptime_sec":1234}` 形式の JSON |
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
    "host": "192.168.10.20",
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
  }
}
```

設定変更は `/admin` 経由が一次手段。`config.json` 直編集は再起動が必要なケースとして許容するが推奨ではない。

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
3. `cd gateway && go build -trimpath -ldflags="-s -w" -o ../dist/gateway.exe ./cmd/gateway`
4. 配布物: `dist/gateway.exe` + サンプル `config.json`

### 4.2 `.gitignore` 方針
- `web/node_modules/` / `web/dist/` をコミットしない
- `gateway/internal/webassets/dist/` は **ビルド成果物のみ配置されるディレクトリ**として、`.gitkeep` だけ追跡し中身は ignore
  - これにより `go:embed` の対象パスは常に存在しつつ、リポジトリは肥大化しない
- `dist/` (リポジトリルート、リリースアーティファクト置き場)も ignore

### 4.3 OS / アーキテクチャ
- 第一ターゲット: **Windows 10/11 amd64**(8.1 はベストエフォート)
- ビルドは Linux/macOS からクロスコンパイル可能(`GOOS=windows GOARCH=amd64 go build ...`)
- 開発時は各自の OS で動作すること(Go と Node があれば動く)

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

### 6.1 Go 側の主要依存(候補)
- 標準ライブラリ中心(`net/http`, `net`, `embed`, `log/slog`)
- WebSocket: `nhooyr.io/websocket` または `gorilla/websocket`(後続 PR で確定)
- 設定: `encoding/json`(YAML は採用しない、運用簡易さ優先)
- ロガー: `log/slog`(Go 1.21+の標準) → **Go 1.20 固定のため `slog` は使えない**。`uber.org/zap` か単純なラップを採用予定(後続 PR で確定)

### 6.2 Web 側の主要依存(候補)
- `vite`, `typescript`
- UI: React + TypeScript(初期想定。Vue/Svelte でも本書の構成は維持可能)
- 状態管理: 軽量に保つ(初期は React の `useState` / Context、必要なら Zustand)
- テスト: `vitest`
- スタイリング: 後続のデザインフェーズで決定(Tailwind 等)

### 6.3 バージョンピン留めの方針
- Go: `gateway/go.mod` に `go 1.20` を明記
- Node: `web/package.json` の `engines` および `web/.nvmrc` で固定
- npm/Go の依存は lock(`package-lock.json`, `go.sum`)を必ずコミット

---

## 7. ログ・観測性

- ログレベル: `error` / `warn` / `info` / `debug`。既定 `info`。
- 出力: ファイル(`logging.dir` 配下、ローテーション)+ 標準出力
- フォーマット: 人間可読 + 重要イベントは構造化(JSON 行)を選択可能(後続 PR で詳細化)
- `/healthz` で簡易ヘルスを公開。詳細メトリクスは LAN 内専用なので最小限。

---

## 8. セキュリティ方針(再掲)
- **LAN 内専用**。インターネット越しに公開しない。
- TLS / 認証は実装しない(`docs/gateway-technical-decision.md` 第7節)。
- Windows Defender Firewall の受信許可は初回ガイドで案内する。
- `/admin` は LAN 信頼前提でアクセス制御なし。将来必要になったら別 Issue で議論。

---

## 9. オープン項目(後続 Issue で対応)

| # | 項目 | 担当ドキュメント / 担当 |
|---|------|------------------------|
| 1 | 採用する WebSocket ライブラリ確定(nhooyr / gorilla) | gateway 骨格 PR |
| 2 | フロントの UI フレームワーク確定(React 想定の検証含む) | web 骨格 PR |
| 3 | ロガー選定(zap / 自前ラップ / その他) | gateway 骨格 PR |
| 4 | `logs/` のローテーション仕様(`lumberjack` 等) | gateway 骨格 PR |
| 5 | `--replay` の速度切替の細かい仕様 | `docs/test-strategy.md` |
| 6 | CI でのアーティファクト生成 | `docs/ci-cd.md` |
| 7 | `web/dist/` を `gateway/internal/webassets/dist/` に同期する手段(`Makefile` / PowerShell スクリプト) | gateway 骨格 PR |

---

## 10. 改訂履歴
- v0.1 (2026-05-04): 初版。実装前の構成合意。
