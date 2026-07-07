# Milestone 12: Gas風の仕組み

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
    // 簡易的なブロック報酬：徐々に減少
    // 注意: 本物のEthereumはWei単位（5 ETH = 5e18）だが、この実装のgenesis残高は
    // 1000程度の小さな整数なので、単位系を合わせて小さな値にしておく。
    // Wei単位を導入する場合はgenesis.jsonの残高も一緒に桁を上げること。
    if blockNumber < 100 {
        return 50
    } else if blockNumber < 200 {
        return 30
    } else {
        return 20
    }
}

// distributeRewardsToState はブロック生成中の一時stateに対して報酬を分配する。
// 重要: StateRoot計算前（マイニング前）に呼ぶこと。マイニング後に c.State へ直接
// 反映すると、ブロックのStateRootにマイナー報酬が含まれず、検証で不一致になる。
func distributeRewardsToState(state map[string]Account, coinbase string, blockNumber uint64, totalGasFees uint64) error {
    blockReward := CalculateBlockReward(blockNumber)
    
    // マイナーに報酬を分配
    miner := state[coinbase]
    if miner.Address == "" {
        // マイナーアカウントがなければ作成
        miner = Account{
            Address: coinbase,
            Balance: 0,
            Nonce:   0,
        }
    }
    
    miner.Balance += blockReward + totalGasFees
    state[coinbase] = miner
    
    return nil
}
```

### 3. ヘルパー型・関数

```go
// TransactionSorter は Milestone 5 と同じ (From, Nonce) 順を維持する。
//
// 注意: 単純に「GasPriceの高い順」でソートすると、同一送信元の nonce=1 が
// nonce=0 より先に並んで適用に失敗する。かといって「同一送信元はnonce順、
// それ以外はgasPrice順」という比較関数は推移律を満たさず sort が壊れる。
// gasPrice優先を本格的にやる場合は、本物のEthereumと同様に
// 「送信元ごとにnonce順のキューを作り、各キューの先頭をgasPriceで選ぶ」
// 方式にする。このマイルストーンではソート順は変えず、gasPriceは手数料の
// 計算にのみ使う。
type TransactionSorter []Transaction

func (s TransactionSorter) Len() int { return len(s) }
func (s TransactionSorter) Less(i, j int) bool {
    if s[i].From != s[j].From {
        return s[i].From < s[j].From
    }
    return s[i].Nonce < s[j].Nonce
}
func (s TransactionSorter) Swap(i, j int) { s[i], s[j] = s[j], s[i] }

// copyState / applyTransactionToState は Milestone 5 で実装済み（ここは再掲）。
// Gas込みの検証・適用は下の applyTransactionToStateWithGas を使う。
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
    
    // 報酬分配もtempStateに対して行う（StateRootに報酬を反映させるため、
    // StateRoot計算・マイニングより前に行う必要がある）
    newBlockNumber := c.Blocks[len(c.Blocks)-1].Number + 1
    if err := distributeRewardsToState(tempState, coinbase, newBlockNumber, totalGasFees); err != nil {
        return nil, err
    }
    
    // ブロック生成（StateRootは報酬分配まで反映したtempStateから計算。
    // computeStateRootOf はMilestone 8で導入済み）
    block := Block{
        Number:       newBlockNumber,
        ParentHash:   c.Blocks[len(c.Blocks)-1].Hash,
        Timestamp:    time.Now().Unix(),
        Nonce:        0,
        Difficulty:   difficulty,
        Coinbase:     coinbase,
        Transactions: appliedTxs,
        StateRoot:    computeStateRootOf(tempState),
    }
    
    // マイニング実行（失敗したらtempStateを破棄するだけで済む）
    minedBlock, err := pow.MineBlock(block, 1000000)
    if err != nil {
        return nil, err
    }
    
    // マイニング成功後に状態をマージ
    c.State = tempState
    
    // ブロックを追加
    c.Blocks = append(c.Blocks, minedBlock)
    
    // 成功したtxだけをmempoolから削除（失敗txは残す。Clear()で全消ししない）
    for _, tx := range appliedTxs {
        c.Mempool.Remove(tx.Hash)
    }
    
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
    // 注意: GasLimit/GasPriceを署名対象に含める設計にする場合、サーバ側での補完は
    // 署名を壊すため、ここで補完せず未設定txを拒否する（文末の注意点を参照）
    if tx.GasLimit == 0 {
        gasCalc := gas.NewGasCalculator()
        estimated, _ := gasCalc.EstimateGas(tx)
        tx.GasLimit = estimated
    }
    
    if tx.GasPrice == 0 {
        // 本物は1 Gwei = 1e9 Wei程度だが、この実装の残高スケールでは
        // 手数料（21000 × gasPrice）が払えなくなるため、デフォルトは1にする
        tx.GasPrice = 1
    }
    
    // 署名・ハッシュを検証（validateTransaction は tx.VerifyAndNormalizeTx を呼ぶ薄いラッパー）
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
    // gasUsed=21000のため、手数料(21000×gasPrice)を払える残高が必要
    chain := setupTestChain(t, map[string]uint64{
        aliceAddr:   1_000_000,
        minerAddr:   0,
    })
    
    tx := createAndSignTransactionWithGas(t, alicePriv, bobAddr, 100, 0, 21000, 10)
    chain.Mempool.Add(tx)
    
    block, err := chain.mineBlockWithGas(1, minerAddr)
    require.NoError(t, err)
    
    // Aliceの残高：1,000,000 - 100 - (21000 * 10) = 789,900
    alice := chain.State[aliceAddr]
    assert.Equal(t, uint64(789_900), alice.Balance)
    
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
        aliceAddr: 1_000_000,
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
go run ./cmd/minieth sign --priv 0x... --to 0x... --value 100 --nonce 0 --gaslimit 21000 --gasprice 1000000000

