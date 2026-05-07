# Release Runbook

本書は **`git tag vX.Y.Z` を切ってから GitHub Releases ページに ZIP が並ぶまで**の操作員手順をまとめる。CI 自動化(`.github/workflows/release.yml`)が大半を引き受けるので、operator が実行するのは **タグ付け前のチェックリスト**と **公開後の verify**の 2 つだけ。

> Status: **Draft v0.1**(Issue #113 で初版、`v0.1.0` を初回適用案件として運用)

---

## 1. リリースの種類とタグ命名

| 種類 | タグ書式 | release.yml の挙動 |
|---|---|---|
| 正式リリース | `v0.1.0`, `v0.1.1`, `v0.2.0` … | GitHub Release を **stable** で公開 |
| リリース候補 | `v0.1.0-rc1`, `v0.1.0-rc2` … | GitHub Release を **prerelease** で公開 |

- タグは **annotated tag** にする(`git tag -a`)。署名タグは現状未採用。
- `vX.Y.0-rc<n>` だけが pre-release 扱い。`v0.1.1-rc<n>` のようなパッチ rc は将来必要になったら別途運用検討(現状では rc は major.minor.0 のみ)。
- pre-release は β-2 前の **release dry-run** で運用する想定。

---

## 2. タグを切る前(operator チェックリスト)

順番にすべて埋めてから `git tag` を打つ。1 つでも未確認なら止める。

### v0.1.0 のとき(初回正式)
- [ ] `docs/field-test-log.md` の β-1 全 9 シナリオが **✅**(B-1〜B-8 + Soak 1h、PR #102 / #106 / #109 で確定済み)
- [ ] **β-2 現地が ✅** で `docs/field-test-log.md` に追記済み(AMB 疎通確認、`docs/field-test-checklist-beta2.md` 経由)
- [ ] `docs/roadmap.md` Status 行が「v0.1.0 リリース直前」相当
- [ ] `README.md` の起動手順節のバージョン記述が `v0.1.0` を指している(PR を別途出していれば main にマージ済み)
- [ ] `docs/architecture.md` の Status 行が最新
- [ ] CI(`ci.yml`)の最新 main run が ✅

### `v0.1.0-rc<n>` のとき(release dry-run)
- [ ] β-1 全 ✅(β-2 はまだでよい)
- [ ] 直前の hotfix が main にマージ済み
- [ ] CI 緑

### `v0.1.1` 以降(将来)
- [ ] 該当の hotfix / 修正がすべて main にマージ済み
- [ ] CI 緑
- [ ] `docs/field-test-log.md` に「hotfix を当てた状態での 5 分 smoke を実施した」記録(ad-hoc でよい)

---

## 3. タグ付けと push(operator 操作)

```sh
# 直近の main を pull
git checkout main
git pull --ff-only origin main

# annotated tag(メッセージは 1 行で OK、CHANGELOG は GitHub Releases ページが担う)
git tag -a v0.1.0 -m "v0.1.0 — first stable release"

# push
git push origin v0.1.0
```

`release.yml` が tag push を拾って約 2〜3 分でジョブが完走する。

---

## 4. 公開後の verify(operator 操作)

`release.yml` 完了後、GitHub のリポジトリ Releases ページを開いて確認:

- [ ] **タイトル**: タグ名と一致(`v0.1.0`)
- [ ] **本文**: 冒頭にカスタム概要(同梱物 / 動作要件 / 起動方法 / 検証済み環境 / SHA-256)、その下に GitHub 自動生成のコミット履歴
- [ ] **添付ファイル**: 2 件
  - `AMB_RC_Lap_Timer-v0.1.0.zip`(~6 MB)
  - `AMB_RC_Lap_Timer-v0.1.0.zip.sha256`(1 行)
- [ ] **stable / prerelease ラベル**が期待どおり(`v0.1.0` は stable、`v0.1.0-rc1` は prerelease)
- [ ] ZIP をダウンロード → 展開 → `gateway.exe` のプロパティで version 列が `v0.1.0` を指していること(PE バージョンリソースは未対応なので、起動して `--version` での確認でも OK)

---

## 5. release dry-run(rc タグ運用)

β-2 当日に「初めて触る」状態を避けるため、**β-2 の数日前に rc タグで実機リハーサル**する。

```sh
git tag -a v0.1.0-rc1 -m "v0.1.0-rc1 — release dry-run"
git push origin v0.1.0-rc1
```

operator が rc1 ZIP を実機 PC で展開 → `gateway.exe --mock` 起動 → スマホで PASSING 受信を確認。問題があれば hotfix → `rc2` を切り直し。

`v0.1.0-rc<n>` は **GitHub の "Latest" には昇格しない**(prerelease)ので、stable ユーザを誤って導かない。

---

## 6. 公開後フォロー(任意)

- [ ] `README.md` の起動手順節のバージョン記述を本リリース版に同期する PR を出す(タグを切る前に既にやっていれば不要)
- [ ] `docs/field-test-log.md` の β-2 セッションに「v0.1.0 で疎通確認した」と一行追記
- [ ] β-2 で派生 Issue が出ていたら、`v0.1.1` 候補として roadmap に積む

---

## 7. トラブルシュート

| 症状 | 対処 |
|---|---|
| `release.yml` が **trigger されない** | タグが `v*.*.*` の glob にマッチしているか(例: `0.1.0` ではダメ、`v0.1.0`) |
| `softprops/action-gh-release` が **403** で失敗 | ワークフロー上部の `permissions: contents: write` が消えていないか |
| ZIP の中身が **空 / 破損** | ジョブログの `Bundle …` ステップで `cp` が失敗していないか、`web/dist` が存在するか確認 |
| **stable のはずが prerelease** で公開される | タグが `v*.*.0-rc*` 形式に合致していないか(例: `v0.1.1-rc1` は本書の判定では stable 扱い) |

---

## 8. 改訂履歴
- v0.1 (Issue #113 PR): 初版。`v0.1.0` 初回正式リリースを運用想定で書く。
