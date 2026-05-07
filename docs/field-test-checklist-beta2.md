# Field Test β-2 当日チェックリスト

`docs/field-test-log.md` §2.2 と対になる **当日操作員用の手順書**。1 ページに収まるよう要点だけ。理屈や設計判断は `docs/test-strategy.md` §6.3 を参照。

> 想定: 会場の業務 WiFi に持ち込みノート PC を WiFi 接続し、AMB へ TCP 疎通を確認する。所要 ~30 分、走行 1 周分。

---

## 1. 持参物

- [ ] **ノート PC**(自宅で `gateway.exe` 動作確認済み、Defender Firewall 「プライベートネットワーク」許可済み)
- [ ] **USB メモリ**(`dist/AMB_RC_Lap_Timer/` 一式コピー済み、リリース版 `v0.1.0` 推奨)
- [ ] **iPhone**(直前にタブを完全に閉じておき、当日新規で URL を開く ← キャッシュ起因のトラブル回避)
- [ ] **PC + iPhone の充電器**
- [ ] **LAN ケーブル 1 本**(WiFi 借用が当日変更になった場合の保険)
- [ ] **自分のトランスポンダー番号メモ**(数字 / decimal)

---

## 2. 想定する設定値(前回採取セッション 2026-05-05 と同じ前提)

| 項目 | 値 |
|---|---|
| AMB IP | `192.168.1.21` |
| AMB Port | `5403` |
| ゲートウェイ listen | `:8080` |

**当日違っていた場合**: 設定の変更は **`/admin` 経由(login → upstream.host / port を更新 → 保存)が推奨**。`config.json` を直接編集しても良いが、その場合は再起動が必要。

---

## 3. 現地手順(5 ステップ)

### Step 1. WiFi 接続
ノート PC を **業務 WiFi**(店長から借用したもの)に接続。来店者用ゲスト WiFi だと AMB に届かない可能性が高いので、SSID を念のため確認。

### Step 2. AMB 疎通確認(1 分)
コマンドプロンプトで:
```
ping 192.168.1.21
```
- ✅ 4 回連続応答 → Step 3
- ❌ Request timed out → **トラブル A**

### Step 3. ゲートウェイ起動(1 分)
USB を挿す → `dist\AMB_RC_Lap_Timer\gateway.exe` をダブルクリック。

黒いウィンドウのログで以下を確認:
```
INFO  source: real (TCP)            addr=192.168.1.21:5403
INFO  upstream connected            addr=192.168.1.21:5403
INFO  http server listening         addr=:8080
```
- ✅ `upstream connected` が出る → Step 4
- ❌ `dial: ...` が連続して出る → **トラブル B**

### Step 4. iPhone から接続(2 分)
1. PC の IP を確認(`ipconfig` で IPv4 アドレス)
2. iPhone Safari で `http://<このPCのIP>:8080/` を開く
3. **設定アイコン**から `transponder=<自分の番号>` を保存(履歴チップが追加される)
4. ヘッダ右の **🔊 ボタン**をタップして読み上げを unlock
   - スマホがマナーモードでないか / 音量が出るかも確認

### Step 5. 走行確認(1 周分)
1 周走り、AMB を通過 → スマホ画面で:
- 大型表示に lap 秒(`21.X`)が表示される
- Lap List に新しい行が追加される
- 「N 秒 X Y」と読み上げられる

これで **β-2 完了**。

---

## 4. トラブル別フロー

### A. ping が通らない
1. PC の **WiFi 接続先 SSID** を確認(業務 WiFi 側か?ゲスト WiFi に切り替わっていないか)
2. 違ったら正しい SSID に再接続
3. それでも通らなければ AMB の IP が変わっている可能性 → 店長に確認 or LAN ケーブル接続に切替

### B. `upstream connected` が出ない / `dial: ...` 連続
1. config.json の `upstream.host` / `port` が `192.168.1.21:5403` か確認
2. AMB 側が起動しているか確認(店長 / 店員に確認)
3. 上記で改善なければ Ctrl+C で停止 → 直ちに撤収判断(空振り)

### C. iPhone から SPA にアクセスできない
1. URL のホスト部分が PC の **IPv4 アドレス**(WiFi 側)か確認 — `127.0.0.1` や別 NIC を見ていないか
2. PC の Firewall 許可ダイアログが出たか確認 — 出なかった場合は `packaging/README.txt` の **Defender Firewall 復旧手順**を実行(管理者 PowerShell で `New-NetFirewallRule`)
3. iPhone と PC が **同じ業務 WiFi** にいるか確認

### D. 音声が鳴らない
1. iPhone のマナーモード解除、音量を上げる
2. `/settings` で「読み上げを有効にする」が ON か
3. ヘッダの 🔊 ボタンを **明示的に 1 回タップ**(iOS Safari の制約)
4. 数 lap 待っても鳴らなければ、Safari タブを完全に閉じて開き直す

### E. アプリは動くが lap 秒が表示されない
1. `/settings` で `transponder` の値が **走行中のトランスポンダー番号と一致**しているか確認
2. mock ではなく実機接続なので、自分の番号を入れていないと「待機中」表示のまま
3. 履歴チップが残っていれば該当タップ → 保存

---

## 5. 撤収手順

1. ゲートウェイの黒いウィンドウで `Ctrl+C` を押す → 「Shutdown completed」のログを確認
2. ウィンドウを閉じる
3. USB メモリの `dist\AMB_RC_Lap_Timer\logs\gateway.log` を後で確認できるよう保管
4. **記録した PASSING があれば**(`--record` オプションをつけて起動した場合)、`records/` フォルダの `.bin` を持ち帰り
   - 第三者のトランスポンダーが含まれる場合は **必ず `gateway/cmd/anonymize` を経由**してから fixture 化(`docs/test-strategy.md` §7.4)
5. USB を抜く

---

## 6. 帰宅後

1. `docs/field-test-log.md` §3 の最上部に β-2 セッションを追記(§2.2 のテンプレートをコピー → 値を埋める)
2. 派生 Issue があれば起票
3. 全項目 ✅ なら **`v0.1.0` リリースの正式記録**として確定
4. 派生があれば β-2-2(再訪 or 自宅 mock 補修)を計画

---

## 7. このチェックリストの扱い

- 印刷して USB に同梱しておくと当日めくれる
- ASCII / 日本語のみで構成、レイアウトは A4 1〜2 ページを想定
- 改訂は `docs/test-strategy.md` §6.3 と同期(片方だけ更新しない)
