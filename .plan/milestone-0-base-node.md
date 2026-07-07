# Milestone 0: 基礎実装（完了）

## 概要
ミニEthereum風ノードの土台を**ゼロから実装する手順書**です。このマイルストーンを終えると、アカウント生成 → 署名付きトランザクション送信 → 残高移動 → ブロック生成、という一連の流れがJSON-RPC経由で動くようになります。

このドキュメントは現リポジトリの実装と一致しています（実装済み。コードを読むときの参照にも使えます）。以降のマイルストーンはすべてこの実装を前提に差分を記述します。

## 目的
- アカウントベースの状態管理（address / balance / nonce）を実装する
- 署名付きトランザクションによる状態遷移を実装する
- nonceによるリプレイ防止と残高チェックを実装する
- ブロックとチェーン（parentHashによる連結、stateRoot）を実装する
- JSON-RPCでノードを操作できるようにする

## 前提環境
- Go 1.22以上（リポジトリは go 1.26）
- curl、jq（動作確認用）
- 外部ライブラリは使わない（標準ライブラリのみ）

## 全体像

### 何を作るか
```text
minieth account                  # Ed25519鍵ペアとアドレスを生成
minieth sign --priv ... --to ... # 送金txに署名してJSONを出力
minieth node --genesis ...       # ノード起動（JSON-RPCサーバー）
```

ノードは `sendTransaction` を受けると、署名・nonce・残高を検証して状態遷移し、**即座に1txのブロックを生成**します（mempoolはMilestone 4で導入）。

### パッケージ構成と依存関係
```text
like-a-ethereum/
├── cmd/minieth/main.go   # CLI（account / sign / node）
├── internal/
│   ├── util/             # hex変換、HashJSON（依存なし。最初に作る）
│   ├── crypto/           # HashJSONの別名（util委譲の薄いラッパー）
│   ├── account/          # Account型、アドレス導出・正規化（→ util）
│   ├── tx/               # Transaction型、署名対象、検証（→ account, util）
│   ├── block/            # Block型、ブロックハッシュ（→ tx, util）
│   ├── chain/            # 状態・ブロック管理、状態遷移（→ account, block, tx, util）
│   ├── rpc/              # JSON-RPCサーバー（→ chain ほか）
│   └── state/            # 空パッケージ（将来拡張用のプレースホルダ）
├── genesis.json
├── Makefile
└── go.mod
```

依存の向きは `cmd → rpc → chain → (block, tx, account) → util` の一方向。実装も依存の少ない順（util → account → tx → block → chain → rpc → cmd）に進めると、常にビルドが通る状態を保てます。

### 設計の決めごと

| 項目 | 決定 | 理由 |
|---|---|---|
| 鍵・署名 | Ed25519 | 標準ライブラリだけで完結する。secp256k1への置き換えはMilestone 14 |
| ハッシュ | SHA-256（JSON化してハッシュ） | 同上。Keccak-256はMilestone 14 |
| アドレス | `SHA-256(公開鍵)` の末尾20バイト | Ethereumの `Keccak256(pubkey)[12:]`（末尾20バイト）と同じ構造 |
| 署名対象 | `from\|to\|value\|nonce` の文字列 | PubKey/Signatureは含めない（署名は自分自身を対象にできない） |
| txハッシュ | 署名まで含めた全フィールドのJSONハッシュ | 「このtxそのもの」の識別子。Ed25519は決定的署名なので同一内容なら同一hash |
| stateRoot | ソート済み全アカウントのJSONハッシュ | 最も単純な状態コミットメント。Merkle Tree化はMilestone 6 |
| ブロック生成 | `sendTransaction` ごとに即1ブロック | コンセンサスを持たない最小構成。mempool分離はMilestone 4 |
| 排他制御 | Chain全体で単一の `sync.Mutex` | 最初は最も単純に。RPC層は `Chain.Lock()/Unlock()` を使う |

