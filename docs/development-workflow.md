# Development Workflow

本リポジトリは「個人練習用ラップタイマー」のソロ〜少人数開発を想定しつつ、後から開発経緯を辿れるよう **教科書的な Issue 駆動開発** を採る。

> 目的: 設計判断・気づき・未解決事項を必ず履歴として残し、半年後の自分が再構築できる状態を保つ。

## 1. 基本ルール

1. **設計が先、実装は後**。新機能・大きな変更は、必ず該当する `docs/*.md` の更新を含める(または新設する)。
2. **Issue → PR で全変更を記録**。会話だけで決めない。決定事項は Issue/PR 本文かコミットメッセージのいずれかに残す。
3. **1 PR = 1 トピック**。設計書の更新もコードと同じくレビュー対象。
4. **コードと仕様の同期**を強制する。仕様変更は同 PR に `docs/` の更新を必ず含める。
5. 実装中・デバッグ中に出た「未決事項」「将来課題」「良いアイデア」は、その場で **新規 Issue を起票**して PR からは外す(PR を肥大化させない)。

## 2. ブランチ命名

| プレフィクス | 用途 |
|---|---|
| `docs/<topic>` | 仕様書・README・運用ドキュメントのみの変更 |
| `feat/<topic>` | 機能追加 |
| `fix/<topic>` | バグ修正 |
| `chore/<topic>` | ビルド・CI・依存更新など、機能に直接関与しない変更 |
| `refactor/<topic>` | 振る舞いを変えない内部変更 |
| `test/<topic>` | テストのみ追加/修正 |

`<topic>` は半角英小文字+ハイフンの短い名前(例: `protocol-p3`, `gateway-skeleton`)。

## 3. Issue 運用

### 起票タイミング
- 新機能・新しい設計検討 → `feature` テンプレート
- バグ・想定外の挙動 → `bug` テンプレート
- アイデア・将来検討事項・運用改善 → `idea` テンプレート
- 実装中に出てきた Out-of-scope な気づき → 即起票して現 PR の対象から外す

### ラベル方針(最小限)
- `type:feature` / `type:bug` / `type:idea` / `type:docs` / `type:chore`
- `area:gateway` / `area:web` / `area:protocol` / `area:ops` / `area:ci`
- `priority:P1` / `priority:P2` / `priority:P3`(必要時のみ)
- `good-first` / `blocked` / `needs-design`(必要時のみ)

### Issue タイトル
- 動詞始まりで具体的に。`gateway: WS fan-out 切断時のリーク疑い` のように `area:` プレフィクス推奨。

## 4. Pull Request 運用

### 必須事項
- PR タイトルは Conventional Commits 互換で書く(`docs: protocol-p3 を追加` など)。
- PR 本文に **対応 Issue を必ず参照**(`Closes #N` で自動クローズ、`Refs #N` で関連付け)。
- レビュー観点(自分でチェックリストを書く)を `## Test plan` か `## Self review` に列挙。
- 仕様書の更新を含む場合、**変更点のサマリを Summary に記載**。
- スクリーンショット/録画があれば添付(UI 変更時)。

### マージ方針
- 履歴を読みやすく保つため、原則 **squash merge** ではなく **通常マージ(merge commit あり)** とする。設計書の段階的合意がコミット粒度で残せるため。
- ブランチは PR マージ後に削除。

### CI が無い間の代替
- CI 整備までは、PR 本文の `## Self review` に「ビルド/テスト/フォーマットを手元で確認した」旨を記載してマージ判断する。

## 5. コミットメッセージ

