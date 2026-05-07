# Field Test Log

`docs/test-strategy.md` §6 で定義した Field Test(実 LAN ✕ 実機なし)の **実施記録**を残す場所。CI で自動化しないため、ここに書いておかないと「現地で何が起きたか」が後から辿れなくなる。

> Status: **Draft v0.1.6**(β-1 自宅 dry-run の全 9 シナリオ ✅、β-2 ブロッカー全消化)

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

### 2.1 β-1 自宅 dry-run 専用テンプレート

`docs/test-strategy.md` §6.3 で定義した「自宅でできる範囲を一気に潰す」セッション用。AMB 実機接続だけ別セッション(β-2)に分離するので、シナリオは固定 9 件に絞ってある。

```markdown
## YYYY-MM-DD β-1 (Windows ___ / iOS ___ / Android ___ / 自宅 WiFi)

### 環境
- ゲートウェイ: vX.Y.Z (commit `<short-sha>`、`scripts\build.ps1` 出力)
- ホスト PC: 持ち込む実機(Windows ___ build ___)
- クライアント: スマホ ___(Safari ___) / タブレット ___(同) / PC ブラウザ ___
- ネットワーク: 自宅 WiFi(2.4 / 5 GHz)、ルータ ___
- 実施者: ___

### 実施シナリオと結果
| シナリオ | 結果 | メモ |
|---|---|---|
| Smoke(`gateway --mock` + 実スマホで PASSING / lap / 音声) | ✅/⚠/❌ | settings.transponder=1、3 ID が流れることを確認 |
| iOS Safari Speech 初回 unlock + 発話 | | 🔊 タップ → 「21秒xxx」と聞こえるか |
| Multi-client(スマホ + タブレット同時) | | 両方で同じ表示 / 音声、片方切断で他方継続 |
| Sleep/Wake(スマホロック → 1〜2 分待機 → 解除) | | WS 自動再接続、表示復元、音声再開 |
| 実 WiFi drop(機内モード ON/OFF or ルータ電源 OFF/ON) | | ゲートウェイ・クライアント両側で再接続 |
| Soak 1h(`scripts\fieldtest-soak.ps1 -DurationMin 60`) | | ws_mb_delta / handle_delta / reconnects |
| `/admin` 手動 E2E(login + 1 項目変更 + logout) | | passphrase 入力、保存トースト、requires_restart バナー |
| USB 起動(自分の USB に展開・ダブルクリック) | | logs / records が USB 配下に作られる |
| USB 抜き挿し(起動中に物理的に抜く → 戻す) | | ログ書込みエラー警告のみで停止しない |

### 観察
- ___(数値・気付き・予想と違ったこと)

### 自動収集ログ
- `dist\fieldtest-runs\soak-<ts>\` (Soak シナリオで `-LeaveArtifacts` 指定時)
- 実施者ローカルの動画 / スクリーンショット(コミットしない、個人保管)

### 派生 Issue / PR
- #___ (<概要>)

### 任意(余裕があれば)
- mDNS `*.local` 解決(iOS / Android / Win それぞれ)
- FAT32 USB(NTFS なら不要)
```

β-1 完了後の **β-2(現地)は AMB 疎通確認のみ**(`docs/test-strategy.md` §6.3 参照)。

## 3. 実施履歴

> ここから下に実施セッションを **新しいものほど上**(逆時系列)で追記していく。

## 2026-05-07 β-1 修正検証 — B-4 を PR #108 マージ後に再実施

β-1 本番(同日先行)で残っていた最後の ⚠ シナリオ B-4 Sleep/Wake を PR #108 マージ後の main(`a4267d5`)で再実施。**自動復帰 ✅** を確認、`/settings` 再保存などのワークアラウンドなしで lap 表示と発話が戻った。これで **β-1 自宅 dry-run の全 9 シナリオが ✅** となり、β-2 現地(AMB 実機疎通)に進む準備が整った。

