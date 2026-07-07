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
// 注意: 実際の関数名は tx.VerfyAndNormalizeTx（Verifyのtypo。メソッドではなく
// パッケージ関数）。テストを付けるこのタイミングで VerifyAndNormalizeTx への
// リネームを推奨（以下はリネーム後の名前で書いている）。
func TestValidSignature(t *testing.T) {
    fromPriv, _ := generateKeyPair(t)
    _, toPub := generateKeyPair(t)
    
    txn := createAndSignTransaction(t, fromPriv, account.AddressFromPubkey(toPub), 100, 0)
    
    _, err := tx.VerifyAndNormalizeTx(txn)
    assert.NoError(t, err)
}

func TestInvalidSignature(t *testing.T) {
    fromPriv, _ := generateKeyPair(t)
    _, toPub := generateKeyPair(t)
    otherPriv, _ := generateKeyPair(t)
    
    txn := createAndSignTransaction(t, fromPriv, account.AddressFromPubkey(toPub), 100, 0)
    
    // 間違った鍵で署名し直す
    sig := ed25519.Sign(otherPriv, tx.TxSignBytes(txn))
    txn.Signature = "0x" + hex.EncodeToString(sig)
    
    _, err := tx.VerifyAndNormalizeTx(txn)
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

注意:

- testifyは外部依存。`go get github.com/stretchr/testify` で追加する（現在のgo.modは依存ゼロ）
- testutilは account / chain / rpc / tx に依存するため、これらのパッケージ内のテストから
  testutilを使うと循環importになる。その場合はテストを `package tx_test` のような
  external test packageにするか、testutilを使わずに書く

```go
package testutil

import (
    "bytes"
    "crypto/ed25519"
    "crypto/rand"
    "encoding/hex"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "os"
    "path/filepath"
    "testing"

    "github.com/stretchr/testify/require"

    "like-a-ethereum/internal/account"
    "like-a-ethereum/internal/chain"
    "like-a-ethereum/internal/rpc"
    "like-a-ethereum/internal/tx"
)

func GenerateKeyPair(t *testing.T) (ed25519.PrivateKey, ed25519.PublicKey) {
    t.Helper()
    pub, priv, err := ed25519.GenerateKey(rand.Reader)
    require.NoError(t, err)
    return priv, pub
}

// SetupTestChain は一時ディレクトリにgenesis.jsonを書き出し、
// 公開API（chain.NewChainFromGenesis）だけでChainを構築する。
// 非公開フィールド・非公開関数に依存しないため、内部リファクタリングに強い。
func SetupTestChain(t *testing.T, balances map[string]uint64) *chain.Chain {
    t.Helper()

    b, err := json.Marshal(map[string]any{"alloc": balances})
    require.NoError(t, err)

    genesisPath := filepath.Join(t.TempDir(), "genesis.json")
    require.NoError(t, os.WriteFile(genesisPath, b, 0o644))

    c, err := chain.NewChainFromGenesis(genesisPath)
    require.NoError(t, err)
    return c
}

// CreateAndSignTransaction はテスト用のトランザクションを作成・署名して返す
func CreateAndSignTransaction(t *testing.T, priv ed25519.PrivateKey, to string, value, nonce uint64) tx.Transaction {
    t.Helper()

    pub := priv.Public().(ed25519.PublicKey)

    toAddr, err := account.NormalizeAddress(to)
    require.NoError(t, err)

    txn := tx.Transaction{
        From:   account.AddressFromPubkey(pub),
        To:     toAddr,
        Value:  value,
        Nonce:  nonce,
        PubKey: "0x" + hex.EncodeToString(pub),
    }

    sig := ed25519.Sign(priv, tx.TxSignBytes(txn))
    txn.Signature = "0x" + hex.EncodeToString(sig)
    txn.Hash = tx.TxHash(txn)

    return txn
}

// CallRPC はhttptestサーバー経由でRPCメソッドを呼び出し、レスポンスをmap[string]anyで返す
func CallRPC(t *testing.T, server *httptest.Server, method string, params []any) map[string]any {
    t.Helper()

    body, err := json.Marshal(map[string]any{
        "jsonrpc": "2.0",
        "method":  method,
        "params":  params,
        "id":      1,
    })
    require.NoError(t, err)

    resp, err := http.Post(server.URL, "application/json", bytes.NewReader(body))
    require.NoError(t, err)
    defer resp.Body.Close()

    var result map[string]any
    require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
    return result
}

// SetupTestServer はChainからhttptestサーバーを起動して返す
// （rpc.HandleRPC(c) は http.HandlerFunc を返す関数。Chainのメソッドではない）
func SetupTestServer(t *testing.T, c *chain.Chain) *httptest.Server {
    t.Helper()
    server := httptest.NewServer(rpc.HandleRPC(c))
    t.Cleanup(server.Close)
    return server
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

## テスト整備と合わせて直す既知の問題

milestone-0の「既知の問題」のうち、以下はこのマイルストーンで（先に現行動作をテストで固定した上で）直す:

- `tx.VerfyAndNormalizeTx` のtypoを `VerifyAndNormalizeTx` にリネームする
- `eth_getTransactionByHash` のレスポンスの `blockNumber` をhex quantityに統一する
- `eth_getBlockByHash` のパラメータエラーメッセージ末尾のデバッグ文字列（`strconv.Itoa(len(arr))`）を除去する
- 未知のhashで `result` フィールドごと省略される問題（`omitempty`）を直し、`"result": null` を返す
  - 現状のままRPCテストに「resultがnull」を書くと、フィールド自体が存在せず期待とズレる点に注意
- （推奨）genesisのTimestampを固定値にする。テストの再現性が上がり、遅くともMilestone 9までに必要になる

## 完了条件

- `go test ./...` がすべて成功する
- カバレッジが80%以上（目標）
- READMEの手動テスト項目がすべて自動テスト化されている
- テストが実用的で、実際のバグを検出できる
- CI/CDで自動実行できる状態

## 次のステップ
テストが整備されたら、Milestone 4のmempool追加に進みます。テストがあることで、リファクタリングや機能追加の安心感が大幅に向上します。