# Milestone 11: Gas風の仕組み

## 概要
現在の実装ではトランザクションは無料で処理されていますが、実際のEthereumではGasという手数料機構があります。このマイルストーンでは、簡易的なGas仕組みを実装して、トランザクション手数料の基本を学びます。

## 目的
- トランザクション手数料の基本を学ぶ
- マイナーへのインセンティブを理解する
- リソース消費のコスト概念を学ぶ

## 単純化したルール
```text
送金txのgasUsed = 21000
手数料 = gasUsed * gasPrice
from.balance -= value + fee
miner.balance += fee
```

## 新しいフィールド

### Transaction構造体の変更
```go
type Transaction struct {
    From     string
    To       string
    Value    uint64
    Nonce    uint64
    GasLimit uint64        // 追加
    GasPrice uint64        // 追加
    PubKey   string
    Signature string
    Hash     string
}
```

### Block構造体の変更
```go
type Block struct {
    Number       uint64
    ParentHash   string
    Timestamp    int64
    Nonce        uint64
    Difficulty   uint64
    Coinbase     string        // 追加：マイナーアドレス
    Transactions []Transaction
    StateRoot    string
    Hash         string
}
```

## 実装計画

### 1. Gas計算ロジック
```go
package gas

import (
    "errors"
    "fmt"
)

const (
    // 簡易的なGasコスト
    GasTransaction = 21000 // 送金トランザクションの基本コスト
    GasContractCreate = 100000 // コントラクト作成（将来用）
)

type GasCalculator struct{}

func NewGasCalculator() *GasCalculator {
    return &GasCalculator{}
}

func (gc *GasCalculator) EstimateGas(tx Transaction) (uint64, error) {
    // トランザクションタイプに応じてGasを見積もる
    if tx.To == "" {
        return GasContractCreate, nil
    }
    
    return GasTransaction, nil
}

func (gc *GasCalculator) CalculateGasUsed(tx Transaction) uint64 {
    // 実際の使用量を計算（現時点では固定）
    if tx.To == "" {
        return GasContractCreate
    }
    
    return GasTransaction
}

func (gc *GasCalculator) CalculateFee(tx Transaction) uint64 {
    gasUsed := gc.CalculateGasUsed(tx)
    return gasUsed * tx.GasPrice
}

func (gc *GasCalculator) ValidateGas(tx Transaction, accountBalance uint64) error {
    // GasLimitのチェック
    gasUsed := gc.CalculateGasUsed(tx)
    if tx.GasLimit < gasUsed {
        return fmt.Errorf("gas limit too low: need %d, got %d", gasUsed, tx.GasLimit)
    }
    
    // 残高チェック（手数料を含む）
    fee := gc.CalculateFee(tx)
    totalCost := tx.Value + fee
    
    if accountBalance < totalCost {
        return fmt.Errorf("insufficient balance for value + fee: need %d, got %d", totalCost, accountBalance)
    }
    
    return nil
}
```

### 2. マイナー報酬
```go
type MinerReward struct {
    Coinbase string
    BlockReward uint64
    GasFees    uint64
    Total      uint64
}

func CalculateBlockReward(blockNumber uint64) uint64 {
    // 簡易的なブロック報酬：最初は5 ETH、徐々に減少
    if blockNumber < 100 {
        return 5 * 1e18 // 5 ETH（Wei単位）
    } else if blockNumber < 200 {
        return 3 * 1e18 // 3 ETH
    } else {
        return 2 * 1e18 // 2 ETH
    }
}

func (c *Chain) distributeRewards(block *Block, totalGasFees uint64) error {
    blockReward := CalculateBlockReward(block.Number)
    
    // マイナーに報酬を分配
    miner := c.State[block.Coinbase]
    if miner.Address == "" {
        // マイナーアカウントがなければ作成
        miner = Account{
            Address: block.Coinbase,
            Balance: 0,
            Nonce:   0,
        }
    }
    
    miner.Balance += blockReward + totalGasFees
    c.State[block.Coinbase] = miner
    
    return nil
}
```

### 3. ヘルパー型・関数

