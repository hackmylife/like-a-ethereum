# Milestone 10: Fork Choice

## 概要
P2Pネットワークでは、複数のノードがほぼ同時にブロックを生成することがあります。これによりチェーンが分岐（フォーク）することがあります。このマイルストーンでは、どのチェーンを正史（canonical chain）として選択するかのFork Choiceルールを実装します。

## 目的
- 複数のチェーン候補がある場合に、どれを正史にするかを決める
- フォークの概念を理解する
- コンセンサスの基本を学ぶ

## 簡易ルール
```text
最も長いチェーンを採用する
```

PoW追加後の拡張：
```text
total difficultyが最大のチェーンを採用する
```

## フォークの例
```text
Genesis (0)
    |
    A (1)
    |
    B (2)
   / \
  C   D' (3)
 /     \
E       F' (4)
```

- 左側のチェーン：Genesis → A → B → C → E (長さ4)
- 右側のチェーン：Genesis → A → B → D' → F' (長さ4)
- 同じ長さの場合は最初に見つかった方を採用（簡易ルール）

## 実装計画

### 1. ブロックツリー構造
```go
type BlockNode struct {
    Block      *Block
    Parent     *BlockNode
    Children   []*BlockNode
    Height     uint64
    TotalDiff  uint64 // PoW用の合計難易度
}

type BlockTree struct {
    nodes    map[string]*BlockNode // hash -> node
    genesis  *BlockNode
    head     *BlockNode           // 現在のヘッド
}

func NewBlockTree(genesis Block) *BlockTree {
    genesisNode := &BlockNode{
        Block:    &genesis,
        Parent:   nil,
        Children: []*BlockNode{},
        Height:   0,
        TotalDiff: genesis.Difficulty,
    }
    
    return &BlockTree{
        nodes:   map[string]*BlockNode{genesis.Hash: genesisNode},
        genesis: genesisNode,
        head:    genesisNode,
    }
}
```

### 2. ブロック追加ロジック
```go
func (bt *BlockTree) AddBlock(block Block) error {
    // 親ブロックを探す
    parentNode, exists := bt.nodes[block.ParentHash]
    if !exists {
        return errors.New("parent block not found")
    }
    
    // 既存のブロックチェック
    if _, exists := bt.nodes[block.Hash]; exists {
        return errors.New("block already exists")
    }
    
    // 新しいノードを作成
    newNode := &BlockNode{
        Block:     &block,
        Parent:    parentNode,
        Children:  []*BlockNode{},
        Height:    parentNode.Height + 1,
        TotalDiff: parentNode.TotalDiff + block.Difficulty,
    }
    
    // 親の子リストに追加
    parentNode.Children = append(parentNode.Children, newNode)
    bt.nodes[block.Hash] = newNode
    
    // Fork Choice：ヘッドを更新するか確認
    if bt.shouldUpdateHead(newNode) {
        bt.head = newNode
    }
    
    return nil
}

func (bt *BlockTree) shouldUpdateHead(newNode *BlockNode) bool {
    current := bt.head
    
    // 1. 高さで比較
    if newNode.Height > current.Height {
        return true
    }
    
    // 2. 同じ高さの場合はTotal Difficultyで比較
    if newNode.Height == current.Height {
        return newNode.TotalDiff > current.TotalDiff
    }
    
    return false
}
```

