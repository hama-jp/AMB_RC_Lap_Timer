# CI/CD

本書は GitHub Actions の構成と運用方針を定義する。実体のワークフロー YAML は本書の合意後に別 PR で `.github/workflows/` に追加する。前提となるリポジトリ構成・ビルドフローは `docs/architecture.md`、開発運用ルールは `docs/development-workflow.md` を参照。

> Status: **Draft v0.1.1**

---

## 1. 目的と適用範囲

| 目的 | 内容 |
|------|------|
| 早期検知 | PR 段階で型・lint・テスト・ビルドの破綻を検出 |
| 再現性 | 各環境での挙動差を抑え、リリースアーティファクトを再生成可能にする |
| 配布の自動化 | タグプッシュで `gateway.exe` + サンプル `config.json` を Releases に配置 |

スコープ: 本リポジトリの `web/` と `gateway/`。実機接続を伴う E2E は対象外(別途 `docs/test-strategy.md`)。

---

## 2. ワークフロー一覧と責務

3 本構成。

| ワークフロー | ファイル(予定)              | トリガー | 主な責務 |
|--------------|------------------------------|----------|----------|
| **CI**       | `.github/workflows/ci.yml`   | `push` to `main`, `pull_request` to `main` | web/gateway のチェック+ビルド |
| **Release**  | `.github/workflows/release.yml` | `push` tag `v*.*.*` | 同梱ビルド → アーティファクト化 → Releases 公開 |
| **Lint Docs**(任意) | `.github/workflows/docs.yml` | `pull_request` の `docs/**` 変更時 | Markdown 整形・リンクチェック |

`docs.yml` は無くても進められるため、優先度は最後。

---

## 3. CI ワークフロー(`ci.yml`)

### 3.1 トリガー
```yaml
on:
  push:
    branches: [ main ]
  pull_request:
    branches: [ main ]
```

### 3.2 並行制御
同一ブランチへの連続 push があれば古い実行はキャンセル。
```yaml
concurrency:
  group: ci-${{ github.ref }}
  cancel-in-progress: true
```

### 3.3 ジョブ構成(マトリクスは使わない:OS は ubuntu-latest 統一)

#### `web` ジョブ
- `actions/checkout@<pinned>`
- `actions/setup-node@<pinned>` with `node-version-file: web/.nvmrc`, `cache: npm`, `cache-dependency-path: web/package-lock.json`
- 手順:
  1. `npm ci` (`working-directory: web`)
  2. `npm run lint` (ESLint)
  3. `npm run typecheck` (`tsc --noEmit`)
  4. `npm run test -- --run` (Vitest)
  5. `npm run build` (Vite)
- 成果物: `web/dist/` を **artifact `web-dist`** としてアップロード(Release ワークフローで再利用検討、初期は CI のみで完結でも可)。

#### `gateway` ジョブ
- `actions/checkout@<pinned>`
- `actions/setup-go@<pinned>` with `go-version: '1.20.x'`, `cache: true`, `cache-dependency-path: gateway/go.sum`
- 手順(`working-directory: gateway`):
  1. `go vet ./...`
  2. `go test ./... -race -count=1`
  3. `go build ./cmd/gateway` (生成物の正常性確認のみ。配布物はこのジョブでは作らない)
- `go test` の `-race` は Linux ランナーで有効(Windows ランナーでも動くが速度都合で初期は Linux のみ)。

#### `integration` ジョブ(任意・初期は省略可)
- 上記 2 ジョブの後段で実行。`needs: [web, gateway]`。
- 内容:
  - `web/dist` artifact を取得 → `gateway/internal/webassets/dist/` に配置
  - `cd gateway && go build -o ../bin/gateway ./cmd/gateway`
  - 起動して `/healthz` を `curl` で叩く軽い smoke
- リリース時の構成と同じ手順を CI で 1 度通すことが目的。
- 初期は省略してもよく、Release ワークフローで実質的にカバーされる。

### 3.4 失敗時の挙動
- いずれかのステップが非ゼロ終了で fail。
- `if: always()` を使った後始末ステップは原則使わない(ノイズ抑制)。
- アーティファクトは失敗時もアップロードして良い(`if: always()`)。

---

## 4. Release ワークフロー(`release.yml`)

### 4.1 トリガー
```yaml
on:
  push:
    tags: [ 'v*.*.*' ]
```

### 4.2 ジョブ
1. `actions/checkout@<pinned>`
2. Node セットアップ → `cd web && npm ci && npm run build`
3. ビルド成果物を `gateway/internal/webassets/dist/` に同期(`scripts/build.ps1` か `Makefile` 経由)
4. Go セットアップ(`go-version: '1.20.x'`)
5. クロスビルド(配布物のディレクトリへ直接出力):
   ```
   mkdir -p dist/AMB_RC_Lap_Timer/{logs,records}
   touch dist/AMB_RC_Lap_Timer/logs/.gitkeep dist/AMB_RC_Lap_Timer/records/.gitkeep
   GOOS=windows GOARCH=amd64 go build -trimpath \
     -ldflags="-s -w -X main.version=${TAG}" \
     -o dist/AMB_RC_Lap_Timer/gateway.exe ./cmd/gateway
   cp config.example.json dist/AMB_RC_Lap_Timer/
   cp packaging/README.txt dist/AMB_RC_Lap_Timer/
   ```