```go
// TransactionSorter は GasPrice の高い順にトランザクションをソートする
type TransactionSorter []Transaction

func (s TransactionSorter) Len() int           { return len(s) }
func (s TransactionSorter) Less(i, j int) bool { return s[i].GasPrice > s[j].GasPrice }
func (s TransactionSorter) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

// copyState は現在の c.State のシャローコピーを返す。
func (c *Chain) copyState() map[string]Account {
    copied := make(map[string]Account, len(c.State))
    for k, v := range c.State {
        copied[k] = v
    }
    return copied
}

// applyTransactionToStateWithGas は tempState に対してトランザクションをGas込みで適用する。
func (c *Chain) applyTransactionToStateWithGas(state map[string]Account, tx Transaction) error {
    from := state[tx.From]
    if from.Address == "" {
        return errors.New("from account does not exist")
    }
    
    gasCalc := gas.NewGasCalculator()
    if err := gasCalc.ValidateGas(tx, from.Balance); err != nil {
        return err
    }
    
    if tx.Nonce != from.Nonce {
        return fmt.Errorf("bad nonce: got %d, want %d", tx.Nonce, from.Nonce)
    }
    
    gasFee := gasCalc.CalculateFee(tx)
    totalCost := tx.Value + gasFee
    if from.Balance < totalCost {
        return errors.New("insufficient balance")
    }
    
    to := state[tx.To]
    if to.Address == "" {
        to = Account{Address: tx.To}
    }
    
    from.Balance -= totalCost
    from.Nonce++
    to.Balance += tx.Value
    
    state[from.Address] = from
    state[to.Address] = to
    return nil
}

// calculateGasUsed はブロック内の全トランザクションの gasUsed 合計を返す。
func (c *Chain) calculateGasUsed(block *Block) uint64 {
    gasCalc := gas.NewGasCalculator()
    var total uint64
    for _, tx := range block.Transactions {
        total += gasCalc.CalculateGasUsed(tx)
    }
    return total
}

// parseMineBlockParamsWithCoinbase は mineBlock RPC のパラメータ
// {"difficulty": N, "coinbase": "0x..."} をパースする。
func parseMineBlockParamsWithCoinbase(params json.RawMessage) (struct {
    Difficulty uint64
    Coinbase   string
}, error) {
    var result struct {
        Difficulty uint64
        Coinbase   string
    }
    
    var arr []json.RawMessage
    if err := json.Unmarshal(params, &arr); err != nil || len(arr) == 0 {
        return result, errors.New("invalid params")
    }
    
    var obj struct {
        Difficulty uint64 `json:"difficulty"`
        Coinbase   string `json:"coinbase"`
    }
    if err := json.Unmarshal(arr[0], &obj); err != nil {
        return result, err
    }
    
    result.Difficulty = obj.Difficulty
    result.Coinbase = obj.Coinbase
    return result, nil
}
```

### 4. トランザクション処理の変更
```go
func (c *Chain) applyTransactionWithGas(tx Transaction) error {
    from := c.State[tx.From]
    if from.Address == "" {
        return errors.New("from account does not exist")
    }
    
    // Gas計算機を初期化
    gasCalc := gas.NewGasCalculator()
    
    // Gas関連の検証
    if err := gasCalc.ValidateGas(tx, from.Balance); err != nil {
        return err
    }
    
    // nonceチェック
    if tx.Nonce != from.Nonce {
        return fmt.Errorf("bad nonce: got %d, want %d", tx.Nonce, from.Nonce)
    }
    
    // Gas計算
    gasUsed := gasCalc.CalculateGasUsed(tx)
    gasFee := gasCalc.CalculateFee(tx)
    
    // 残高チェック（再確認）
    totalCost := tx.Value + gasFee
    if from.Balance < totalCost {
        return errors.New("insufficient balance")
    }
    
    // 宛先アカウントの準備
    to := c.State[tx.To]
    if to.Address == "" {
        to = Account{
            Address: tx.To,
            Balance: 0,
            Nonce:   0,
        }
    }
    
    // 状態更新
    from.Balance -= totalCost
    from.Nonce++
    to.Balance += tx.Value
    
    c.State[from.Address] = from
    c.State[to.Address] = to
    
    return nil
}

func (c *Chain) mineBlockWithGas(difficulty uint64, coinbase string) (*Block, error) {
    c.mu.Lock()
    defer c.mu.Unlock()
    
    // mempoolからトランザクションを取得
    allTxs := c.Mempool.GetAll()
    if len(allTxs) == 0 {
        return nil, errors.New("no transactions in mempool")
    }
    
    // トランザクションをソートと検証
    sort.Sort(TransactionSorter(allTxs))
    
    var appliedTxs []Transaction
    var totalGasFees uint64
    tempState := c.copyState()
    
    for _, tx := range allTxs {
        // Gasを含めて検証
        if err := c.applyTransactionToStateWithGas(tempState, tx); err != nil {
            continue
        }
        
        appliedTxs = append(appliedTxs, tx)
        
        // Gas手数料を計算
        gasCalc := gas.NewGasCalculator()
        totalGasFees += gasCalc.CalculateFee(tx)
    }
    
    if len(appliedTxs) == 0 {
        return nil, errors.New("no valid transactions")
    }
    
    // 状態をマージ
    c.State = tempState
    
    // ブロック生成
    block := Block{
        Number:       c.Blocks[len(c.Blocks)-1].Number + 1,
        ParentHash:   c.Blocks[len(c.Blocks)-1].Hash,
        Timestamp:    time.Now().Unix(),
        Nonce:        0,
        Difficulty:   difficulty,
        Coinbase:     coinbase,
        Transactions: appliedTxs,
        StateRoot:    c.computeStateRootLocked(),
    }
    
    // マイニング実行
    minedBlock, err := pow.MineBlock(block, 1000000)
    if err != nil {
        return nil, err
    }
    
    // 報酬分配
    if err := c.distributeRewards(&minedBlock, totalGasFees); err != nil {
        return nil, err
    }
    
    // ブロックを追加
    c.Blocks = append(c.Blocks, minedBlock)
    
    // mempoolをクリア
    c.Mempool.Clear()
    
    return &minedBlock, nil
}
```

