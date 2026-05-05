# AMB P3 Protocol Specification

本書はゲートウェイから受信した AMB P3 デコーダーの **TCP バイト列を、ブラウザ(TypeScript)でデコードするために必要な仕様** をまとめたものである。実装根拠は参考実装 [hama-jp/AMB_Lap_Speak](https://github.com/hama-jp/AMB_Lap_Speak/tree/master) の `AmbP3/` 配下(`decoder.py` / `crc16.py` / `records.py` / `time_client.py`)。

> Status: **Draft v0.1.2**(2026-05-05 採取データで §9 主要項目を確定)

---

## 1. 接続パラメータ
- トランスポート: **TCP**
- デフォルトポート: **5403**(参考実装より)
- 接続種別: クライアントから AMB デコーダーへ常時接続。デコーダーが受動的に passing イベントなどを送出。
- 受信バッファ目安: **10240 bytes**(参考実装の `recv` サイズ)

ゲートウェイは本接続を握り、受信バイト列をそのまま WebSocket クライアントへ fan-out する(`docs/gateway-technical-decision.md` 第4節)。

---

## 2. フレーム構造の概略

```
+------+---------+-----------------------------+------+
| SOR  | Header  |          Body (TLV...)      | EOR  |
| 0x8E | 9 bytes |     可変長(escape された)     | 0x8F |
+------+---------+-----------------------------+------+
```

- **SOR**(Start Of Record): 単一バイト `0x8E`
- **EOR**(End Of Record): 単一バイト `0x8F`
- SOR と EOR の間のすべてのバイトは **エスケープ規則**(§4)に従う。受信側は SOR/EOR を取り除き、エスケープを解いた**素のバイト列**に対して以降の解釈を行う。

### 2.1 連結セパレータ
1 回の TCP `recv` で複数レコードが連続して届く場合、レコード境界は **`0x8F 0x8E`(EOR の直後に SOR)** で識別する。受信側は

1. ストリームを `0x8F 0x8E` で分割
2. 各レコードの先頭が `0x8E`、末尾が `0x8F` であることを確認
3. 各レコードを §4 でエスケープ解除

の手順でレコード単位に切り出す。

> 実装メモ: TCP は連続バイトストリームで、1 つのレコードが複数 `recv` にまたがる、または 1 `recv` に複数レコードが入ることが起こり得る。**フレームバッファは持続させ、`0x8F 0x8E` を見つけたら 1 レコードを払い出す**実装にする。

---

## 3. ヘッダ(SOR を含めて 10 バイト)

エスケープ解除後、フレーム先頭から 10 バイトがヘッダ。

| Offset | Bytes | Field          | エンディアン     | 備考 |
|-------:|------:|----------------|------------------|------|
| 0      | 1     | SOR            | -                | 必ず `0x8E` |
| 1      | 1     | Version        | -                | プロトコルバージョン |
| 2      | 2     | **Frame Length** | **little-endian**| **unescaped 全長(SOR + 9 byte header + body + EOR、escape 解除後)。** 2026-05-05 採取 58 frame で全件一致(§9 #1 確定)。本書では一貫して **Frame Length** と呼び、§5 の Field Length と区別する |
| 4      | 2     | CRC            | little-endian    | §6 |
| 6      | 2     | Flags          | little-endian    | 詳細未確認 |
| 8      | 2     | TOR            | little-endian    | Type Of Record。§7 |

> **エンディアン**: 多バイト値は **すべてリトルエンディアン**(参考実装は `bytes[::-1]` で逆順にして hex 表示)。受信側は `DataView` の `getUint16(offset, /*littleEndian*/ true)` 等で読む。

---

## 4. バイトエスケープ規則

SOR/EOR/エスケープ印を区別するため、ボディ中の以下のバイトはエスケープされる。

| 生バイト | 符号化(2 バイト) |
|---------:|-----------------:|
| `0x8D`   | `0x8D 0xAD` (`0x8D` + `0x8D + 0x20`) |
| `0x8E`   | `0x8D 0xAE` (`0x8D` + `0x8E + 0x20`) |
| `0x8F`   | `0x8D 0xAF` (`0x8D` + `0x8F + 0x20`) |

**復号アルゴリズム**:
1. 先頭バイト(SOR `0x8E`)と末尾バイト(EOR `0x8F`)を取り除く
2. 残りを左から走査し、`0x8D` を見つけたら次のバイトを「次バイト - `0x20`」に置換し、`0x8D` 自体は捨てる
3. 走査後、必要なら頭尾に `0x8E` / `0x8F` を再付与してヘッダ計算位置を保つ

**TypeScript 実装の擬似コード**:
```ts
function unescape(frame: Uint8Array): Uint8Array {
  // frame は SOR/EOR を含む 1 レコード。
  const inner = frame.subarray(1, frame.length - 1);
  const out: number[] = [];
  let escapeNext = false;
  for (const b of inner) {
    if (escapeNext) { out.push(b - 0x20); escapeNext = false; continue; }
    if (b === 0x8D) { escapeNext = true; continue; }
    out.push(b);
  }
  return new Uint8Array([0x8E, ...out, 0x8F]);
}
```

> ⚠ **参考実装の不具合に注意**: `decoder.py:_unescape` のエスケープ起動条件が `byte in [141, 141, 142]`(= `[0x8D, 0x8D, 0x8E]`)となっており、**`0x8F` のエスケープ復号に対応していない**。同ファイルにある `_lunescape` は `[b'8d', b'8e', b'8f']` で正しい。本仕様では「**エスケープ起動印は `0x8D` のみ**(`0x8E` / `0x8F` はエスケープされた本来のデータ内には現れない、SOR/EOR 以外の位置で生で出現することは無い)」とする。実装ではエスケープ起動を `0x8D` で判定すれば十分。

---

## 5. ボディ(TLV 列)

ヘッダに続くバイト列は、レコード末尾 `0x8F` (内側用)に到達するまで、以下の TLV 構造を繰り返す。

```
+-------------+--------------+----------------------------+
| Field ID    | Field Length |  Value (Field Length bytes)|
|   1 byte    |    1 byte    |       little-endian        |
+-------------+--------------+----------------------------+
```

> 用語: 本書では §3 ヘッダの長さフィールドを **Frame Length**、本節の TLV の長さフィールドを **Field Length** と呼び分ける。

- **Field ID**(1 byte): `0x81` などのレコード属性 ID。TOR ごとの定義(§7)を引く。`GENERAL`(§7.1)に該当する場合は共通フィールド。
- **Field Length**(1 byte): 値の長さ(バイト数)。`uint8` として読む。
- **Value**(Field Length bytes): リトルエンディアン。文字列か数値かは Field ID により決まる(§9 で精緻化予定)。

**終端条件**: ボディ走査中に `0x8F` を読んだら TLV 列の終端とみなす(参考実装と同等)。

> ⚠ **参考実装の不具合に注意**: `decoder.py:_decode_record` で Field Length バイトを `int(codecs.encode(byte, 'hex'))` で取得しており、`0x10` が **10**(本来は 16)として解釈される。本仕様では Field Length は **`uint8` として直接読む**(`bytes[1]`)。

> **観測上の補足(2026-05-05 採取)**: PASSING の `id=0x01 PASSING_NUMBER` は **デコーダー全体のグローバル連番(4-byte LE 単調増加)** で、ポンダー別の周回回数ではない。本セッションでは `0x001185E3〜0x001185F1` の 15 連番、欠番なし。実装側で「自分の周回回数」を出すには、自ポンダーの PASSING を 1 から数え直す必要がある。

---

## 6. CRC16

参考実装 `crc16.py` より:

| 項目        | 値       |
|-------------|----------|
| 多項式      | `0x1021` |
| 初期値      | `0xFFFF` |
| 入力反転    | なし     |
| 出力反転    | なし     |
| Final XOR   | なし     |
| 結果のバイトスワップ | あり(`(crc << 8 \| crc >> 8) & 0xFFFF`) |

これはいわゆる **CRC-16/CCITT-FALSE** に近いが、**最終結果のハイ/ローバイトを入れ替えてから出力**する点が特殊。フレームに格納されるのはバイトスワップ後の値。

**CRC 適用範囲**: 参考実装 `_check_crc` は単にデータを返すスタブで、**実際の検証ロジックは未実装**。仕様としての確定範囲は次のいずれかが想定される(要検証):
- ヘッダの CRC フィールドを 0 にした上で「ヘッダ+ボディ(エスケープ解除後、SOR/EOR 除く)」を計算
- ヘッダ Version 〜 ボディ末尾を計算

→ §9 のオープン項目として残す。**初期実装では CRC 検証をスキップし、ログのみ出力**で運用する(LAN 内信頼前提)。

---

## 7. TOR(Type Of Record)とフィールド定義

参考実装 `records.py` を一次ソースとして転記。Field ID は **TOR ごとの辞書 + 共通(`GENERAL`)** をマージして引く。

### 7.1 GENERAL(全 TOR 共通)

| ID    | 名称           |
|-------|----------------|
| `0x81`| `DECODER_ID`   |
| `0x83`| `CONTROLLER_ID`|
| `0x85`| `REQUEST_ID`   |

### 7.2 TOR 一覧

TOR は 16 ビット値(リトルエンディアン)。

> **表記注**: 下表の `TOR (表示)` 列は **可読性のため big-endian で表示**(例: `0x0001` = PASSING)。実際のワイヤ上は **little-endian** で並ぶ(`0x0001` は `01 00`)。実装では `DataView.getUint16(offset, /*littleEndian*/ true)` で読み、その結果と本表の値を比較する。

| TOR (表示, BE) | ワイヤ上のバイト列 (LE) | 名称              | フィールド辞書 |
|---------:|:------------------------|-------------------|----------------|
| `0x0000` | `00 00`                 | `RESET`           | (無)           |
| `0x0001` | `01 00`                 | `PASSING`         | §7.3           |
| `0x0002` | `02 00`                 | `STATUS`          | §7.4           |
| `0x0003` | `03 00`                 | `VERSION`         | §7.5           |
| `0x0004` | `04 00`                 | `RESEND`          | §7.6           |
| `0x0005` | `05 00`                 | `CLEAR_PASSING`   | (無)           |
| `0x0013` | `13 00`                 | `SERVER_SETTINGS` | (無)           |
| `0x0015` | `15 00`                 | `SESSION`         | (無)           |
| `0x0016` | `16 00`                 | `NETWORK_SETTINGS`| (無)           |
| `0x0018` | `18 00`                 | `WATCHDOG`        | (無)           |
| `0x001C` | `1C 00`                 | (undocumented; 接続直後の handshake 系と推定) | §7.9 |
| `0x0020` | `20 00`                 | `PING`            | (無)           |
| `0x0024` | `24 00`                 | `GET_TIME`        | §7.7           |
| `0x0028` | `28 00`                 | `GENERAL_SETTINGS`| (無)           |
| `0x002D` | `2D 00`                 | `SIGNALS`         | (無)           |
| `0x002F` | `2F 00`                 | `LOOP_TRIGGER`    | (無)           |
| `0x0030` | `30 00`                 | `GPS_INFO`        | (無)           |
| `0x0045` | `45 00`                 | `FIRST_CONTACT`   | (無)           |
| `0x004A` | `4A 00`                 | `TIMELINE`        | (無)           |
| `0xFFFF` | `FF FF`                 | `ERROR`           | §7.8           |

### 7.3 PASSING(`0x0001`)— **本アプリの主たる関心**

| ID    | 名称              | Field Length | 観測メモ(2026-05-05) |
|-------|-------------------|--------------|------------------------|
| `0x01`| `PASSING_NUMBER`  | 4            | デコーダーグローバル連番(LE)。本セッションで `0x001185E3〜0x001185F1` 連続。**ポンダー別の周回回数ではない**(§5 補足参照) |
| `0x03`| `TRANSPONDER`     | 4            | LE。本リポジトリの captured fixture では匿名化済み synthetic ID(`0x00000001` / `0x00000002`)。生 ID は MyLaps 個人識別子相当 |
| `0x04`| `RTC_TIME`        | 8            | **単位 = μs**(§8 確定)。LE。同一ポンダー連続 PASSING の差分が 21 sec 前後 ≒ 典型 RC EP ラップ |
| `0x05`| `STRENGTH`        | 2            | LE。観測値域 162-174(15 サンプル)。実値域は 1 byte に収まるが Field Length は 2 |
| `0x06`| `HITS`            | 2            | LE。観測値域 119-287(15 サンプル)。**1 byte を超える例あり**(0x0117 等)。2 byte LE で読むこと |
| `0x08`| `FLAGS`           | 2            | LE。本セッションは全 15 frame で `0x0000`。bit 意味は未確定(§9 #4) |
| `0x0A`| `TRAN_CODE`       | (未観測)     | (本セッションでは出現せず) |
| `0x0E`| `USER_FLAG`       | (未観測)     | (本セッションでは出現せず) |
| `0x0F`| `DRIVER_ID`       | (未観測)     | (本セッションでは出現せず) |
| `0x10`| `UTC_TIME`        | (未観測)     | (本セッションでは出現せず) |
| `0x13`| `RTC_ID`          | (未観測)     | (本セッションでは出現せず) |
| `0x14`| `SPORT`           | (未観測)     | (本セッションでは出現せず) |
| `0x30`| `VOLTAGE`         | (未観測)     | (本セッションでは出現せず) |
| `0x31`| `TEMPERATURE`     | (未観測)     | (本セッションでは出現せず) |

ラップタイム計算には主に `TRANSPONDER` と `RTC_TIME` を用いる(§8)。**`PASSING_NUMBER` は周回回数ではないので注意**。

> 観測されたフィールド構成(本セッション全 15 frame):
> `PASSING_NUMBER` (id=0x01, 4) + `TRANSPONDER` (id=0x03, 4) + `RTC_TIME` (id=0x04, 8) + `STRENGTH` (id=0x05, 2) + `HITS` (id=0x06, 2) + `FLAGS` (id=0x08, 2) + `DECODER_ID` (id=0x81, 4) = TLV 部 40 byte。SOR(1) + ヘッダ 9 + TLV 40 + EOR(1) = unescaped 51 byte = ヘッダの Frame Length 値と完全一致。

### 7.4 STATUS(`0x0002`)

| ID    | 名称           | Field Length | 観測メモ(2026-05-05) |
|-------|----------------|--------------|------------------------|
| `0x01`| `NOISE`        | 2            | 観測値 `0x0006`(LE)。全 42 frame で同値 |
| `0x06`| `GPS`          | 1            | 観測値 `0x00`(屋外コースだが GPS 機能 OFF または非対応か) |
| `0x07`| `TEMPERATURE`  | 2            | 観測値 `0x001B` = 27(LE)。℃ と推定 |
| `0x0A`| `SATINUSE`     | (未観測)     | (本セッションでは出現せず) |
| `0x0B`| `LOOP_TRIGGERS`| (未観測)     | (本セッションでは出現せず) |
| `0x0C`| `INPUT_VOLTAGE`| 1            | 観測値 `0x79` = 121。電圧の 100 倍などの単位推定(12.1 V?)、要追加採取で確定 |

> 観測されたフィールド構成(本セッション全 42 frame):
> `NOISE` (id=0x01, 2) + `TEMPERATURE` (id=0x07, 2) + `INPUT_VOLTAGE` (id=0x0C, 1) + `GPS` (id=0x06, 1) + `DECODER_ID` (id=0x81, 4) = TLV 部 21 byte。unescaped 31 byte = ヘッダの Frame Length 値と完全一致。

### 7.5 VERSION(`0x0003`)

| ID    | 名称            |
|-------|-----------------|
| `0x01`| `DECODER_TYPE`  |
| `0x02`| `DESCRIPTION`   |
| `0x03`| `VERSION`       |
| `0x04`| `RELEASE`       |
| `0x08`| `REGISTRATION`  |
| `0x0A`| `BUILD_NUMBER`  |
| `0x0C`| `OPTIONS`       |

`DECODER_TYPE` の値域:

| 値    | 機種        |
|-------|-------------|
| `0x10`| AMBrc       |
| `0x11`| AMBMX       |
| `0x12`| TranX       |
| `0x13`| TranX Pro   |
| `0x14`| TranX Pro   |

### 7.6 RESEND(`0x0004`)

| ID    | 名称   |
|-------|--------|
| `0x01`| `FROM` |
| `0x02`| `UNTIL`|

### 7.7 GET_TIME(`0x0024`)

| ID    | 名称       |
|-------|------------|
| `0x01`| `RTC_TIME` |
| `0x04`| `FLAGS`    |
| `0x05`| `UTC_TIME` |

### 7.8 ERROR(`0xFFFF`)

| ID    | 名称          |
|-------|---------------|
| `0x1` | `CODE`        |
| `0x02`| `DESCRIPTION` |

`CODE` の値域(参考実装 `ERROR_CODES`):

| Code     | 内容                  |
|----------|-----------------------|
| `0x0001` | CRC Error             |
| `0x0002` | SOR Missing           |
| `0x0003` | Header corrupt        |
| `0x0004` | TOR Unknown           |
| `0x0005` | Parameters missing    |
| `0x0006` | Length of record too long |

> ⚠ 参考実装で `ERROR` 辞書のキー `b'1'` は 1 バイトでなく 1 文字の hex 表現。正しくは `0x01` の Field ID と思われる(要検証)。

### 7.9 (undocumented) `0x001C`(2026-05-05 採取で観測)

参考実装の `records.py:type_of_records` には載っていない TOR が、実機接続直後の **1 フレームだけ** 観測された:

| ID    | 名称(推定)        | Field Length | 観測値(2026-05-05) |
|-------|---------------------|--------------|----------------------|
| `0x01`| (handshake payload) | 8            | `0x6171a759d7df5a37`(LE 8 byte、ランダム/nonce 風) |
| `0x81`| `DECODER_ID` (general) | 4         | `0x00041D17` |

unescaped frame 全長 27 byte(`Frame Length` フィールドと一致)、CRC `0x8e0b`、Flags `0x0000`、TOR `0x001C`。

**性質の推定**: AMB 接続直後の handshake / hello / session-id 配布系。spec §7.2 の `FIRST_CONTACT` (`0x0045`) とは TOR 値が異なるため別レコード。実機ベンダ仕様待ち。実装側では **未文書 TOR として認識して捨てる or ログのみ**で問題ない(本アプリの主たる関心 = PASSING には影響しない)。

---

## 8. 時刻と RTC_TIME

- `PASSING.RTC_TIME` および `GET_TIME.RTC_TIME` がデコーダー内部時計のタイムスタンプ。
- **単位 = μs(マイクロ秒)。8-byte little-endian の符号なし整数**。2026-05-05 採取で確定:
  - 同一トランスポンダー(synth-1)の連続する `PASSING.RTC_TIME` の差分は **20.8〜21.9 sec**(μs 解釈)。典型 RC EP ラップ秒(15-30 sec)と整合。他の単位仮定(ms / 100ns ticks / 10μs 等)はいずれも非現実値。
  - 異なるトランスポンダー間の差分も時系列順に単調増加し、レコード生成順と一致。
- **基準時刻**: デコーダー内部 RTC。**桁感は Unix epoch (1970-01-01 UTC) と一致**(2026-05-05 採取分の絶対値が 2026-05-05 02:19 UTC 相当)するが、ノートしたゲートウェイ起動時刻(11:35 JST = 02:35 UTC)より約 16 分早く、**実際には decoder の自走 RTC が UTC からドリフトしている**と推定。**ラップタイム計算は差分のみで成立するため、絶対値の正確性に依存しない**。
- `UTC_TIME` を併用するか、`GET_TIME` 応答の値で起点を補正するかは将来の判断(`docs/architecture.md` § ロードマップ #5 / #6 検討時)。

ラップタイム算出ロジック(確定):
- 同一トランスポンダー ID に対する連続する `PASSING.RTC_TIME` の差分をラップタイムとする
- 値の単位を **μs として扱い**、表示時に ms/sec へ変換(`Math.floor(diff / 1000)` 等)

---

## 9. オープン項目(2026-05-05 採取後の状況)

| # | 項目 | 状況 | 根拠 |
|---|------|------|------|
| 1 | ヘッダ `Frame Length` フィールドの示す範囲 | ✅ **確定**: SOR/EOR 込みの **unescaped 全長** | 採取 58/58 frame で `flen` フィールドと unescape 後の全長が完全一致 |
| 2 | CRC の計算対象範囲と検証手順 | ⏸ **未確定**(ベンダ仕様待ち) | 標準 CCITT-FALSE / X-25 / KERMIT / AUG-CCITT 等 50+ 組合せ × 12 候補範囲(SOR込/抜、ヘッダ込/抜、CRC ゼロ化、wire/unescaped 等)で一致せず。AMB 独自実装の可能性。**初期実装は引き続き検証スキップ + ログのみ非ブロッキング**(`docs/architecture.md` §3.1 の `/healthz` 等で観察) |
| 3 | `RTC_TIME` の単位と基準時刻 | ✅ **確定**: 単位 = **μs**(§8) | 同一ポンダー連続 PASSING の差分が 20.8〜21.9 sec で典型 RC ラップと整合 |
| 4 | Flags フィールド(ヘッダ)の各ビット意味 | ⚠ **部分**: 全 58 frame で `0x0000` | 異常系(エラー応答 / 再送)が含まれていなかったため bit 意味は本セッションで検出できず。Field Test α の現地観察で追加サンプル必要 |
| 5 | `PASSING.HITS` / `STRENGTH` / `FLAGS` の値域・解釈 | ✅ **確定(本セッション範囲)**: STRENGTH 162-174, HITS 119-287, FLAGS 全 0(§7.3 観測メモ参照) | 15 サンプル。HITS は 1 byte 超の値あり、必ず 2 byte LE で読むこと |
| 6 | `RESEND` / `WATCHDOG` の運用フロー | ⏸ 採取範囲外 | 本セッションでは観測せず |
| 7 | エンコード(送信)側の仕様一式 | ⏸ 本プロジェクトでは現状不要 | byte pipe 方針(`docs/gateway-technical-decision.md` 第4節)維持 |
| 8 | 実機 `Version`(TOR `0x0003`)フィールドの組み合わせ確認 | ⏸ 観測なし | 本セッションでは TOR `0x0003` は出現せず。代わりに **未文書 TOR `0x001C`** が接続直後に 1 frame 出現(§7.9)。VERSION とは別物 |

副次的に確認できた事項:
- **PASSING_NUMBER はデコーダーグローバル連番**(ポンダー別の周回回数ではない、§5 補足)
- **STATUS フィールド**(NOISE / GPS / TEMPERATURE / INPUT_VOLTAGE)はすべて参考実装の spec と整合(§7.4 観測メモ)
- **未文書 TOR `0x001C`** を §7.9 として追記、**実装側は未知 TOR として捨てる or ログのみ**で運用

未確定項目は実機接続フェーズ(Field Test α / β)で追加採取して順次解消する。

---

## 10. 参考実装の既知バグ(踏まないこと)

| 場所 | 内容 |
|------|------|
| `decoder.py:_unescape` | エスケープ判定リストが `[141, 141, 142]` で、**`0x8F` を見落とす**。本仕様の擬似コードでは `0x8D` 検出のみで判定する。 |
| `decoder.py:_decode_record` | Field Length を `int(hex(byte))` として **10 進文字列パース**しており、`0x10` 以上で誤読。**`uint8` で直読**すること。 |
| `decoder.py:_check_crc` | CRC 検証スタブで未実装。**初期実装でもログのみで非ブロッキング**にする。 |
| `records.py:ERROR` の `b'1'` | キーが 1 文字でフォーマット不整合。`0x01` と推定。 |

---

## 11. 受信側の参考処理フロー(TS 想定)

1. WebSocket から到着した `Uint8Array` を内部リングバッファに連結
2. `0x8F 0x8E` の出現位置を探し、見つかれば 1 レコードを払い出し
3. 払い出したフレームに対して:
   1. SOR/EOR の妥当性チェック(`frame[0] === 0x8E && frame[length-1] === 0x8F`)
   2. §4 の手順でエスケープ解除
   3. 先頭 10 バイトをヘッダとして §3 を読む
   4. 残りを §5 の TLV 列としてループ展開
   5. `TOR` 値で §7 の辞書を引き、Field ID → 名称マップを適用
   6. `PASSING` ならアプリ層へ通知(対象トランスポンダーフィルタは UI 側で実施)

---

## 12. 改訂履歴
- v0.1.2 (2026-05-05): 2026-05-05 採取データ(`gateway/testdata/captured/session-2026-05-05.bin`)を解析して §9 オープン項目を更新。§9 #1 Frame Length(SOR/EOR 込み unescaped 全長)/ #3 RTC_TIME 単位(μs)/ #5 値域(STRENGTH 162-174 / HITS 119-287)を確定。§7.2 に未文書 TOR `0x001C` を追記し §7.9 で詳細記述。§7.3 / §7.4 に観測メモ列を追加。§5 に PASSING_NUMBER がグローバル連番である補足を追加。§9 #2 CRC 範囲は標準 CRC 候補で一致せず未確定で残置。
- v0.1.1 (2026-05-04): 用語明確化(Frame Length / Field Length)、TOR 表に LE/BE 注記とワイヤ列を追加。仕様の意味は変更なし。
- v0.1 (2026-05-04): 初版。参考実装の静的読解ベース。実機検証は未。
