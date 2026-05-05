# Milestone 6: stateRootをMerkle Tree風にする

## 概要
現在のstateRootは「ソート済みアカウント一覧のSHA-256」ですが、実際のEthereumではMerkle Patricia Trieが使用されています。このステップでは、Merkle Treeの基本構造を学ぶために、stateRootをMerkle Tree風に実装します。

## 現在の実装
```text
sort(accounts)
json.Marshal(accounts)
sha256
```

## 変更後の実装
```text
leaf = hash(address, balance, nonce)
parent = hash(left, right)
root = root hash
```

## 目的
- Merkle Treeの基本構造を学ぶ
- 状態証明への準備をする
- 効率的な状態変更検出を理解する

## Merkle Treeの基本

### 1. リーフノード
各アカウントからハッシュを生成：
```go
type StateLeaf struct {
    Address string
    Balance uint64
    Nonce   uint64
}

func (l StateLeaf) Hash() string {
    data := fmt.Sprintf("%s|%d|%d", l.Address, l.Balance, l.Nonce)
    return hashString(data)
}
```

### 2. 親ノード
2つの子ノードのハッシュを結合：
```go
func hashParent(left, right string) string {
    data := fmt.Sprintf("%s|%s", left, right)
    return hashString(data)
}
```

### 3. ツリー構築
リーフノードから上向きにツリーを構築：
```go
type MerkleNode struct {
    Left  *MerkleNode
    Right *MerkleNode
    Hash  string
}

type MerkleTree struct {
    Root *MerkleNode
}
```

## 実装計画

### 1. 基本的なMerkle Tree実装
```go
package merkletree

import (
    "fmt"
    "sort"
)

type MerkleNode struct {
    Left  *MerkleNode
    Right *MerkleNode
    Hash  string
}

type MerkleTree struct {
    Root *MerkleNode
}

type StateLeaf struct {
    Address string
    Balance uint64
    Nonce   uint64
}

func (l StateLeaf) Hash() string {
    data := fmt.Sprintf("%s|%d|%d", l.Address, l.Balance, l.Nonce)
    return hashString(data)
}

func NewMerkleTree(leaves []StateLeaf) *MerkleTree {
    if len(leaves) == 0 {
        return &MerkleTree{
            Root: &MerkleNode{Hash: hashString("")},
        }
    }
    
    // リーフノードを作成
    var nodes []*MerkleNode
    for _, leaf := range leaves {
        nodes = append(nodes, &MerkleNode{
            Hash: leaf.Hash(),
        })
    }
    
    // ツリーを構築
    for len(nodes) > 1 {
        var nextLevel []*MerkleNode
        
        // ペアで親ノードを作成
        for i := 0; i < len(nodes); i += 2 {
            left := nodes[i]
            var right *MerkleNode
            
            if i+1 < len(nodes) {
                right = nodes[i+1]
            } else {
                // 奇数の場合は最後のノードを複製
                right = &MerkleNode{Hash: left.Hash}
            }
            
            parent := &MerkleNode{
                Left:  left,
                Right: right,
                Hash:  hashParent(left.Hash, right.Hash),
            }
            nextLevel = append(nextLevel, parent)
        }
        
        nodes = nextLevel
    }
    
    return &MerkleTree{Root: nodes[0]}
}

func hashString(data string) string {
    // SHA-256ハッシュ関数
    sum := sha256.Sum256([]byte(data))
    return "0x" + hex.EncodeToString(sum[:])
}

func hashParent(left, right string) string {
    data := fmt.Sprintf("%s|%s", left, right)
    return hashString(data)
}
```

### 2. Chainへの統合
```go
func (c *Chain) computeStateRootLocked() string {
    // アカウントをソート
    keys := make([]string, 0, len(c.State))
    for k := range c.State {
        keys = append(keys, k)
    }
    sort.Strings(keys)
    
    // リーフを作成
    var leaves []merkletree.StateLeaf
    for _, k := range keys {
        a := c.State[k]
        leaves = append(leaves, merkletree.StateLeaf{
            Address: a.Address,
            Balance: a.Balance,
            Nonce:   a.Nonce,
        })
    }
    
    // Merkle Treeを構築
    tree := merkletree.NewMerkleTree(leaves)
    return tree.Root.Hash
}
```

