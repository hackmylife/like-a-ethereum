# Milestone 5: 複数トランザクション入りブロック

## 概要
現在の実装では1トランザクション＝1ブロックですが、実際のEthereumでは1ブロックに複数のトランザクションが含まれます。この機能を実装し、ブロック内でのトランザクション処理順序や状態管理を学びます。

## 現在の制約
- 1ブロックに1トランザクションのみ
- `mineBlock` が最大10件のトランザクションを処理するが、実際には1件しか処理していない

## 目標
- 1ブロックに複数トランザクションを含める
- ブロック内で順番に状態遷移する
- 失敗したトランザクションの適切な扱いを学ぶ

## 技術的課題

### 1. 同じ送信元のnonce順処理
- Aliceから nonce=0, 1, 2 のトランザクションがmempoolにある場合
- ブロック内では 0 → 1 → 2 の順で処理する必要がある
- nonce=1だけ先に来た場合は処理できない

### 2. 途中で失敗するtxの扱い
- 残高不足で途中のtxが失敗した場合
- 後続のtxはどうするか？
- 方針：失敗したtxはスキップし、後続のtxを処理する

### 3. ブロック内での一時状態管理
- ブロック生成中の状態遷移をどう管理するか
- 方針：ブロックごとにstateをコピーし、成功した場合のみマージする

## 実装計画

### 1. トランザクションのソートロジック
```go
type TransactionSorter []Transaction

func (t TransactionSorter) Len() int { return len(t) }
func (t TransactionSorter) Swap(i, j int) { t[i], t[j] = t[j], t[i] }
func (t TransactionSorter) Less(i, j int) bool {
    // 1. 送信元アドレスでソート
    if t[i].From != t[j].From {
        return t[i].From < t[j].From
    }
    // 2. nonceでソート
    return t[i].Nonce < t[j].Nonce
}
```

### 2. 状態コピー機能
```go
func (c *Chain) copyState() map[string]Account {
    copy := make(map[string]Account, len(c.State))
    for k, v := range c.State {
        copy[k] = v
    }
    return copy
}

func (c *Chain) applyTransactionToState(state map[string]Account, tx Transaction) error {
    from := state[tx.From]
    if from.Address == "" {
        return errors.New("from account does not exist")
    }
    
    // nonceチェック
    if tx.Nonce != from.Nonce {
        return fmt.Errorf("bad nonce: got %d, want %d", tx.Nonce, from.Nonce)
    }
    
    // 残高チェック
    if from.Balance < tx.Value {
        return errors.New("insufficient balance")
    }
    
    // 状態更新
    to := state[tx.To]
    if to.Address == "" {
        to = Account{
            Address: tx.To,
            Balance: 0,
            Nonce:   0,
        }
    }
    
    from.Balance -= tx.Value
    from.Nonce++
    to.Balance += tx.Value
    
    state[from.Address] = from
    state[to.Address] = to
    
    return nil
}
```

### 3. トランザクションソートと選別ロジック
```go
// 適用可能なトランザクションを選別
func (c *Chain) selectApplicableTransactions(state map[string]Account, txs []Transaction) []Transaction {
    sort.Sort(TransactionSorter(txs))
    
    var applicable []Transaction
    processedNonces := make(map[string]uint64) // from -> next nonce
    
    for _, tx := range txs {
        from := state[tx.From]
        if from.Address == "" {
            continue // 送信元アカウントが存在しない
        }
        
        expectedNonce := processedNonces[tx.From]
        if tx.Nonce != expectedNonce {
            continue // nonceが期待値と異なる
        }
        
        // 残高チェック
        if from.Balance < tx.Value {
            continue // 残高不足
        }
        
        // このトランザクションは適用可能
        applicable = append(applicable, tx)
        
        // 状態を更新して次のトランザクションチェックに備える
        from.Balance -= tx.Value
        from.Nonce++
        state[tx.From] = from
        
        // 宛先アカウントを準備
        to := state[tx.To]
        if to.Address == "" {
            to = Account{
                Address: tx.To,
                Balance: 0,
                Nonce:   0,
            }
        }
        to.Balance += tx.Value
        state[tx.To] = to
        
        processedNonces[tx.From] = expectedNonce + 1
    }
    
    return applicable
}
```

