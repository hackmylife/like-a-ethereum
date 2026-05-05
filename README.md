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
| `eth_getTransactionCount` | 指定アドレスのnonceを返す |
| `eth_getTransactionByHash` | トランザクションをhashで取得する |
| `eth_getBlockByNumber` | 指定番号のブロックを返す |
| `eth_getBlockByHash` | 指定hashのブロックを返す |
| `sendTransaction` | 署名済みトランザクションを送信する |

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
- jq（任意）

## セットアップ

```bash
git clone <this-repo>
cd like-a-ethereum
make build
```

`bin/minieth` が生成されます。

## コマンド

```bash
# アカウント生成
./bin/minieth account

# トランザクション署名
./bin/minieth sign --priv <privateKeyHex> --to <address> --value <amount> --nonce <nonce>

# ノード起動
./bin/minieth node --genesis genesis.json --addr :8545
```

## Makefile

| ターゲット | 内容 |
|---|---|
| `make build` | `./bin/minieth` をビルド |
| `make run` | ビルド後にノードを起動（`:8545`） |
| `make clean` | `./bin/` を削除 |
| `make test` | `go test ./...` |
| `make lint` | `go vet ./...` |

## クイックスタート

### 1. Aliceアカウントを作成

```bash
./bin/minieth account
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
```

### 2. Bobアカウントを作成

```bash
./bin/minieth account
```

以下の値を控えます。

```bash
BOB_ADDRESS=0x...
```

### 3. genesis.jsonを作成

Aliceに初期残高1000を割り当てます。

```bash
cat > genesis.json <<'JSON'
{
  "alloc": {
    "0xAliceのaddress": 1000,
    "0xBobのaddress": 0
  }
}
JSON
```

### 4. ノードを起動

```bash
make run
# または
./bin/minieth node --genesis genesis.json --addr :8545
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
  -d '{"jsonrpc":"2.0","method":"eth_blockNumber","params":[],"id":1}'
```

期待結果:

```json
{"jsonrpc":"2.0","result":"0x0","id":1}
```

### eth_getBalance

```bash
curl -s -X POST localhost:8545 \
  -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","method":"eth_getBalance","params":["0xAliceのaddress","latest"],"id":2}'
```

期待結果（残高1000 = `0x3e8`）:

```json
{"jsonrpc":"2.0","result":"0x3e8","id":2}
```

## 送金手順

### 1. AliceからBobへのトランザクションを署名

```bash
./bin/minieth sign \
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

blockNumber:   0    -> 1
```

## テスト手順

### テスト1: ビルドできること

```bash
make build
```

### テスト2: アカウントを生成できること

```bash
./bin/minieth account
```

期待結果:

- `address` が `0x` から始まる40 hex chars（20バイト）
- `publicKey`、`privateKey` が出力される

### テスト3: Genesisブロックが作られること

ノードを起動して `eth_blockNumber` が `0x0` を返すことを確認します。

### テスト4: sendTransactionでブロックが増えること

署名済みトランザクションを送信後、`eth_blockNumber` が `0x1` になることを確認します。

### テスト5: 残高が状態遷移していること

- Alice: `0x352`（850）
- Bob: `0x96`（150）

### テスト6: 同じトランザクションを再送するとnonceエラーになること

期待結果:

```json
{"error":{"code":-32602,"message":"bad nonce: got 0, want 1"}}
```

### テスト7: 残高不足の送金が失敗すること

期待結果:

```json
{"error":{"code":-32602,"message":"insufficient balance"}}
```

## jqを使った確認例

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

送信したトランザクションのnonceが、現在のアカウントnonceと一致していません。次に送るべきnonceは `eth_getTransactionCount` で確認できます。

### insufficient balance

送金元の残高が不足しています。

### invalid signature

署名対象、秘密鍵、公開鍵、fromアドレスのいずれかが一致していません。

## ディレクトリ構成

```text
like-a-ethereum/
├── cmd/
│   └── minieth/
│       └── main.go       # エントリポイント（account / sign / node）
├── internal/
│   ├── account/          # Account型、アドレス正規化
│   ├── block/            # Block型、ハッシュ計算
│   ├── chain/            # Chain（状態・ブロック管理）
│   ├── crypto/           # HashJSON（util委譲）
│   ├── rpc/              # JSON-RPCサーバー
│   ├── state/            # （将来拡張用）
│   ├── tx/               # Transaction型、署名検証
│   └── util/             # 共通ユーティリティ（hex変換など）
├── bin/                  # ビルド成果物（.gitignore済み）
├── genesis.json          # 初期アカウント設定
├── go.mod
├── Makefile
└── README.md
```

## ライセンス

学習用サンプルとして自由に利用してください。
