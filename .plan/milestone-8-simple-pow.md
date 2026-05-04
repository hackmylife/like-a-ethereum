# Milestone 8: 簡易PoW

## 概要
現在の実装では `mineBlock` を呼ぶと即座にブロックが生成されますが、実際のEthereumではProof of Work（PoW）によって計算努力が必要です。このマイルストーンでは、簡易的なPoWを実装して、ブロック生成と難易度の概念を学びます。

## 目的
- ブロック生成と難易度の概念を学ぶ
- マイニングの基本的な仕組みを理解する
- Nonceの役割を学ぶ

## PoWの基本概念
- マイナーは特定の条件を満たすハッシュを見つける
- 条件：ブロックハッシュが特定のパターンで始まる
- 難易度：条件の厳しさ（例：0000で始まる）
- Nonce：条件を満たすために変更する値

## 新しいフィールド

### Block構造体の変更
```go
type Block struct {
    Number       uint64
    ParentHash   string
    Timestamp    int64
    Nonce        uint64        // 追加
    Difficulty   uint64        // 追加
    Transactions []Transaction
    StateRoot    string
    Hash         string
}
```

### Genesisブロックの変更
```go
func createGenesisBlock(state map[string]Account) Block {
    return Block{
        Number:     0,
        ParentHash: "0x0000000000000000000000000000000000000000000000000000000000000000",
        Timestamp:  time.Now().Unix(),
        Nonce:      0,
        Difficulty: 1, // 初期難易度
        Transactions: []Transaction{},
        StateRoot:  computeStateRoot(state),
    }
}
```

## 実装計画

### 1. PoW検証ロジック
```go
package pow

import (
    "crypto/sha256"
    "encoding/hex"
    "strconv"
    "strings"
)

func IsValidProof(blockHash string, difficulty uint64) bool {
    // 難易度に応じたゼロの数を計算
    requiredZeros := difficulty
    if requiredZeros > 64 {
        requiredZeros = 64 // SHA-256は64文字
    }
    
    // ブロックハッシュが指定された数の0で始まるかチェック
    prefix := strings.Repeat("0", int(requiredZeros))
    return strings.HasPrefix(blockHash[2:], prefix) // 0xを除いてチェック
}

func CalculateHash(block Block) string {
    // Nonceを含めてハッシュ計算
    data := struct {
        Number     uint64   `json:"number"`
        ParentHash string   `json:"parentHash"`
        Timestamp  int64    `json:"timestamp"`
        Nonce      uint64   `json:"nonce"`
        Difficulty uint64   `json:"difficulty"`
        TxHashes   []string `json:"txHashes"`
        StateRoot  string   `json:"stateRoot"`
    }{
        Number:     block.Number,
        ParentHash: block.ParentHash,
        Timestamp:  block.Timestamp,
        Nonce:      block.Nonce,
        Difficulty: block.Difficulty,
        TxHashes:   getTxHashes(block.Transactions),
        StateRoot:  block.StateRoot,
    }
    
    jsonData, _ := json.Marshal(data)
    sum := sha256.Sum256(jsonData)
    return "0x" + hex.EncodeToString(sum[:])
}
```

### 2. マイニングロジック
```go
func MineBlock(block Block, maxAttempts uint64) (Block, error) {
    for nonce := uint64(0); nonce < maxAttempts; nonce++ {
        block.Nonce = nonce
        block.Hash = CalculateHash(block)
        
        if IsValidProof(block.Hash, block.Difficulty) {
            return block, nil
        }
    }
    
    return Block{}, errors.New("failed to find valid proof")
}

// 並列マイニング（オプション）
func MineBlockParallel(block Block, maxAttempts, workers uint64) (Block, error) {
    type result struct {
        block Block
        err   error
    }
    
    results := make(chan result)
    attemptsPerWorker := maxAttempts / workers
    
    for i := uint64(0); i < workers; i++ {
        go func(startNonce uint64) {
            localBlock := block
            for nonce := startNonce; nonce < startNonce+attemptsPerWorker; nonce++ {
                localBlock.Nonce = nonce
                localBlock.Hash = CalculateHash(localBlock)
                
                if IsValidProof(localBlock.Hash, localBlock.Difficulty) {
                    results <- result{block: localBlock}
                    return
                }
            }
            results <- result{err: errors.New("not found")}
        }(i * attemptsPerWorker)
    }
    
    // 最初の成功を待つ
    for i := uint64(0); i < workers; i++ {
        res := <-results
        if res.err == nil {
            return res.block, nil
        }
    }
    
    return Block{}, errors.New("failed to find valid proof")
}
```