6. **ZIP アーカイブ化**(USB ポータブル配布、`docs/architecture.md` §4.4):
   ```
   cd dist
   zip -r AMB_RC_Lap_Timer-${TAG}.zip AMB_RC_Lap_Timer/
   sha256sum AMB_RC_Lap_Timer-${TAG}.zip > AMB_RC_Lap_Timer-${TAG}.zip.sha256
   ```
7. `softprops/action-gh-release@<pinned>` で **`dist/AMB_RC_Lap_Timer-${TAG}.zip` と `.sha256`** を Releases に添付
8. リリースノートはタグ間のコミットから自動生成(`generate_release_notes: true`)

> 配布物は **ZIP 単一ファイル**。USB に展開してそのまま起動する想定(`docs/architecture.md` §4.4 ポータブル運用)。`packaging/README.txt` は SmartScreen / Firewall 通過手順を含む現地ガイド(別 PR で同梱物を準備)。

### 4.3 タグ運用
- セマンティックバージョン: `vMAJOR.MINOR.PATCH`(例 `v0.1.0`)
- アルファ/ベータは `v0.1.0-rc.1` など。**初期は `v0.x.y` で振る**(API 安定前のため)。
- リリースは `main` 上のコミットからタグを切る(`git tag` → `git push origin <tag>`)。

---

## 5. ランナー・バージョンの方針

| 項目 | 値 |
|------|----|
| ランナー | `ubuntu-latest`(リリースの最終 build もクロスコンパイルで ubuntu) |
| Go     | **1.20.x**(`docs/gateway-technical-decision.md` の決定) |
| Node   | **20 LTS**(`web/.nvmrc` で固定) |
| OS マトリクス | 当面なし。Windows ランナーは費用と速度の問題で必要時のみ追加 |

> CI 上で「Win8.1 で動くか」を直接検証することはしない。CI は Linux でビルドが通り単体テストが通ることを保証するに留める。Windows での実機/手動検証は別途。

---

## 6. キャッシュ

- **npm**: `actions/setup-node` の `cache: npm` を利用。`web/package-lock.json` を key に。
- **Go modules**: `actions/setup-go` の `cache: true` を利用。`gateway/go.sum` を key に。
- **Vite/Go ビルドキャッシュ**は CI では基本オフ(再現性優先)。必要なら別途検討。

---

## 7. セキュリティと Actions 利用ルール

- **権限の最小化**:
  ```yaml
  permissions:
    contents: read
  ```
  Release ワークフローのみ `contents: write` を付与(リリース作成のため)。
- **Action のバージョンピン**:
  - 公式の `actions/*` は **メジャータグ参照(例: `@v4`)を許可**。Dependabot で更新を追跡する。
  - サードパーティの Action(例: `softprops/action-gh-release`)は **コミット SHA で固定**。タグは可変なため。
- **シークレット**:
  - 当面、リポジトリのシークレットは使わない(LAN 内専用配布、外部公開やパッケージ署名なし)。
  - 将来コード署名する場合は別 Issue で議論。
- **PR からのフォーク実行**: `pull_request_target` は使わない。`pull_request` のみ。

---

## 8. ステータスバッジ

`README.md` 冒頭に CI バッジを掲示する(本 PR スコープ外)。
```
![CI](https://github.com/hama-jp/AMB_RC_Lap_Timer/actions/workflows/ci.yml/badge.svg)
```
バッジ追加は `ci.yml` 追加の PR と同時、もしくはその直後の小 PR で対応。

---

## 9. 想定タイミングとフェーズ

| フェーズ | CI | Release |
|---------|----|---------|
| 仕様整備中(現在) | 未導入で良い(self review で代替、`docs/development-workflow.md` §4 通り) | 未導入 |
| gateway 骨格 PR 着手後 | `gateway` ジョブだけ先行で導入してもよい | 未導入 |
| web 骨格 PR 着手後 | `web` ジョブを追加 | 未導入 |
| 実機疎通後 | 全 CI 完成 | `release.yml` 導入、`v0.1.0` タグ運用開始 |

→ 本書の合意 → **gateway 骨格 PR**(別 Issue/PR)→ そこに `ci.yml` の最小版を含めるのが現実的な順番。

---

## 10. オープン項目

| # | 項目 | 解消タイミング |
|---|------|----------------|
| 1 | `integration` ジョブを CI に入れるか、Release だけで良いか | gateway 骨格 PR |
| 2 | コードカバレッジ計測を導入するか(初期は不要) | 後日 |
| 3 | `docs.yml` の Markdown lint を導入するか | 任意 |
| 4 | Dependabot 設定(`.github/dependabot.yml`) | CI 安定後 |
| 5 | Windows ランナーでの go test 補強の要否 | 実機接続後 |
| 6 | リリースアーティファクトのチェックサム生成(SHA256) | §4.2 で `.sha256` 同梱を反映済み。署名は別途 |
| 7 | `packaging/README.txt`(現地手順)の同梱物作成 | リリース PR(#9) |
| 8 | コード署名(SmartScreen 警告抑止) | 後日(必要になったら) |

---

## 11. 改訂履歴
- v0.1.1 (2026-05-04): §4.2 Release ジョブを USB ポータブル ZIP 配布(`AMB_RC_Lap_Timer-vX.Y.Z.zip` + `.sha256`)に変更。`docs/architecture.md` §4.4 と整合。オープン項目に `packaging/README.txt` 同梱(#7)とコード署名(#8)を追加。
- v0.1 (2026-05-04): 初版。実装着手前の方針合意。