### 3. チェーン再構築
```go
func (bt *BlockTree) GetCanonicalChain() []Block {
    var chain []Block
    node := bt.head
    
    // ヘッドからGenesisまで遡る
    for node != nil {
        chain = append([]Block{*node.Block}, chain...)
        node = node.Parent
    }
    
    return chain
}

// ReorgToNewHead は currentBlocks（現在の正史ブロック配列）を受け取り、
// 新しい正史との差分として (削除すべき古いブロック, 追加すべき新しいブロック) を返す。
func (bt *BlockTree) ReorgToNewHead(currentBlocks []Block) ([]Block, []Block) {
    // 新しい正史チェーンを取得
    newChain := bt.GetCanonicalChain()
    
    // 古いチェーン（現在のBlocks）との差分を計算
    var commonAncestor *Block
    var oldBlocksToRemove []Block
    var newBlocksToAdd []Block
    
    // 共通の祖先を見つける
    for i := 0; i < len(newChain) && i < len(currentBlocks); i++ {
        if newChain[i].Hash == currentBlocks[i].Hash {
            commonAncestor = &newChain[i]
        } else {
            break
        }
    }
    
    if commonAncestor != nil {
        // 共通祖先以降の古いブロックを削除
        for _, block := range currentBlocks {
            if block.Number > commonAncestor.Number {
                oldBlocksToRemove = append(oldBlocksToRemove, block)
            }
        }
        
        // 共通祖先以降の新しいブロックを追加
        for _, block := range newChain {
            if block.Number > commonAncestor.Number {
                newBlocksToAdd = append(newBlocksToAdd, block)
            }
        }
    }
    
    return oldBlocksToRemove, newBlocksToAdd
}
```

### 4. Chainへの統合
```go
// Chain構造体に genesisPath と blockTree フィールドを追加する。
type Chain struct {
    mu          sync.Mutex
    State       map[string]Account
    Blocks      []Block
    Mempool     *Mempool
    TxIndex     map[string]TxLocation
    BlockIndex  map[string]uint64
    persistence.Persistence
    p2p         *p2p.P2PManager
    blockTree   *BlockTree // 追加
    genesisPath string     // 追加：rollbackState でGenesis再ロードに使用
}

func (c *Chain) addBlockFromPeer(block Block) error {
    c.mu.Lock()
    defer c.mu.Unlock()
    
    // ブロックツリーに追加
    if err := c.blockTree.AddBlock(block); err != nil {
        return err
    }
    
    // ヘッドが変更されたか確認
    if c.blockTree.head.Block.Hash == block.Hash {
        // リオーグが必要
        return c.reorgToNewHead()
    }
    
    return nil
}

func (c *Chain) reorgToNewHead() error {
    // c.Blocks を渡して現在の正史との差分を計算
    oldBlocks, newBlocks := c.blockTree.ReorgToNewHead(c.Blocks)
    
    // 状態をロールバック
    if err := c.rollbackState(oldBlocks); err != nil {
        return err
    }
    
    // 新しいブロックを適用
    for _, block := range newBlocks {
        if err := c.applyBlock(block); err != nil {
            return err
        }
    }
    
    // Blocks配列を更新
    c.Blocks = c.blockTree.GetCanonicalChain()
    
    // 永続化
    return c.saveToDisk()
}

func (c *Chain) rollbackState(blocks []Block) error {
    // 簡易実装：Genesisから再計算
    // 本物ではより効率的なロールバックが必要
    
    // Genesis状態にリセット（c.genesisPath は Chain 構造体のフィールド）
    genesis, err := NewChainFromGenesis(c.genesisPath)
    if err != nil {
        return err
    }
    c.State = genesis.State
    
    // 新しい正史のブロックを再適用
    for _, block := range c.blockTree.GetCanonicalChain() {
        if block.Number == 0 {
            continue // Genesisはスキップ
        }
        
        for _, tx := range block.Transactions {
            if err := c.applyTransaction(tx); err != nil {
                return err
            }
        }
    }
    
    return nil
}
```

