# Milestone 4: mempoolの追加

## 概要
現在の `sendTransaction` は即座にブロックを生成していますが、実際のEthereumではトランザクションはmempool（トランザクションプール）に一度格納され、マイナーによってブロックに取り込まれます。この概念を学ぶためにmempool機能を実装します。

## 現在のフロー
```text
sendTransaction
  -> 検証
  -> 状態遷移
  -> ブロック生成
```

## 変更後のフロー
```text
sendTransaction
  -> 検証
  -> mempoolに追加

mineBlock
  -> mempoolからtxを取り出す
  -> 状態遷移
  -> ブロック生成
```

## 新しい構造

### Mempool構造体
```go
type Mempool struct {
    mu            sync.Mutex
    Transactions  []Transaction
    TxIndex       map[string]int // hash -> index
}

// コンストラクタ
func NewMempool() *Mempool {
    return &Mempool{
        Transactions: make([]Transaction, 0),
        TxIndex:      make(map[string]int),
    }
}

// トランザクションを追加
func (m *Mempool) Add(tx Transaction) error {
    m.mu.Lock()
    defer m.mu.Unlock()
    
    // 重複チェック
    if _, exists := m.TxIndex[tx.Hash]; exists {
        return errors.New("transaction already exists in mempool")
    }
    
    // 検証
    if err := m.validateTransaction(tx); err != nil {
        return err
    }
    
    // 追加
    m.Transactions = append(m.Transactions, tx)
    m.TxIndex[tx.Hash] = len(m.Transactions) - 1
    
    return nil
}

// トランザクションを取り出す
func (m *Mempool) Take(count int) []Transaction {
    m.mu.Lock()
    defer m.mu.Unlock()
    
    if len(m.Transactions) == 0 {
        return nil
    }
    
    if count > len(m.Transactions) {
        count = len(m.Transactions)
    }
    
    taken := make([]Transaction, count)
    copy(taken, m.Transactions[:count])
    
    // 残りのトランザクションを前に詰める
    m.Transactions = m.Transactions[count:]
    m.rebuildIndex()
    
    return taken
}

// すべてのトランザクションを取得
func (m *Mempool) GetAll() []Transaction {
    m.mu.Lock()
    defer m.mu.Unlock()
    
    result := make([]Transaction, len(m.Transactions))
    copy(result, m.Transactions)
    return result
}

// クリア
func (m *Mempool) Clear() {
    m.mu.Lock()
    defer m.mu.Unlock()
    
    m.Transactions = make([]Transaction, 0)
    m.TxIndex = make(map[string]int)
}

// 重複チェック
func (m *Mempool) Exists(hash string) bool {
    m.mu.Lock()
    defer m.mu.Unlock()
    
    _, exists := m.TxIndex[hash]
    return exists
}

// インデックス再構築
func (m *Mempool) rebuildIndex() {
    m.TxIndex = make(map[string]int)
    for i, tx := range m.Transactions {
        m.TxIndex[tx.Hash] = i
    }
}

// トランザクション検証
func (m *Mempool) validateTransaction(tx Transaction) error {
    // 基本的な形式チェック
    if tx.From == "" || tx.To == "" {
        return errors.New("invalid from/to address")
    }
    
    if tx.Value == 0 {
        return errors.New("zero value transaction")
    }
    
    // 署名検証はChainレベルで行う
    return nil
}
```

### Chain構造体の変更
```go
type Chain struct {
    mu     sync.Mutex
    State  map[string]Account
    Blocks []Block
    Mempool *Mempool  // 追加
}
```

## 実装計画

### 1. Mempoolパッケージの作成
**ファイル**: `internal/mempool/mempool.go`

#### 主要メソッド
```go
// トランザクションをmempoolに追加
func (m *Mempool) Add(tx Transaction) error

// トランザクションをmempoolから取り出す
func (m *Mempool) Take(count int) []Transaction

// mempool内のトランザクションを取得
func (m *Mempool) GetAll() []Transaction

// 重複チェック
func (m *Mempool) Exists(hash string) bool

// クリア
func (m *Mempool) Clear()
```

### 2. トランザクションの検証ルール
mempool追加時の検証項目：
- 署名の妥当性
- nonceの妥当性（現在のアカウントnonce以上）
- 残高不足チェック（オプション：mempool段階ではチェックしない選択も）

### 3. トランザクションの順序付け
mempool内でのトランザクション順序：
- 同一送信元からのトランザクションはnonce順にソート
- 異なる送信元はgas price順（将来的な拡張）
- 現在は単純に追加順

### 4. RPCの変更

#### sendTransactionの変更
```go
case "sendTransaction":
    var tx Transaction
    if err := parseSingleObjectParam(params, &tx); err != nil {
        return nil, rpcInvalidParams(err)
    }
    
    // 検証のみで、mempoolに追加
    if err := c.mempool.Add(tx); err != nil {
        return nil, rpcInvalidParams(err)
    }
    
    return map[string]any{
        "transactionHash": tx.Hash,
        "status": "pending",
    }, nil
```

#### 新しいRPCの追加

##### mineBlock
```json
{
  "jsonrpc": "2.0",
  "method": "mineBlock",
  "params": [],
  "id": 1
}
```

