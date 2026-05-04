# Milestone 3: 自動テストの追加

## 概要
現在READMEにある手動でのcurl確認だけでなく、Goのテストフレームワークを使って自動テストを追加します。これにより、リグレッションを防ぎ、開発効率を向上させます。

## テスト対象の分類

### 3.1 Accountテスト
**ファイル**: `internal/account/account_test.go`

#### テスト項目
- アカウント生成でaddressが20 bytesになる
- 同じ公開鍵から同じaddressが生成される
- 異なる公開鍵から異なるaddressが生成される
- addressの正規化が正しく動作する

#### 実装例
```go
func TestAddressGeneration(t *testing.T) {
    pub, _, err := ed25519.GenerateKey(rand.Reader)
    require.NoError(t, err)
    
    addr := account.AddressFromPubkey(pub)
    assert.Equal(t, 42, len(addr)) // 0x + 40 hex chars
    assert.True(t, strings.HasPrefix(addr, "0x"))
}

func TestAddressConsistency(t *testing.T) {
    pub, _, err := ed25519.GenerateKey(rand.Reader)
    require.NoError(t, err)
    
    addr1 := account.AddressFromPubkey(pub)
    addr2 := account.AddressFromPubkey(pub)
    assert.Equal(t, addr1, addr2)
}
```

### 3.2 Transactionテスト
**ファイル**: `internal/tx/transaction_test.go`

#### テスト項目
- 正しい署名は検証成功
- 異なる秘密鍵の署名は検証失敗
- `from` と `pubKey` が一致しない場合は失敗
- トランザクションハッシュが一意である
- nonceが同じでも内容が違えばハッシュが違う

#### 実装例
```go
func TestValidSignature(t *testing.T) {
    fromPriv, fromPub := generateKeyPair(t)
    toPriv, _ := generateKeyPair(t)
    
    tx := createTransaction(t, fromPub, toPub, 100, 0)
    signTransaction(t, fromPriv, &tx)
    
    _, err := tx.VerifyAndNormalize()
    assert.NoError(t, err)
}

func TestInvalidSignature(t *testing.T) {
    fromPriv, fromPub := generateKeyPair(t)
    toPriv, _ := generateKeyPair(t)
    otherPriv, _ := generateKeyPair(t)
    
    tx := createTransaction(t, fromPub, toPub, 100, 0)
    signTransaction(t, otherPriv, &tx) // 間違った鍵で署名
    
    _, err := tx.VerifyAndNormalize()
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "invalid signature")
}
```

### 3.3 State Transitionテスト
**ファイル**: `internal/chain/chain_test.go`

#### テスト項目
- 正常送金で残高が移動する
- nonceが増える
- 残高不足は失敗する
- nonce不一致は失敗する
- 存在しないアカウントへの送金で新規アカウントが作成される

#### 実装例
```go
func TestSuccessfulTransfer(t *testing.T) {
    chain := setupTestChain(t, map[string]uint64{
        aliceAddr: 1000,
        bobAddr:   0,
    })
    
    tx := createAndSignTransaction(t, alicePriv, bobAddr, 150, 0)
    
    receipt, err := chain.AddTransaction(tx)
    assert.NoError(t, err)
    assert.NotNil(t, receipt)
    
    // 状態確認
    alice := chain.State[aliceAddr]
    assert.Equal(t, uint64(850), alice.Balance)
    assert.Equal(t, uint64(1), alice.Nonce)
    
    bob := chain.State[bobAddr]
    assert.Equal(t, uint64(150), bob.Balance)
    assert.Equal(t, uint64(0), bob.Nonce)
}

func TestInsufficientBalance(t *testing.T) {
    chain := setupTestChain(t, map[string]uint64{
        aliceAddr: 100,
        bobAddr:   0,
    })
    
    tx := createAndSignTransaction(t, alicePriv, bobAddr, 150, 0)
    
    _, err := chain.AddTransaction(tx)
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "insufficient balance")
}
```

### 3.4 Blockテスト
**ファイル**: `internal/block/block_test.go`

