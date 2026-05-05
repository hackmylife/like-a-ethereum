# Milestone 1: 基本RPCの拡充

## 概要
現在のJSON-RPC APIを拡張して、Ethereum互換の基本的なクエリ機能を追加します。

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
1. `dispatch` に `eth_getTransactionCount` を追加
2. `State[address].Nonce` をhex quantityで返す
3. 存在しないアカウントは `0x0` を返す

#### 具体的な実装コード

**main.go に追加するdispatchケース:**
```go
case "eth_getTransactionCount":
    addr, err := parseGetTransactionCountParams(params)
    if err != nil {
        return nil, rpcInvalidParams(err)
    }

    c.mu.Lock()
    defer c.mu.Unlock()

    acct := c.State[addr]
    if acct.Address == "" {
        return toHex(uint64(0)), nil
    }
    return toHex(acct.Nonce), nil
```

**rpc.go に追加するパラメータ解析関数:**
```go
func parseGetTransactionCountParams(params json.RawMessage) (string, error) {
    var arr []json.RawMessage

    if err := json.Unmarshal(params, &arr); err != nil || len(arr) < 1 {
        return "", errors.New("eth_getTransactionCount expects [address, blockTag]")
    }

    var addr string
    // err != nil だけで十分。この時点で len(arr) >= 1 は確認済みなので || len(arr) < 1 は不要
    if err := json.Unmarshal(arr[0], &addr); err != nil {
        return "", err
    }

    return normalizeAddress(addr)
}
```

**Chain構造体にTxIndexを追加:**
```go
type Chain struct {
    mu         sync.Mutex
    State      map[string]Account
    Blocks     []Block
    TxIndex    map[string]TxLocation  // 追加
    BlockIndex map[string]uint64      // 追加
}

type TxLocation struct {
    BlockNumber uint64
    TxIndex     int
}
```

**Chain初期化時に必ずmakeで初期化する（nilのままだとpanicになる）:**
```go
c := &Chain{
    State:      state,
    TxIndex:    make(map[string]TxLocation),
    BlockIndex: make(map[string]uint64),
}
```

**トランザクション追加時にインデックスを更新:**
```go
func (c *Chain) addTransaction(tx Transaction) (map[string]any, error) {
    // ... 既存の検証と処理 ...
    
    // ブロック生成後にインデックスを更新
    // キーはトランザクションハッシュ（verified.Hash）を使う。block.Hash ではない
    c.TxIndex[verified.Hash] = TxLocation{
        BlockNumber: block.Number,
        TxIndex:     0, // 1トランザクションなので0
    }
    
    return receipt, nil
}
```

### テストケース
- Genesis直後のAlice nonceが `0x0`
- 1回送金後のAlice nonceが `0x1`
- Bobのnonceは受信だけでは増えない

## 1.2 eth_getTransactionByHash

### 目的
- トランザクションハッシュからトランザクションを検索できるようにする

### 実装案
1. `Chain` に `TxIndex map[string]TxLocation` を追加（1.1で実装済み）
2. `TxLocation` 構造体を定義（1.1で実装済み）:
```go
type TxLocation struct {
    BlockNumber uint64
    TxIndex     int
}
```

### 具体的な実装コード

**main.go に追加するdispatchケース:**
```go
case "eth_getTransactionByHash":
    hash, err := parseGetTransactionByHashParams(params)
    if err != nil {
        return nil, rpcInvalidParams(err)
    }

    c.mu.Lock()
    defer c.mu.Unlock()

    loc, ok := c.TxIndex[hash]
    if !ok {
        return nil, nil
    }

    block := c.Blocks[loc.BlockNumber]
    tx := block.Transactions[loc.TxIndex]

    return map[string]any{
        "hash":        tx.Hash,
        "from":        tx.From,
        "to":          tx.To,
        "value":       toHex(tx.Value),
        "nonce":       toHex(tx.Nonce),
        "blockHash":   block.Hash,
        "blockNumber": toHex(block.Number),
        "transactionIndex": toHex(uint64(loc.TxIndex)),
    }, nil
```

**rpc.go に追加するパラメータ解析関数:**
```go
func parseGetTransactionByHashParams(params json.RawMessage) (string, error) {
    var arr []json.RawMessage

    // &arr に Unmarshal すること
    if err := json.Unmarshal(params, &arr); err != nil || len(arr) < 1 {
        return "", errors.New("eth_getTransactionByHash expects [hash]")
    }

    var hash string
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

### 実装案
1. `Chain` に `BlockIndex map[string]uint64` を追加

### 具体的な実装コード

**ブロック追加時にBlockIndexを更新（addTransaction内、およびGenesis生成後）:**
```go
// ブロックをBlocksに追加した直後に記録する
c.BlockIndex[block.Hash] = block.Number
```

**Genesis生成時にも忘れず追加（NewChainFromGenesis内）:**
```go
c.Blocks = []Block{genesis}
c.BlockIndex[genesis.Hash] = genesis.Number
```

**main.go に追加するdispatchケース:**
```go
case "eth_getBlockByHash":
    hash, fullTx, err := parseGetBlockByHashParams(params)
    if err != nil {
        return nil, rpcInvalidParams(err)
    }

    c.mu.Lock()
    defer c.mu.Unlock()

    idx, ok := c.BlockIndex[hash]
    if !ok {
        return nil, nil
    }

    return toRPCBlock(c.Blocks[idx], fullTx), nil
```

**rpc.go に追加するパラメータ解析関数:**
```go
func parseGetBlockByHashParams(params json.RawMessage) (hash string, fullTx bool, err error) {
    var arr []json.RawMessage

    // &arr に Unmarshal すること。&err や &hash は誤り
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

### テストケース
- Genesisブロックhashで取得できる
- 最新ブロックhashで取得できる
- 未知のhashでは `null`

## 実装手順

1. **eth_getTransactionCount**から実装
   - `main.go` の `dispatch` にケースを追加
   - パラメータ解析関数を実装
   - テスト用curlコマンドで動作確認

2. **eth_getTransactionByHash**を実装
   - `model.go` に `TxLocation` を追加
   - `Chain` 構造体に `TxIndex` を追加
   - トランザクション追加時にインデックスを更新

3. **eth_getBlockByHash**を実装
   - `Chain` 構造体に `BlockIndex` を追加
   - ブロック追加時にインデックスを更新

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