### 3. Chainへの統合
```go
func (c *Chain) mineBlockWithDifficulty(difficulty uint64) (*Block, error) {
    c.mu.Lock()
    defer c.mu.Unlock()
    
    // mempoolからトランザクションを取得
    txs := c.Mempool.Take(10)
    if len(txs) == 0 {
        return nil, errors.New("no transactions in mempool")
    }
    
    // ブロックの雛形を作成
    block := Block{
        Number:       c.Blocks[len(c.Blocks)-1].Number + 1,
        ParentHash:   c.Blocks[len(c.Blocks)-1].Hash,
        Timestamp:    time.Now().Unix(),
        Nonce:        0,
        Difficulty:   difficulty,
        Transactions: txs,
        StateRoot:    c.computeStateRootLocked(),
    }
    
    // マイニング実行
    minedBlock, err := pow.MineBlock(block, 1000000) // 最大100万回試行
    if err != nil {
        return nil, err
    }
    
    // 状態遷移
    for _, tx := range txs {
        if err := c.applyTransaction(tx); err != nil {
            continue
        }
    }
    
    // mempoolをクリア
    c.Mempool.Clear()
    
    // ブロックを追加
    c.Blocks = append(c.Blocks, minedBlock)
    
    // 永続化
    if err := c.saveToDisk(); err != nil {
        return nil, err
    }
    
    return &minedBlock, nil
}
```

### 4. RPCの変更
```go
case "mineBlock":
    // 難易度パラメータをサポート
    difficulty, err := parseMineBlockParams(params)
    if err != nil {
        return nil, rpcInvalidParams(err)
    }
    
    block, err := c.mineBlockWithDifficulty(difficulty)
    if err != nil {
        return nil, rpcInvalidParams(err)
    }
    
    return map[string]any{
        "blockNumber": toHex(block.Number),
        "blockHash":   block.Hash,
        "nonce":       toHex(block.Nonce),
        "difficulty":  toHex(block.Difficulty),
        "stateRoot":   block.StateRoot,
    }, nil

case "setDifficulty":
    // 難易度設定RPC（テスト用）
    var difficulty uint64
    if err := parseSingleObjectParam(params, &difficulty); err != nil {
        return nil, rpcInvalidParams(err)
    }
    
    c.mu.Lock()
    c.currentDifficulty = difficulty
    c.mu.Unlock()
    
    return map[string]any{
        "difficulty": toHex(difficulty),
    }, nil
```

### 5. 難易度調整（簡易版）
```go
func (c *Chain) adjustDifficulty() uint64 {
    if len(c.Blocks) < 2 {
        return 1 // 初期難易度
    }
    
    lastBlock := c.Blocks[len(c.Blocks)-1]
    prevBlock := c.Blocks[len(c.Blocks)-2]
    
    // ブロック間の時間差を計算
    timeDiff := lastBlock.Timestamp - prevBlock.Timestamp
    targetTime := int64(10) // 目標10秒
    
    var newDifficulty uint64
    if timeDiff < targetTime/2 {
        // 早すぎる：難易度を上げる
        newDifficulty = lastBlock.Difficulty + 1
    } else if timeDiff > targetTime*2 {
        // 遅すぎる：難易度を下げる
        if lastBlock.Difficulty > 1 {
            newDifficulty = lastBlock.Difficulty - 1
        } else {
            newDifficulty = 1
        }
    } else {
        // 適切：難易度を維持
        newDifficulty = lastBlock.Difficulty
    }
    
    return newDifficulty
}
```

## テストケース

### 1. 基本的なPoW検証
```go
func TestBasicPoW(t *testing.T) {
    block := Block{
        Number:     1,
        ParentHash: "0x0000000000000000000000000000000000000000000000000000000000000000",
        Timestamp:  time.Now().Unix(),
        Nonce:      0,
        Difficulty: 2, // 2つの0が必要
        Transactions: []Transaction{},
        StateRoot:  "0x1234567890abcdef",
    }
    
    // マイニング実行
    minedBlock, err := pow.MineBlock(block, 100000)
    require.NoError(t, err)
    
    // 検証
    assert.True(t, pow.IsValidProof(minedBlock.Hash, 2))
    assert.True(t, strings.HasPrefix(minedBlock.Hash[2:], "00"))
    assert.NotEqual(t, uint64(0), minedBlock.Nonce)
}
```