### 4. ブロック生成ロジックの改善
```go
func (c *Chain) mineBlock() (*Block, error) {
    c.mu.Lock()
    defer c.mu.Unlock()
    
    // mempoolからトランザクションを取得
    allTxs := c.Mempool.GetAll()
    if len(allTxs) == 0 {
        return nil, errors.New("no transactions in mempool")
    }
    
    // トランザクションをソート
    sort.Sort(TransactionSorter(allTxs))
    
    // ブロック用の状態をコピー
    tempState := c.copyState()
    
    // 適用するトランザクションを選別
    var appliedTxs []Transaction
    var failedTxs []Transaction
    
    for _, tx := range allTxs {
        if err := c.applyTransactionToState(tempState, tx); err != nil {
            failedTxs = append(failedTxs, tx)
            continue
        }
        appliedTxs = append(appliedTxs, tx)
    }
    
    if len(appliedTxs) == 0 {
        return nil, errors.New("no valid transactions")
    }
    
    // 状態をマージ
    c.State = tempState
    
    // 成功したトランザクションをmempoolから削除
    for _, tx := range appliedTxs {
        c.Mempool.Remove(tx.Hash)
    }
    
    // ブロック生成
    block := Block{
        Number:       c.Blocks[len(c.Blocks)-1].Number + 1,
        ParentHash:   c.Blocks[len(c.Blocks)-1].Hash,
        Timestamp:    time.Now().Unix(),
        Transactions: appliedTxs,
        StateRoot:    c.computeStateRootLocked(),
    }
    
    block.Hash = blockHash(block)
    c.Blocks = append(c.Blocks, block)
    
    return &block, nil
}
```

### 4. Mempoolの改善
```go
func (m *Mempool) Remove(hash string) bool {
    m.mu.Lock()
    defer m.mu.Unlock()
    
    if idx, exists := m.TxIndex[hash]; exists {
        // スライスから削除
        m.Transactions = append(
            m.Transactions[:idx],
            m.Transactions[idx+1:]...,
        )
        
        // インデックスを再構築
        m.rebuildIndex()
        return true
    }
    return false
}

func (m *Mempool) rebuildIndex() {
    m.TxIndex = make(map[string]int)
    for i, tx := range m.Transactions {
        m.TxIndex[tx.Hash] = i
    }
}
```

## テストケース

### 1. 基本的な複数tx処理
```go
func TestMultipleTransactionsInBlock(t *testing.T) {
    chain := setupTestChain(t, map[string]uint64{
        aliceAddr: 1000,
        bobAddr:   0,
        charlieAddr: 0,
    })
    
    // AliceからBobへ100 (nonce=0)
    tx1 := createAndSignTransaction(t, alicePriv, bobAddr, 100, 0)
    chain.Mempool.Add(tx1)
    
    // AliceからCharlieへ200 (nonce=1)
    tx2 := createAndSignTransaction(t, alicePriv, charlieAddr, 200, 1)
    chain.Mempool.Add(tx2)
    
    // AliceからBobへ50 (nonce=2)
    tx3 := createAndSignTransaction(t, alicePriv, bobAddr, 50, 2)
    chain.Mempool.Add(tx3)
    
    // ブロック採掘
    block, err := chain.MineBlock()
    require.NoError(t, err)
    
    // 検証
    assert.Len(t, block.Transactions, 3)
    
    // Aliceの状態
    alice := chain.State[aliceAddr]
    assert.Equal(t, uint64(650), alice.Balance) // 1000-100-200-50
    assert.Equal(t, uint64(3), alice.Nonce)
    
    // Bobの状態
    bob := chain.State[bobAddr]
    assert.Equal(t, uint64(150), bob.Balance) // 100+50
    
    // Charlieの状態
    charlie := chain.State[charlieAddr]
    assert.Equal(t, uint64(200), charlie.Balance)
}
```

### 2. Nonce順の検証
```go
func TestNonceOrdering(t *testing.T) {
    chain := setupTestChain(t, map[string]uint64{
        aliceAddr: 1000,
        bobAddr:   0,
    })
    
    // nonce=1を先に追加
    tx1 := createAndSignTransaction(t, alicePriv, bobAddr, 100, 1)
    chain.Mempool.Add(tx1)
    
    // nonce=0を後から追加
    tx2 := createAndSignTransaction(t, alicePriv, bobAddr, 50, 0)
    chain.Mempool.Add(tx2)
    
    // ブロック採掘
    block, err := chain.MineBlock()
    require.NoError(t, err)
    
    // nonce=0が先に処理される
    assert.Equal(t, uint64(0), block.Transactions[0].Nonce)
    assert.Equal(t, uint64(1), block.Transactions[1].Nonce)
    
    // 状態も正しく遷移
    alice := chain.State[aliceAddr]
    assert.Equal(t, uint64(850), alice.Balance) // 1000-50-100
    assert.Equal(t, uint64(2), alice.Nonce)
}
```

