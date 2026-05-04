# Milestone 9: P2Pの追加

## 概要
現在の実装では単一ノードで動作していますが、実際のブロックチェーンネットワークでは複数のノードが互いに通信します。このマイルストーンでは、簡易的なP2Pネットワークを実装して、ノード間のトランザクションとブロックの伝播を学びます。

## 目的
- 複数ノード間でtxとblockを伝播する
- ネットワークの基本概念を学ぶ
- ノード間の同期を理解する

## 設計方針
本物のdevp2pプロトコルではなく、まずはHTTPベースの簡易実装から始めます：

```text
node A -- HTTP -- node B -- HTTP -- node C
```

## 新しい概念

### 1. Peer管理
- 自ノードの情報
- 接続先ノードのリスト
- ノードの状態管理

### 2. メッセージ伝播
- トランザクションのブロードキャスト
- ブロックのブロードキャスト
- 重複メッセージの防止

### 3. チェーン同期
- 他ノードからのチェーン取得
- フォークの処理（簡易版）

## 実装計画

### 1. Peer管理構造体
```go
package p2p

import (
    "sync"
    "time"
)

type Peer struct {
    ID       string    `json:"id"`
    Address  string    `json:"address"`
    LastSeen time.Time `json:"lastSeen"`
    Active   bool      `json:"active"`
}

type P2PManager struct {
    mu       sync.RWMutex
    selfID   string
    selfAddr string
    peers    map[string]*Peer
    client   *http.Client
}

func NewP2PManager(selfAddr string) *P2PManager {
    return &P2PManager{
        selfID:   generateNodeID(),
        selfAddr: selfAddr,
        peers:    make(map[string]*Peer),
        client:   &http.Client{Timeout: 5 * time.Second},
    }
}

func generateNodeID() string {
    // ランダムなノードIDを生成
    pub, _, _ := ed25519.GenerateKey(rand.Reader)
    sum := sha256.Sum256(pub)
    return "0x" + hex.EncodeToString(sum[:])
}
```

### 2. Peer管理RPC
```go
// admin_addPeer - ピアを追加
func (p *P2PManager) AddPeer(address string) error {
    p.mu.Lock()
    defer p.mu.Unlock()
    
    if address == p.selfAddr {
        return errors.New("cannot add self as peer")
    }
    
    // 既存のピアチェック
    for _, peer := range p.peers {
        if peer.Address == address {
            return errors.New("peer already exists")
        }
    }
    
    // ピア情報を取得
    peerID, err := p.getPeerID(address)
    if err != nil {
        return fmt.Errorf("failed to get peer info: %w", err)
    }
    
    peer := &Peer{
        ID:       peerID,
        Address:  address,
        LastSeen: time.Now(),
        Active:   true,
    }
    
    p.peers[peerID] = peer
    return nil
}

func (p *P2PManager) getPeerID(address string) (string, error) {
    resp, err := p.client.Get(address + "/nodeinfo")
    if err != nil {
        return "", err
    }
    defer resp.Body.Close()
    
    var info struct {
        ID string `json:"id"`
    }
    if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
        return "", err
    }
    
    return info.ID, nil
}

// net_peers - ピア一覧を取得
func (p *P2PManager) GetPeers() []Peer {
    p.mu.RLock()
    defer p.mu.RUnlock()
    
    var peers []Peer
    for _, peer := range p.peers {
        peers = append(peers, *peer)
    }
    return peers
}
```

### 3. メッセージ伝播
```go
// トランザクションのブロードキャスト
func (p *P2PManager) BroadcastTransaction(tx Transaction) error {
    p.mu.RLock()
    defer p.mu.RUnlock()
    
    var errors []error
    
    for _, peer := range p.peers {
        if !peer.Active {
            continue
        }
        
        if err := p.sendTransactionToPeer(peer.Address, tx); err != nil {
            errors = append(errors, err)
            peer.Active = false // 一時的に非アクティブ化
        } else {
            peer.LastSeen = time.Now()
        }
    }
    
    if len(errors) > 0 && len(errors) == len(p.peers) {
        return fmt.Errorf("failed to broadcast to all peers: %v", errors)
    }
    
    return nil
}

func (p *P2PManager) sendTransactionToPeer(address string, tx Transaction) error {
    data := map[string]any{
        "jsonrpc": "2.0",
        "method":  "receiveTransaction",
        "params":  []any{tx},
        "id":      1,
    }
    
    jsonData, _ := json.Marshal(data)
    resp, err := p.client.Post(address+"/", "application/json", bytes.NewBuffer(jsonData))
    if err != nil {
        return err
    }
    defer resp.Body.Close()
    
    if resp.StatusCode != http.StatusOK {
        return fmt.Errorf("peer returned status: %d", resp.StatusCode)
    }
    
    return nil
}

// ブロックのブロードキャスト
func (p *P2PManager) BroadcastBlock(block Block) error {
    p.mu.RLock()
    defer p.mu.RUnlock()
    
    for _, peer := range p.peers {
        if !peer.Active {
            continue
        }
        
        go func(addr string) {
            if err := p.sendBlockToPeer(addr, block); err != nil {
                // エラーをログに記録
            }
        }(peer.Address)
    }
    
    return nil
}
```

