# Milestone 13: Ethereum形式への接近

## 概要
これまでの実装は学習用に簡略化されてきましたが、この最終マイルストーンでは、本物のEthereumに少しずつ近づけていきます。完全な互換性を目指すのではなく、本物の仕様との差分を理解し、段階的に置き換えていくことを目的とします。

## 目的
- 本物のEthereumとの差分を明確に理解する
- 段階的な互換性向上を学ぶ
- 実装の置き換えポイントを理解する

## 現在の実装 vs 本物のEthereum

| 項目 | 現在の実装 | 本物のEthereum | 置き換え優先度 |
|------|------------|----------------|--------------|
| 署名方式 | Ed25519 | secp256k1 | 高 |
| ハッシュ | SHA-256 | Keccak-256 | 高 |
| トランザクション形式 | JSON | RLP / Typed Transaction | 高 |
| address生成 | SHA-256(pubkey)[12:] | Keccak-256(pubkey)[12:] | 高 |
| RPC | 独自 `sendTransaction` | `eth_sendRawTransaction` | 高 |
| stateRoot | 簡易Merkle Tree | Merkle Patricia Trie | 中 |
| ブロックハッシュ | SHA-256 | Keccak-256 | 中 |
| chainId | なし | あり | 中 |
| EVM | なし | あり | 低 |

## 実装計画

### 1. secp256k1署名への移行
```go
package crypto

import (
    "crypto/ecdsa"
    "crypto/rand"
    "encoding/hex"
    gocrypto "github.com/ethereum/go-ethereum/crypto"       // Keccak256 等
    "github.com/ethereum/go-ethereum/crypto/secp256k1"
)

// 現在のEd25519からsecp256k1へ。
// NOTE: elliptic.P256() は NIST P-256 曲線であり secp256k1 ではない。
//       go-ethereum の GenerateKey() は内部で secp256k1 曲線を使用する。
func GenerateSecp256k1Key() (*ecdsa.PrivateKey, error) {
    return gocrypto.GenerateKey()
}

func SignTransactionSecp256k1(privKey *ecdsa.PrivateKey, hash []byte) ([]byte, error) {
    return secp256k1.Sign(hash, privKey.D.Bytes())
}

func VerifySecp256k1(pubKey *ecdsa.PublicKey, hash, signature []byte) bool {
    return secp256k1.VerifySignature(ellipticMarshal(pubKey), hash, signature[:64])
}

// ellipticMarshal は公開鍵を非圧縮形式 (65 bytes) に変換する。
func ellipticMarshal(pub *ecdsa.PublicKey) []byte {
    return gocrypto.FromECDSAPub(pub)
}

// Address生成（Ethereum方式）
func AddressFromSecp256k1Pubkey(pubKey *ecdsa.PublicKey) string {
    // 非圧縮公開鍵の先頭 0x04 バイトを除いた 64 バイトの Keccak256 を計算し
    // 後ろ 20 バイト (40 hex) を使う
    pubBytes := gocrypto.Keccak256(ellipticMarshal(pubKey)[1:])
    return "0x" + hex.EncodeToString(pubBytes[12:]) // 後ろ20バイト
}
```

### 2. Keccak-256への移行
```go
package crypto

import (
    "encoding/hex"
    "encoding/json"
    "strings"
    gocrypto "github.com/ethereum/go-ethereum/crypto"
)

// SHA-256からKeccak-256へ
func Keccak256Hash(data []byte) []byte {
    return gocrypto.Keccak256Hash(data).Bytes()
}

func HashJSONKeccak(v any) string {
    b, err := json.Marshal(v)
    if err != nil {
        panic(err)
    }
    
    hash := gocrypto.Keccak256Hash(b)
    return "0x" + hex.EncodeToString(hash.Bytes())
}

// トランザクションハッシュ
func TxHashKeccak(tx Transaction) string {
    // RLPエンコードが本物だが、まずはJSONで
    payload := map[string]any{
        "from":      strings.ToLower(tx.From),
        "to":        strings.ToLower(tx.To),
        "value":     tx.Value,
        "nonce":     tx.Nonce,
        "gasLimit":  tx.GasLimit,
        "gasPrice":  tx.GasPrice,
    }
    
    return HashJSONKeccak(payload)
}
```