### 環境
- ゲートウェイ: `dev-a4267d5`(commit `a4267d5`、PR #108 マージ後の main で `scripts\build.ps1` 出力)
- ホスト PC: Windows 11 Home, build 10.0.26200.8246(本番セッションと同一)
- クライアント: iPhone Safari(自宅 WiFi)
- ネットワーク: 自宅 WiFi(同前)
- 実施者: 操作員(iPhone 実機)+ Windows Claude Code エージェント(伴走)

### 再検証シナリオと結果
| シナリオ | 前回 | 今回 | メモ |
|---|---|---|---|
| B-4 Sleep/Wake | ⚠ 自動復帰せず、`/settings` 再保存で復旧 | ✅ | 画面ロック 1〜2 分 → 解除 → **約 20 秒で自動復帰**(mock は ponder 1/2/3 を 6 秒周期で回しており、自分の transponder=1 向け lap は 18 秒周期 = 復帰 + 1 周期分の妥当な所要時間) |

### 観察
- 1 回目の retest では「アンロック直後 5〜10 秒で待機中のまま」と判断して reload してしまった経緯あり。**mock の rotation 周期(自分の transponder 向け 18 秒)を超えて待つ**ことが正しい検証手順。docs/field-test-log.md §2.1 のシナリオメモに「30 秒以上待つ」と書き足す候補(任意)。
- ゲートウェイログには visibility-triggered force reconnect と既存の reconnect timer が **0.07 秒差で 2 つの ws connect** を残しており、PR #108 の visibilitychange ハンドラが期待どおり発火していたことを確認。
- iPhone Safari の SPA バンドルが古い版(visibility 修正なし)を引き続き使っていないか念のため確認するため、再テストでは **タブを完全に閉じてから新規で URL を開く** 手順で実施した。
- B-2 / B-7 / B-8 は PR #103 / #105 で既に ✅ 化済み(2026-05-07 β-1 修正検証 セクション参照)。本セッションは B-4 のみが対象。

### 残課題
- なし(β-1 残 ⚠ / ❌ シナリオ 0 件、β-2 ブロッカー解消)
- 派生していた **#98** / **#99** / **#100** / **#101** はすべてマージ済み

### 自動収集ログ
- ゲートウェイログ: PC 側 `dist\AMB_RC_Lap_Timer\logs\gateway.log`(`ws client connected` の連続 entry が visibility hook の実証)
- 実施者ローカルの観察メモ(コミットしない)

### 次のステップ
- β-2 現地(AMB 実機疎通)に進める状態
- リリース #9(v0.1.0)の前段が整った

## 2026-05-07 β-1 修正検証 — B-2 / B-7 / B-8 を PR #103 / #105 マージ後に再実施

β-1 本番(下記)で発見した派生 Issue のうち **#98**(Speech 発話崩壊)と **#101**(USB 移植性破綻)が PR #103 / #105 で修正されたので、該当の B-2 / B-7 / B-8 を再実施。同 PC・同 iPhone・同 USB を使用した条件で再検証し、すべて ✅ で合格。本セッションは β-1 本番(同日先行)の補完であり、新規シナリオは追加していない。

### 環境
- ゲートウェイ: `dev-5c95882`(commit `5c95882`、PR #103 / #104 / #105 マージ後の main で `scripts\build.ps1` 出力)
- ホスト PC: Windows 11 Home, build 10.0.26200.8246(本番セッションと同一)
- クライアント: iPhone Safari(Wi-Fi)、Windows Chrome(Ethernet)
- ネットワーク: 自宅 WiFi(同前)
- 実施者: 操作員(iPhone 実機)+ Windows Claude Code エージェント(伴走)

### 再検証シナリオと結果
| シナリオ | 前回(本番) | 今回(再検証) | メモ |
|---|---|---|---|
| B-2 iOS Safari Speech | ⚠ 「いちまんきゅうせん…」 | ✅ | PR #103 で `formatLapTimeForSpeech` 追加。「**じゅうきゅうびょうよんじゅうきゅう**」と期待どおり発話 |
| B-7 USB 起動 | ❌ logs が C: を指す | ✅ | PR #105 で `/admin/api/config` を Raw / Resolved 分離。USB 起動ログに `"logs": "F:\\AMB_RC_Lap_Timer\\logs"` / `"records": "F:\\AMB_RC_Lap_Timer\\records"`(C: 漏れなし) |
| B-8 USB 抜き挿し | ⚠ #101 連鎖で fail-soft 検証不可 | ✅ | 抜去時に `write error: ... no longer valid.` の **fail-soft 警告を console に出力**、process は継続、Ctrl+C で正常終了。`docs/architecture.md` §4.4.3 仕様どおり。再挿入時の自動復旧は仕様上保証なし(handle は dead のまま)で挙動一致 |

### 観察
- **B-7 ログ抜粋**(USB 起動時):
  ```
  baseDir=F:\AMB_RC_Lap_Timer
  config=F:\AMB_RC_Lap_Timer\config.json
  logs=F:\AMB_RC_Lap_Timer\logs        ← 前回は C:
  records=F:\AMB_RC_Lap_Timer\records  ← 前回は C:
  ```
  PR #105(Raw / Resolved 分離)が **実機 USB 起動でも期待どおり動作**。
- **B-8 fail-soft 警告**:
  ```
  write error: write F:\AMB_RC_Lap_Timer\logs\gateway.log:
  The volume for a file has been externally altered so that the opened file is no longer valid.
  ```
  zap/lumberjack の internal error sink が stderr に吐き、プロセスは continued。シャットダウン時にもう 1 度同じ警告が出るのは「ハンドル dead で再挿入で自動復旧しない」という設計上の許容範囲(architecture.md §4.4.3 明記)。
- **修正前の汚染 config.json** が USB に残っていたため、本セッションでは clean dist で上書きしてから USB 起動した。架構上の「過去の汚染済み config.json の自動 self-heal」は Issue #101 のスコープ外で、operator が手で直す前提どおり。
- B-2 の音声検証は B-7 セットアップ中に副次的に確認できる位置で、**iPhone Safari + Windows Chrome 両方で同じ新フォーマット発話**(.\` を読まず「N秒MM」)が再現。

### 残る派生 Issue
- **#100** [fix(web): iPhone Safari Sleep/Wake 後に LapList が "待機中" のまま固まる](https://github.com/hama-jp/AMB_RC_Lap_Timer/issues/100) — 未着手、β-2 までに修正したい
- **#99** [docs(packaging): Defender Firewall ダイアログが出ないケースの復旧手順を追加](https://github.com/hama-jp/AMB_RC_Lap_Timer/issues/99) — PR #104 で対応済み(本セッションで該当復旧手順を実機適用、機能した)

### 自動収集ログ
- 本セッションのゲートウェイログ: PC 側 `dist\AMB_RC_Lap_Timer\logs\gateway.log` および USB 側 `F:\AMB_RC_Lap_Timer\logs\gateway.log`(両方に書かれた = #101 修正の副次証跡)
- 実施者ローカルの観察メモ(コミットしない)

## 2026-05-07 β-1 自宅 dry-run 本番 (Win 11 build 26200.8246 / iPhone Safari + Win Chrome / 自宅 WiFi)

PR #95 で定義した β-1 9 シナリオの本番セッション。Soak 1h は PR #96 で先行済みのため、本セッションでは残 8 シナリオを順番に実施した。Windows エージェント(伴走)が PC 側のゲートウェイ起動・USB へのコピー・派生 Issue 起票を担当し、操作員(ユーザ)が iPhone Safari + Windows Chrome で各シナリオを実機操作する役割分担。

### 環境
- ゲートウェイ: `dev-bf09187`(commit `bf09187`、`scripts\build.ps1` 出力)
- ホスト PC: Windows 11 Home, build 10.0.26200.8246
- クライアント: iPhone Safari + Windows Chrome(B-3 Multi-client は両方同時)
- ネットワーク: 自宅 WiFi(PC は Ethernet、iPhone は WiFi、同一 LAN セグメント `192.168.11.x`)
- 実施者: 操作員(iPhone 実機)+ Windows Claude Code エージェント(伴走)

### 実施シナリオと結果
| シナリオ | 結果 | メモ |
|---|---|---|
| Smoke(`gateway --mock` + 実スマホで PASSING / lap / 音声) | ✅ | 大型表示 / Lap List / ★ ハイライト全部表示。最初に Defender Firewall inbound block でハマり、admin PowerShell で `New-NetFirewallRule -Profile Any -Program gateway.exe` を 1 行実行して復旧 → #99 |
| iOS Safari Speech 初回 unlock + 発話 | ⚠ | 🔊 ボタン unlock + 連続発話は OK。ただし `19.486秒` を **「いちまんきゅうせんよんひゃくはちじゅうろくびょう」(= 19,486 秒 / 5h24m)** と読み上げる致命的な発話崩壊 → #98 |
| Multi-client(スマホ + PC ブラウザ) | ✅ | iPhone(ponder=1) + Windows Chrome(ponder=2)で交互に発声。fan-out + per-client transponder filter が同時 2 client で正常動作 |
| Sleep/Wake(iPhone ロック → 1〜2 分待機 → 解除) | ⚠ | 自動復帰せず「PASSING を待機中」表示で固定。`/settings` でトランスポンダーを再保存すると復旧。`docs/test-strategy.md` §6.1 が予測していた iOS suspension での silent socket kill → #100 |
| 実 WiFi drop(機内モード ON 30s → OFF) | ✅ | 機内モード OFF 後すぐに WS 自動再接続 + 音声再開。Sleep/Wake と対比して **wsClient の reconnect ロジック自体は正常**、iOS 側で onclose 発火するか否かが分岐点と確定(#100 仮説 #1 補強) |
| Soak 1h | ✅ | PR #96 で実施済み: ws_mb_delta=+4.8% / handle_delta=+0.2% / reconnects=0 |
| `/admin` 手動 E2E(login + 1 項目変更 + logout) | ✅ | passphrase 入力・upstream.host を架空値に変更・保存トースト「保存しました(1 項目変更)」表示・元値に戻して再保存・ログアウト、すべて想定どおり。**ただし副作用として #101 を露呈**(下記 USB 起動参照) |
| USB 起動(F:\AMB_RC_Lap_Timer に展開) | ❌ | `os.Executable()` 経由の baseDir 解決は OK / config.json も USB から読まれる ✅ だが、**`/admin` POST が resolved 絶対パスを焼き込んでいたため `logs.dir` / `records.dir` が `C:\…\dist\AMB_RC_Lap_Timer\logs` を指したまま** USB 起動。USB 配下にログが書かれない → #101 **β-2 ブロッカー** |
| USB 抜き挿し(USB 上で起動 → 物理抜去 → 再挿入) | ⚠ | プロセスは抜去後もクラッシュせず継続 ✅(EXE のメモリマップ生存)。ただし #101 でログが USB 配下に無いため、本来の fail-soft I/O 検証(警告のみで停止しない確認)は **#101 修正後に再実施** |

### 観察
- **致命度の高い 2 件**: #100(Sleep/Wake)と #101(USB 移植性)。前者はフィールド利用での実用性、後者は USB 配布の正当性に直結。**両方とも β-2 現地までに修正したい**。
- **音声フォーマット #98**: 当初「`.` を「テン」と読む」程度の認識だったが、実機検証で **5 桁整数として読まれる** ことが判明。lap タイム発話が事実上使えない状態で、これも β-2 までに直したい。
- **WS reconnect の切り分け**: B-4 ❌ vs B-5 ✅ の対比で、`wsClient` の reconnect 自体は正しく動くこと、iOS suspension のときだけ socket が silent kill される点が確定。修正範囲を `transport/wsClient.ts` の visibility 検出に絞れる。
- **B-6 が #101 を露呈**した連鎖: `/admin` 単体は ✅ 動作だが、saved config が C: 絶対パスを焼き込み → 続く B-7 でその config が USB に持ち込まれて致命傷。シナリオ順序が悪かったわけではなく、**`/admin` 経由で 1 度でも保存すると config.json が汚染される**事実を発見できたこと自体が β-1 の成果。
- **Defender Firewall #99**: PC が Public プロファイルだとダイアログが出ずに silent block。家庭の WiFi ルータ配下では Public 分類になりがちで、`packaging/README.txt` の現状の説明では復旧手順が無い。
- **harness の所要時間**: B-1 から B-8 まで実機セッションで約 60 分(うち Firewall 復旧調査 15 分 + 派生 Issue 起票時間が断続的に挟まる)。Soak 1h は PR #96 で済ませてあるので合計 2 時間で β-1 完了。

### 自動収集ログ
- Soak 1h: PR #96 で残置の `dist\fieldtest-runs\soak-20260507-072048\`
- 本セッションのゲートウェイログ: PC 側 `dist\AMB_RC_Lap_Timer\logs\gateway.log`(USB 側 F: は #101 の影響で書かれていない)
- 実施者ローカルの観察メモ・Safari スクリーンショット等(コミットしない、個人保管)

### 派生 Issue
- **#98** [feat(speech): lap 発話を「19秒49」形式に](https://github.com/hama-jp/AMB_RC_Lap_Timer/issues/98)`.` を含めない / 2 桁精度 / iOS Safari TTS の整数結合回避
- **#99** [docs(packaging): README.txt に Defender Firewall ダイアログが出ないケースの復旧手順を追加](https://github.com/hama-jp/AMB_RC_Lap_Timer/issues/99)
- **#100** [fix(web): iPhone Safari Sleep/Wake 後に LapList が "待機中" のまま固まる](https://github.com/hama-jp/AMB_RC_Lap_Timer/issues/100) — **β-2 までに修正**
- **#101** [fix(admin): /admin/api/config POST が resolved 絶対パスを config.json に焼き込み USB 移植性を壊す](https://github.com/hama-jp/AMB_RC_Lap_Timer/issues/101) — **β-2 ブロッカー**

### 次セッションで再検証したい項目
- B-2 Speech: #98 修正後に「じゅうきゅうびょうよんじゅうきゅう」と聞こえるか実機確認
- B-4 Sleep/Wake: #100 修正後に画面ロック → 解除で表示自動復帰するか
- B-7 USB 起動: #101 修正後に F:\AMB_RC_Lap_Timer\logs\ に gateway.log が書かれるか
- B-8 USB 抜き挿し: #101 修正後に「ログ書込みエラー警告のみで停止しない」fail-soft I/O が働くか

### 任意項目
- mDNS `*.local` 解決: 本セッションでは未実施
- FAT32 USB: 本セッションでは USB のフォーマット未確認(NTFS と推定)

## 2026-05-07 β-1 自宅 dry-run 先行 (Win 11 build 26200.8246 / Soak 1h のみ)

β-1 本番(自宅 9 シナリオ一括)に入る前の Soak 1h **先行検証**。`docs/test-strategy.md` §6.3 で β-1 に組み込まれている Soak だけを切り出し、PR #95 マージ後の main(`fc56fe4`)で 60 分回した結果を残す。残りの 8 シナリオ(iOS Speech / Sleep-Wake / 実 WiFi drop / Multi-client / `/admin` 手動 E2E / USB 起動 / USB 抜き挿し / SmartScreen)は β-1 本番セッションでまとめて実施する。

### 環境
- ゲートウェイ: `dev-fc56fe4`(commit `fc56fe4`、`scripts\build.ps1` でその場ビルド)
- ホスト PC: Windows 11 Home, build 10.0.26200.8246
- クライアント: なし(`ws-recorder` ヘッドレス × 1、`gateway --mock` を localhost で消費)
- ネットワーク: localhost のみ(LAN・無線は β-1 本番)
- 実施者: Windows Claude Code エージェント(`.\scripts\fieldtest-soak.ps1 -DurationMin 60 -LeaveArtifacts` を一回起動)

### 実施シナリオと結果
| シナリオ | 結果 | メモ |
|---|---|---|
| Soak 1h(`scripts\fieldtest-soak.ps1 -DurationMin 60`) | ✅ | ws_mb_delta=+4.8%, handle_delta=+0.2%, reconnects=0 |

### 観察
- **ws_mb 推移**: 9.81 MB → 10.31 MB(5 分頭/末平均 +4.8%)。閾値 +20% の 1/4 程度で leak の徴候なし。線形外挿でも 8 時間で +20% 程度に収まる見込みで、Soak 8h を回す根拠になる。
- **handles 推移**: 124 → 126(+0.2%)。fd / goroutine の累積なし。
- **threads / cpu**: 8 → 7(安定)、cpu_seconds 累計 0.03 → 0.16(60 分で約 0.13 sec、~0.004% CPU)。アイドル状態の gateway が想定どおりの軽さで動いていることを確認。
- **recorder events**: connect=1, frame=601, shutdown=1。disconnect=0 で **clean exit**(PR #80 race fix と PR #88 の `shutdown` event 区別が期待どおり作動)。
- **mock のフレーム rate**: 601 frames / 60 min ≈ 1 frame / 6 sec。PR #91 の multi-transponder mock(3 ponders rotating)が想定 rate で安定動作(3 ponder × 18 sec lap ÷ 3 = 6 sec stride、観測と一致)。
- **gateway.stderr.log は空**: 60 分間にエラーログなし。
- **soak-monitor.csv の `established` / `listen` 列が空**: `Get-NetTCPConnection -OwningProcess` がユーザ権限で gateway の所有接続を列挙できていない様子。α-1(10 分版)でも同じ症状で、Soak 判定そのものには影響なし。改善は別 Issue 候補(本 PR では起票せず追跡メモのみ)。

### 自動収集ログ
- `dist\fieldtest-runs\soak-20260507-072048\` (`-LeaveArtifacts` 指定で残置 — gateway logs / soak-monitor.csv 120 行 / recorder.csv 603 行)
- 実施者ローカル `soak-60m.log`(コミットしない — 個人 PC のパスとタイムスタンプを含む)

### 派生 Issue / PR
- なし(harness の不具合は検出されず。`established` / `listen` 列の空白は β-1 本番の所感と合わせて起票判断する)

### 未実施(β-1 本番セッションに持ち越し)
- iOS Safari Speech 初回 unlock + 発話
- Multi-client(スマホ + タブレット同時)
- Sleep/Wake(スマホロック → 復帰)
- 実 WiFi drop(機内モード or ルータ電源 OFF/ON)
- `/admin` 手動 E2E(login + 1 項目変更 + logout)
- USB 起動 / USB 抜き挿し
- (任意)mDNS / FAT32 USB

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
- v0.1.6 (2026-05-07): §3 に β-1 修正検証(B-4)セッションを追記。PR #108(#100 Sleep/Wake)マージ後に B-4 を実機再実施し ✅ 自動復帰を確認。**β-1 自宅 dry-run の全 9 シナリオが ✅** となり、β-2 現地に進める状態。
- v0.1.5 (2026-05-07): §3 に β-1 修正検証セッションを追記。PR #103(#98 Speech)と PR #105(#101 USB)マージ後に B-2 / B-7 / B-8 を実機再実施し、**3 件すべて ✅** で β-2 ブロッカー解消。残課題は #100(Sleep/Wake)のみ。
- v0.1.4 (2026-05-07): §3 に β-1 自宅 dry-run 本番セッション(残 8 シナリオ)を追記。✅ 4 件 / ⚠ 3 件 / ❌ 1 件、派生 Issue #98 / #99 / #100 / #101 を起票。**β-2 現地までに #100 と #101 を修正**(後者は USB 配布の正当性に直結)。
- v0.1.3 (2026-05-07): §3 に β-1 自宅 dry-run の Soak 1h 先行検証を追記。Soak 単独で +4.8% / +0.2% / 0 disconnects と green、β-1 本番(残り 8 シナリオ)に進む根拠を残す。
- v0.1.2 (2026-05-06): §2.1 に β-1 自宅 dry-run 専用テンプレートを追加。`docs/test-strategy.md` §6.3 の β-1 / β-2 分割に対応。シナリオ 9 件を固定し、AMB 実機なしで完結するセッション用に整える。
- v0.1.1 (2026-05-06): §3 に α-1 セッション(自走分)を追記。Soak は 10 分短縮版で実施。
- v0.1 (2026-05-06): 初版。フォーマット骨格とテンプレートのみ。初回 Field Test α 実施前。