### 5. RPCの追加
```go
// getForkStatus - フォーク状態を取得
case "getForkStatus":
    c.mu.Lock()
    defer c.mu.Unlock()
    
    return map[string]any{
        "headHash":     c.blockTree.head.Block.Hash,
        "headNumber":   toHex(c.blockTree.head.Block.Number),
        "totalDiff":    toHex(c.blockTree.head.TotalDiff),
        "chainLength":  toHex(uint64(len(c.Blocks))),
        "treeNodes":    toHex(uint64(len(c.blockTree.nodes))),
    }, nil

// getBlockByHash - ツリーからブロックを取得
case "getBlockByHash":
    var hash string
    if err := parseSingleObjectParam(params, &hash); err != nil {
        return nil, rpcInvalidParams(err)
    }
    
    c.mu.Lock()
    defer c.mu.Unlock()
    
    if node, exists := c.blockTree.nodes[hash]; exists {
        return map[string]any{
            "block":      node.Block,
            "height":     toHex(node.Height),
            "totalDiff":  toHex(node.TotalDiff),
            "isHead":     node == c.blockTree.head,
            "parentHash": node.Block.ParentHash,
        }, nil
    }
    
    return nil, nil
```

## テストケース

### 1. 基本的なフォーク処理
```go
func TestBasicFork(t *testing.T) {
    tree := NewBlockTree(createGenesisBlock())
    
    // ブロックAを追加
    blockA := createBlock(1, tree.genesis.Block.Hash, 1)
    require.NoError(t, tree.AddBlock(blockA))
    assert.Equal(t, blockA.Hash, tree.head.Block.Hash)
    
    // ブロックBを追加
    blockB := createBlock(2, blockA.Hash, 1)
    require.NoError(t, tree.AddBlock(blockB))
    assert.Equal(t, blockB.Hash, tree.head.Block.Hash)
    
    // フォーク：ブロックB'を追加
    blockB2 := createBlock(2, blockA.Hash, 1)
    blockB2.Hash = "different_hash" // 別のハッシュ
    require.NoError(t, tree.AddBlock(blockB2))
    
    // ヘッドは変わらない（同じ高さ）
    assert.Equal(t, blockB.Hash, tree.head.Block.Hash)
}
```

### 2. 長いチェーンへの切り替え
```go
func TestLongerChainWins(t *testing.T) {
    tree := NewBlockTree(createGenesisBlock())
    
    // チェーン1：Genesis -> A -> B
    blockA := createBlock(1, tree.genesis.Block.Hash, 1)
    tree.AddBlock(blockA)
    
    blockB := createBlock(2, blockA.Hash, 1)
    tree.AddBlock(blockB)
    
    assert.Equal(t, blockB.Hash, tree.head.Block.Hash)
    
    // チェーン2：Genesis -> A' -> B' -> C'
    blockA2 := createBlock(1, tree.genesis.Block.Hash, 1)
    blockA2.Hash = "alt_A_hash"
    tree.AddBlock(blockA2)
    
    blockB2 := createBlock(2, blockA2.Hash, 1)
    blockB2.Hash = "alt_B_hash"
    tree.AddBlock(blockB2)
    
    // まだチェーン1の方が長い
    assert.Equal(t, blockB.Hash, tree.head.Block.Hash)
    
    // チェーン2を延長
    blockC2 := createBlock(3, blockB2.Hash, 1)
    blockC2.Hash = "alt_C_hash"
    tree.AddBlock(blockC2)
    
    // チェーン2の方が長くなったのでヘッドが切り替わる
    assert.Equal(t, blockC2.Hash, tree.head.Block.Hash)
}
```

### 3. Total Difficultyでの選択
```go
func TestTotalDifficultyChoice(t *testing.T) {
    tree := NewBlockTree(createGenesisBlock())
    
    // チェーン1：低難易度
    blockA := createBlock(1, tree.genesis.Block.Hash, 1)
    tree.AddBlock(blockA)
    
    blockB := createBlock(2, blockA.Hash, 1)
    tree.AddBlock(blockB)
    
    // チェーン2：高難易度
    blockA2 := createBlock(1, tree.genesis.Block.Hash, 2)
    blockA2.Hash = "high_diff_A"
    tree.AddBlock(blockA2)
    
    blockB2 := createBlock(2, blockA2.Hash, 2)
    blockB2.Hash = "high_diff_B"
    tree.AddBlock(blockB2)
    
    // 同じ高さでもTotal Diffが高いチェーン2が勝つ
    assert.Equal(t, blockB2.Hash, tree.head.Block.Hash)
    
    // 確認
    nodeB := tree.nodes[blockB.Hash]
    nodeB2 := tree.nodes[blockB2.Hash]
    assert.Equal(t, uint64(3), nodeB.TotalDiff)   // 1+1+1
    assert.Equal(t, uint64(5), nodeB2.TotalDiff)  // 1+2+2
}
```