### 3. 残高不足の場合
```go
func TestInsufficientBalanceInBlock(t *testing.T) {
    chain := setupTestChain(t, map[string]uint64{
        aliceAddr: 100,
        bobAddr:   0,
        charlieAddr: 0,
    })
    
    // 正常なトランザクション
    tx1 := createAndSignTransaction(t, alicePriv, bobAddr, 50, 0)
    chain.Mempool.Add(tx1)
    
    // 残高不足のトランザクション
    tx2 := createAndSignTransaction(t, alicePriv, charlieAddr, 200, 1)
    chain.Mempool.Add(tx2)
    
    // ブロック採掘
    block, err := chain.MineBlock()
    require.NoError(t, err)
    
    // 成功したトランザクションのみ含まれる
    assert.Len(t, block.Transactions, 1)
    assert.Equal(t, tx1.Hash, block.Transactions[0].Hash)
    
    // 失敗したトランザクションはmempoolに残る
    assert.Len(t, chain.Mempool.GetAll(), 1)
    assert.Equal(t, tx2.Hash, chain.Mempool.GetAll()[0].Hash)
}
```

### 4. 異なる送信元の混在
```go
func TestMultipleSenders(t *testing.T) {
    chain := setupTestChain(t, map[string]uint64{
        aliceAddr:   500,
        bobAddr:     300,
        charlieAddr: 0,
    })
    
    // AliceからCharlieへ100
    tx1 := createAndSignTransaction(t, alicePriv, charlieAddr, 100, 0)
    chain.Mempool.Add(tx1)
    
    // BobからCharlieへ50
    tx2 := createAndSignTransaction(t, bobPriv, charlieAddr, 50, 0)
    chain.Mempool.Add(tx2)
    
    // ブロック採掘
    block, err := chain.MineBlock()
    require.NoError(t, err)
    
    // 両方のトランザクションが含まれる
    assert.Len(t, block.Transactions, 2)
    
    // Charlieの残高は合算される
    charlie := chain.State[charlieAddr]
    assert.Equal(t, uint64(150), charlie.Balance)
}
```

## 実装手順

1. **トランザクションソート機能を実装**
   - 送信元、nonce順のソートロジック

2. **状態コピー機能を実装**
   - `copyState` メソッド
   - `applyTransactionToState` メソッド

3. **ブロック生成ロジックを改善**
   - 複数txを処理できるように
   - 失敗したtxのスキップ

4. **Mempoolを改善**
   - `Remove` メソッド
   - インデックス再構築

5. **テストを実装**
   - 基本的な複数tx処理
   - nonce順の検証
   - エラーケース

## 検証方法

### 手動テスト
```bash
# 1. 複数のトランザクションをmempoolに追加
curl -s -X POST localhost:8545 -d '{"jsonrpc":"2.0","method":"sendTransaction","params":[tx1],"id":1}'
curl -s -X POST localhost:8545 -d '{"jsonrpc":"2.0","method":"sendTransaction","params":[tx2],"id":2}'
curl -s -X POST localhost:8545 -d '{"jsonrpc":"2.0","method":"sendTransaction","params":[tx3],"id":3}'

# 2. mempool確認（3件見える）
curl -s -X POST localhost:8545 -d '{"jsonrpc":"2.0","method":"txpool_content","params":[],"id":4}'

# 3. ブロック採掘
curl -s -X POST localhost:8545 -d '{"jsonrpc":"2.0","method":"mineBlock","params":[],"id":5}'

# 4. ブロック確認（複数txが含まれる）
curl -s -X POST localhost:8545 -d '{"jsonrpc":"2.0","method":"eth_getBlockByNumber","params":["latest",true],"id":6}'

# 5. mempool確認（空になる）
curl -s -X POST localhost:8545 -d '{"jsonrpc":"2.0","method":"txpool_content","params":[],"id":7}'
```

### 自動テスト
```bash
go test ./internal/chain -run TestMultipleTransactions
go test ./internal/chain -run TestNonceOrdering
go test ./internal/chain -run TestInsufficientBalanceInBlock
```

## 完了条件
- 1ブロックに複数のトランザクションが含められる
- 同じ送信元のトランザクションがnonce順に処理される
- 残高不足のトランザクションがスキップされる
- 異なる送信元のトランザクションが混在できる
- 既存のテストがすべて通る

## 次のステップ
複数トランザクション処理が実装できたら、Milestone 6でstateRootをMerkle Tree風に改善します。これにより、状態証明の基本概念を学べます。