### 4. チェーン同期
```go
func (p *P2PManager) SyncChain(targetChain *Chain) error {
    p.mu.RLock()
    defer p.mu.RUnlock()
    
    if len(p.peers) == 0 {
        return errors.New("no peers available")
    }
    
    // 最も長いチェーンを持つピアを探す
    var bestPeer *Peer
    var maxHeight uint64
    
    for _, peer := range p.peers {
        if !peer.Active {
            continue
        }
        
        height, err := p.getPeerHeight(peer.Address)
        if err != nil {
            continue
        }
        
        if height > maxHeight {
            maxHeight = height
            bestPeer = peer
        }
    }
    
    if bestPeer == nil {
        return errors.New("no active peers with chain data")
    }
    
    // チェーンを取得
    return p.fetchChainFromPeer(bestPeer.Address, targetChain)
}

func (p *P2PManager) getPeerHeight(address string) (uint64, error) {
    data := map[string]any{
        "jsonrpc": "2.0",
        "method":  "eth_blockNumber",
        "params":  []any{},
        "id":      1,
    }
    
    jsonData, _ := json.Marshal(data)
    resp, err := p.client.Post(address+"/", "application/json", bytes.NewBuffer(jsonData))
    if err != nil {
        return 0, err
    }
    defer resp.Body.Close()
    
    var rpcResp struct {
        Result string `json:"result"`
    }
    if err := json.NewDecoder(resp.Body).Decode(&rpcResp); err != nil {
        return 0, err
    }
    
    return parseHexQuantity(rpcResp.Result)
}

func (p *P2PManager) fetchChainFromPeer(address string, targetChain *Chain) error {
    // 簡易実装：全ブロックを取得
    data := map[string]any{
        "jsonrpc": "2.0",
        "method":  "getFullChain",
        "params":  []any{},
        "id":      1,
    }
    
    jsonData, _ := json.Marshal(data)
    resp, err := p.client.Post(address+"/", "application/json", bytes.NewBuffer(jsonData))
    if err != nil {
        return err
    }
    defer resp.Body.Close()
    
    var rpcResp struct {
        Result struct {
            Blocks []Block `json:"blocks"`
            State  map[string]Account `json:"state"`
        } `json:"result"`
    }
    
    if err := json.NewDecoder(resp.Body).Decode(&rpcResp); err != nil {
        return err
    }
    
    // チェーンを更新（簡易版：完全に置き換え）
    targetChain.mu.Lock()
    defer targetChain.mu.Unlock()
    
    targetChain.Blocks = rpcResp.Result.Blocks
    targetChain.State = rpcResp.Result.State
    
    return nil
}
```

### 5. Chainとの統合
```go
type Chain struct {
    mu         sync.Mutex
    State      map[string]Account
    Blocks     []Block
    Mempool    *Mempool
    TxIndex    map[string]TxLocation
    BlockIndex map[string]uint64
    persistence.Persistence
    p2p *p2p.P2PManager // 追加
}

func (c *Chain) addTransactionWithBroadcast(tx Transaction) (map[string]any, error) {
    c.mu.Lock()
    defer c.mu.Unlock()
    
    // 既存のトランザクション処理
    receipt, err := c.addTransaction(tx)
    if err != nil {
        return nil, err
    }
    
    // P2Pでブロードキャスト
    if c.p2p != nil {
        go func() {
            if err := c.p2p.BroadcastTransaction(tx); err != nil {
                log.Printf("Failed to broadcast transaction: %v", err)
            }
        }()
    }
    
    return receipt, nil
}

func (c *Chain) mineBlockWithBroadcast(difficulty uint64) (*Block, error) {
    c.mu.Lock()
    defer c.mu.Unlock()
    
    // 既存のマイニング処理
    block, err := c.mineBlockWithDifficulty(difficulty)
    if err != nil {
        return nil, err
    }
    
    // P2Pでブロードキャスト
    if c.p2p != nil {
        go func() {
            if err := c.p2p.BroadcastBlock(*block); err != nil {
                log.Printf("Failed to broadcast block: %v", err)
            }
        }()
    }
    
    return block, nil
}
```