# マイナー指定でブロック採掘
curl -s -X POST localhost:8545 -d '{"jsonrpc":"2.0","method":"mineBlock","params":[{"difficulty":1,"coinbase":"0xminer"}],"id":1}'

# マイナーの残高を確認（報酬＋手数料）
curl -s -X POST localhost:8545 -d '{"jsonrpc":"2.0","method":"eth_getBalance","params":["0xminer","latest"],"id":2}'

# Gas見積もり
curl -s -X POST localhost:8545 -d '{"jsonrpc":"2.0","method":"eth_estimateGas","params":[{"from":"0x...","to":"0x...","value":100}],"id":3}'
# 注意: Transaction.Value はuint64なので、この実装ではJSON数値で渡す
#（本物のeth_estimateGasは "0x64" のようなhex quantity文字列を受ける）
```

## 完了条件
- 送金額だけでなくfeeも引かれる
- minerにfeeが入る
- 残高不足なら失敗する
- GasLimitが足りないと失敗する
- 既存の機能がすべて動作する

## 次のステップ
Gas仕組みが実装できたら、Milestone 13でContract風機能を実装します。これにより、スマートコントラクトの基本概念を学べます。

## 注意点
この実装は本物のEthereum Gas機構とは異なります：

- 本物：EVM実行、複雑なGasスケジュール
- この実装：固定Gasコスト
- 本物：Gasリファンド、複雑な計算
- この実装：単純な手数料計算

あくまでGasの基本概念を学ぶための簡易実装です。

### 署名対象にGasフィールドを含めること

GasLimit / GasPrice を署名対象（txSignBytes）に含めないと、第三者が手数料部分を
改ざんできてしまいます。一方で署名対象に含めるなら、サーバ側でデフォルト値を
補完する設計（上のsendTransaction例）とは両立しません（補完した時点で署名不一致になる）。
実装時はクライアント（signコマンド）側で必ず値を決めてから署名する方式に統一してください。

### 単位系について

本物のEthereumはWei（1 ETH = 1e18 Wei）単位ですが、この実装のgenesis残高は1000程度の
小さな整数です。gasUsed=21000を採用する場合、手数料（21000 × gasPrice）を払える残高が
必要になるため、genesis.jsonの初期残高を十分大きく（例: 1,000,000以上）してください。

### コンセンサスモードとの関係

この章のコード例はPoWモード（mineBlock）前提です。QBFTモード（Milestone 11）には
mineBlockが無いため、proposerのブロック構築処理（BuildBlockProposal）に同じ
Gas検証・報酬分配ロジックを組み込み、報酬の受け取り先は `block.Proposer` にします。