## 実装手順

### Step 1: プロジェクト初期化

```bash
mkdir like-a-ethereum && cd like-a-ethereum
go mod init like-a-ethereum
mkdir -p cmd/minieth internal/{util,crypto,account,tx,block,chain,rpc,state}
echo 'bin/' > .gitignore
```

**Makefile:**
```makefile
BINARY  := minieth
CMD     := ./cmd/minieth
BIN_DIR := ./bin

.PHONY: build run clean test lint

build:
	go build -o $(BIN_DIR)/$(BINARY) $(CMD)

run: build
	$(BIN_DIR)/$(BINARY) node --genesis genesis.json --addr :8545

clean:
	rm -rf $(BIN_DIR)

test:
	go test ./...

lint:
	go vet ./...
```

### Step 2: utilパッケージ

**ファイル**: `internal/util/util.go`

hex表現（`0x`プレフィックス）とハッシュの共通処理。全パッケージがここに依存する。

```go
package util

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
)

const Prefix = "0x"

// HashJSON はJSONシリアライズしてSHA-256ハッシュを返す（0x + 64 hex chars）
func HashJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}

	sum := sha256.Sum256(b)

	return Prefix + hex.EncodeToString(sum[:])
}

// ToHex は数値をhex quantity（0x1, 0x3e8 など）に変換する
func ToHex(n uint64) string {
	return Prefix + strconv.FormatUint(n, 16)
}

func DecodeHex(s string) ([]byte, error) {
	s = strings.TrimPrefix(strings.TrimSpace(s), Prefix)
	return hex.DecodeString(s)
}

func ParseHexQuantity(s string) (uint64, error) {
	if !strings.HasPrefix(s, Prefix) {
		return 0, errors.New("quantity must be hex string such as 0x1 or latest")
	}

	return strconv.ParseUint(strings.TrimPrefix(s, Prefix), 16, 64)
}
```

**ファイル**: `internal/crypto/crypto.go`（薄いラッパー。cryptoという名前で参照したい場合用）

```go
package crypto

import "like-a-ethereum/internal/util"

// HashJSON はJSONシリアライズしてSHA-256ハッシュを返す。
// util.HashJSON の薄いラッパー。crypto パッケージ経由でも呼べるようにしている。
var HashJSON = util.HashJSON
```

**ファイル**: `internal/state/state.go`（空パッケージ。将来の状態遷移ロジック分離用）

```go
package state
```

確認: `go build ./...`

### Step 3: accountパッケージ

**ファイル**: `internal/account/account.go`

```go
package account

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"

	"like-a-ethereum/internal/util"
)

type Account struct {
	Address string
	Balance uint64
	Nonce   uint64
}

// AddressFromPubkey は公開鍵のSHA-256の末尾20バイトをアドレスにする
//（Ethereumの Keccak256(pubkey)[12:] と同じ構造）
func AddressFromPubkey(pub ed25519.PublicKey) string {
	sum := sha256.Sum256(pub)

	return util.Prefix + hex.EncodeToString(sum[len(sum)-20:])
}

// NormalizeAddress は小文字化・trimし、0xプレフィックスと40 hex charsを検証する
func NormalizeAddress(s string) (string, error) {
	s = strings.ToLower(strings.TrimSpace(s))

	if !strings.HasPrefix(s, util.Prefix) {
		return "", errors.New("address must start with 0x")
	}

	h := strings.TrimPrefix(s, util.Prefix)

	if len(h) != 40 {
		return "", errors.New("address must be 20 bytes / 40 hex chars")
	}

	if _, err := hex.DecodeString(h); err != nil {
		return "", err
	}

	return util.Prefix + h, nil
}
```

### Step 4: txパッケージ

**ファイル**: `internal/tx/transaction.go`

重要な設計ポイント:

- **署名対象**（`TxSignBytes`）は `from|to|value|nonce` のみ。署名は自分自身を対象にできないので、PubKey/Signatureは含めない
- **txハッシュ**（`TxHash`）は署名まで含めた全フィールドのJSONハッシュ。「この署名済みtxそのもの」の識別子になる
- 検証は「アドレス正規化 → 公開鍵とfromの一致 → 署名検証 → hash確定」の順

```go
package tx

import (
	"crypto/ed25519"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"like-a-ethereum/internal/account"
	"like-a-ethereum/internal/util"
)

type Transaction struct {
	From      string
	To        string
	Value     uint64
	Nonce     uint64
	PubKey    string
	Signature string
	Hash      string
}

// VerfyAndNormalizeTx はtxを検証し、正規化して返す。
// （関数名はtypo。Milestone 3で VerifyAndNormalizeTx にリネームする — 既知の問題2）
func VerfyAndNormalizeTx(tx Transaction) (Transaction, error) {
	from, err := account.NormalizeAddress(tx.From)
	if err != nil {
		return tx, err
	}

	to, err := account.NormalizeAddress(tx.To)
	if err != nil {
		return tx, err
	}

	tx.From = from
	tx.To = to

	pubBytes, err := util.DecodeHex(tx.PubKey)
	if err != nil {
		return tx, fmt.Errorf("bad public key: %w", err)
	}

	if len(pubBytes) != ed25519.PublicKeySize {
		return tx, fmt.Errorf("public key must be %d bytes", ed25519.PublicKeySize)
	}

	// 公開鍵から導出したアドレスとfromの一致を確認する。
	// これがないと「他人のアドレスをfromに書いた自分の署名」が通ってしまう
	if account.AddressFromPubkey(ed25519.PublicKey(pubBytes)) != tx.From {
		return tx, errors.New("public key does not match from address")
	}

	sigBytes, err := util.DecodeHex(tx.Signature)
	if err != nil {
		return tx, fmt.Errorf("bad signature: %w", err)
	}

	if !ed25519.Verify(ed25519.PublicKey(pubBytes), TxSignBytes(tx), sigBytes) {
		return tx, errors.New("invalid signature")
	}

	tx.PubKey = util.Prefix + hex.EncodeToString(pubBytes)
	tx.Signature = util.Prefix + hex.EncodeToString(sigBytes)
	tx.Hash = TxHash(tx)

	return tx, nil
}

// TxSignBytes は署名対象バイト列を返す（from|to|value|nonce）
func TxSignBytes(tx Transaction) []byte {
	return []byte(fmt.Sprintf(
		"%s|%s|%d|%d",
		strings.ToLower(tx.From),
		strings.ToLower(tx.To),
		tx.Value,
		tx.Nonce,
	))
}

// TxHash は署名まで含めたtx全体のハッシュを返す
func TxHash(tx Transaction) string {
	payload := struct {
		From      string `json:"from"`
		To        string `json:"to"`
		Value     uint64 `json:"value"`
		Nonce     uint64 `json:"nonce"`
		PubKey    string `json:"pubKey"`
		Signature string `json:"signature"`
	}{
		tx.From,
		tx.To,
		tx.Value,
		tx.Nonce,
		tx.PubKey,
		tx.Signature,
	}

	return util.HashJSON(payload)
}
```

### Step 5: blockパッケージ

**ファイル**: `internal/block/block.go`

ブロックハッシュはtx本体ではなく**txハッシュの列**から計算する（ブロックの同一性はtxの同一性で決まる）。