### 3. RLPエンコーディングの導入
```go
package rlp

import (
    "github.com/ethereum/go-ethereum/rlp"
)

// トランザクションのRLPエンコード
type RLPTx struct {
    Nonce    uint64
    GasPrice uint64
    GasLimit uint64
    To       string // 空文字列はコントラクト作成
    Value    uint64
    Data     []byte
    V, R, S  uint64 // 署名
}

func EncodeTransactionRLP(tx Transaction) ([]byte, error) {
    rlpTx := RLPTx{
        Nonce:    tx.Nonce,
        GasPrice: tx.GasPrice,
        GasLimit: tx.GasLimit,
        To:       tx.To,
        Value:    tx.Value,
        Data:     []byte{}, // 現時点では空
        // V, R, S は署名から設定
    }
    
    return rlp.EncodeToBytes(rlpTx)
}

func DecodeTransactionRLP(data []byte) (Transaction, error) {
    var rlpTx RLPTx
    if err := rlp.DecodeBytes(data, &rlpTx); err != nil {
        return Transaction{}, err
    }
    
    return Transaction{
        Nonce:    rlpTx.Nonce,
        GasPrice: rlpTx.GasPrice,
        GasLimit: rlpTx.GasLimit,
        To:       rlpTx.To,
        Value:    rlpTx.Value,
        // 署名情報を復元
    }, nil
}
```

### 4. eth_sendRawTransactionの実装
```go
// sendTransactionからeth_sendRawTransactionへ
case "eth_sendRawTransaction":
    var rawTxHex string
    if err := parseSingleObjectParam(params, &rawTxHex); err != nil {
        return nil, rpcInvalidParams(err)
    }
    
    // RLPデコード
    rawTx, err := hex.DecodeString(strings.TrimPrefix(rawTxHex, "0x"))
    if err != nil {
        return nil, rpcInvalidParams(err)
    }
    
    tx, err := rlp.DecodeTransactionRLP(rawTx)
    if err != nil {
        return nil, rpcInvalidParams(err)
    }
    
    // 署名検証
    if err := c.verifyTransactionSignature(tx); err != nil {
        return nil, rpcInvalidParams(err)
    }
    
    // mempoolに追加
    if err := c.Mempool.Add(tx); err != nil {
        return nil, rpcInvalidParams(err)
    }
    
    return map[string]any{
        "transactionHash": tx.Hash,
    }, nil
```

### 4b. verifyTransactionSignature の実装
```go
// verifyTransactionSignature は secp256k1 署名を検証し、送信元アドレスを確認する。
// UseSecp256k1 が false の場合は既存の Ed25519 検証にフォールバックする。
func (c *Chain) verifyTransactionSignature(tx Transaction) error {
    if !UseSecp256k1 {
        // 既存の検証ロジックへフォールバック
        return validateTransaction(tx)
    }
    
    // secp256k1 署名の場合：
    // 1. トランザクションハッシュを計算
    txHash := TxHashKeccak(tx)
    hashBytes, err := hex.DecodeString(strings.TrimPrefix(txHash, "0x"))
    if err != nil {
        return fmt.Errorf("invalid tx hash: %w", err)
    }
    
    // 2. 署名から公開鍵を復元
    sigBytes, err := hex.DecodeString(strings.TrimPrefix(tx.Signature, "0x"))
    if err != nil {
        return fmt.Errorf("invalid signature: %w", err)
    }
    
    pubKeyBytes, err := secp256k1.RecoverPubkey(hashBytes, sigBytes)
    if err != nil {
        return fmt.Errorf("failed to recover pubkey: %w", err)
    }
    
    // 3. 公開鍵からアドレスを導出して一致を確認
    pubKey, err := gocrypto.UnmarshalPubkey(pubKeyBytes)
    if err != nil {
        return fmt.Errorf("failed to unmarshal pubkey: %w", err)
    }
    
    recoveredAddr := AddressFromSecp256k1Pubkey(pubKey)
    if !strings.EqualFold(recoveredAddr, tx.From) {
        return fmt.Errorf("signature mismatch: got %s, want %s", recoveredAddr, tx.From)
    }
    
    return nil
}
```

