# AMB P3 Protocol Specification

本書はゲートウェイから受信した AMB P3 デコーダーの **TCP バイト列を、ブラウザ(TypeScript)でデコードするために必要な仕様** をまとめたものである。実装根拠は参考実装 [hama-jp/AMB_Lap_Speak](https://github.com/hama-jp/AMB_Lap_Speak/tree/master) の `AmbP3/` 配下(`decoder.py` / `crc16.py` / `records.py` / `time_client.py`)。

> Status: **Draft v0.1**(参考実装読解ベース。実機検証で更新予定)

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
| 2      | 2     | **Frame Length** | **little-endian**| フレーム長(範囲は要検証、§9 参照)。本書では一貫して **Frame Length** と呼び、§5 の Field Length と区別する |
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

| ID    | 名称              |
|-------|-------------------|
| `0x01`| `PASSING_NUMBER`  |
| `0x03`| `TRANSPONDER`     |
| `0x04`| `RTC_TIME`        |
| `0x05`| `STRENGTH`        |
| `0x06`| `HITS`            |
| `0x08`| `FLAGS`           |
| `0x0A`| `TRAN_CODE`       |
| `0x0E`| `USER_FLAG`       |
| `0x0F`| `DRIVER_ID`       |
| `0x10`| `UTC_TIME`        |
| `0x13`| `RTC_ID`          |
| `0x14`| `SPORT`           |
| `0x30`| `VOLTAGE`         |
| `0x31`| `TEMPERATURE`     |

ラップタイム計算には主に `TRANSPONDER` と `RTC_TIME` を用いる(§8)。

### 7.4 STATUS(`0x0002`)

| ID    | 名称           |
|-------|----------------|
| `0x01`| `NOISE`        |
| `0x06`| `GPS`          |
| `0x07`| `TEMPERATURE`  |
| `0x0A`| `SATINUSE`     |
| `0x0B`| `LOOP_TRIGGERS`|
| `0x0C`| `INPUT_VOLTAGE`|

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

---

## 8. 時刻と RTC_TIME

- `PASSING.RTC_TIME` および `GET_TIME.RTC_TIME` がデコーダー内部時計のタイムスタンプ。
- 単位・基準時刻は参考実装からは厳密に確定できない(`time_client.py` は単に整数を取り出してアプリ層に渡すのみ)。**初期実装では「デコーダーから受信した整数を相対時刻として扱う」**(同一接続内で `RTC_TIME` の差分=経過時間と仮定)。
- `UTC_TIME` を併用するか、`GET_TIME` 応答の値で起点を補正するかは、実機検証で詰める(§9)。

ラップタイム算出ロジックの初期方針:
- 同一トランスポンダー ID に対する連続する `PASSING.RTC_TIME` の差分をラップタイムとする
- 単位は **要検証**(マイクロ秒/ミリ秒/任意単位)

---

## 9. オープン項目(実機検証で埋める)

| # | 項目 | 影響範囲 |
|---|------|----------|
| 1 | ヘッダ `Frame Length` フィールドの示す範囲(SOR 込み? エスケープ前/後?) | 受信バッファ管理 |
| 2 | CRC の計算対象範囲と検証手順 | データ整合性 |
| 3 | `RTC_TIME` の単位と基準時刻 | ラップタイム表示 |
| 4 | Flags フィールド(ヘッダ)の各ビット意味 | 例外/再送判定 |
| 5 | `PASSING.HITS` / `STRENGTH` / `FLAGS` の値域・解釈 | 表示と除外判定 |
| 6 | `RESEND` / `WATCHDOG` の運用フロー | 接続安定性 |
| 7 | エンコード(送信)側の仕様一式 | 本プロジェクトでは現状不要 |
| 8 | 実機 `Version` フィールドの組み合わせ確認 | 互換性ログ |

→ 実機接続テスト時に派生 Issue を起票して順次解決する。

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
- v0.1.1 (2026-05-04): 用語明確化(Frame Length / Field Length)、TOR 表に LE/BE 注記とワイヤ列を追加。仕様の意味は変更なし。
- v0.1 (2026-05-04): 初版。参考実装の静的読解ベース。実機検証は未。