```go
package block

import (
	"like-a-ethereum/internal/tx"
	"like-a-ethereum/internal/util"
)

type Block struct {
	Number       uint64
	ParentHash   string
	Timestamp    int64
	Transactions []tx.Transaction
	StateRoot    string
	Hash         string
}

func BlockHash(b Block) string {
	txHashes := make([]string, len(b.Transactions))

	for i, t := range b.Transactions {
		txHashes[i] = t.Hash
	}

	payload := struct {
		Number     uint64   `json:"number"`
		ParentHash string   `json:"parentHash"`
		Timestamp  int64    `json:"timestamp"`
		TxHashes   []string `json:"txHashes"`
		StateRoot  string   `json:"stateRoot"`
	}{
		b.Number,
		b.ParentHash,
		b.Timestamp,
		txHashes,
		b.StateRoot,
	}

	return util.HashJSON(payload)
}

// ToRPCBlock はRPCレスポンス用の形式に変換する。
// fullTx=falseの場合はtxハッシュの列だけを返す（eth_getBlockByNumberの仕様に合わせる）
func ToRPCBlock(b Block, fullTx bool) map[string]any {
	var txs any

	if fullTx {
		txs = b.Transactions
	} else {
		hashes := make([]string, len(b.Transactions))

		for i, t := range b.Transactions {
			hashes[i] = t.Hash
		}

		txs = hashes
	}

	return map[string]any{
		"number":       util.ToHex(b.Number),
		"hash":         b.Hash,
		"parentHash":   b.ParentHash,
		"timestamp":    util.ToHex(uint64(b.Timestamp)),
		"transactions": txs,
		"stateRoot":    b.StateRoot,
	}
}
```

### Step 6: chainパッケージ

**ファイル**: `internal/chain/chain.go`

このマイルストーンの中心。状態遷移のルールは:

1. 署名検証・正規化（`tx.VerfyAndNormalizeTx`）
2. fromアカウントの存在チェック
3. nonce一致チェック（リプレイ防止）
4. 残高チェック
5. 宛先が未知ならアカウントを新規作成
6. 残高移動・nonceインクリメント
7. **遷移後の状態**からstateRootを計算してブロック生成

```go
package chain

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"sync"
	"time"

	"like-a-ethereum/internal/account"
	"like-a-ethereum/internal/block"
	"like-a-ethereum/internal/tx"
	"like-a-ethereum/internal/util"
)

type Chain struct {
	mu     sync.Mutex
	State  map[string]account.Account
	Blocks []block.Block
}

type Genesis struct {
	Alloc map[string]uint64 `json:"alloc"`
}

func NewChainFromGenesis(path string) (*Chain, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var g Genesis
	if err := json.Unmarshal(b, &g); err != nil {
		return nil, err
	}

	state := make(map[string]account.Account)

	for addr, bal := range g.Alloc {
		n, err := account.NormalizeAddress(addr)
		if err != nil {
			return nil, fmt.Errorf("bad genesis address %q: %w", addr, err)
		}

		state[n] = account.Account{
			Address: n,
			Balance: bal,
			Nonce:   0,
		}
	}

	c := &Chain{State: state}

	genesis := block.Block{
		Number:     0,
		ParentHash: "0x0000000000000000000000000000000000000000000000000000000000000000",
		// 注意: 起動時刻を使うとgenesis hashが起動ごとに変わる（既知の問題1）。
		// 遅くともMilestone 9（P2P）までに固定値化が必要
		Timestamp:    time.Now().Unix(),
		Transactions: []tx.Transaction{},
		StateRoot:    c.computeStateRootLocked(),
	}

	genesis.Hash = block.BlockHash(genesis)
	c.Blocks = []block.Block{genesis}

	return c, nil
}

// AddTransaction は検証 → 状態遷移 → 即ブロック生成を行う
//（mempool分離はMilestone 4で行う）
func (c *Chain) AddTransaction(t tx.Transaction) (map[string]any, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	verified, err := tx.VerfyAndNormalizeTx(t)
	if err != nil {
		return nil, err
	}

	from := c.State[verified.From]
	if from.Address == "" {
		return nil, errors.New("from account does not exist")
	}

	if verified.Nonce != from.Nonce {
		return nil, fmt.Errorf("bad nonce: got %d, want %d", verified.Nonce, from.Nonce)
	}

	if from.Balance < verified.Value {
		return nil, errors.New("insufficient balance")
	}

	to := c.State[verified.To]
	if to.Address == "" {
		to = account.Account{
			Address: verified.To,
			Balance: 0,
			Nonce:   0,
		}
	}

	from.Balance -= verified.Value
	from.Nonce++
	to.Balance += verified.Value

	c.State[from.Address] = from
	c.State[to.Address] = to

	blk := block.Block{
		Number:       c.Blocks[len(c.Blocks)-1].Number + 1,
		ParentHash:   c.Blocks[len(c.Blocks)-1].Hash,
		Timestamp:    time.Now().Unix(),
		Transactions: []tx.Transaction{verified},
		StateRoot:    c.computeStateRootLocked(), // 遷移後の状態から計算する
	}

	blk.Hash = block.BlockHash(blk)
	c.Blocks = append(c.Blocks, blk)

	return map[string]any{
		"transactionHash": verified.Hash,
		"blockNumber":     util.ToHex(blk.Number),
		"blockHash":       blk.Hash,
		"stateRoot":       blk.StateRoot,
	}, nil
}

// computeStateRootLocked は全アカウントをアドレス順にソートし、
// JSON化してSHA-256を取る（Merkle Tree化はMilestone 6）
func (c *Chain) computeStateRootLocked() string {
	type item struct {
		Address string `json:"address"`
		Balance uint64 `json:"balance"`
		Nonce   uint64 `json:"nonce"`
	}

	keys := make([]string, 0, len(c.State))
	for k := range c.State {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	items := make([]item, 0, len(keys))
	for _, k := range keys {
		a := c.State[k]
		items = append(items, item{
			Address: a.Address,
			Balance: a.Balance,
			Nonce:   a.Nonce,
		})
	}

	return util.HashJSON(items)
}

// RPC層がロックを取るための公開メソッド
func (c *Chain) Lock()   { c.mu.Lock() }
func (c *Chain) Unlock() { c.mu.Unlock() }
```