### 3. Merkle Proof機能
```go
type MerkleProof struct {
    Hashes []string
    Index  int
}

func (t *MerkleTree) GenerateProof(targetHash string) (*MerkleProof, error) {
    // ターゲットのリーフノードを見つける
    leafIndex, path := t.findPath(t.Root, targetHash, []string{}, 0, 1)
    if leafIndex == -1 {
        return nil, errors.New("target not found")
    }
    
    return &MerkleProof{
        Hashes: path,
        Index:  leafIndex,
    }, nil
}

// findPath は再帰的にリーフを探索し、(リーフインデックス, siblingハッシュ列) を返す。
// leafIndex はビットマスクとして VerifyProof に渡される（i ビット目が 0 なら左、1 なら右）。
// depth は現在の深さ、bitPos はそのレベルでの右ビットの重み。
func (t *MerkleTree) findPath(node *MerkleNode, target string, path []string, leafIndex int, bitPos int) (int, []string) {
    if node.Left == nil && node.Right == nil {
        // リーフノード
        if node.Hash == target {
            return leafIndex, path
        }
        return -1, nil
    }
    
    // 左側を探索（右兄弟ハッシュを path に追加、ビットは立てない）
    leftIndex, leftResult := t.findPath(node.Left, target,
        append(path, node.Right.Hash), leafIndex, bitPos<<1)
    if leftIndex != -1 {
        return leftIndex, leftResult
    }
    
    // 右側を探索（左兄弟ハッシュを path に追加、ビットを立てる）
    rightIndex, rightResult := t.findPath(node.Right, target,
        append(path, node.Left.Hash), leafIndex|bitPos, bitPos<<1)
    return rightIndex, rightResult
}

func VerifyProof(rootHash, targetHash string, proof *MerkleProof) bool {
    current := targetHash
    
    for i, hash := range proof.Hashes {
        if proof.Index&(1<<i) == 0 {
            // 左側
            current = hashParent(current, hash)
        } else {
            // 右側
            current = hashParent(hash, current)
        }
    }
    
    return current == rootHash
}
```

### 4. RPCの追加
```go
case "getAccountProof":
    params, err := parseGetAccountProofParams(req.Params)
    if err != nil {
        return nil, rpcInvalidParams(err)
    }
    
    c.mu.Lock()
    defer c.mu.Unlock()
    
    account, exists := c.State[params.Address]
    if !exists {
        return nil, rpcInvalidParams(errors.New("account not found"))
    }
    
    // 現在のstateRootでMerkle Treeを構築
    tree := c.buildMerkleTree()
    
    // Proofを生成
    leaf := merkletree.StateLeaf{
        Address: account.Address,
        Balance: account.Balance,
        Nonce:   account.Nonce,
    }
    
    proof, err := tree.GenerateProof(leaf.Hash())
    if err != nil {
        return nil, rpcInvalidParams(err)
    }
    
    return map[string]any{
        "address":    params.Address,
        "balance":    toHex(account.Balance),
        "nonce":      toHex(account.Nonce),
        "accountHash": leaf.Hash(),
        "proof":      proof.Hashes,
        "rootHash":   tree.Root.Hash,
    }, nil
```

## テストケース

### 1. 基本的なMerkle Tree
```go
func TestBasicMerkleTree(t *testing.T) {
    leaves := []merkletree.StateLeaf{
        {Address: "0x1", Balance: 100, Nonce: 0},
        {Address: "0x2", Balance: 200, Nonce: 1},
        {Address: "0x3", Balance: 300, Nonce: 2},
        {Address: "0x4", Balance: 400, Nonce: 3},
    }
    
    tree := merkletree.NewMerkleTree(leaves)
    assert.NotNil(t, tree.Root)
    assert.NotEmpty(t, tree.Root.Hash)
    
    // 同じリーフで同じツリーができる
    tree2 := merkletree.NewMerkleTree(leaves)
    assert.Equal(t, tree.Root.Hash, tree2.Root.Hash)
}
```

### 2. 状態変更によるRoot Hash変化
```go
func TestStateRootChange(t *testing.T) {
    chain := setupTestChain(t, map[string]uint64{
        aliceAddr: 1000,
        bobAddr:   0,
    })
    
    initialRoot := chain.computeStateRootLocked()
    
    // トランザクション処理
    tx := createAndSignTransaction(t, alicePriv, bobAddr, 100, 0)
    _, err := chain.AddTransaction(tx)
    require.NoError(t, err)
    
    newRoot := chain.computeStateRootLocked()
    assert.NotEqual(t, initialRoot, newRoot)
}
```

