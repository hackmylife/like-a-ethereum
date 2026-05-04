# Milestone 7: 永続化

## 概要
現在の実装では、ノードを停止するとすべての状態（ブロック、アカウント情報など）が失われます。このマイルストーンでは、データを永続化し、ノードを再起動しても状態が維持されるようにします。

## 現在の問題
- 状態はメモリ上のみ
- ノードを止めるとチェーンが消える
- 再起動するとGenesisからやり直し

## 実装方針
1. 最初はJSONファイルでの永続化（シンプルで理解しやすい）
2. 将来的にはBoltDBやBadgerDBへの移行を視野に入れる

## 保存対象
- Blocks（ブロックデータ）
- State（アカウント状態）
- TxIndex（トランザクションインデックス）
- BlockIndex（ブロックインデックス）
- Mempool（オプション：再起動時にクリアしても良い）

## 実装計画

### 1. データディレクトリ構造
```
data/
  ├── blocks.json      # ブロックデータ
  ├── state.json       # アカウント状態
  ├── tx_index.json    # トランザクションインデックス
  └── block_index.json # ブロックインデックス
```

### 2. 永続化インターフェース
```go
type Persistence interface {
    SaveBlocks(blocks []Block) error
    LoadBlocks() ([]Block, error)
    SaveState(state map[string]Account) error
    LoadState() (map[string]Account, error)
    SaveTxIndex(index map[string]TxLocation) error
    LoadTxIndex() (map[string]TxLocation, error)
    SaveBlockIndex(index map[string]uint64) error
    LoadBlockIndex() (map[string]uint64, error)
}
```

### 3. JSON永続化実装
```go
package persistence

import (
    "encoding/json"
    "os"
    "path/filepath"
)

type JSONPersistence struct {
    dataDir string
}

func NewJSONPersistence(dataDir string) *JSONPersistence {
    return &JSONPersistence{dataDir: dataDir}
}

func (p *JSONPersistence) ensureDir() error {
    return os.MkdirAll(p.dataDir, 0755)
}

func (p *JSONPersistence) SaveBlocks(blocks []Block) error {
    if err := p.ensureDir(); err != nil {
        return err
    }
    
    data, err := json.MarshalIndent(blocks, "", "  ")
    if err != nil {
        return err
    }
    
    return os.WriteFile(filepath.Join(p.dataDir, "blocks.json"), data, 0644)
}

func (p *JSONPersistence) LoadBlocks() ([]Block, error) {
    path := filepath.Join(p.dataDir, "blocks.json")
    
    data, err := os.ReadFile(path)
    if os.IsNotExist(err) {
        return nil, nil // ファイルがなければ空を返す
    }
    if err != nil {
        return nil, err
    }
    
    var blocks []Block
    if err := json.Unmarshal(data, &blocks); err != nil {
        return nil, err
    }
    
    return blocks, nil
}

func (p *JSONPersistence) SaveState(state map[string]Account) error {
    if err := p.ensureDir(); err != nil {
        return err
    }
    
    data, err := json.MarshalIndent(state, "", "  ")
    if err != nil {
        return err
    }
    
    return os.WriteFile(filepath.Join(p.dataDir, "state.json"), data, 0644)
}

func (p *JSONPersistence) LoadState() (map[string]Account, error) {
    path := filepath.Join(p.dataDir, "state.json")
    
    data, err := os.ReadFile(path)
    if os.IsNotExist(err) {
        return make(map[string]Account), nil
    }
    if err != nil {
        return nil, err
    }
    
    var state map[string]Account
    if err := json.Unmarshal(data, &state); err != nil {
        return nil, err
    }
    
    return state, nil
}

// TxIndexとBlockIndexも同様に実装...
```

