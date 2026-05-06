# Field Test Log

`docs/test-strategy.md` §6 で定義した Field Test(実 LAN ✕ 実機なし)の **実施記録**を残す場所。CI で自動化しないため、ここに書いておかないと「現地で何が起きたか」が後から辿れなくなる。

> Status: **Draft v0.1**(初回 α 実施前のフォーマット骨格)

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

<!-- 例:
## 2026-05-10 α-1 (Win 11 / iPhone 15 / WiFi 5GHz)
...
-->

_(まだ実施なし)_

---

## 4. 改訂履歴
- v0.1 (2026-05-06): 初版。フォーマット骨格とテンプレートのみ。初回 Field Test α 実施前。