確認: `go build ./...`

### Step 7: rpcパッケージ

**ファイル**: `internal/rpc/server.go`

JSON-RPC 2.0の最小実装。このマイルストーンで実装するメソッドは4つ（`eth_getTransactionCount` などの拡充はMilestone 1）。

| メソッド | params | 挙動 |
|---|---|---|
| `eth_blockNumber` | `[]` | 最新ブロック番号（hex quantity） |
| `eth_getBalance` | `[address, blockTag]` | 残高。未知アドレスは `0x0`。blockTagは無視（常に最新） |
| `sendTransaction` | `[txオブジェクト]` | 検証→状態遷移→即ブロック生成。receiptを返す |
| `eth_getBlockByNumber` | `["latest" \| "0xN", fullTx]` | ブロック。範囲外はnull |

エラーコード: `-32700`（parse error）/ `-32601`（method not found）/ `-32602`（パラメータ不正。業務エラーもここに畳まれる — 既知の問題6）

```go
package rpc

import (
	"encoding/json"
	"errors"
	"net/http"

	"like-a-ethereum/internal/account"
	"like-a-ethereum/internal/block"
	"like-a-ethereum/internal/chain"
	"like-a-ethereum/internal/tx"
	"like-a-ethereum/internal/util"
)

const jsonrpc = "2.0"

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
	ID      any             `json:"id"`
}

type rpcResponse struct {
	JSONRPC string    `json:"jsonrpc"`
	Result  any       `json:"result,omitempty"` // 既知の問題5: nullのときフィールドごと消える
	Error   *rpcError `json:"error,omitempty"`
	ID      any       `json:"id"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func HandleRPC(c *chain.Chain) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		var req rpcRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeRPC(w, nil, nil, &rpcError{
				Code:    -32700,
				Message: "parse error",
			})
			return
		}

		result, rpcErr := dispatch(c, req.Method, req.Params)
		writeRPC(w, req.ID, result, rpcErr)
	}
}

