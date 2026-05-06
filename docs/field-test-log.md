# Field Test Log

`docs/test-strategy.md` §6 で定義した Field Test(実 LAN ✕ 実機なし)の **実施記録**を残す場所。CI で自動化しないため、ここに書いておかないと「現地で何が起きたか」が後から辿れなくなる。

> Status: **Draft v0.1.1**(α-1 自走分まで実施)

---

## 1. 記録方針

- **1 セッション = 1 セクション**。見出しは `## YYYY-MM-DD <段階>-<連番> (<環境概要>)`。
  - 段階: `α` / `β`(`docs/test-strategy.md` §6.3 に対応)
  - 例: `## 2026-05-10 α-1 (Win 11 / iPhone 15 / WiFi 5GHz)`
- 各シナリオに **`✅` / `⚠` / `❌`** を必ず付ける。中間状態は使わない。
- 観察した事象から **派生 Issue** を立てた場合は番号を必ず記録(`#NN`)。本ファイルから事象 → Issue → PR を辿れるようにする。
- 自動収集ログ(`tools/fieldtest/ws-recorder` の CSV、`soak-monitor.ps1` の CSV など)は **本ファイルには貼らない**。`docs/captured-sessions/<date>/` 等の別ディレクトリに置き、本ファイルからは要約と相対パスのみを参照する。
- 個人識別子(他人のトランスポンダー番号 / WiFi SSID 全文 / 自宅 IP 等)は書かない。`docs/test-strategy.md` §7.4 と同じ匿名化方針を適用する。

## 2. テンプレート

新しいセッションを追記するときは下のブロックをコピーして埋める。**未実施シナリオは行ごと削除**してよい(空欄を残さない)。

```markdown
## YYYY-MM-DD <α|β>-<N> (<OS> / <端末> / <ネットワーク>)

### 環境
- ゲートウェイ: vX.Y.Z (commit `<short-sha>`)
- ホスト PC: Windows ___ build ___
- クライアント: ___(iOS ___ Safari / Android ___ Chrome / Win ___ Edge 等)
- ネットワーク: ___(SSID は伏せ、5GHz/2.4GHz / 有線 などの種別だけ)
- 実施者: ___

### 実施シナリオと結果
| シナリオ | 結果 | メモ |
|---|---|---|
| Smoke | ✅/⚠/❌ | |
| Multi-client | | |
| Sleep/Wake | | |
| WiFi drop | | |
| Soak (Xh) | | |
| Firewall fresh | | |
| mDNS | | |
| USB 起動 | | |
| USB 抜き挿し | | |
| FAT32 配置 | | |

### 観察
- ___

### 自動収集ログ
- `<相対パス>` (ws-recorder CSV / soak-monitor CSV / gateway logs など)

### 派生 Issue / PR
- #___ (<概要>)
```

## 3. 実施履歴

> ここから下に実施セッションを **新しいものほど上**(逆時系列)で追記していく。

## 2026-05-06 α-1 (Win 11 build 26200.8246 / 自走 — 人手シナリオなし)

### 環境
- ゲートウェイ: `dev-da4d7bc`(commit `da4d7bc`、`scripts\build.ps1` でその場ビルド)
- ホスト PC: Windows 11 Home, build 10.0.26200.8246
- クライアント: なし(`ws-recorder` ヘッドレス × 3 のみ。人手シナリオは未実施)
- ネットワーク: localhost のみ(LAN・無線は次回 α-2 以降)
- 実施者: Windows Claude Code エージェント(`.\scripts\fieldtest-runall.ps1 -SoakDurationMin 10 -SkipBuild` を一回起動)

### 実施シナリオと結果
| シナリオ | 結果 | メモ |
|---|---|---|
| Smoke | ✅ | bytes=2703, clients=3, passings=20, unknown_tors=0, fanout_cv=0.0% |
| Replay round-trip | ✅ | bytes_in=2097, bytes_out=2097, diff=0(`session-2026-05-05.bin` を 1500ms 遅延 timing.csv 経由で replay) |
| ZIP shape | ⚠ | size=6.01MB; missing optional: `packaging\README.txt`(#37 未着手のため既知。`gateway.exe` / `config.example.json` は揃っている) |
| USB pathshift | ✅ | drive=Z:, healthz=ok, logs_created=true(`subst Z:` 経由で EXE を起動し、`os.Executable()` 相対のパス解決が効くことを確認) |
| Soak (10m) | ✅ | ws_mb_delta=+2.3%, handle_delta=+1.0%, reconnects=0(本セッションは Soak 短縮版。フル 60 分版は α-2 以降) |

### 観察
- **fan-out CV = 0.0%**: 3 つの ws-recorder が受け取ったバイト総量が完全一致。`internal/hub` の broadcast に skew が無いことを定量で確認。
- **WorkingSet drift +2.3% / Handles +1.0%**: いずれも閾値(+20% / +10%)に対して大きく余裕あり。10 分スケールでの目立つ leak は無し。フル 60 分の傾向は α-2 で要追確認。
- **reconnects = 0**: localhost / `--mock` 構成で、PR #72 で入れた「`ctx.Err() == nil` でゲートする disconnect 計上ロジック」が期待通り作動。phantom disconnect は再現せず。
- **harness 自体の所要時間**: 5 シナリオ + 集計で約 13 分(Smoke ~30s, Replay ~10s, ZIP shape ~0s with `-SkipBuild`, USB ~5s, Soak 10m, 集計数秒)。フル版(Soak 60m)は単純加算で約 63 分の見込み。

### 自動収集ログ
- `dist\fieldtest-runs\smoke-20260506-113152\` (3 つの recorder.csv + raw bin + gateway logs)
- `dist\fieldtest-runs\replay-rt-20260506-113229\` (合成 timing.csv / rt.csv / rt.bin)
- 実施者ローカル `fieldtest-runall-alpha-1.log`(コミットしない — 個人 PC のパスを含む)
- Soak (`soak-20260506-113240\`) は成功時に `fieldtest-runall.ps1` が runDir をクリーンアップしているため残っていない。傾向の追跡が必要になったら次回は `-LeaveArtifacts` を付けて再実行する。

### 派生 Issue / PR
- なし(harness の不具合・閾値見直しの要否は本セッションでは検出されず)

### 未実施(人手要)
- iOS Safari Speech / Sleep-Wake / 実 WiFi drop / 物理 USB / SmartScreen / mDNS — 現地セッション(α-2 以降)で実施予定

---

## 4. 改訂履歴
- v0.1.1 (2026-05-06): §3 に α-1 セッション(自走分)を追記。Soak は 10 分短縮版で実施。
- v0.1 (2026-05-06): 初版。フォーマット骨格とテンプレートのみ。初回 Field Test α 実施前。