### 6. RPCの追加
```go
// admin_addPeer
case "admin_addPeer":
    var address string
    if err := parseSingleObjectParam(params, &address); err != nil {
        return nil, rpcInvalidParams(err)
    }
    
    if c.p2p == nil {
        return nil, rpcInvalidParams(errors.New("P2P not enabled"))
    }
    
    err := c.p2p.AddPeer(address)
    if err != nil {
        return nil, rpcInvalidParams(err)
    }
    
    return map[string]any{"status": "success"}, nil

// net_peers
case "net_peers":
    if c.p2p == nil {
        return nil, rpcInvalidParams(errors.New("P2P not enabled"))
    }
    
    peers := c.p2p.GetPeers()
    return map[string]any{"peers": peers}, nil

// receiveTransaction（受信用）
case "receiveTransaction":
    var tx Transaction
    if err := parseSingleObjectParam(params, &tx); err != nil {
        return nil, rpcInvalidParams(err)
    }
    
    // 重複チェック
    if c.Mempool.Exists(tx.Hash) {
        return map[string]any{"status": "duplicate"}, nil
    }
    
    // mempoolに追加
    if err := c.Mempool.Add(tx); err != nil {
        return nil, rpcInvalidParams(err)
    }
    
    return map[string]any{"status": "accepted"}, nil

// getFullChain（同期用）
case "getFullChain":
    c.mu.Lock()
    defer c.mu.Unlock()
    
    return map[string]any{
        "blocks": c.Blocks,
        "state":  c.State,
    }, nil
```

## テストケース

### 1. 基本的なピア管理
```go
func TestPeerManagement(t *testing.T) {
    manager1 := p2p.NewP2PManager("http://localhost:8545")
    manager2 := p2p.NewP2PManager("http://localhost:8546")
    
    // ピア追加
    err := manager1.AddPeer("http://localhost:8546")
    require.NoError(t, err)
    
    peers := manager1.GetPeers()
    assert.Len(t, peers, 1)
    assert.Equal(t, "http://localhost:8546", peers[0].Address)
}
```

### 2. トランザクション伝播
```go
func TestTransactionBroadcast(t *testing.T) {
    // 2つのノードをセットアップ
    node1 := setupTestNode(t, 8545)
    node2 := setupTestNode(t, 8546)
    
    // ピア接続
    node1.Chain.p2p.AddPeer("http://localhost:8546")
    
    // node1でトランザクション送信
    tx := createAndSignTransaction(t, alicePriv, bobAddr, 100, 0)
    _, err := node1.Chain.addTransactionWithBroadcast(tx)
    require.NoError(t, err)
    
    // node2のmempoolに伝播されていることを確認
    time.Sleep(100 * time.Millisecond) // 伝播待ち
    assert.Len(t, node2.Chain.Mempool.GetAll(), 1)
}
```

### 3. ブロック伝播
```go
func TestBlockBroadcast(t *testing.T) {
    node1 := setupTestNode(t, 8545)
    node2 := setupTestNode(t, 8546)
    
    // ピア接続
    node1.Chain.p2p.AddPeer("http://localhost:8546")
    
    // node1でマイニング
    tx := createAndSignTransaction(t, alicePriv, bobAddr, 100, 0)
    node1.Chain.Mempool.Add(tx)
    
    _, err := node1.Chain.mineBlockWithBroadcast(1)
    require.NoError(t, err)
    
    // node2にブロックが伝播されていることを確認
    time.Sleep(100 * time.Millisecond)
    assert.Equal(t, uint64(1), node2.Chain.GetLatestBlockNumber())
}
```

## 実装手順

1. **P2Pパッケージを作成**
   - Peer管理
   - メッセージ伝播

2. **ChainにP2Pを統合**
   - ブロードキャスト機能

3. **RPCを追加**
   - admin_addPeer
   - net_peers
   - receiveTransaction

4. **テストを実装**
   - 基本機能
   - 伝播テスト

## 検証方法

### 手動テスト
```bash
# ターミナル1：ノード1起動
go run . node --genesis genesis.json --datadir ./data1 --addr :8545

# ターミナル2：ノード2起動
go run . node --genesis genesis.json --datadir ./data2 --addr :8546

# ターミナル3：ピア接続
curl -s -X POST localhost:8545 -d '{"jsonrpc":"2.0","method":"admin_addPeer","params":["http://localhost:8546"],"id":1}'

# ピア確認
curl -s -X POST localhost:8545 -d '{"jsonrpc":"2.0","method":"net_peers","params":[],"id":2}'

# ノード1でトランザクション送信
curl -s -X POST localhost:8545 -d '{"jsonrpc":"2.0","method":"sendTransaction","params":[tx],"id":3}'

# ノード2のmempool確認
curl -s -X POST localhost:8546 -d '{"jsonrpc":"2.0","method":"txpool_content","params":[],"id":4}'
```

## 完了条件
- node Aで送信したtxがnode Bに届く
- node Aで作ったblockがnode Bに届く
- node BのblockNumberが追いつく
- ピア管理が正常に動作する

## 次のステップ
P2Pが実装できたら、Milestone 10でFork Choiceを実装します。これにより、競合するチェーンから正史を選択するロジックを学べます。

## 注意点
この実装は本物のdevp2pとは大きく異なります：

- 本物：専用のP2Pプロトコル、暗号化、ノード発見
- この実装：HTTPベース、単純なピア管理
- 本物：複雑なネットワークトポロジー
- この実装：スター型の単純な接続

あくまでP2Pの基本概念を学ぶための実装です。