func dispatch(c *chain.Chain, method string, params json.RawMessage) (any, *rpcError) {
	switch method {
	case "eth_blockNumber":
		c.Lock()
		defer c.Unlock()

		return util.ToHex(c.Blocks[len(c.Blocks)-1].Number), nil

	case "eth_getBalance":
		addr, err := parseGetBalanceParams(params)
		if err != nil {
			return nil, rpcInvalidParams(err)
		}

		c.Lock()
		defer c.Unlock()

		acct := c.State[addr]
		return util.ToHex(acct.Balance), nil

	case "sendTransaction":
		var t tx.Transaction

		if err := parseSingleObjectParam(params, &t); err != nil {
			return nil, rpcInvalidParams(err)
		}

		receipt, err := c.AddTransaction(t)
		if err != nil {
			return nil, rpcInvalidParams(err)
		}

		return receipt, nil

	case "eth_getBlockByNumber":
		number, fullTx, err := parseGetBlockParams(params)
		if err != nil {
			return nil, rpcInvalidParams(err)
		}

		c.Lock()
		defer c.Unlock()

		var idx uint64

		if number == "latest" {
			idx = uint64(len(c.Blocks) - 1)
		} else {
			var err error
			idx, err = util.ParseHexQuantity(number)
			if err != nil {
				return nil, rpcInvalidParams(err)
			}
		}

		if idx >= uint64(len(c.Blocks)) {
			return nil, nil
		}

		return block.ToRPCBlock(c.Blocks[idx], fullTx), nil

	default:
		return nil, &rpcError{
			Code:    -32601,
			Message: "method not found",
		}
	}
}

// rpc utils

func parseGetBalanceParams(params json.RawMessage) (string, error) {
	var arr []json.RawMessage

	if err := json.Unmarshal(params, &arr); err != nil || len(arr) < 1 {
		return "", errors.New("eth_getBalance expects [address, blockTag]")
	}

	var addr string
	if err := json.Unmarshal(arr[0], &addr); err != nil {
		return "", err
	}

	return account.NormalizeAddress(addr)
}

func parseGetBlockParams(params json.RawMessage) (number string, fullTx bool, err error) {
	var arr []json.RawMessage

	if err := json.Unmarshal(params, &arr); err != nil || len(arr) < 1 {
		return "", false, errors.New("eth_getBlockByNumber expects [number, fullTx]")
	}

	if err := json.Unmarshal(arr[0], &number); err != nil {
		return "", false, err
	}

	if len(arr) >= 2 {
		_ = json.Unmarshal(arr[1], &fullTx)
	}

	return number, fullTx, nil
}

// parseSingleObjectParam は [obj] 形式と生のobj形式の両方を受ける
func parseSingleObjectParam(params json.RawMessage, out any) error {
	var arr []json.RawMessage

	if err := json.Unmarshal(params, &arr); err == nil && len(arr) == 1 {
		return json.Unmarshal(arr[0], out)
	}

	return json.Unmarshal(params, out)
}

func writeRPC(w http.ResponseWriter, id any, result any, errObj *rpcError) {
	resp := rpcResponse{
		JSONRPC: jsonrpc,
		ID:      id,
	}

	if errObj != nil {
		resp.Error = errObj
	} else {
		resp.Result = result
	}

	_ = json.NewEncoder(w).Encode(resp)
}

func rpcInvalidParams(err error) *rpcError {
	return &rpcError{
		Code:    -32602,
		Message: err.Error(),
	}
}
```

### Step 8: CLI

**ファイル**: `cmd/minieth/main.go`

```go
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	"like-a-ethereum/internal/account"
	"like-a-ethereum/internal/chain"
	"like-a-ethereum/internal/rpc"
	"like-a-ethereum/internal/tx"
	"like-a-ethereum/internal/util"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		return
	}

	switch os.Args[1] {
	case "account":
		cmdAccount()
	case "sign":
		cmdSign(os.Args[2:])
	case "node":
		cmdNode(os.Args[2:])
	default:
		usage()
	}
}