### 4. RPCの変更
```go
// sendTransaction - Gasフィールドをサポート
case "sendTransaction":
    var tx Transaction
    if err := parseSingleObjectParam(params, &tx); err != nil {
        return nil, rpcInvalidParams(err)
    }
    
    // デフォルト値を設定
    if tx.GasLimit == 0 {
        gasCalc := gas.NewGasCalculator()
        estimated, _ := gasCalc.EstimateGas(tx)
        tx.GasLimit = estimated
    }
    
    if tx.GasPrice == 0 {
        tx.GasPrice = 1e9 // 1 Gwei（デフォルト）
    }
    
    // 署名・ハッシュを検証（既存の validateTransaction を使用）
    if err := validateTransaction(tx); err != nil {
        return nil, rpcInvalidParams(err)
    }
    
    if err := c.Mempool.Add(tx); err != nil {
        return nil, rpcInvalidParams(err)
    }
    
    return map[string]any{
        "transactionHash": tx.Hash,
        "gasLimit":       toHex(tx.GasLimit),
        "gasPrice":       toHex(tx.GasPrice),
        "status":         "pending",
    }, nil

// mineBlock - coinbaseパラメータをサポート
case "mineBlock":
    params, err := parseMineBlockParamsWithCoinbase(req.Params)
    if err != nil {
        return nil, rpcInvalidParams(err)
    }
    
    block, err := c.mineBlockWithGas(params.Difficulty, params.Coinbase)
    if err != nil {
        return nil, rpcInvalidParams(err)
    }
    
    return map[string]any{
        "blockNumber": toHex(block.Number),
        "blockHash":   block.Hash,
        "coinbase":    block.Coinbase,
        "gasUsed":     toHex(c.calculateGasUsed(block)),
    }, nil

// eth_estimateGas - Gas見積もり
case "eth_estimateGas":
    var tx Transaction
    if err := parseSingleObjectParam(params, &tx); err != nil {
        return nil, rpcInvalidParams(err)
    }
    
    gasCalc := gas.NewGasCalculator()
    estimated, err := gasCalc.EstimateGas(tx)
    if err != nil {
        return nil, rpcInvalidParams(err)
    }
    
    return toHex(estimated), nil
```

### 5. CLIの変更
```go
func cmdSign(args []string) {
    fs := flag.NewFlagSet("sign", flag.ExitOnError)
    
    privHex := fs.String("priv", "", "private key hex")
    to := fs.String("to", "", "recipient address")
    value := fs.Uint64("value", 0, "value")
    nonce := fs.Uint64("nonce", 0, "nonce")
    gasLimit := fs.Uint64("gaslimit", 0, "gas limit (optional)")
    gasPrice := fs.Uint64("gasprice", 0, "gas price in wei (optional)")
    
    must(fs.Parse(args))
    
    // ... 既存の処理 ...
    
    tx := Transaction{
        From:     addressFromPubkey(pub),
        To:       toAddr,
        Value:    *value,
        Nonce:    *nonce,
        GasLimit: *gasLimit,
        GasPrice: *gasPrice,
        PubKey:   PREFIX + hex.EncodeToString(pub),
    }
    
    // ... 署名処理 ...
}
```

## テストケース

