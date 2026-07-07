# Milestone 0: 基礎実装（完了）

## 概要
学習計画の出発点となる、実装済みベースノードのスナップショットです。Milestone 1（基本RPCの拡充）とMilestone 2（コード分割）の完了後の状態を記録しています。

このドキュメントの役割:

- 以降のマイルストーンが前提とする「現在の実装」の正確な記述
- Milestone 3（自動テスト）でテストに固定すべき振る舞いの一覧
- 既知の問題の記録（どのマイルストーンまでに直すべきかを含む）

## パッケージ構成

```text
like-a-ethereum/
├── cmd/minieth/main.go   # CLI（account / sign / node）
├── internal/
│   ├── account/          # Account型、アドレス導出・正規化
│   ├── block/            # Block型、ブロックハッシュ、RPC用整形（ToRPCBlock）
│   ├── chain/            # Chain（状態・ブロック・インデックス管理、状態遷移）
│   ├── crypto/           # HashJSON（util委譲の薄いラッパー）
│   ├── rpc/              # JSON-RPCサーバー（dispatch、パラメータ解析）
│   ├── state/            # 空パッケージ（将来拡張用のプレースホルダ）
│   ├── tx/               # Transaction型、署名対象、検証、ハッシュ
│   └── util/             # hex変換、HashJSON、ToHex、ParseHexQuantity
├── genesis.json          # 初期残高（alloc）
├── Makefile              # build / run / clean / test / lint
└── go.mod                # module like-a-ethereum（外部依存なし、go 1.26）
```

## データ構造（実コード）

```go
// internal/account
type Account struct {
    Address string
    Balance uint64
    Nonce   uint64
}

// internal/tx
type Transaction struct {
    From      string
    To        string
    Value     uint64
    Nonce     uint64
    PubKey    string
    Signature string
    Hash      string
}

type TxLocation struct {
    BlockNumber uint64
    TxIndex     int
}

// internal/block
type Block struct {
    Number       uint64
    ParentHash   string
    Timestamp    int64
    Transactions []tx.Transaction
    StateRoot    string
    Hash         string
}

// internal/chain
type Chain struct {
    mu         sync.Mutex
    State      map[string]account.Account
    Blocks     []block.Block
    TxIndex    map[string]tx.TxLocation // txハッシュ -> ブロック番号+ブロック内位置
    BlockIndex map[string]uint64        // ブロックハッシュ -> ブロック番号
}
```

## 暗号と識別子

| 項目 | 実装 |
|---|---|
| 鍵ペア | Ed25519（標準ライブラリ `crypto/ed25519`） |
| ハッシュ | SHA-256。`util.HashJSON(v)` = JSONエンコード → SHA-256 → `0x`+64hex |
| アドレス | `SHA-256(公開鍵32バイト)` の**末尾20バイト**を `0x`+40hexで表記。Ethereumの `Keccak256(pubkey)[12:]` の構造を模したもの |
| アドレス正規化 | 小文字化・trim・`0x`必須・40hexチェック（`account.NormalizeAddress`） |

署名対象とtxハッシュの対象は**異なる**（Milestone 3のテスト観点として重要）:

```go
// 署名対象: from|to|value|nonce のパイプ区切り文字列。PubKey/Signatureは含まない
func TxSignBytes(tx Transaction) []byte {
    return []byte(fmt.Sprintf("%s|%s|%d|%d",
        strings.ToLower(tx.From), strings.ToLower(tx.To), tx.Value, tx.Nonce))
}
```

- txハッシュ（`tx.TxHash`）: from / to / value / nonce / pubKey / **signature** まで含めた6フィールドのJSONのSHA-256
- ブロックハッシュ（`block.BlockHash`）: number / parentHash / timestamp / txハッシュ列 / stateRoot のJSONのSHA-256

## stateRoot

全アカウントをアドレス昇順にソートし、`{address, balance, nonce}` の配列をJSON化してSHA-256を取ったもの（`chain.computeStateRootLocked`）。Merkle Treeではありません（Milestone 6で置き換え予定）。

## 状態遷移（chain.AddTransaction）

`sendTransaction` 1回につき以下を実行し、**即座に1txのブロックを生成します**（mempoolなし。Milestone 4で分離予定）:

1. `tx.VerfyAndNormalizeTx`: アドレス正規化 → 公開鍵とfromアドレスの一致確認 → Ed25519署名検証 → txハッシュ再計算
2. from存在チェック（エラー: `from account does not exist`）
3. nonce一致チェック（エラー: `bad nonce: got X, want Y`）
4. 残高チェック（エラー: `insufficient balance`）
5. 宛先が未知ならAccountを新規作成（balance 0 / nonce 0）
6. `from.Balance -= value; from.Nonce++; to.Balance += value`
7. 新ブロックを生成（StateRootは**遷移後**の状態から計算）し、`TxIndex` / `BlockIndex` を更新

排他制御はChain全体で単一の `sync.Mutex`。RPC層は公開メソッド `Chain.Lock()` / `Chain.Unlock()` で直接ロックを取る設計です。

## Genesis

