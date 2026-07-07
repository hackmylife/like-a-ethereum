# Milestone 1: 基本RPCの拡充（完了）

## 概要
Milestone 0のJSON-RPC API（4メソッド）を拡張して、Ethereum互換の基本的なクエリ機能を追加します。

追加するのは3メソッドと、それを支える2つのインデックスです。

| 追加RPC | 必要になる仕組み |
|---|---|
| `eth_getTransactionCount` | なし（Stateから引くだけ） |
| `eth_getTransactionByHash` | `TxIndex`（txハッシュ → ブロック番号＋ブロック内位置） |
| `eth_getBlockByHash` | `BlockIndex`（ブロックハッシュ → ブロック番号） |

## 前提
- Milestone 0が完了していること（パッケージ構成、RPC4メソッド）

## 1.1 eth_getTransactionCount

### 目的
- アカウントの現在nonceをRPCで取得できるようにする
- 次に使うべきnonceをクライアントが取得できるようにする

### API仕様
```json
{
  "jsonrpc": "2.0",
  "method": "eth_getTransactionCount",
  "params": ["0xaddress", "latest"],
  "id": 1
}
```

### 期待結果
```json
{
  "jsonrpc": "2.0",
  "result": "0x1",
  "id": 1
}
```

### 実装内容
1. `internal/rpc/server.go` の `dispatch` に `eth_getTransactionCount` を追加
2. `State[address].Nonce` をhex quantityで返す
3. 存在しないアカウントは `0x0` を返す

**internal/rpc/server.go に追加するdispatchケース:**
```go
case "eth_getTransactionCount":
    addr, err := parseGetTransactionCountParams(params)
    if err != nil {
        return nil, rpcInvalidParams(err)
    }

    c.Lock()
    defer c.Unlock()

    acct := c.State[addr]
    if acct.Address == "" {
        return util.ToHex(uint64(0)), nil
    }
    return util.ToHex(acct.Nonce), nil
```

**internal/rpc/server.go に追加するパラメータ解析関数:**
```go
func parseGetTransactionCountParams(params json.RawMessage) (string, error) {
    var arr []json.RawMessage

    if err := json.Unmarshal(params, &arr); err != nil || len(arr) < 1 {
        return "", errors.New("eth_getTransactionCount expects [address, blockTag]")
    }

    var addr string
    if err := json.Unmarshal(arr[0], &addr); err != nil {
        return "", err
    }

    return account.NormalizeAddress(addr)
}
```

### テストケース
- Genesis直後のAlice nonceが `0x0`
- 1回送金後のAlice nonceが `0x1`
- Bobのnonceは受信だけでは増えない

## 1.2 eth_getTransactionByHash

### 目的
- トランザクションハッシュからトランザクションを検索できるようにする

### 実装内容

**internal/tx/transaction.go に位置情報の型を追加:**
```go
type TxLocation struct {
    BlockNumber uint64
    TxIndex     int
}
```

**internal/chain/chain.go の Chain 構造体にインデックスを追加:**
```go
type Chain struct {
    mu         sync.Mutex
    State      map[string]account.Account
    Blocks     []block.Block
    TxIndex    map[string]tx.TxLocation // 追加: txハッシュ -> 位置
    BlockIndex map[string]uint64        // 追加: ブロックハッシュ -> 番号（1.3で使用）
}
```

**Chain初期化時に必ずmakeで初期化する（nilのままだとpanicになる）:**
```go
c := &Chain{
    State:      state,
    TxIndex:    make(map[string]tx.TxLocation),
    BlockIndex: make(map[string]uint64),
}
```

**AddTransaction のブロック生成後にインデックスを更新:**
```go
func (c *Chain) AddTransaction(t tx.Transaction) (map[string]any, error) {
    // ... 既存の検証と状態遷移、ブロック生成 ...

    // キーはトランザクションハッシュ（verified.Hash）を使う。blk.Hash ではない
    c.TxIndex[verified.Hash] = tx.TxLocation{
        BlockNumber: blk.Number,
        TxIndex:     0, // 現時点は1ブロック1トランザクションなので常に0
    }

    return receipt, nil
}
```

**internal/rpc/server.go に追加するdispatchケース:**
```go
case "eth_getTransactionByHash":
    hash, err := parseGetTransactionByHashParams(params)
    if err != nil {
        return nil, rpcInvalidParams(err)
    }

    c.Lock()
    defer c.Unlock()

    loc, ok := c.TxIndex[hash]
    if !ok {
        return nil, nil // 未知のhashはnull
    }

    b := c.Blocks[loc.BlockNumber]
    t := b.Transactions[loc.TxIndex]

    return map[string]any{
        "hash":             t.Hash,
        "from":             t.From,
        "to":               t.To,
        "value":            util.ToHex(t.Value),
        "nonce":            util.ToHex(t.Nonce),
        "blockHash":        b.Hash,
        "blockNumber":      util.ToHex(b.Number),
        "transactionIndex": util.ToHex(uint64(loc.TxIndex)),
    }, nil
```

注意: 現リポジトリの実装は `blockNumber` を `util.ToHex` に通し忘れて数値のまま返している
（milestone-0の既知の問題3。Milestone 3で上のコードのとおりに修正する）。

**internal/rpc/server.go に追加するパラメータ解析関数:**
```go
func parseGetTransactionByHashParams(params json.RawMessage) (hash string, err error) {
    var arr []json.RawMessage

    if err := json.Unmarshal(params, &arr); err != nil || len(arr) < 1 {
        return "", errors.New("eth_getTransactionByHash expects [hash]")
    }

    if err := json.Unmarshal(arr[0], &hash); err != nil {
        return "", err
    }

    return hash, nil
}
```

### テストケース
- 送信済みtxのhashで取得できる
- 未知のhashでは `null`