### 4. リオーグテスト
```go
func TestReorganization(t *testing.T) {
    chain := setupTestChain(t, map[string]uint64{
        aliceAddr: 1000,
    })
    
    // 初期状態
    initialBalance := chain.State[aliceAddr].Balance
    
    // ブロック1：AliceからBobへ100送金
    tx1 := createAndSignTransaction(t, alicePriv, bobAddr, 100, 0)
    block1 := createBlockWithTxs(1, chain.Blocks[0].Hash, []Transaction{tx1})
    chain.addBlockFromPeer(block1)
    
    assert.Equal(t, uint64(900), chain.State[aliceAddr].Balance)
    
    // フォークブロック：AliceからCharlieへ50送金
    tx2 := createAndSignTransaction(t, alicePriv, charlieAddr, 50, 0)
    block1Fork := createBlockWithTxs(1, chain.Blocks[0].Hash, []Transaction{tx2})
    block1Fork.Hash = "fork_block_hash"
    
    // フォークチェーンを延長して長くする
    block2 := createBlock(2, block1Fork.Hash, 1)
    chain.addBlockFromPeer(block1Fork)
    chain.addBlockFromPeer(block2)
    
    // リオーグが発生し、状態が変わる
    assert.Equal(t, uint64(950), chain.State[aliceAddr].Balance) // 1000-50
    assert.Equal(t, uint64(50), chain.State[charlieAddr].Balance)
}
```

## 実装手順

1. **BlockTree構造を実装**
   - ノード管理
   - 追加ロジック

2. **Fork Choiceルールを実装**
   - ヘッド選択
   - チェーン比較

3. **リオーグ機能を実装**
   - 状態ロールバック
   - 再適用

4. **Chainに統合**
   - ブロック受信時の処理

5. **テストを実装**
   - 基本機能
   - リオーグ

## 検証方法

### 手動テスト
```bash
# 3つのノードを起動
go run . node --genesis genesis.json --datadir ./data1 --addr :8545 &
go run . node --genesis genesis.json --datadir ./data2 --addr :8546 &
go run . node --genesis genesis.json --datadir ./data3 --addr :8547 &

# ピア接続
curl -s -X POST localhost:8545 -d '{"jsonrpc":"2.0","method":"admin_addPeer","params":["http://localhost:8546"],"id":1}'
curl -s -X POST localhost:8545 -d '{"jsonrpc":"2.0","method":"admin_addPeer","params":["http://localhost:8547"],"id":2}'

# 異なるノードで同時にマイニングしてフォークを作成
# フォーク状態を確認
curl -s -X POST localhost:8545 -d '{"jsonrpc":"2.0","method":"getForkStatus","params":[],"id":3}'
```

## 完了条件
- 同じ親から2つの子ブロックができる
- 長い方がheadになる
- head切り替え時にstateも切り替わる
- リオーグが正しく動作する

## 次のステップ
Fork Choiceが実装できたら、Milestone 11でGas風の仕組みを実装します。これにより、トランザクション手数料の概念を学べます。

## 注意点
この実装は簡易的なFork Choiceです：

- 本物のEthereum：GHOST协议、Uncleブロック考慮
- この実装：単純な最長チェーンルール
- 本物：複雑なリオーグ最適化
- この実装：単純な全状態再計算

実際のEthereumではより効率的な状態管理と、より高度なFork Choiceアルゴリズムが使用されています。