func usage() {
	fmt.Println(`usage:
  go run ./cmd/minieth account
  go run ./cmd/minieth sign --priv <privateKeyHex> --to <address> --value <amount> --nonce <nonce>
  go run ./cmd/minieth node --genesis genesis.json --addr :8545`)
}

func cmdAccount() {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	must(err)

	out := map[string]string{
		"address":    account.AddressFromPubkey(pub),
		"publicKey":  "0x" + hex.EncodeToString(pub),
		"privateKey": "0x" + hex.EncodeToString(priv),
	}

	printJSON(out)
}

func cmdSign(args []string) {
	fs := flag.NewFlagSet("sign", flag.ExitOnError)

	privHex := fs.String("priv", "", "private key hex")
	to := fs.String("to", "", "recipient address")
	value := fs.Uint64("value", 0, "value")
	nonce := fs.Uint64("nonce", 0, "nonce")

	must(fs.Parse(args))

	privBytes, err := util.DecodeHex(*privHex)
	must(err)

	if len(privBytes) != ed25519.PrivateKeySize {
		must(fmt.Errorf("private key must be %d bytes", ed25519.PrivateKeySize))
	}

	priv := ed25519.PrivateKey(privBytes)
	pub := priv.Public().(ed25519.PublicKey)

	toAddr, err := account.NormalizeAddress(*to)
	must(err)

	t := tx.Transaction{
		From:   account.AddressFromPubkey(pub),
		To:     toAddr,
		Value:  *value,
		Nonce:  *nonce,
		PubKey: "0x" + hex.EncodeToString(pub),
	}

	sig := ed25519.Sign(priv, tx.TxSignBytes(t))
	t.Signature = "0x" + hex.EncodeToString(sig)
	t.Hash = tx.TxHash(t)

	printJSON(t)
}

func cmdNode(args []string) {
	fs := flag.NewFlagSet("node", flag.ExitOnError)

	genesisPath := fs.String("genesis", "genesis.json", "genesis file")
	listenAddr := fs.String("addr", ":8545", "listen address")

	must(fs.Parse(args))

	c, err := chain.NewChainFromGenesis(*genesisPath)
	must(err)

	http.HandleFunc("/", rpc.HandleRPC(c))

	log.Printf("mini ethereum-like node listening on %s", *listenAddr)
	log.Printf("genesis block hash: %s stateRoot: %s", c.Blocks[0].Hash, c.Blocks[0].StateRoot)

	must(http.ListenAndServe(*listenAddr, nil))
}

func printJSON(v any) {
	b, err := json.MarshalIndent(v, "", "  ")
	must(err)

	fmt.Println(string(b))
}

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
```

確認: `make build` で `bin/minieth` が生成される。

### Step 9: genesis.jsonと動作確認

```bash
# 1. アカウントを2つ生成（AliceとBob）。address / privateKey を控える
./bin/minieth account
./bin/minieth account

# 2. genesis.jsonを作成（Aliceに1000、Bobに0）
cat > genesis.json <<'JSON'
{
    "alloc": {
        "0x<AliceのAddress>": 1000,
        "0x<BobのAddress>": 0
    }
}
JSON

# 3. ノード起動
make run
```

## 検証方法（手動テスト）

以下の7項目が完了の判定基準。Milestone 3でこれらをGoテストとして固定する。

```bash
# テスト1: ビルドできること
make build

# テスト2: アカウント生成でaddressが 0x + 40 hex chars になること
./bin/minieth account