### 1. 基本的なGas手数料
```go
func TestBasicGasFee(t *testing.T) {
    chain := setupTestChain(t, map[string]uint64{
        aliceAddr:   1000,
        minerAddr:   0,
    })
    
    tx := createAndSignTransactionWithGas(t, alicePriv, bobAddr, 100, 0, 21000, 10)
    
    block, err := chain.mineBlockWithGas(1, minerAddr)
    require.NoError(t, err)
    
    // Aliceの残高：1000 - 100 - (21000 * 10) = 790
    alice := chain.State[aliceAddr]
    assert.Equal(t, uint64(790), alice.Balance)
    
    // Bobの残高：100
    bob := chain.State[bobAddr]
    assert.Equal(t, uint64(100), bob.Balance)
    
    // マイナーの残高：ブロック報酬 + Gas手数料
    miner := chain.State[minerAddr]
    expectedReward := CalculateBlockReward(block.Number) + (21000 * 10)
    assert.Equal(t, expectedReward, miner.Balance)
}
```

### 2. Gas不足のトランザクション
```go
func TestInsufficientGas(t *testing.T) {
    chain := setupTestChain(t, map[string]uint64{
        aliceAddr: 100,
    })
    
    // GasLimitが小さいトランザクション
    tx := createAndSignTransactionWithGas(t, alicePriv, bobAddr, 50, 0, 1000, 10) // GasLimit不足
    
    err := chain.Mempool.Add(tx)
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "gas limit too low")
}
```

### 3. 残高不足（Gas手数料込み）
```go
func TestInsufficientBalanceWithGas(t *testing.T) {
    chain := setupTestChain(t, map[string]uint64{
        aliceAddr: 100,
    })
    
    // 残高不足：100 < 50 + (21000 * 10)
    tx := createAndSignTransactionWithGas(t, alicePriv, bobAddr, 50, 0, 21000, 10)
    
    err := chain.Mempool.Add(tx)
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "insufficient balance")
}
```

### 4. GasPriceの影響
```go
func TestGasPriceImpact(t *testing.T) {
    chain := setupTestChain(t, map[string]uint64{
        aliceAddr: 1000,
        minerAddr: 0,
    })
    
    // 高いGasPrice
    tx1 := createAndSignTransactionWithGas(t, alicePriv, bobAddr, 100, 0, 21000, 20)
    
    // 低いGasPrice
    tx2 := createAndSignTransactionWithGas(t, alicePriv, charlieAddr, 100, 1, 21000, 5)
    
    chain.Mempool.Add(tx1)
    chain.Mempool.Add(tx2)
    
    block, err := chain.mineBlockWithGas(1, minerAddr)
    require.NoError(t, err)
    
    // 高いGasPriceのトランザクションが多くの手数料を発生
    miner := chain.State[minerAddr]
    expectedFees := (21000 * 20) + (21000 * 5) // 合計Gas手数料
    blockReward := CalculateBlockReward(block.Number)
    assert.Equal(t, blockReward+expectedFees, miner.Balance)
}
```

## 実装手順

1. **Gasパッケージを作成**
   - 計算ロジック
   - 検証ロジック

2. **トランザクション構造体を変更**
   - GasLimit, GasPrice追加

3. **トランザクション処理を変更**
   - Gasを含めた検証と処理

4. **マイナー報酬を実装**
   - Coinbaseフィールド
   - 報酬分配

5. **CLIとRPCを変更**
   - パラメータ追加
   - 新しいRPC

6. **テストを実装**
   - 基本機能
   - エラーケース

## 検証方法

### 手動テスト
```bash
# Gas付きでトランザクション署名
go run . sign --priv 0x... --to 0x... --value 100 --nonce 0 --gaslimit 21000 --gasprice 1000000000

# マイナー指定でブロック採掘
curl -s -X POST localhost:8545 -d '{"jsonrpc":"2.0","method":"mineBlock","params":[{"difficulty":1,"coinbase":"0xminer"}],"id":1}'

# マイナーの残高を確認（報酬＋手数料）
curl -s -X POST localhost:8545 -d '{"jsonrpc":"2.0","method":"eth_getBalance","params":["0xminer","latest"],"id":2}'

# Gas見積もり
curl -s -X POST localhost:8545 -d '{"jsonrpc":"2.0","method":"eth_estimateGas","params":[{"from":"0x...","to":"0x...","value":"0x64"}],"id":3}'
```

## 完了条件
- 送金額だけでなくfeeも引かれる
- minerにfeeが入る
- 残高不足なら失敗する
- GasLimitが足りないと失敗する
- 既存の機能がすべて動作する

## 次のステップ
Gas仕組みが実装できたら、Milestone 12でContract風機能を実装します。これにより、スマートコントラクトの基本概念を学べます。

## 注意点
この実装は本物のEthereum Gas機構とは異なります：

- 本物：EVM実行、複雑なGasスケジュール
- この実装：固定Gasコスト
- 本物：Gasリファンド、複雑な計算
- この実装：単純な手数料計算

あくまでGasの基本概念を学ぶための簡易実装です。