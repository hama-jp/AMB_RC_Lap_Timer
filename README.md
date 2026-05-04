# AMB_RC_Lap_Timer

AMB P3 デコーダーに対応した、**個人練習用** のラップタイマー Web アプリ。

> Status: **pre-alpha / 設計フェーズ**(実装着手前。準備フェーズの仕様書整備が完了)

---

## このアプリは何をするか

- LAN 上の AMB P3 デコーダーから受信したラップ通過情報を、**自分のスマホ/タブレットのブラウザ** にリアルタイム表示する
- 自分が指定したトランスポンダーのラップタイムを **音声で読み上げる**(走行中でも耳で確認できる)
- 個人練習に特化(他人の順位や履歴の長期保存はしない)

## このアプリが何をしないか

- ラップタイムの長期保存(MyLaps 等の専用アプリで十分)
- 他者のラップ管理・順位整理(個人練習特化)

---

## 構成

```
+----------------+   TCP    +----------------------+   HTTP/WS  +-----------------+
|  AMB Decoder   | <------> |  Gateway (Go EXE)    | <--------> |  Browser (SPA)  |
|  LAN内, 5403   |          |  - TCP↔WS の薄い橋   |            |  - P3 パース    |
+----------------+          |  - SPA も同梱配信     |            |  - 表示・音声   |
                            +----------------------+            +-----------------+
```

- **ブラウザは生 TCP を扱えない**ため、LAN 内の Windows PC 上に **薄いゲートウェイ EXE** を 1 つ常駐させる構成。
- ゲートウェイは TCP↔WebSocket の **byte pipe** に徹し、P3 プロトコルの解釈は **ブラウザ側 (TypeScript)** で行う。
- ゲートウェイが SPA も同梱配信(`go:embed`)するため、外部 Web ホスティングは不要。スマホからは `http://<ゲートウェイIP>:8080/` を開くだけ。
- LAN 内専用。インターネット越しの利用は想定しない。

詳細は [`docs/gateway-technical-decision.md`](docs/gateway-technical-decision.md) と [`docs/architecture.md`](docs/architecture.md) を参照。

---

## 動作要件

| 種別 | 要件 |
|---|---|
| ゲートウェイ動作 PC | Windows 8.1 以上(10/11 推奨)。**インストール不要**(USB から直接起動可) |
| クライアント | 同 LAN 内のスマホ/タブレット/PC のモダンブラウザ(Web Speech API 対応) |
| ネットワーク | AMB デコーダー / ゲートウェイ PC / クライアントが同一 LAN |
| ビルド環境 | Go 1.20.x / Node.js 20 LTS(開発者のみ) |

## 起動手順(現地、Windows PC)

1. USB メモリを挿す
2. `AMB_RC_Lap_Timer-vX.Y.Z.zip` を任意の場所(USB 直下推奨)に展開
3. 展開された `AMB_RC_Lap_Timer\config.json` で AMB の IP/ポートを編集(初回のみ)
4. `AMB_RC_Lap_Timer\gateway.exe` をダブルクリック
   - 初回は **Microsoft Defender SmartScreen** の警告が出る → **「詳細情報」→「実行」**
   - 続いて **Windows Defender Firewall** の許可ダイアログ → **「プライベートネットワーク」のみ**チェック → 許可
5. ゲートウェイの起動ログに `TCP 接続成功` が出ることを確認
6. スマホ/タブレットのブラウザで `http://<ゲートウェイPCのIP>:8080/` を開く

詳細は同梱の `README.txt`(現地ガイド)と [`docs/architecture.md`](docs/architecture.md) §4.4「ポータブル運用」を参照。

---

## ドキュメント索引

| パス | 内容 |
|---|---|
| [`docs/development-workflow.md`](docs/development-workflow.md) | 開発運用規約(Issue→PR、Conventional Commits、マルチエージェント運用) |
| [`docs/gateway-technical-decision.md`](docs/gateway-technical-decision.md) | ゲートウェイ技術選定(Go/Win 互換/byte pipe 方針) |
| [`docs/architecture.md`](docs/architecture.md) | リポジトリ構成・配信トポロジ・ビルドフロー・開発手順 |
| [`docs/protocol-p3.md`](docs/protocol-p3.md) | AMB P3 プロトコル仕様(TS パーサ実装の根拠) |
| [`docs/ci-cd.md`](docs/ci-cd.md) | GitHub Actions の構成と運用方針 |
| [`docs/test-strategy.md`](docs/test-strategy.md) | テスト方針と Mock/Replay/Record 運用 |
| [`docs/roadmap.md`](docs/roadmap.md) | 実装フェーズの順序と各フェーズの責務(採取先行方針) |

UI デザイン仕様(`docs/design.md`)は実装フェーズで追加予定。

---

## 開発状況とロードマップ

- ✅ **準備フェーズ**(進行中): 仕様書整備、開発運用規約、テンプレート整備
- 🚧 **実装フェーズ**(これから):「**採取先行**」方針 — まず最小ゲートウェイで実機データを採取し、それを根拠に TS パーサを実装する
  1. `gateway-recorder` MVP(TCP + `--record` + `--mock`、現地持ち込み品質)
  - ★ 実 LAN 現地データ採取セッション
  2. P3 パーサ TS 実装(採取データを fixture に)
  3. `gateway-full`(WS fan-out + `go:embed` + `/healthz` + `/admin`)
  4. SPA 骨格 → 5. ラップ計算と表示 → 6. 音声読み上げ
  - ★ Field Test α
  7. replay モード → 8. 設定 WebUI
  - ★ Field Test β
  9. リリース自動化(`v0.1.0`)
- 詳細は **[`docs/roadmap.md`](docs/roadmap.md)** を参照

---

## 開発に参加する場合

[`docs/development-workflow.md`](docs/development-workflow.md) を読んでから着手してください。要点:

- 1 PR = 1 トピック
- Issue → PR で履歴化、Conventional Commits
- 仕様変更は同 PR に `docs/` 更新を含める
- 開発は **Web 版 Claude Code(統制)+ ローカル Windows 版 Claude Code(実装/検証)+ 人間(マージ判断)** の 3 者体制(同 §8)

---

## 参考実装

- [hama-jp/AMB_Lap_Speak](https://github.com/hama-jp/AMB_Lap_Speak/tree/master) — Python/Flask による先行実装。本プロジェクトの P3 仕様書 ([`docs/protocol-p3.md`](docs/protocol-p3.md)) はここの `AmbP3/` 配下を読解して整理した

## ライセンス

[LICENSE](LICENSE) を参照。