### 2. 難易度による時間差
```go
func TestDifficultyImpact(t *testing.T) {
    block := Block{
        Number:     1,
        ParentHash: "0x0000000000000000000000000000000000000000000000000000000000000000",
        Timestamp:  time.Now().Unix(),
        Difficulty: 1,
        Transactions: []Transaction{},
        StateRoot:  "0x1234567890abcdef",
    }
    
    // 難易度1でマイニング
    start := time.Now()
    _, err := pow.MineBlock(block, 100000)
    duration1 := time.Since(start)
    require.NoError(t, err)
    
    // 難易度3でマイニング
    block.Difficulty = 3
    start = time.Now()
    _, err = pow.MineBlock(block, 100000)
    duration2 := time.Since(start)
    require.NoError(t, err)
    
    // 難易度が高いほど時間がかかる（統計的に）
    assert.True(t, duration2 > duration1/2) // ばらつきがあるので緩いチェック
}
```

### 3. 無効なProofの拒否
```go
func TestInvalidProofRejection(t *testing.T) {
    block := Block{
        Number:     1,
        ParentHash: "0x0000000000000000000000000000000000000000000000000000000000000000",
        Timestamp:  time.Now().Unix(),
        Nonce:      123,
        Difficulty: 2,
        Transactions: []Transaction{},
        StateRoot:  "0x1234567890abcdef",
    }
    
    block.Hash = pow.CalculateHash(block)
    
    // ランダムなNonceでは無効なはず
    assert.False(t, pow.IsValidProof(block.Hash, 2))
}
```

### 4. 並列マイニング
```go
func TestParallelMining(t *testing.T) {
    block := Block{
        Number:     1,
        ParentHash: "0x0000000000000000000000000000000000000000000000000000000000000000",
        Timestamp:  time.Now().Unix(),
        Difficulty: 2,
        Transactions: []Transaction{},
        StateRoot:  "0x1234567890abcdef",
    }
    
    // シングルスレッド
    start := time.Now()
    _, err := pow.MineBlock(block, 100000)
    singleDuration := time.Since(start)
    require.NoError(t, err)
    
    // 並列
    start = time.Now()
    _, err = pow.MineBlockParallel(block, 100000, 4)
    parallelDuration := time.Since(start)
    require.NoError(t, err)
    
    // 並列の方が速い（環境による）
    t.Logf("Single: %v, Parallel: %v", singleDuration, parallelDuration)
}
```

## 実装手順

1. **PoWパッケージを作成**
   - 検証ロジック
   - マイニングロジック

2. **Block構造体を変更**
   - Nonce, Difficultyフィールド追加

3. **Chainに統合**
   - マイニングロジックの組み込み
   - 難易度調整

4. **RPCを変更**
   - mineBlockの拡張
   - setDifficultyの追加

5. **テストを実装**
   - 基本機能
   - 難易度の影響
   - 並列処理

## 検証方法

### 手動テスト
```bash
# 1. 難易度1でマイニング
curl -s -X POST localhost:8545 -d '{"jsonrpc":"2.0","method":"setDifficulty","params":[1],"id":1}'
time curl -s -X POST localhost:8545 -d '{"jsonrpc":"2.0","method":"mineBlock","params":[],"id":2}'

# 2. 難易度3でマイニング（より時間がかかる）
curl -s -X POST localhost:8545 -d '{"jsonrpc":"2.0","method":"setDifficulty","params":[3],"id":3}'
time curl -s -X POST localhost:8545 -d '{"jsonrpc":"2.0","method":"mineBlock","params":[],"id":4}'

# 3. ブロックのNonceとDifficultyを確認
curl -s -X POST localhost:8545 -d '{"jsonrpc":"2.0","method":"eth_getBlockByNumber","params":["latest",true],"id":5}'
```

### 自動テスト
```bash
go test ./internal/pow
go test ./internal/chain -run TestPoW
```

## 完了条件
- 採掘されたブロックhashが難易度条件を満たす
- nonceを変更するとhashが変わる
- difficultyを上げると採掘に時間がかかる
- 無効なProofが拒否される
- 既存の機能がすべて動作する

## 次のステップ
簡易PoWが実装できたら、Milestone 9でP2P機能を追加します。これにより、複数ノード間でのブロック伝播を学べます。

## 注意点
この実装は本物のEthereum PoW（Ethash）とは異なります：

- 本物：DAG（Directed Acyclic Graph）を使用したメモリハードな計算
- この実装：単純なSHA-256ハッシュ
- 本物：複雑な難易度調整アルゴリズム
- この実装：簡易的な難易度調整

あくまでPoWの基本概念を学ぶための実装です。