##### txpool_content
```json
{
  "jsonrpc": "2.0",
  "method": "txpool_content",
  "params": [],
  "id": 1
}
```

### 5. ブロック生成ロジックの変更
```go
func (c *Chain) mineBlock() (*Block, error) {
    c.mu.Lock()
    defer c.mu.Unlock()
    
    // mempoolからトランザクションを取得
    txs := c.Mempool.Take(10) // 最大10件
    
    if len(txs) == 0 {
        return nil, errors.New("no transactions in mempool")
    }
    
    // 状態遷移処理（既存ロジックを流用）
    for _, tx := range txs {
        if err := c.applyTransaction(tx); err != nil {
            // 失敗したトランザクションは破棄
            continue
        }
    }
    
    // ブロック生成
    block := Block{
        Number:       c.Blocks[len(c.Blocks)-1].Number + 1,
        ParentHash:   c.Blocks[len(c.Blocks)-1].Hash,
        Timestamp:    time.Now().Unix(),
        Transactions: txs,
        StateRoot:    c.computeStateRootLocked(),
    }
    
    block.Hash = blockHash(block)
    c.Blocks = append(c.Blocks, block)
    
    return &block, nil
}
```

## 考慮点

### 1. Nonceの扱い
- mempool追加時：nonce >= current_nonce
- ブロック適用時：nonce == current_nonce

### 2. 重複トランザクション
- 同じハッシュのトランザクションは拒否
- 同じ送信元+nonceの組み合わせも拒否

### 3. 残高不足トランザクション
- 方針A：mempool追加時にチェックし、拒否する
- 方針B：mempoolには追加し、採掘時にチェックする

今回は方針Aを採用します。

### 4. mempoolのサイズ制限
- 最大1000件などの制限を設ける
- 古いトランザクションから削除する

## テストケース

### 基本的なフロー
```go
func TestMempoolFlow(t *testing.T) {
    chain := setupTestChain(t, map[string]uint64{
        aliceAddr: 1000,
    })
    
    // 1. sendTransactionしてもblockNumberは増えない
    tx := createAndSignTransaction(t, alicePriv, bobAddr, 100, 0)
    resp := callRPC(t, server, "sendTransaction", []any{tx})
    assert.Equal(t, "pending", resp["status"])
    
    blockNumber := callRPC(t, server, "eth_blockNumber", []any{})
    assert.Equal(t, "0x0", blockNumber["result"])
    
    // 2. txpool_contentでtxが見える
    poolContent := callRPC(t, server, "txpool_content", []any{})
    assert.Len(t, poolContent["result"], 1)
    
    // 3. mineBlockでブロックが増える
    mineResp := callRPC(t, server, "mineBlock", []any{})
    assert.NotNil(t, mineResp["result"])
    
    blockNumber = callRPC(t, server, "eth_blockNumber", []any{})
    assert.Equal(t, "0x1", blockNumber["result"])
    
    // 4. 採掘後mempoolが空になる
    poolContent = callRPC(t, server, "txpool_content", []any{})
    assert.Len(t, poolContent["result"], 0)
}
```

### エラーケース
```go
func TestMempoolErrors(t *testing.T) {
    // 重複トランザクション
    // nonceが小さいトランザクション
    // 残高不足トランザクション
}
```

## 実装手順

1. **Mempoolパッケージを作成**
   - 基本的なCRUD操作を実装

2. **ChainにMempoolを追加**
   - コンストラクタで初期化

3. **sendTransactionを変更**
   - 即ブロック生成からmempool追加へ

4. **mineBlock RPCを実装**
   - mempoolからトランザクションを取り出してブロック生成

5. **txpool_content RPCを実装**
   - mempoolの状態を返す

6. **テストを実装**
   - 基本的なフロー
   - エラーケース

## 検証方法

### 手動テスト
```bash
# 1. ノード起動
go run . node --genesis genesis.json --addr :8545

# 2. トランザクション送信（ブロックは増えない）
curl -s -X POST localhost:8545 -d '{"jsonrpc":"2.0","method":"sendTransaction","params":[...],"id":1}'

# 3. ブロック番号確認（0x0のまま）
curl -s -X POST localhost:8545 -d '{"jsonrpc":"2.0","method":"eth_blockNumber","params":[],"id":2}'

# 4. mempool確認
curl -s -X POST localhost:8545 -d '{"jsonrpc":"2.0","method":"txpool_content","params":[],"id":3}'

# 5. ブロック採掘
curl -s -X POST localhost:8545 -d '{"jsonrpc":"2.0","method":"mineBlock","params":[],"id":4}'

# 6. ブロック番号確認（0x1に増える）
curl -s -X POST localhost:8545 -d '{"jsonrpc":"2.0","method":"eth_blockNumber","params":[],"id":5}'
```

### 自動テスト
```bash
go test ./internal/mempool
go test ./internal/chain
```

## 完了条件
- `sendTransaction` だけでblockNumberが増えない
- `txpool_content` でtxが確認できる
- `mineBlock` でblockNumberが増える
- 採掘後mempoolが空になる
- 既存のテストがすべて通る

## 次のステップ
mempoolが実装できたら、Milestone 5で複数トランザクション入りブロックを実装します。これにより、より現実的なブロック生成フローを学べます。