#### テスト項目
- ブロック生成でnumberが増える
- parentHashが前ブロックを指す
- 状態が変わるとstateRootが変わる
- ブロックハッシュが一意である
- タイムスタンプが設定されている

#### 実装例
```go
func TestBlockGeneration(t *testing.T) {
    chain := setupTestChain(t, map[string]uint64{
        aliceAddr: 1000,
    })
    
    initialBlockCount := len(chain.Blocks)
    genesisHash := chain.Blocks[0].Hash
    
    tx := createAndSignTransaction(t, alicePriv, bobAddr, 100, 0)
    _, err := chain.AddTransaction(tx)
    require.NoError(t, err)
    
    assert.Equal(t, initialBlockCount+1, len(chain.Blocks))
    
    newBlock := chain.Blocks[len(chain.Blocks)-1]
    assert.Equal(t, uint64(1), newBlock.Number)
    assert.Equal(t, genesisHash, newBlock.ParentHash)
    assert.NotEqual(t, chain.Blocks[0].StateRoot, newBlock.StateRoot)
}
```

### 3.5 RPCテスト
**ファイル**: `internal/rpc/server_test.go`

#### テスト項目
- `httptest` でRPCを呼び出す
- `eth_blockNumber` が正しく応答する
- `eth_getBalance` が正しく応答する
- `sendTransaction` が正しく応答する
- `eth_getBlockByNumber` が正しく応答する
- エラーレスポンスが正しくフォーマットされている

#### 実装例
```go
func TestEthBlockNumber(t *testing.T) {
    chain := setupTestChain(t, map[string]uint64{})
    server := setupTestServer(t, chain)
    
    resp := callRPC(t, server, "eth_blockNumber", []any{})
    
    result, ok := resp["result"].(string)
    require.True(t, ok)
    assert.Equal(t, "0x0", result)
}

func TestEthGetBalance(t *testing.T) {
    chain := setupTestChain(t, map[string]uint64{
        aliceAddr: 1000,
    })
    server := setupTestServer(t, chain)
    
    resp := callRPC(t, server, "eth_getBalance", []any{aliceAddr, "latest"})
    
    result, ok := resp["result"].(string)
    require.True(t, ok)
    assert.Equal(t, "0x3e8", result) // 1000 in hex
}
```

## テストユーティリティ

### 共通ヘルパー関数
**ファイル**: `internal/testutil/testutil.go`

```go
package testutil

import (
    "crypto/ed25519"
    "crypto/rand"
    "testing"
    
    "github.com/stretchr/testify/require"
)

func GenerateKeyPair(t *testing.T) (ed25519.PrivateKey, ed25519.PublicKey) {
    pub, priv, err := ed25519.GenerateKey(rand.Reader)
    require.NoError(t, err)
    return priv, pub
}

func SetupTestChain(t *testing.T, balances map[string]uint64) *chain.Chain {
    // テスト用のチェーンをセットアップ
}

func CreateAndSignTransaction(t *testing.T, priv ed25519.PrivateKey, to string, value, nonce uint64) tx.Transaction {
    // テスト用のトランザクションを作成・署名
}

func CallRPC(t *testing.T, server *http.Server, method string, params []any) map[string]any {
    // RPC呼び出しのヘルパー
}
```

## 実行方法

### すべてのテストを実行
```bash
go test ./...
```

### 特定のパッケージのテストを実行
```bash
go test ./internal/account
go test ./internal/tx
go test ./internal/chain
go test ./internal/rpc
```

### カバレッジを確認
```bash
go test -cover ./...
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

### ベンチマーク実行
```bash
go test -bench=. ./...
```

## 完了条件

- `go test ./...` がすべて成功する
- カバレッジが80%以上（目標）
- READMEの手動テスト項目がすべて自動テスト化されている
- テストが実用的で、実際のバグを検出できる
- CI/CDで自動実行できる状態

## 次のステップ
テストが整備されたら、Milestone 4のmempool追加に進みます。テストがあることで、リファクタリングや機能追加の安心感が大幅に向上します。