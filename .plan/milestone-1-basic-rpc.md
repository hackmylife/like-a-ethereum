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

### テストケース
- Genesis直後のAlice nonceが `0x0`
- 1回送金後のAlice nonceが `0x1`
- Bobのnonceは受信だけでは増えない

## 1.2 eth_getTransactionByHash

### 目的
- トランザクションハッシュからトランザクションを検索できるようにする

### 実装案
1. `Chain` に `TxIndex map[string]TxLocation` を追加
2. `TxLocation` 構造体を定義:
```go
type TxLocation struct {
    BlockNumber uint64
    TxIndex     int
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
# eth_getTransactionCount
curl -s -X POST localhost:8545 \
  -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","method":"eth_getTransactionCount","params":["0xaddress","latest"],"id":1}'

# eth_getTransactionByHash
curl -s -X POST localhost:8545 \
  -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","method":"eth_getTransactionByHash","params":["0xhash"],"id":1}'

# eth_getBlockByHash
curl -s -X POST localhost:8545 \
  -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","method":"eth_getBlockByHash","params":["0xhash",true],"id":1}'
```

## 完了条件
- すべてのAPIが期待通りに応答する
- 既存のREADME手順が変わらず動作する
- エラーケース（存在しないhash/address）で適切に `null` を返す