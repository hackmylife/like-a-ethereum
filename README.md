# mini-eth-node

Goで実装する、学習用の「ミニEthereum風ノード」です。

Ethereum互換ノードではありません。アカウント、残高、nonce、署名付きトランザクション、ブロック、簡易stateRoot、JSON-RPCを小さく実装して、ブロックチェーンの基本概念を理解するための教材です。

## 目的

この実装で学ぶこと:

- アカウントベースの状態管理
- トランザクションによる状態遷移
- 署名検証
- nonceによるリプレイ防止
- ブロック生成
- stateRoot風ハッシュ
- JSON-RPC API

## 実装している機能

### アカウント

```go
type Account struct {
    Address string
    Balance uint64
    Nonce   uint64
}
```

### トランザクション

```go
type Transaction struct {
    From      string
    To        string
    Value     uint64
    Nonce     uint64
    PubKey    string
    Signature string
    Hash      string
}
```

### ブロック

```go
type Block struct {
    Number       uint64
    ParentHash   string
    Timestamp    int64
    Transactions []Transaction
    StateRoot    string
    Hash         string
}
```

### JSON-RPC

| メソッド | 説明 |
|---|---|
| `eth_blockNumber` | 最新ブロック番号を返す |
| `eth_getBalance` | 指定アドレスの残高を返す |
| `sendTransaction` | 署名済みトランザクションを送信する |
| `eth_getBlockByNumber` | 指定番号のブロックを返す |

## Ethereumとの違い

| 項目 | Ethereum | この実装 |
|---|---|---|
| 署名方式 | secp256k1 | Ed25519 |
| ハッシュ | Keccak-256 | SHA-256 |
| stateRoot | Merkle Patricia Trie | ソート済みアカウント一覧のSHA-256 |
| トランザクション形式 | RLP / Typed Transaction | JSON |
| ブロック生成 | PoS / Consensus | `sendTransaction`時に即ブロック生成 |
| 送信RPC | `eth_sendRawTransaction` | 独自 `sendTransaction` |
| P2P | あり | なし |
| EVM | あり | なし |
| Gas | あり | なし |

## 必要環境

- Go 1.22以上推奨
- curl
- jq 任意

## セットアップ

```bash
mkdir mini-eth-node
cd mini-eth-node
go mod init mini-eth-node
```

`main.go` を配置します。

```bash
go build
```

ビルドに成功すれば準備完了です。

## コマンド

```bash
go run . account
go run . sign --priv <privateKeyHex> --to <address> --value <amount> --nonce <nonce>
go run . node --genesis genesis.json --addr :8545
```

## クイックスタート

### 1. Aliceアカウントを作成

```bash
go run . account
```

出力例:

```json
{
  "address": "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
  "privateKey": "0x...",
  "publicKey": "0x..."
}
```

以下の値を控えます。

```bash
ALICE_ADDRESS=0x...
ALICE_PRIVATE_KEY=0x...
ALICE_PUBLIC_KEY=0x...
```

### 2. Bobアカウントを作成

```bash
go run . account
```

以下の値を控えます。

```bash
BOB_ADDRESS=0x...
BOB_PRIVATE_KEY=0x...
BOB_PUBLIC_KEY=0x...
```

### 3. genesis.jsonを作成

Aliceに初期残高1000、Bobに0を割り当てます。

```json
{
  "alloc": {
    "0xAliceのaddress": 1000,
    "0xBobのaddress": 0
  }
}
```

例:

```bash
cat > genesis.json <<'JSON'
{
  "alloc": {
    "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa": 1000,
    "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb": 0
  }
}
JSON
```

実際には、上記のアドレスを自分が生成したAlice/Bobのアドレスに置き換えてください。

### 4. ノードを起動

```bash
go run . node --genesis genesis.json --addr :8545
```

ログ例:

```text
mini ethereum-like node listening on :8545
genesis block hash: 0x... stateRoot: 0x...
```

以降の手順は別ターミナルで実行します。

## JSON-RPCの使い方

### eth_blockNumber

```bash
curl -s -X POST localhost:8545 \
  -H 'Content-Type: application/json' \
  -d '{
    "jsonrpc": "2.0",
    "method": "eth_blockNumber",
    "params": [],
    "id": 1
  }'
```

初期状態ではGenesisブロックのみなので、期待結果は `0x0` です。

```json
{
  "jsonrpc": "2.0",
  "result": "0x0",
  "id": 1
}
```

### eth_getBalance

Aliceの残高を確認します。