### 4. Chain構造体の変更
```go
type Chain struct {
    mu         sync.Mutex
    State      map[string]Account
    Blocks     []Block
    Mempool    *Mempool
    TxIndex    map[string]TxLocation
    BlockIndex map[string]uint64
    persistence.Persistence // 追加
}

func NewChainWithPersistence(genesisPath, dataDir string) (*Chain, error) {
    // Genesisから初期化
    chain, err := NewChainFromGenesis(genesisPath)
    if err != nil {
        return nil, err
    }
    
    // 永続化層を設定
    chain.Persistence = persistence.NewJSONPersistence(dataDir)
    
    // 既存データを読み込み
    if err := chain.loadFromDisk(); err != nil {
        return nil, err
    }
    
    return chain, nil
}

func (c *Chain) loadFromDisk() error {
    // ブロックを読み込み
    blocks, err := c.LoadBlocks()
    if err != nil {
        return err
    }
    
    if len(blocks) > 0 {
        // 既存データがある場合はGenesisを上書き
        c.Blocks = blocks
        
        // 状態を読み込み
        state, err := c.LoadState()
        if err != nil {
            return err
        }
        c.State = state
        
        // インデックスを読み込み
        txIndex, err := c.LoadTxIndex()
        if err != nil {
            return err
        }
        c.TxIndex = txIndex
        
        blockIndex, err := c.LoadBlockIndex()
        if err != nil {
            return err
        }
        c.BlockIndex = blockIndex
    }
    
    return nil
}

func (c *Chain) saveToDisk() error {
    if err := c.SaveBlocks(c.Blocks); err != nil {
        return err
    }
    
    if err := c.SaveState(c.State); err != nil {
        return err
    }
    
    if err := c.SaveTxIndex(c.TxIndex); err != nil {
        return err
    }
    
    if err := c.SaveBlockIndex(c.BlockIndex); err != nil {
        return err
    }
    
    return nil
}
```

### 5. トランザクションとブロック追加時の永続化
```go
func (c *Chain) addTransaction(tx Transaction) (map[string]any, error) {
    c.mu.Lock()
    defer c.mu.Unlock()
    
    // 既存の処理...
    
    // ブロック追加後に永続化
    if err := c.saveToDisk(); err != nil {
        // 永続化失敗時の処理
        log.Printf("Failed to save to disk: %v", err)
    }
    
    return receipt, nil
}

func (c *Chain) mineBlock() (*Block, error) {
    c.mu.Lock()
    defer c.mu.Unlock()
    
    // 既存の処理...
    
    // ブロック追加後に永続化
    if err := c.saveToDisk(); err != nil {
        return nil, fmt.Errorf("failed to save block: %w", err)
    }
    
    return block, nil
}
```

### 6. CLIの変更
```go
func cmdNode(args []string) {
    fs := flag.NewFlagSet("node", flag.ExitOnError)
    
    genesisPath := fs.String("genesis", "genesis.json", "genesis file")
    listenAddr := fs.String("addr", ":8545", "listen address")
    dataDir := fs.String("datadir", "./data", "data directory")
    
    must(fs.Parse(args))
    
    chain, err := NewChainWithPersistence(*genesisPath, *dataDir)
    must(err)
    
    http.HandleFunc("/", chain.handleRPC)
    
    log.Printf("mini ethereum-like node listening on %s", *listenAddr)
    log.Printf("data directory: %s", *dataDir)
    log.Printf("genesis block hash: %s stateRoot: %s", chain.Blocks[0].Hash, chain.Blocks[0].StateRoot)
    
    must(http.ListenAndServe(*listenAddr, nil))
}
```

## テストケース

### 1. 基本的な永続化
```go
func TestBasicPersistence(t *testing.T) {
    tempDir := t.TempDir()
    
    // 1. チェーンを作成してトランザクション処理
    chain, err := NewChainWithPersistence("", tempDir)
    require.NoError(t, err)
    
    // Genesisブロックを設定
    chain.State = map[string]Account{
        aliceAddr: {Address: aliceAddr, Balance: 1000, Nonce: 0},
    }
    chain.Blocks = []Block{createGenesisBlock(chain.State)}
    
    // トランザクション処理
    tx := createAndSignTransaction(t, alicePriv, bobAddr, 100, 0)
    _, err = chain.AddTransaction(tx)
    require.NoError(t, err)
    
    // 2. 新しいチェーンインスタンスを作成（データ読み込み）
    chain2, err := NewChainWithPersistence("", tempDir)
    require.NoError(t, err)
    
    // 3. 状態が復元されていることを確認
    assert.Equal(t, len(chain.Blocks), len(chain2.Blocks))
    assert.Equal(t, chain.Blocks[len(chain.Blocks)-1].Hash, chain2.Blocks[len(chain2.Blocks)-1].Hash)
    
    alice := chain2.State[aliceAddr]
    assert.Equal(t, uint64(900), alice.Balance)
    assert.Equal(t, uint64(1), alice.Nonce)
}
```