### 5. ChainIdの導入
```go
type ChainConfig struct {
    ChainID uint64 `json:"chainId"`
}

var DefaultChainConfig = ChainConfig{
    ChainID: 1337, // ローカルテスト用
}

// トランザクション署名にChainIDを含める
func (tx *Transaction) SignWithChainID(privKey *ecdsa.PrivateKey, chainID uint64) error {
    // EIP-155準拠の署名
    hash := tx.hashForSigning(chainID)
    
    signature, err := SignTransactionSecp256k1(privKey, hash)
    if err != nil {
        return err
    }
    
    // VにchainIDを含める
    tx.V = uint64(chainID)*2 + 35 + 27 // 簡略化
    // R, Sを設定
    
    return nil
}
```

### 6. 段階的な移行戦略

#### Phase 1: ハッシュ関数の置き換え
```go
// 互換性のためのフラグ
var UseKeccak256 = false

func Hash(data []byte) []byte {
    if UseKeccak256 {
        return crypto.Keccak256Hash(data).Bytes()
    }
    return hashSHA256(data)
}

// CLIで切り替え
func cmdNode(args []string) {
    fs := flag.NewFlagSet("node", flag.ExitOnError)
    useKeccak := fs.Bool("keccak", false, "use Keccak-256 instead of SHA-256")
    
    // ...
    
    UseKeccak256 = *useKeccak
}
```

#### Phase 2: 署名方式の置き換え
```go
var UseSecp256k1 = false

func GenerateKey() (interface{}, interface{}, error) {
    if UseSecp256k1 {
        return GenerateSecp256k1Key()
    }
    return ed25519.GenerateKey(rand.Reader)
}
```

#### Phase 3: トランザクション形式の置き換え
```go
var UseRLP = false

func SerializeTransaction(tx Transaction) ([]byte, error) {
    if UseRLP {
        return EncodeTransactionRLP(tx)
    }
    return json.Marshal(tx)
}
```

## テストケース

### 1. ハッシュ関数の互換性
```go
func TestHashCompatibility(t *testing.T) {
    data := []byte("hello")
    
    // SHA-256
    sha256Hash := hashSHA256(data)
    
    // Keccak-256
    keccakHash := crypto.Keccak256Hash(data).Bytes()
    
    // 異なることを確認
    assert.NotEqual(t, sha256Hash, keccakHash)
    
    // 切り替えテスト
    UseKeccak256 = true
    switchedHash := Hash(data)
    assert.Equal(t, keccakHash, switchedHash)
}
```

### 2. 署名方式の互換性
```go
func TestSignatureCompatibility(t *testing.T) {
    message := []byte("test message")
    
    // Ed25519
    edPub, edPriv, _ := ed25519.GenerateKey(rand.Reader)
    edSig := ed25519.Sign(edPriv, message)
    
    // secp256k1
    secPriv, _ := GenerateSecp256k1Key()
    secSig, _ := SignTransactionSecp256k1(secPriv, message)
    
    // 異なる長さと形式
    assert.NotEqual(t, len(edSig), len(secSig))
    
    // 検証
    assert.True(t, ed25519.Verify(edPub, message, edSig))
    assert.True(t, VerifySecp256k1(&secPriv.PublicKey, message, secSig))
}
```

### 3. アドレス生成の互換性
```go
func TestAddressCompatibility(t *testing.T) {
    // secp256k1キーペア
    priv, _ := GenerateSecp256k1Key()
    
    // 現在の方式（SHA-256）
    currentAddr := addressFromPubkey(ed25519.PublicKey{}) // 異なる型なので比較用
    
    // Ethereum方式
    ethAddr := AddressFromSecp256k1Pubkey(&priv.PublicKey)
    
    // 形式チェック
    assert.Equal(t, 42, len(ethAddr)) // 0x + 40 hex chars
    assert.True(t, strings.HasPrefix(ethAddr, "0x"))
}
```

