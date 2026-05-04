# PLAN

このファイルは `mini-eth-node` の今後の実装計画です。

目的は、本物のEthereumを一気に再実装することではなく、ブロックチェーンの概念を段階的に理解できるように、小さな機能を順番に追加していくことです。

## 現在の到達点

実装済み:

- アカウント
  - `address`
  - `balance`
  - `nonce`

- トランザクション
  - `from`
  - `to`
  - `value`
  - `nonce`
  - `pubKey`
  - `signature`
  - `hash`

- ブロック
  - `number`
  - `parentHash`
  - `timestamp`
  - `transactions`
  - `stateRoot`
  - `hash`

- 状態遷移
  - 残高移動
  - nonce更新
  - 残高不足チェック
  - 署名検証

- JSON-RPC
  - `eth_blockNumber`
  - `eth_getBalance`
  - `sendTransaction`
  - `eth_getBlockByNumber`

- 簡易チェーン
  - Genesisブロック生成
  - `sendTransaction` ごとに1ブロック生成

## 基本方針

### やること

- 小さく実装する
- 動く状態を保つ
- テストしながら進める
- Ethereumの概念に対応づける
- 本物との差分を明記する

### すぐにはやらないこと

- EVM互換
- Ethereumメインネット互換
- 本物の `eth_sendRawTransaction`
- RLP
- Merkle Patricia Trie
- devp2p
- PoS
- Gas完全再現

## Milestone 1: 基本RPCの拡充

### 1.1 eth_getTransactionCount

目的:

- アカウントの現在nonceをRPCで取得できるようにする
- 次に使うべきnonceをクライアントが取得できるようにする

API:

```json
{
  "jsonrpc": "2.0",
  "method": "eth_getTransactionCount",
  "params": ["0xaddress", "latest"],
  "id": 1
}
```

期待結果:

```json
{
  "jsonrpc": "2.0",
  "result": "0x1",
  "id": 1
}
```

実装内容:

- `dispatch` に `eth_getTransactionCount` を追加
- `State[address].Nonce` をhex quantityで返す
- 存在しないアカウントは `0x0` を返す

テスト:

- Genesis直後のAlice nonceが `0x0`
- 1回送金後のAlice nonceが `0x1`
- Bobのnonceは受信だけでは増えない

### 1.2 eth_getTransactionByHash

目的:

- トランザクションハッシュからトランザクションを検索できるようにする

実装案:

- `Chain` に `TxIndex map[string]TxLocation` を追加
- `TxLocation` は `BlockNumber` と `TxIndex` を持つ

```go
type TxLocation struct {
    BlockNumber uint64
    TxIndex     int
}
```

テスト:

- 送信済みtxのhashで取得できる
- 未知のhashでは `null`

### 1.3 eth_getBlockByHash

目的:

- ブロックハッシュからブロックを取得できるようにする

実装案:

- `Chain` に `BlockIndex map[string]uint64` を追加

テスト:

- Genesisブロックhashで取得できる
- 最新ブロックhashで取得できる
- 未知のhashでは `null`

## Milestone 2: コード分割

目的:

- 1ファイルからパッケージ構成へ移行する
- 以降の拡張をしやすくする

候補構成:

```text
mini-eth-node/
  cmd/
    minieth/
      main.go
  internal/
    account/
      account.go
    block/
      block.go
    chain/
      chain.go
    crypto/
      crypto.go
    rpc/
      server.go
    state/
      state.go
    tx/
      transaction.go
```

作業:

- 型定義を分割
- RPC処理を分離
- 署名・ハッシュ処理を `crypto` に分離
- 状態遷移処理を `state` または `chain` に分離
- CLIコマンドを `cmd/minieth` に移動

テスト:

- 分割前後でREADMEの手順が同じように通る
- `go test ./...` が通る

## Milestone 3: 自動テストの追加

目的:

- 手動curl確認だけでなく、Goのテストで検証できるようにする

追加するテスト:

### 3.1 account test

- アカウント生成でaddressが20 bytesになる
- 同じ公開鍵から同じaddressが生成される

### 3.2 transaction test

- 正しい署名は検証成功
- 異なる秘密鍵の署名は検証失敗
- `from` と `pubKey` が一致しない場合は失敗

### 3.3 state transition test