### 2. データディレクトリがない場合
```go
func TestEmptyDataDirectory(t *testing.T) {
    tempDir := t.TempDir()
    
    chain, err := NewChainWithPersistence("", tempDir)
    require.NoError(t, err)
    
    // 空の状態で初期化される
    assert.Empty(t, chain.Blocks)
    assert.Empty(t, chain.State)
}
```

### 3. ファイル破損時の処理
```go
func TestCorruptedData(t *testing.T) {
    tempDir := t.TempDir()
    
    // 不正なJSONファイルを作成
    blocksFile := filepath.Join(tempDir, "blocks.json")
    os.WriteFile(blocksFile, []byte("invalid json"), 0644)
    
    _, err := NewChainWithPersistence("", tempDir)
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "invalid character")
}
```

### 4. パーミッションエラー
```go
func TestPermissionError(t *testing.T) {
    if os.Getuid() == 0 {
        t.Skip("running as root, permission test skipped")
    }
    
    // 読み取り専用ディレクトリを作成
    tempDir := t.TempDir()
    os.Chmod(tempDir, 0444)
    defer os.Chmod(tempDir, 0755)
    
    chain, err := NewChainWithPersistence("", tempDir)
    require.NoError(t, err)
    
    // 保存時にエラーになる
    err = chain.saveToDisk()
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "permission denied")
}
```

## 実装手順

1. **Persistenceパッケージを作成**
   - インターフェース定義
   - JSON実装

2. **Chain構造体を変更**
   - Persistenceフィールド追加
   - 読み込み/保存メソッド

3. **CLIを変更**
   - `--datadir` オプション追加

4. **トランザクション/ブロック処理に永続化を追加**
   - 各処理後にsaveToDiskを呼び出す

5. **テストを実装**
   - 基本機能
   - エラーケース

## 検証方法

### 手動テスト
```bash
# 1. データディレクトリを指定してノード起動
mkdir -p ./test-data
go run . node --genesis genesis.json --datadir ./test-data --addr :8545

# 2. トランザクション処理
curl -s -X POST localhost:8545 -d '{"jsonrpc":"2.0","method":"sendTransaction","params":[tx],"id":1}'
curl -s -X POST localhost:8545 -d '{"jsonrpc":"2.0","method":"mineBlock","params":[],"id":2}'

# 3. ノード停止（Ctrl+C）

# 4. 同じデータディレクトリで再起動
go run . node --genesis genesis.json --datadir ./test-data --addr :8545

# 5. 状態が維持されていることを確認
curl -s -X POST localhost:8545 -d '{"jsonrpc":"2.0","method":"eth_blockNumber","params":[],"id":3}'
curl -s -X POST localhost:8545 -d '{"jsonrpc":"2.0","method":"eth_getBalance","params":["0xaddress","latest"],"id":4}'
```

### 自動テスト
```bash
go test ./internal/persistence
go test ./internal/chain -run TestPersistence
```

## 完了条件
- ノード停止後もブロックが残る
- 再起動後にblockNumberが維持される
- 残高が維持される
- TxIndexとBlockIndexも復元される
- データディレクトリがない場合は新規作成される
- 既存の機能がすべて動作する

## 次のステップ
永続化が実装できたら、Milestone 8で簡易PoWを実装します。これにより、ブロック生成の難易度という概念を学べます。

## 将来的な改善点
1. **BoltDB/BadgerDBへの移行**
   - パフォーマンス向上
   - トランザクションサポート
   - コンパクトなデータ形式

2. **スナップショット機能**
   - 定期的な状態スナップショット
   - 高速な同期

3. **プルーニング**
   - 古い状態データの削除
   - ディスク容量の節約