### 4. RLPエンコーディング
```go
func TestRLPEncoding(t *testing.T) {
    tx := Transaction{
        Nonce:    1,
        GasPrice: 1000000000,
        GasLimit: 21000,
        To:       "0x742d35Cc6634C0532925a3b8D4C9db96c4b4d8b6",
        Value:    1000000000000000000,
    }
    
    // JSONエンコーディング
    jsonBytes, _ := json.Marshal(tx)
    
    // RLPエンコーディング
    rlpBytes, _ := EncodeTransactionRLP(tx)
    
    // RLPの方がコンパクト
    assert.True(t, len(rlpBytes) < len(jsonBytes))
    
    // 復元テスト
    decodedTx, err := DecodeTransactionRLP(rlpBytes)
    assert.NoError(t, err)
    assert.Equal(t, tx.Nonce, decodedTx.Nonce)
    assert.Equal(t, tx.To, decodedTx.To)
}
```

## 実装手順

### Phase 1: ハッシュ関数
1. Keccak-256実装を追加
2. 切り替えフラグを導入
3. テストで互換性を確認

### Phase 2: 署名方式
1. secp256k1実装を追加
2. アドレス生成をEthereum方式に
3. 既存機能との互換性を維持

### Phase 3: トランザクション形式
1. RLPエンコーディングを実装
2. eth_sendRawTransactionを追加
3.段階的な移行

### Phase 4: その他の互換性
1. ChainIdを導入
2. EIP-155署名に対応
3. RPCの完全互換化

## 検証方法

### 手動テスト
```bash
# Keccak-256モードで起動
go run . node --genesis genesis.json --keccak --secp256k1 --rlp --addr :8545

# secp256k1キーを生成
go run . account --secp256k1

# Rawトランザクションを送信
curl -s -X POST localhost:8545 -d '{"jsonrpc":"2.0","method":"eth_sendRawTransaction","params":["0x..."],"id":1}'

# 互換性確認
curl -s -X POST localhost:8545 -d '{"jsonrpc":"2.0","method":"eth_blockNumber","params":[],"id":2}'
```

## 完了条件
- ハッシュ関数をKeccak-256に切り替え可能
- 署名方式をsecp256k1に切り替え可能
- eth_sendRawTransactionが動作する
- アドレスがEthereum準拠になる
- 既存機能との後方互換性が維持される

## 最終目標

このプロジェクトの学習目標達成のためのチェックリスト：

✅ **ブロックチェーンの基本概念**
- アカウント、残高、nonce
- トランザクションと署名
- ブロックとチェーン
- 状態遷移とstateRoot

✅ **ネットワークとコンセンサス**
- P2P通信
- フォークとFork Choice
- 簡易PoW
- マイニングと報酬

✅ **高度な概念**
- Gasと手数料
- スマートコントラクトの基本
- 永続化
- RPC API

✅ **実践的な実装**
- コード分割とテスト
- 段階的な機能追加
- 互換性への配慮

## 注意点

このマイルストーンでは完全なEthereum互換を目指すものではありません：

- **EVM**: 実装せず、簡易コントラクトのまま
- **Merkle Patricia Trie**: 簡易Merkle Treeのまま
- **完全なGasモデル**: 簡易モデルのまま
- **ネットワークプロトコル**: HTTPベースのまま

重要なのは「本物との差分を理解し、どこを置き換えれば良いかを知ること」です。これにより、将来的に本物のEthereum実装を学ぶ際の基礎知識が身につきます。

## おわりに

このプロジェクトを通して、ブロックチェーンの核心概念を段階的に学びました。各マイルストーンは独立しつつも連携し、全体像を理解できるように設計されています。

この学習経験が、実際のEthereumや他のブロックチェーンプロジェクトを理解するための強固な基礎となることを願っています。