- 正常送金で残高が移動する
- nonceが増える
- 残高不足は失敗する
- nonce不一致は失敗する

### 3.4 block test

- ブロック生成でnumberが増える
- parentHashが前ブロックを指す
- 状態が変わるとstateRootが変わる

### 3.5 rpc test

- `httptest` でRPCを呼び出す
- `eth_blockNumber`
- `eth_getBalance`
- `sendTransaction`
- `eth_getBlockByNumber`

完了条件:

```bash
go test ./...
```

が成功すること。

## Milestone 4: mempoolの追加

現在:

```text
sendTransaction
  -> 検証
  -> 状態遷移
  -> ブロック生成
```

変更後:

```text
sendTransaction
  -> 検証
  -> mempoolに追加

mineBlock
  -> mempoolからtxを取り出す
  -> 状態遷移
  -> ブロック生成
```

追加する構造:

```go
type Mempool struct {
    Transactions []Transaction
}
```

追加RPC:

- `txpool_content`
- `mineBlock`

考慮点:

- mempool内でのnonce順序
- 同一tx hashの重複拒否
- 残高不足txの扱い
- 同一アカウントから複数txが来た場合の順序

テスト:

- `sendTransaction` だけではblockNumberが増えない
- `txpool_content` でtxが見える
- `mineBlock` でブロックが増える
- 採掘後mempoolが空になる

## Milestone 5: 複数トランザクション入りブロック

目的:

- 1ブロックに複数txを含める
- ブロック内で順番に状態遷移する

課題:

- 同じ送信元のnonce順処理
- 途中で失敗するtxの扱い
- ブロック内での一時状態管理

方針:

- ブロック作成時にstateをコピーする
- txを順に適用する
- 成功したtxだけをブロックに入れる
- 失敗したtxはmempoolに残すか破棄する

テスト:

- Alice nonce 0, 1, 2 のtxが同じブロックに入る
- nonce 1だけ先に来た場合は処理されない
- Bobへの複数受信で残高が合算される

## Milestone 6: stateRootをMerkle Tree風にする

現在:

```text
sort(accounts)
json.Marshal(accounts)
sha256
```

変更後:

```text
leaf = hash(address, balance, nonce)
parent = hash(left, right)
root = root hash
```

目的:

- Merkle Treeの基本構造を学ぶ
- 状態証明への準備をする

実装内容:

```go
type StateLeaf struct {
    Address string
    Balance uint64
    Nonce   uint64
}
```

追加機能:

- `BuildMerkleRoot(accounts)`
- `BuildProof(address)`
- `VerifyProof(root, address, account, proof)`

追加RPC候補:

- `eth_getProof` 風の独自RPC `getAccountProof`

テスト:

- 状態が同じならrootが同じ
- balanceが変わるとrootが変わる
- proof検証が成功する
- 改ざんしたaccountではproof検証が失敗する

## Milestone 7: 永続化

現在:

- 状態はメモリ上のみ
- ノードを止めるとチェーンが消える

追加案:

- 最初はJSONファイル保存
- 次にBoltDBやBadgerDBへ移行

保存対象:

- Blocks
- State
- TxIndex
- BlockIndex

CLI案:

```bash
go run . node --genesis genesis.json --datadir ./data --addr :8545
```

テスト:

- ノード停止後もブロックが残る
- 再起動後にblockNumberが維持される
- 残高が維持される

## Milestone 8: 簡易PoW

目的:

- ブロック生成と難易度の概念を学ぶ

追加するフィールド:

```go
type Block struct {
    Number       uint64
    ParentHash   string
    Timestamp    int64
    Nonce        uint64
    Difficulty   uint64
    Transactions []Transaction
    StateRoot    string
    Hash         string
}
```

ルール例:

```text
blockHash が 0000 から始まるnonceを探す
```

追加RPC:

- `mineBlock`

テスト:

- 採掘されたブロックhashが難易度条件を満たす
- nonceを変更するとhashが変わる
- difficultyを上げると採掘に時間がかかる

## Milestone 9: P2Pの追加

目的:

- 複数ノード間でtxとblockを伝播する

最初の簡易設計:

```text
node A -- HTTP -- node B
node B -- HTTP -- node C
```

本物のdevp2pではなく、まずはHTTPで十分です。

追加機能:

- peer登録
- block broadcast
- transaction broadcast
- chain sync

追加RPC:

- `admin_addPeer`
- `net_peers`
- `broadcastTransaction`
- `broadcastBlock`

課題:

- 同じブロックの重複受信
- 親ブロックがない場合の扱い
- chain heightの比較
- fork choice

テスト:

- node Aで送信したtxがnode Bに届く
- node Aで作ったblockがnode Bに届く
- node BのblockNumberが追いつく

## Milestone 10: Fork Choice

目的:

- 複数のチェーン候補がある場合に、どれを正史にするかを決める

簡易ルール:

```text
最も長いチェーンを採用する
```

PoW追加後:

```text
total difficultyが最大のチェーンを採用する
```

必要な変更:

- 単一 `Blocks []Block` ではなく、ブロックツリーを持つ
- head blockを別で管理する
- stateをブロックごとに再計算または保存する

テスト:

- 同じ親から2つの子ブロックができる
- 長い方がheadになる
- head切り替え時にstateも切り替わる

## Milestone 11: Gas風の仕組み

目的:

- トランザクション手数料の基本を学ぶ

追加フィールド:

```go
type Transaction struct {
    From     string
    To       string
    Value    uint64
    Nonce    uint64
    GasLimit uint64
    GasPrice uint64
}
```

単純化したルール:

```text
送金txのgasUsed = 21000
手数料 = gasUsed * gasPrice
from.balance -= value + fee
miner.balance += fee
```

追加要素:

- coinbase / miner address
- block reward任意
- gas不足チェック

テスト:

- 送金額だけでなくfeeも引かれる
- minerにfeeが入る
- 残高不足なら失敗する

## Milestone 12: Contract風機能

EVMをいきなり作らず、まずは簡易コントラクトを実装します。

案:

```go
type Contract interface {
    Call(state *State, input []byte) ([]byte, error)
}
```

最初のコントラクト:

- Counter
- KeyValueStore
- SimpleToken

追加RPC:

- `eth_call` 風の `call`
- `sendTransaction` でcontract call

学べること:

- 通常アカウントとコントラクトアカウントの違い
- storage
- callとtransactionの違い
- state changing call

## Milestone 13: Ethereum形式への接近

目的:

- 少しずつ本物のEthereumに近づける

候補:

- SHA-256からKeccak-256へ
- Ed25519からsecp256k1へ
- JSON txからRLPへ
- 独自 `sendTransaction` から `eth_sendRawTransaction` へ
- address生成をEthereum方式へ
- chain id
- transaction signing payload

注意:

この段階から本物のEthereum仕様との差分管理が重要になります。

## 優先順位

推奨順:

1. `eth_getTransactionCount`
2. `eth_getTransactionByHash`
3. 自動テスト
4. コード分割
5. mempool
6. 複数tx入りブロック
7. Merkle Tree stateRoot
8. 永続化
9. PoW
10. P2P
11. Fork Choice
12. Gas
13. Contract風機能
14. Ethereum形式への接近

## 次に着手するIssue候補

### Issue 1: eth_getTransactionCountを追加する

概要:

- 指定アドレスのnonceを返すRPCを追加する

受け入れ条件:

- Genesis直後のnonceが `0x0`
- 送金後の送信者nonceが `0x1`
- 存在しないアドレスは `0x0`

### Issue 2: Go testを追加する

概要:

- 現在READMEにある手動テストのうち、コアロジックをGo test化する

受け入れ条件:

- `go test ./...` が成功する
- 正常送金のテストがある
- nonce不一致のテストがある
- 残高不足のテストがある
- 署名不正のテストがある

### Issue 3: mempoolを追加する

概要:

- `sendTransaction` は即ブロック生成せず、mempoolに追加する
- `mineBlock` でブロックを生成する

受け入れ条件:

- `sendTransaction` 後にblockNumberが増えない
- `txpool_content` でtxを確認できる
- `mineBlock` 後にblockNumberが増える
- 採掘後mempoolが空になる

## メモ

このプロジェクトでは「本物っぽさ」よりも「概念が見えること」を優先します。

本物のEthereum仕様に近づける場合は、各機能を追加するたびに以下を明記します。

- 本物のEthereumではどうなっているか
- この実装では何を簡略化しているか
- 後から差し替えるにはどこを変えるべきか