[Conventional Commits](https://www.conventionalcommits.org/) を採用。

```
<type>(<scope>): <subject>

<body>

<footer>
```

- `type`: `feat` / `fix` / `docs` / `chore` / `refactor` / `test` / `build` / `ci`
- `scope`(任意): `gateway` / `web` / `protocol` / `ci` / `docs` など
- `subject`: 命令形・現在形・末尾ピリオドなし
- `footer`: `Refs #N` / `Closes #N`、破壊的変更は `BREAKING CHANGE:` で開始

例:
```
docs(protocol): P3 フレーム/エスケープ仕様の初版を追加

参考実装 AmbP3/decoder.py を読解し、SOR/EOR、エスケープ規則、
CRC16、ヘッダ、TLV ボディの仕様を整理。TS パーサ実装の根拠とする。

Refs #3
```

## 6. ドキュメント配置

| パス | 内容 |
|---|---|
| `README.md` | プロジェクト概要・目的・使い方の入口 |
| `docs/development-workflow.md` | 本書。開発運用規約 |
| `docs/architecture.md` | リポジトリ構成・配信トポロジ・ビルドフロー |
| `docs/gateway-technical-decision.md` | ゲートウェイ技術選定の決定書(ADR的) |
| `docs/protocol-p3.md` | AMB P3 プロトコル仕様(TS パーサの根拠) |
| `docs/ci-cd.md` | GitHub Actions の構成と運用 |
| `docs/test-strategy.md` | Mock/Replay/Record の運用とテスト方針 |
| `docs/decisions/` (将来) | ADR を増やす場合の置き場 |

## 7. ワークフロー手順(典型例)

1. `main` を最新化: `git fetch origin && git checkout main && git pull`
2. ブランチ作成: `git checkout -b feat/gateway-skeleton`
3. (必要に応じて)対応する Issue を起票し番号を控える
4. 設計書を先に更新 → コードを書く
5. コミット(Conventional Commits)
6. プッシュ: `git push -u origin <branch>`
7. PR 作成(テンプレートに沿って Summary / Test plan / 関連 Issue を記入)
8. セルフレビュー → マージ → ブランチ削除
9. 派生した気づきは新規 Issue で外出し

## 8. マルチエージェント運用(Web 版 / ローカル Windows 版)

本プロジェクトは **2 つの Claude Code セッション** が連携して進める。両者は直接対話できないため、**すべての連携は Issue / PR / コミットコメントを介して行う**。

### 8.1 役割分担

| 区分 | 役割 |
|---|---|
| **Web 版 Claude Code(統制)** | 仕様書ドラフト・改訂、Issue 起票・スコープ設計、ライブラリ比較・選定提案、PR レビュー、設計の整合性維持 |
| **ローカル Windows 版 Claude Code(実装/検証)** | 実装、Windows 上での `go build` / `go test -race`、`npm ci` → `npm run build` → `vitest`、ゲートウェイ EXE の起動確認、`/healthz` 疎通、実機 AMB 接続テスト、`--record` でのフレーム採取 |
| **人間(リポジトリオーナー)** | 方針決定の最終判断、**PR マージの最終承認**、リリース判断、現地での実機/会場運用 |

### 8.2 ハンドオフ・プロトコル

```
[Web 版]                        [ローカル Windows 版]              [人間]
   │
   ├ Issue 起票 (詳細仕様)─────► (待機)
   │                              │
   │                              ├ Issue にコメントで開始宣言
   │                              ├ ブランチ作成 → 実装 → push
   │                              ├ PR 作成 (Closes #N)
   │                              ├ Issue/PR にコメントで完了報告
   ├ PR レビュー (差分確認、整合性)◄
   │                                                              │
   ├ 承認コメント ─────────────────────────────────────────────► マージ
```

#### 8.2.1 Web 版 → ローカル版への引き渡し(Issue)
**Issue は「会話履歴ゼロの新規エージェントが拾って完遂できる粒度」で書く。** 必須項目:
1. **背景・目的**: なぜ必要か。関連 docs へのリンク。
2. **影響ファイルパス**: `gateway/internal/hub/` のように具体的に列挙。新規ファイルなら命名案も。
3. **依存関係**: `Depends on #N`(先に必要)、`Blocks #M`(これが終わると進む)。
4. **完了条件チェックリスト**(Acceptance Criteria)。
5. **ローカル検証手順**: 実行コマンドを `pwsh` 等で具体的に記載。CI と重複してもよい(ローカルは強制)。
6. **先に読むべきドキュメント**: 該当 `docs/*.md` を箇条書き。

#### 8.2.2 ローカル版 → Web 版への戻し(PR)
PR は **Issue で定義された完了条件を満たすこと**が前提。本文に必須:
1. `Closes #N`
2. **`## Local verification (Windows)`** 節: 実行したコマンドと結果(緑/赤)
3. 想定外の選択をした場合はその理由(例: Issue で候補 A だったが、現場検証で B を採用、等)
4. 派生した気づき・未解決事項は **新規 Issue を併せて起票**し、その番号を本 PR から `Refs #X` で参照

#### 8.2.3 ブロッカー時の挙動
ローカル版が以下に該当した場合、**作業を止めて Issue にコメントで報告**する。Web 版はそれを受けて Issue を修正/追加 Issue を起票する。
- 仕様書と実装のギャップが大きい(Issue だけでは決められない)
- 環境/ツールの不足(Go 1.20 が入っていない、実機が見つからない 等)
- 完了条件が現実的でない(再検討が必要)

### 8.3 ローカル(Windows)側で必須の検証

Web 版がレビューする前に、ローカル版は以下を **PR 本文の `## Local verification (Windows)` に書く**。

| 種別 | コマンド(例) | 何を保証するか |
|---|---|---|
| Go ビルド | `cd gateway; go build ./...` | Windows で生成物が作れる |
| Go 単体 | `cd gateway; go test -race -count=1 ./...` | レース無し、決定論的に通る |
| Go バイナリ起動 | `go run ./cmd/gateway --mock` + 別シェルで `curl http://localhost:8080/healthz` | 配布形態で動く |
| Web 依存 | `cd web; npm ci` | lock からの再現セットアップが通る |
| Web ビルド | `cd web; npm run build` | dist が生成される |
| Web 型/lint | `cd web; npm run typecheck; npm run lint` | 静的解析が通る |
| Web 単体 | `cd web; npm run test -- --run` | Vitest が通る |
| 結合(必要時) | `web build → gateway/internal/webassets/dist/ コピー → go build → 実 EXE 起動 → ブラウザで /` | 同梱配信が機能する |
| 実機(該当時) | 実機 IP 指定で起動、自分のポンダーで通過、UI 反映と読み上げを確認 | 実環境で機能する |

> CI(Linux)では `go build` / `go test` / `npm` 系の **静的・単体レイヤを並行で回す**。Windows 固有の挙動・実 EXE 起動・実機接続は **CI ではなくローカル側で担保**する。これは `docs/ci-cd.md` §5 の方針と一致する。

### 8.4 環境差異の典型(踏んだら Issue 化)

CI(Linux)で通って Windows でだけ落ちる、またはその逆になりがちな項目:
- **パス区切り**: `/` vs `\`、`filepath.Join` を使う
- **改行コード**: `LF` vs `CRLF`、`.gitattributes` で正規化
- **ファイル権限**: 実行ビットの有無
- **ポート競合**: `:0` で OS 任せにする
- **Windows Defender**: ファイル削除 / 一時ファイル作成のレース
- **mDNS / `.local` 解決**: クライアント OS で挙動が異なる
- **TCP の `Close` 直後の `Accept` タイミング**: テストでスリープを避け、リトライで対応

これらに遭遇したら **`type:bug` で Issue 化** し、影響範囲(`area:gateway` / `area:web` / `area:ops`)をラベル付ける。

### 8.5 セッション間で誤解を避ける書き方
- Issue/PR 本文では **「あなた」「私」の人称を使わない**(両エージェントが読むため)。「実装担当」「レビュー担当」など役割名を使う。
- **暗黙の参照禁止**: 直近の Issue/PR を「直近のもの」「先ほどの」で参照しない。番号(`#42`)で書く。
- **コマンド例は OS を明示**: `pwsh` / `bash` のどちらで動くかを冒頭に書く。

## 9. 例外

- 軽微な誤字修正やフォーマット差分は Issue を起票せず PR 単独で良い(`docs:` / `chore:`)。
- セキュリティに関わる修正は Issue にせず非公開で対応(本プロジェクトは LAN 内専用のためほぼ発生しない想定)。