# テスト3: 起動直後の eth_blockNumber が 0x0
curl -s -X POST localhost:8545 -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","method":"eth_blockNumber","params":[],"id":1}'
# → {"jsonrpc":"2.0","result":"0x0","id":1}

# テスト4: 送金でブロックが増えること
./bin/minieth sign --priv 0x<AlicePrivKey> --to 0x<BobAddr> --value 150 --nonce 0
# 出力されたJSONをそのままparamsに入れて送信
curl -s -X POST localhost:8545 -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","method":"sendTransaction","params":[<署名済みtx JSON>],"id":2}'
# → blockNumber "0x1" を含むreceiptが返る

# テスト5: 残高が状態遷移していること
curl -s -X POST localhost:8545 -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","method":"eth_getBalance","params":["0x<AliceAddr>","latest"],"id":3}'
# → "0x352"（850）。Bobは "0x96"（150）

# テスト6: 同じtxの再送がnonceエラーになること（リプレイ防止）
# → {"error":{"code":-32602,"message":"bad nonce: got 0, want 1"}}

# テスト7: 残高不足の送金が失敗すること
# → {"error":{"code":-32602,"message":"insufficient balance"}}
```

## 完了条件
- `make build` / `go vet ./...` が通る
- 上の手動テスト1〜7がすべて期待どおりに動く
- パッケージ構成が「全体像」のとおりになっている

## 次のステップ
Milestone 1で基本RPCを拡充します（`eth_getTransactionCount` / `eth_getTransactionByHash` / `eth_getBlockByHash` と、そのためのTxIndex / BlockIndex）。

## 既知の問題

実リポジトリの実装に存在する問題の記録（このマイルストーンの手順書どおりに再実装する場合も同じ問題を踏むので、修正時期とセットで把握しておく）:

| # | 内容 | 直す時期 |
|---|---|---|
| 1 | genesisのTimestampが `time.Now()` のため、genesis hashが起動ごと・ノードごとに変わる。永続化（M7）の再起動同一性や、P2P（M9）の「全ノードが同一genesisを持つ」前提を壊す | **遅くともM9まで**（テストの再現性向上のためM3で直すのが望ましい。genesis.jsonにtimestampフィールドを持たせるか固定値にする） |
| 2 | `tx.VerfyAndNormalizeTx` は関数名がtypo（Verify → Verfy） | M3（テスト整備時にリネーム） |
| 3 | Milestone 1で追加した `eth_getTransactionByHash` の実装が、`blockNumber` を `util.ToHex` に通し忘れて数値のまま返している（M1の手順書の意図はhex quantity） | M3 |
| 4 | Milestone 1で追加した `eth_getBlockByHash` のパラメータエラーメッセージ末尾に、デバッグ用の `strconv.Itoa(len(arr))` が残っている | M3 |
| 5 | resultがnullのとき、レスポンス構造体の `omitempty` により `result` フィールドごと消える。JSON-RPC 2.0では成功時 `result` は必須なので `"result": null` を返すべき | M3 |
| 6 | 業務エラー（bad nonce / insufficient balance / invalid signature等）がすべて `-32602 invalid params` にマップされる | M3以降（エラーコード整理のタイミングで） |
| 7 | 秘密鍵をCLI引数で渡す設計（シェル履歴・psに残る）。学習用として許容 | 対応しない（記録のみ） |

## Ethereumとの違い（このマイルストーン時点）

| 項目 | Ethereum | この実装 |
|---|---|---|
| 署名方式 | secp256k1 | Ed25519 |
| ハッシュ | Keccak-256 | SHA-256 |
| stateRoot | Merkle Patricia Trie | ソート済みアカウント一覧のSHA-256 |
| トランザクション形式 | RLP / Typed Transaction | JSON |
| ブロック生成 | PoS / Consensus | `sendTransaction` 時に即ブロック生成 |
| 送信RPC | `eth_sendRawTransaction` | 独自 `sendTransaction` |
| P2P / EVM / Gas | あり | なし |