```json
{ "alloc": { "0x<address>": 1000 } }
```

- genesisブロック: Number 0、ParentHashはゼロ32バイト（`0x00...0`）、**Timestampは起動時刻**、StateRootはalloc反映後の状態から計算
- Timestampが起動時刻のため、**同じgenesis.jsonでもgenesis hashは起動ごとに変わります**（既知の問題1）

## JSON-RPC

エンドポイントは `POST /` のみ（POST以外は405）。paramsは配列形式（`sendTransaction` のみ配列1要素と生オブジェクトの両対応）。

| メソッド | params | 挙動 |
|---|---|---|
| `eth_blockNumber` | `[]` | 最新ブロック番号（hex quantity） |
| `eth_getBalance` | `[address, blockTag]` | 残高。未知アドレスは `0x0`。blockTagは無視（常に最新状態） |
| `eth_getTransactionCount` | `[address, blockTag]` | nonce。未知アドレスは `0x0`。blockTagは無視 |
| `eth_getTransactionByHash` | `[hash]` | tx詳細。未知hashはnull（既知の問題3・5に注意） |
| `eth_getBlockByHash` | `[hash, fullTx]` | ブロック。未知hashはnull。fullTx=falseならtxハッシュ列のみ |
| `eth_getBlockByNumber` | `["latest" \| "0xN", fullTx]` | ブロック。範囲外はnull |
| `sendTransaction` | `[txオブジェクト]` | 検証 → 状態遷移 → 即ブロック生成。結果は `{transactionHash, blockNumber, blockHash, stateRoot}` |

エラーコード:

| code | 意味 |
|---|---|
| -32700 | JSONパースエラー |
| -32601 | method not found |
| -32602 | パラメータ不正。**業務エラー（bad nonce / insufficient balance / invalid signature等）もすべてここに畳まれている**（既知の問題6） |

## CLI

```bash
minieth account   # Ed25519鍵ペア生成（address / publicKey / privateKey をJSONで出力）
minieth sign --priv <hex> --to <address> --value <n> --nonce <n>   # 署名済みtxをJSONで出力
minieth node --genesis genesis.json --addr :8545                    # ノード起動
```

## 動作確認済みの振る舞い（READMEのテスト手順）

Milestone 3ではこれらをGoテストとして固定します:

1. `make build` が成功する
2. `minieth account` が `0x`+40hexのaddressを出力する
3. 起動直後の `eth_blockNumber` が `0x0`
4. `sendTransaction` 成功後に `eth_blockNumber` が `0x1` になる
5. 送金後の残高が正しく移動している（例: 1000/0 → 850/150）
6. 同一txの再送が `bad nonce` で失敗する
7. 残高を超える送金が `insufficient balance` で失敗する

## 既知の問題

| # | 内容 | 直す時期 |
|---|---|---|
| 1 | genesisのTimestampが `time.Now()` のため、genesis hashが起動ごと・ノードごとに変わる。永続化（M7）の再起動同一性や、P2P（M9）の「全ノードが同一genesisを持つ」前提を壊す | **遅くともM9まで**（テストの再現性向上のためM3で直すのが望ましい。genesis.jsonにtimestampフィールドを持たせるか固定値にする） |
| 2 | `tx.VerfyAndNormalizeTx` は関数名がtypo（Verify → Verfy） | M3（テスト整備時にリネーム） |
| 3 | `eth_getTransactionByHash` のレスポンスで `blockNumber` だけhex quantityでなく数値のまま返る（他フィールドと不整合） | M3 |
| 4 | `eth_getBlockByHash` のパラメータエラーメッセージ末尾にデバッグ用の `strconv.Itoa(len(arr))` が連結されたまま | M3 |
| 5 | resultがnullのとき、レスポンス構造体の `omitempty` により `result` フィールドごと消える。JSON-RPC 2.0では成功時 `result` は必須なので `"result": null` を返すべき | M3 |
| 6 | 業務エラーがすべて `-32602 invalid params` にマップされる（本来はサーバ定義のエラーコードを分けるべき） | M3以降（エラーコード整理のタイミングで） |
| 7 | 秘密鍵をCLI引数で渡す設計（シェル履歴・psに残る）。学習用として許容 | 対応しない（記録のみ） |

## Ethereumとの違い（このベース実装時点）

| 項目 | Ethereum | この実装 |
|---|---|---|
| 署名方式 | secp256k1 | Ed25519 |
| ハッシュ | Keccak-256 | SHA-256 |
| stateRoot | Merkle Patricia Trie | ソート済みアカウント一覧のSHA-256 |
| トランザクション形式 | RLP / Typed Transaction | JSON |
| ブロック生成 | PoS / Consensus | `sendTransaction` 時に即ブロック生成 |
| 送信RPC | `eth_sendRawTransaction` | 独自 `sendTransaction` |
| P2P / EVM / Gas | あり | なし |

## 完了条件（記録）

- READMEのクイックスタートとテスト手順1〜7が手動で確認済み
- `make build` / `go vet ./...` が通る
- 自動テストは未整備（Milestone 3で追加する）