### 3. Merkle Proofの検証
```go
func TestMerkleProof(t *testing.T) {
    leaves := []merkletree.StateLeaf{
        {Address: "0x1", Balance: 100, Nonce: 0},
        {Address: "0x2", Balance: 200, Nonce: 1},
        {Address: "0x3", Balance: 300, Nonce: 2},
    }
    
    tree := merkletree.NewMerkleTree(leaves)
    
    // 2番目のリーフのProofを生成
    targetLeaf := leaves[1]
    proof, err := tree.GenerateProof(targetLeaf.Hash())
    require.NoError(t, err)
    
    // Proofを検証
    isValid := merkletree.VerifyProof(tree.Root.Hash, targetLeaf.Hash(), proof)
    assert.True(t, isValid)
    
    // 改ざんしたデータでは検証失敗
    fakeLeaf := merkletree.StateLeaf{
        Address: "0x2",
        Balance: 999, // 改ざん
        Nonce:   1,
    }
    isFakeValid := merkletree.VerifyProof(tree.Root.Hash, fakeLeaf.Hash(), proof)
    assert.False(t, isFakeValid)
}
```

### 4. 奇数個のリーフ
```go
func TestOddNumberOfLeaves(t *testing.T) {
    leaves := []merkletree.StateLeaf{
        {Address: "0x1", Balance: 100, Nonce: 0},
        {Address: "0x2", Balance: 200, Nonce: 1},
        {Address: "0x3", Balance: 300, Nonce: 2}, // 奇数個
    }
    
    tree := merkletree.NewMerkleTree(leaves)
    assert.NotNil(t, tree.Root)
    
    // 4個にした場合と結果が違うことを確認
    leaves4 := append(leaves, merkletree.StateLeaf{Address: "0x4", Balance: 400, Nonce: 3})
    tree4 := merkletree.NewMerkleTree(leaves4)
    assert.NotEqual(t, tree.Root.Hash, tree4.Root.Hash)
}
```

## 実装手順

1. **Merkle Treeパッケージを作成**
   - 基本的なツリー構築
   - ハッシュ計算

2. **Chainに統合**
   - `computeStateRootLocked` を書き換え

3. **Merkle Proof機能を実装**
   - Proof生成
   - Proof検証

4. **RPCを追加**
   - `getAccountProof` メソッド

5. **テストを実装**
   - 基本機能
   - 状態変更
   - Proof検証

## 検証方法

### 手動テスト
```bash
# 1. トランザクション処理
curl -s -X POST localhost:8545 -d '{"jsonrpc":"2.0","method":"sendTransaction","params":[tx],"id":1}'
curl -s -X POST localhost:8545 -d '{"jsonrpc":"2.0","method":"mineBlock","params":[],"id":2}'

# 2. ブロックのstateRootを確認
curl -s -X POST localhost:8545 -d '{"jsonrpc":"2.0","method":"eth_getBlockByNumber","params":["latest",true],"id":3}'

# 3. Account Proofを取得
curl -s -X POST localhost:8545 -d '{"jsonrpc":"2.0","method":"getAccountProof","params":["0xaddress"],"id":4}'

# 4. 手動でProofを検証（別スクリプトで）
```

### 自動テスト
```bash
go test ./internal/merkletree
go test ./internal/chain -run TestStateRoot
```

## 完了条件
- 状態が同じならMerkle rootが同じ
- balanceが変わるとrootが変わる
- proof検証が成功する
- 改ざんしたaccountではproof検証が失敗する
- 既存の機能がすべて動作する

## 次のステップ
Merkle Tree風stateRootが実装できたら、Milestone 7で永続化機能を追加します。これにより、ノードを再起動しても状態が維持されるようになります。

## 注意点
この実装は本物のEthereumのMerkle Patricia Trieとは異なります。あくまでMerkle Treeの基本概念を学ぶための簡略化された実装です。本物との違い：

- 本物：Merkle Patricia Trie（16分木）
- この実装：バイナリMerkle Tree
- 本物：アドレスをパスとして使用
- この実装：ソート済みリストをベースに構築