## 1.3 eth_getBlockByHash

### 目的
- ブロックハッシュからブロックを取得できるようにする

### 実装内容

**ブロック追加時にBlockIndexを更新（AddTransaction内）:**
```go
// ブロックをBlocksに追加した直後に記録する
c.BlockIndex[blk.Hash] = blk.Number
```

**Genesis生成時にも忘れず追加（NewChainFromGenesis内）:**
```go
c.Blocks = []block.Block{genesis}
c.BlockIndex[genesis.Hash] = genesis.Number
```

**internal/rpc/server.go に追加するdispatchケース:**
```go
case "eth_getBlockByHash":
    hash, fullTx, err := parseGetBlockByHashParams(params)
    if err != nil {
        return nil, rpcInvalidParams(err)
    }

    c.Lock()
    defer c.Unlock()

    idx, ok := c.BlockIndex[hash]
    if !ok {
        return nil, nil // 未知のhashはnull
    }

    return block.ToRPCBlock(c.Blocks[idx], fullTx), nil
```

**internal/rpc/server.go に追加するパラメータ解析関数:**
```go
func parseGetBlockByHashParams(params json.RawMessage) (hash string, fullTx bool, err error) {
    var arr []json.RawMessage

    if err := json.Unmarshal(params, &arr); err != nil || len(arr) < 1 {
        return "", false, errors.New("eth_getBlockByHash expects [hash, fullTx]")
    }

    if err := json.Unmarshal(arr[0], &hash); err != nil {
        return "", false, err
    }

    if len(arr) >= 2 {
        _ = json.Unmarshal(arr[1], &fullTx)
    }

    return hash, fullTx, nil
}
```

注意: 現リポジトリの実装はエラーメッセージ末尾にデバッグ用の `strconv.Itoa(len(arr))` が
連結されたままになっている（milestone-0の既知の問題4。Milestone 3で除去する）。

### テストケース
- Genesisブロックhashで取得できる
- 最新ブロックhashで取得できる
- 未知のhashでは `null`

## 実装手順

1. **eth_getTransactionCount** から実装
   - `internal/rpc/server.go` の `dispatch` にケースを追加
   - パラメータ解析関数を実装
   - curlで動作確認

2. **eth_getTransactionByHash** を実装
   - `internal/tx/transaction.go` に `TxLocation` を追加
   - `internal/chain/chain.go` の `Chain` に `TxIndex` / `BlockIndex` を追加し、初期化とインデックス更新を入れる
   - dispatchケースとパラメータ解析関数を追加

3. **eth_getBlockByHash** を実装
   - Genesis生成時と `AddTransaction` 内で `BlockIndex` を更新
   - dispatchケースとパラメータ解析関数を追加

## 検証方法
各APIを実装後、READMEのテスト手順に加えて以下のcurlコマンドで確認：

```bash
# eth_getTransactionCount（Genesis直後、Alice nonce=0）
curl -s -X POST localhost:8545 \
  -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","method":"eth_getTransactionCount","params":["0x<aliceAddress>","latest"],"id":1}'
# 期待出力:
# {"jsonrpc":"2.0","result":"0x0","id":1}

# sendTransactionで1回送金後のAlice nonce=1
curl -s -X POST localhost:8545 \
  -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","method":"eth_getTransactionCount","params":["0x<aliceAddress>","latest"],"id":2}'
# 期待出力:
# {"jsonrpc":"2.0","result":"0x1","id":2}

# 存在しないアドレスは0x0
curl -s -X POST localhost:8545 \
  -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","method":"eth_getTransactionCount","params":["0x0000000000000000000000000000000000000000","latest"],"id":3}'
# 期待出力:
# {"jsonrpc":"2.0","result":"0x0","id":3}

# eth_getTransactionByHash（送信済みtxのhashで取得）
curl -s -X POST localhost:8545 \
  -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","method":"eth_getTransactionByHash","params":["0x<txHash>"],"id":4}'
# 期待出力:
# {"jsonrpc":"2.0","result":{"blockHash":"0x...","blockNumber":"0x1","from":"0x...","hash":"0x...","nonce":"0x0","to":"0x...","transactionIndex":"0x0","value":"0x..."},"id":4}

# eth_getTransactionByHash（未知のhashはnull）
curl -s -X POST localhost:8545 \
  -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","method":"eth_getTransactionByHash","params":["0x0000000000000000000000000000000000000000000000000000000000000000"],"id":5}'
# 期待出力:
# {"jsonrpc":"2.0","id":5}
#（本来は "result":null を含むべき。milestone-0の既知の問題5）

# eth_getBlockByHash（Genesisブロックhashで取得）
curl -s -X POST localhost:8545 \
  -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","method":"eth_getBlockByHash","params":["0x<genesisHash>",true],"id":6}'
# 期待出力:
# {"jsonrpc":"2.0","result":{"hash":"0x...","number":"0x0","parentHash":"0x0000...","stateRoot":"0x...","timestamp":"0x...","transactions":[]},"id":6}

# eth_getBlockByHash（未知のhashはnull）
curl -s -X POST localhost:8545 \
  -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","method":"eth_getBlockByHash","params":["0x0000000000000000000000000000000000000000000000000000000000000000",true],"id":7}'
# 期待出力:
# {"jsonrpc":"2.0","id":7}
```

## 完了条件
- すべてのAPIが期待通りに応答する
- 既存のREADME手順が変わらず動作する
- エラーケース（存在しないhash/address）で適切に `null` を返す

## 次のステップ
Milestone 2はフラット構成からパッケージ構成へのリファクタリングの記録です。Milestone 0の手順書どおりに最初からパッケージ構成で実装している場合は読み物としてスキップし、Milestone 3（自動テスト）に進んでください。