```bash
curl -s -X POST localhost:8545 \
  -H 'Content-Type: application/json' \
  -d '{
    "jsonrpc": "2.0",
    "method": "eth_getBalance",
    "params": ["0xAliceのaddress", "latest"],
    "id": 2
  }'
```

Aliceの初期残高が1000の場合、1000は16進数で `0x3e8` です。

```json
{
  "jsonrpc": "2.0",
  "result": "0x3e8",
  "id": 2
}
```

## 送金手順

### 1. AliceからBobへのトランザクションを署名

AliceからBobに150送ります。

```bash
go run . sign \
  --priv 0xAliceのprivateKey \
  --to 0xBobのaddress \
  --value 150 \
  --nonce 0
```

出力例:

```json
{
  "from": "0xAliceのaddress",
  "to": "0xBobのaddress",
  "value": 150,
  "nonce": 0,
  "pubKey": "0x...",
  "signature": "0x...",
  "hash": "0x..."
}
```

このJSONを `tx.json` として保存します。

### 2. sendTransaction

```bash
curl -s -X POST localhost:8545 \
  -H 'Content-Type: application/json' \
  -d '{
    "jsonrpc": "2.0",
    "method": "sendTransaction",
    "params": [
      {
        "from": "0xAliceのaddress",
        "to": "0xBobのaddress",
        "value": 150,
        "nonce": 0,
        "pubKey": "0x...",
        "signature": "0x..."
      }
    ],
    "id": 3
  }'
```

成功すると、ブロックが1つ生成されます。

```json
{
  "jsonrpc": "2.0",
  "result": {
    "blockHash": "0x...",
    "blockNumber": "0x1",
    "stateRoot": "0x...",
    "transactionHash": "0x..."
  },
  "id": 3
}
```

状態は以下のように変わります。

```text
Alice balance: 1000 -> 850
Alice nonce:   0    -> 1

Bob balance:   0    -> 150
Bob nonce:     0    -> 0

blockNumber:   0    -> 1
```

### 3. Aliceの残高を確認

```bash
curl -s -X POST localhost:8545 \
  -H 'Content-Type: application/json' \
  -d '{
    "jsonrpc": "2.0",
    "method": "eth_getBalance",
    "params": ["0xAliceのaddress", "latest"],
    "id": 4
  }'
```

期待結果:

```json
{
  "jsonrpc": "2.0",
  "result": "0x352",
  "id": 4
}
```

`0x352` は10進数で850です。

### 4. Bobの残高を確認

```bash
curl -s -X POST localhost:8545 \
  -H 'Content-Type: application/json' \
  -d '{
    "jsonrpc": "2.0",
    "method": "eth_getBalance",
    "params": ["0xBobのaddress", "latest"],
    "id": 5
  }'
```

期待結果:

```json
{
  "jsonrpc": "2.0",
  "result": "0x96",
  "id": 5
}
```

`0x96` は10進数で150です。

### 5. 最新ブロックを確認

```bash
curl -s -X POST localhost:8545 \
  -H 'Content-Type: application/json' \
  -d '{
    "jsonrpc": "2.0",
    "method": "eth_getBlockByNumber",
    "params": ["latest", true],
    "id": 6
  }'
```

期待結果の例:

```json
{
  "jsonrpc": "2.0",
  "result": {
    "hash": "0x...",
    "number": "0x1",
    "parentHash": "0x...",
    "stateRoot": "0x...",
    "timestamp": "0x...",
    "transactions": [
      {
        "from": "0xAliceのaddress",
        "to": "0xBobのaddress",
        "value": 150,
        "nonce": 0,
        "pubKey": "0x...",
        "signature": "0x...",
        "hash": "0x..."
      }
    ]
  },
  "id": 6
}
```

## テスト手順

この節では、実装が正しく動作しているかを手動テストします。

### テスト1: ビルドできること

```bash
go build
```

期待結果:

```text
エラーなし
```

### テスト2: アカウントを生成できること

```bash
go run . account
```

期待結果:

- `address` が `0x` から始まる
- `address` が20 bytes、つまり40 hex charsである
- `publicKey` が出力される
- `privateKey` が出力される

例:

```json
{
  "address": "0x...",
  "publicKey": "0x...",
  "privateKey": "0x..."
}
```

### テスト3: Genesisブロックが作られること

1. `genesis.json` を作成します。
2. ノードを起動します。

```bash
go run . node --genesis genesis.json --addr :8545
```

3. ブロック番号を確認します。

```bash
curl -s -X POST localhost:8545 \
  -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","method":"eth_blockNumber","params":[],"id":1}'
```

期待結果:

```json
{"jsonrpc":"2.0","result":"0x0","id":1}
```

### テスト4: Genesis残高を取得できること

AliceのGenesis残高を1000にしている場合:

```bash
curl -s -X POST localhost:8545 \
  -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","method":"eth_getBalance","params":["0xAliceのaddress","latest"],"id":2}'
```

期待結果:

```json
{"jsonrpc":"2.0","result":"0x3e8","id":2}
```

### テスト5: 署名済みトランザクションを作成できること

```bash
go run . sign \
  --priv 0xAliceのprivateKey \
  --to 0xBobのaddress \
  --value 150 \
  --nonce 0
```

期待結果:

- `from` がAliceのaddressになる
- `to` がBobのaddressになる
- `value` が150になる
- `nonce` が0になる
- `signature` が出力される
- `hash` が出力される

### テスト6: sendTransactionでブロックが増えること

署名済みトランザクションを `sendTransaction` します。

期待結果:

```json
{
  "jsonrpc": "2.0",
  "result": {
    "transactionHash": "0x...",
    "blockNumber": "0x1",
    "blockHash": "0x...",
    "stateRoot": "0x..."
  },
  "id": 3
}
```

続いてブロック番号を確認します。

```bash
curl -s -X POST localhost:8545 \
  -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","method":"eth_blockNumber","params":[],"id":4}'
```

期待結果:

```json
{"jsonrpc":"2.0","result":"0x1","id":4}
```

### テスト7: 残高が状態遷移していること

Aliceの残高:

```bash
curl -s -X POST localhost:8545 \
  -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","method":"eth_getBalance","params":["0xAliceのaddress","latest"],"id":5}'
```

期待結果:

```json
{"jsonrpc":"2.0","result":"0x352","id":5}
```

Bobの残高:

```bash
curl -s -X POST localhost:8545 \
  -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","method":"eth_getBalance","params":["0xBobのaddress","latest"],"id":6}'
```

期待結果:

```json
{"jsonrpc":"2.0","result":"0x96","id":6}
```

### テスト8: 同じトランザクションを再送するとnonceエラーになること

同じ署名済みトランザクションをもう一度 `sendTransaction` します。

期待結果:

```json
{
  "jsonrpc": "2.0",
  "error": {
    "code": -32602,
    "message": "bad nonce: got 0, want 1"
  },
  "id": 3
}
```

これにより、nonceがリプレイ攻撃を防いでいることを確認できます。

### テスト9: 残高不足の送金が失敗すること

Aliceの残高以上の値を送金するトランザクションを作成します。

```bash
go run . sign \
  --priv 0xAliceのprivateKey \
  --to 0xBobのaddress \
  --value 999999 \
  --nonce 1
```

そのトランザクションを `sendTransaction` します。

期待結果:

```json
{
  "jsonrpc": "2.0",
  "error": {
    "code": -32602,
    "message": "insufficient balance"
  },
  "id": 7
}
```

### テスト10: ブロック内容を取得できること

```bash
curl -s -X POST localhost:8545 \
  -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","method":"eth_getBlockByNumber","params":["0x1",true],"id":8}'
```

期待結果:

- `number` が `0x1`
- `transactions` に送金トランザクションが入っている
- `stateRoot` が入っている
- `parentHash` がGenesisブロックのhashを指している

## jqを使った確認例

レスポンスを読みやすく表示したい場合:

```bash
curl -s -X POST localhost:8545 \
  -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","method":"eth_blockNumber","params":[],"id":1}' | jq
```

## よくあるエラー

### address must start with 0x

アドレスが `0x` から始まっていません。

### address must be 20 bytes / 40 hex chars

アドレス長が不正です。

### bad nonce

送信したトランザクションのnonceが、現在のアカウントnonceと一致していません。

現在の実装では `eth_getTransactionCount` が未実装なので、送信回数を手元で管理してください。

### insufficient balance

送金元の残高が不足しています。

### invalid signature

署名対象、秘密鍵、公開鍵、fromアドレスのいずれかが一致していません。

## ディレクトリ構成

現時点では学習しやすさを優先してフラットなファイル構成です。

```text
mini-eth-node/
  go.mod
  main.go
  model.go
  rpc.go
  transaction.go
  command.go
  utils.go
  genesis.json
  README.md
  PLAN.md
```

今後、機能が増えたら以下のように分割します。

```text
mini-eth-node/
  cmd/
  internal/
    account/
    block/
    chain/
    crypto/
    rpc/
    state/
    tx/
```

## ライセンス

学習用サンプルとして自由に利用してください。
