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

## 8. 例外

- 軽微な誤字修正やフォーマット差分は Issue を起票せず PR 単独で良い(`docs:` / `chore:`)。
- セキュリティに関わる修正は Issue にせず非公開で対応(本プロジェクトは LAN 内専用のためほぼ発生しない想定)。
