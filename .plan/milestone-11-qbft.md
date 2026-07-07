# Milestone 11: QBFTコンセンサス

## 概要
PoW（Milestone 8）は「計算量勝負＋最長チェーンルール」による確率的ファイナリティでした。このマイルストーンでは、Hyperledger BesuやGoQuorumで実際に使われているBFT系コンセンサス **QBFT** を実装し、**即時ファイナリティ**を学びます。

QBFTでは、既知のvalidator集合が3フェーズのメッセージ交換（PRE-PREPARE → PREPARE → COMMIT）で合意します。2f+1のCOMMITが集まった時点でブロックは**確定**し、原理的にフォークが起きません。つまりMilestone 10で作ったfork choice・リオーグの仕組みが**不要になる**——この対比がこのマイルストーンの核心です。

## 目的
- BFT合意の基本（3フェーズ合意、quorum = 2f+1、なぜ3f+1ノード必要か）を学ぶ
- 即時ファイナリティと確率的ファイナリティ（PoW）の違いを体験する
- proposer選出とround changeの概念を学ぶ
- commit sealによるブロック検証（PoWチェックの置き換え）を学ぶ

## 前提となるマイルストーン
- Milestone 4（mempool）、Milestone 5（複数txブロック）: ブロック生成の材料
- Milestone 9（P2P）: validator間のメッセージ交換の土台（**必須**。QBFTは1ノードでは動かない）
- Milestone 10（Fork Choice）: 直接は使わないが、「QBFTではなぜ不要になるか」を理解するための前提知識

## QBFTの基本

### Validator集合と障害耐性
- 合意に参加するノード（validator）は事前に既知
- f台の故障・悪意ノードに耐えるには **3f+1台以上** 必要
- 合意には **2f+1票（quorum）** が必要
- 最小構成: 4ノード（f=1。1台落ちても進む、2台落ちると止まる）

### 3フェーズ合意（happy path）
```text
sequence = ブロック番号, round = 何回目の試行か

1. PRE-PREPARE: proposerがブロックを作って全validatorに提案
2. PREPARE:     各validatorが提案を検証し、賛同を全validatorに送る
3. COMMIT:      2f+1のPREPAREを見たvalidatorが、確定署名を全validatorに送る
4. 確定:        2f+1のCOMMITを見たvalidatorがブロックをチェーンに追加
```

### Proposer選出
sequenceとroundから決定的に選ぶ（全ノードが同じ計算をして同じ結論になることが重要）:

```go
func (q *QBFT) proposerFor(sequence, round uint64) string {
    idx := (sequence + round) % uint64(len(q.validators))
    return q.validators[idx]
}
```

### Round change
proposerが落ちている・提案が届かない場合、タイムアウトしたvalidatorがROUND-CHANGEメッセージを送り、2f+1集まったら round+1（＝次のproposer）でやり直す。

## 実装スコープ（段階を分ける）

一気に作らず、以下の順で進めます。

1. **Step 1: happy path** — 固定validator set、proposerは落ちない前提。round changeなし
2. **Step 2: commit seal検証** — 受信ブロックの検証をPoWチェックから「2f+1のvalidator署名チェック」に差し替え
3. **Step 3: round change** — proposer停止からの復帰
4. **発展（スコープ外でも良い）**: validator投票による追加・削除、自動ブロック生成タイマー

## 実装計画

### 1. Genesisへのvalidator設定

```json
{
  "alloc": {
    "0xalice...": 1000
  },
  "qbft": {
    "validators": [
      "0xvalidator1...",
      "0xvalidator2...",
      "0xvalidator3...",
      "0xvalidator4..."
    ]
  }
}
```

- validatorのアドレスは既存の `minieth account` で生成したものを使う
- 各ノードは起動時に `--nodekey`（validator秘密鍵）を受け取り、自分がvalidatorかどうかを判定する

```bash
go run ./cmd/minieth node --genesis genesis.json --datadir ./data1 --addr :8545 --nodekey 0x...
```

### 2. Block構造体の変更

```go
type Block struct {
    Number       uint64
    ParentHash   string
    Timestamp    int64
    Round        uint64        // 追加: 確定したround
    Proposer     string        // 追加: 提案したvalidator
    Transactions []Transaction
    StateRoot    string
    Hash         string
    CommitSeals  []string      // 追加: 2f+1のvalidator署名（ブロックハッシュへの署名）
}
```

重要: **CommitSealsはブロックハッシュの計算対象から除外する**こと。

- 署名対象は「CommitSealsを除いたブロック内容のハッシュ」
- CommitSealsを含めてハッシュを計算すると、「署名対象のハッシュ」が「署名を含んだハッシュ」に依存する循環が生まれて成立しない
- 本物のQBFT（Besu）でも、extraData内のcommit sealを除外してハッシュを計算する同じ問題と解法がある

PoW用の `Nonce` / `Difficulty` フィールドは、QBFTモードでは使わない（0のまま）。起動フラグ `--consensus pow|qbft` で切り替えられるようにしておくと、両方の動作を比較できて学習に良い。

### 3. QBFTメッセージ

```go
package qbft

type MsgType string

const (
    MsgPrePrepare  MsgType = "PRE-PREPARE"
    MsgPrepare     MsgType = "PREPARE"
    MsgCommit      MsgType = "COMMIT"
    MsgRoundChange MsgType = "ROUND-CHANGE"
)

type Message struct {
    Type       MsgType `json:"type"`
    Sequence   uint64  `json:"sequence"`  // 対象ブロック番号
    Round      uint64  `json:"round"`
    BlockHash  string  `json:"blockHash"` // PREPARE/COMMITの対象
    Block      *Block  `json:"block,omitempty"`      // PRE-PREPAREのみ
    CommitSeal string  `json:"commitSeal,omitempty"` // COMMITのみ: ブロックハッシュへの署名（BlockのCommitSealsに収録される）
    Sender     string  `json:"sender"`    // validatorアドレス
    Signature  string  `json:"signature"` // メッセージ署名（Signature以外の全フィールドへのEd25519署名）
}
```

メッセージ署名（Signature）とcommit seal（CommitSeal）は役割が違うので別フィールドにする。
前者は「このメッセージをこのvalidatorが送った」ことの証明で全メッセージに付き、
後者は「このブロックを確定に賛成した」ことの証明でブロックに永続化される。
1つのフィールドを使い回すと、メッセージ検証とsealの収集が混ざって混乱する。

- メッセージ自体に署名を付け、受信側は「senderが本物のvalidatorであること」「署名が正しいこと」を必ず検証する
- 伝播はMilestone 9のHTTP P2Pに `qbft_message` RPCを1つ足すだけでよい

### 4. コンセンサスの状態機械

```go
type QBFT struct {
    mu         sync.Mutex
    chain      *Chain
    p2p        *p2p.P2PManager
    validators []string
    selfAddr   string
    selfKey    ed25519.PrivateKey

    // 現在合意中の (sequence, round) の状態
    sequence uint64
    round    uint64
    proposal *Block                    // PRE-PREPAREで受けた提案
    prepares map[string]bool           // sender -> PREPARE受信済み
    commits  map[string]string         // sender -> commit seal（署名）

    // Step 3（round change）で使用
    roundChanges map[string]bool // sender -> ROUND-CHANGE受信済み
    timer        *time.Timer    // roundタイムアウト
}

func (q *QBFT) quorum() int {
    // n = 3f+1 に対して 2f+1。切り上げ計算: n=4 -> 3, n=7 -> 5
    n := len(q.validators)
    f := (n - 1) / 3
    return 2*f + 1
}
```

処理の流れ（happy path）:

```go
// proposerのみ: ブロックを提案する
func (q *QBFT) Propose() error {
    q.mu.Lock()
    defer q.mu.Unlock()

    if q.proposerFor(q.sequence, q.round) != q.selfAddr {
        return errors.New("not my turn to propose")
    }

    // Milestone 5と同じ手順でブロック候補を作る（tempStateに適用、成功txのみ）
    // ただしまだチェーンには追加せず、提案として保持する
    block, err := q.chain.BuildBlockProposal(q.sequence, q.round, q.selfAddr)
    if err != nil {
        return err
    }

    q.proposal = block
    q.prepares = map[string]bool{q.selfAddr: true} // 提案者は暗黙にPREPARE済み扱い

    q.broadcast(Message{
        Type: MsgPrePrepare, Sequence: q.sequence, Round: q.round,
        BlockHash: block.Hash, Block: block,
    })
    return nil
}

// 全validator: メッセージ受信時の処理
func (q *QBFT) HandleMessage(msg Message) error {
    q.mu.Lock()
    defer q.mu.Unlock()

    // 1. senderがvalidatorであること、メッセージ署名が正しいことを検証
    if err := q.verifyMessage(msg); err != nil {
        return err
    }

    // 2. sequence/roundが現在のものと一致すること（過去・未来のものは無視 or 保留）
    if msg.Sequence != q.sequence || msg.Round != q.round {
        return nil
    }

    switch msg.Type {
    case MsgPrePrepare:
        // 正しいproposerからの提案か
        if msg.Sender != q.proposerFor(msg.Sequence, msg.Round) {
            return errors.New("pre-prepare from wrong proposer")
        }
        // ブロック自体の検証（ハッシュ再計算・tx署名・parentHash。PoWチェックはしない）
        if err := q.chain.ValidateProposal(msg.Block); err != nil {
            return err
        }
        q.proposal = msg.Block
        q.prepares[q.selfAddr] = true
        q.broadcast(Message{Type: MsgPrepare, Sequence: q.sequence,
            Round: q.round, BlockHash: msg.Block.Hash})

    case MsgPrepare:
        if q.proposal == nil || msg.BlockHash != q.proposal.Hash {
            // 提案をまだ知らない場合は無視（簡易実装）。本物は保留バッファに積み、
            // PRE-PREPARE到着後に再処理する。無視方式はメッセージの到着順に
            // よっては票が足りず止まりうる（テストでは配送順を制御して回避する）
            return nil
        }
        q.prepares[msg.Sender] = true
        if len(q.prepares) >= q.quorum() && q.commits[q.selfAddr] == "" {
            seal := q.signBlockHash(q.proposal.Hash)
            q.commits[q.selfAddr] = seal
            q.broadcast(Message{Type: MsgCommit, Sequence: q.sequence,
                Round: q.round, BlockHash: q.proposal.Hash, CommitSeal: seal})
        }

    case MsgCommit:
        if q.proposal == nil || msg.BlockHash != q.proposal.Hash {
            return nil
        }
        q.commits[msg.Sender] = msg.CommitSeal
        if len(q.commits) >= q.quorum() {
            return q.finalize()
        }
    }
    return nil
}

// 2f+1のCOMMITが揃った: ブロックを確定してチェーンに追加
func (q *QBFT) finalize() error {
    block := *q.proposal
    for _, seal := range q.commits {
        block.CommitSeals = append(block.CommitSeals, seal)
    }

    // 状態遷移を反映してチェーンに追加（リオーグは起きないので追加のみ）
    if err := q.chain.CommitBlock(&block); err != nil {
        return err
    }

    // 次のsequenceへ
    q.sequence++
    q.round = 0
    q.proposal = nil
    q.prepares = map[string]bool{}
    q.commits = map[string]string{}
    return nil
}
```

注意（学習ポイント）:

- 自分がCOMMITを送る条件は「2f+1のPREPAREを見たこと」。PREPAREとCOMMITを混同しないこと
- `finalize` は全validatorが各自の判断で行う。「2f+1のCOMMITを見た」という条件が全員で同じ結論を生むのがBFTの肝
- COMMITの集まり方はノードごとに違う（どの2f+1が先に届くかは不定）ため、**CommitSealsの中身はノード間で完全一致しない**。だからこそCommitSealsをブロックハッシュから除外する必要がある

### 5. ブロック検証の差し替え

Milestone 10の `validateBlock` を、コンセンサスモードで分岐する:

```go
func (c *Chain) validateBlock(block Block) error {
    // 共通: ハッシュ再計算（CommitSeals除外）とtx署名の検証
    if calculateHashWithoutSeals(block) != block.Hash {
        return errors.New("invalid block hash")
    }
    for _, tx := range block.Transactions {
        if err := validateTransaction(tx); err != nil {
            return fmt.Errorf("invalid tx in block: %w", err)
        }
    }

    switch c.consensus {
    case "pow":
        if !pow.IsValidProof(block.Hash, block.Difficulty) {
            return errors.New("invalid proof of work")
        }
    case "qbft":
        // 2f+1の正当なvalidator署名（commit seal）があること
        valid := 0
        seen := map[string]bool{}
        for _, seal := range block.CommitSeals {
            signer, err := recoverSealSigner(block.Hash, seal)
            if err != nil || !c.isValidator(signer) || seen[signer] {
                continue // 不正・重複sealは数えない
            }
            seen[signer] = true
            valid++
        }
        if valid < c.qbftQuorum() {
            return fmt.Errorf("not enough commit seals: got %d, want %d", valid, c.qbftQuorum())
        }
        // proposerがそのsequence/roundの正当なproposerであること
        if block.Proposer != c.proposerFor(block.Number, block.Round) {
            return errors.New("invalid proposer")
        }
    }
    return nil
}
```

注意: Ed25519は署名から公開鍵を復元できない（secp256k1のecrecoverと違う）ため、`recoverSealSigner` は実装できない。代わりにsealを `{Validator, Signature}` のペアにするか、validatorアドレス→公開鍵の対応表をgenesisに持たせる。ここは実装時に選ぶこと（Milestone 14でsecp256k1に移行するとecrecover方式にできる、という伏線になる）。

### 6. RPCの追加

```go
// qbft_propose - proposerにブロック提案をさせる（最初は手動トリガーが学習しやすい）
case "qbft_propose":
    if err := c.qbft.Propose(); err != nil {
        return nil, rpcInvalidParams(err)
    }
    return map[string]any{"status": "proposed"}, nil

// qbft_message - validator間のメッセージ受信（P2P用。人間は叩かない）
case "qbft_message":
    var msg qbft.Message
    if err := parseSingleObjectParam(params, &msg); err != nil {
        return nil, rpcInvalidParams(err)
    }
    if err := c.qbft.HandleMessage(msg); err != nil {
        return nil, rpcInvalidParams(err)
    }
    return map[string]any{"status": "ok"}, nil

// qbft_getValidators - validator一覧
case "qbft_getValidators":
    return map[string]any{"validators": c.qbft.Validators()}, nil

// qbft_getState - 現在の合意状態（デバッグ用に非常に役立つ）
case "qbft_getState":
    return map[string]any{
        "sequence": toHex(c.qbft.Sequence()),
        "round":    toHex(c.qbft.Round()),
        "proposer": c.qbft.CurrentProposer(),
        "prepares": c.qbft.PrepareCount(),
        "commits":  c.qbft.CommitCount(),
    }, nil
```

`mineBlock` はQBFTモードでは無効化する（呼ばれたらエラーを返す）。

### 7. Round change（Step 3）

```go
// PRE-PREPARE待ち・合意進行にタイムアウトを設定する
func (q *QBFT) startRoundTimer() {
    timeout := time.Duration(5<<q.round) * time.Second // roundごとに指数的に延ばす
    q.timer = time.AfterFunc(timeout, q.onRoundTimeout)
}

func (q *QBFT) onRoundTimeout() {
    q.mu.Lock()
    defer q.mu.Unlock()

    // 現在のroundを諦めて ROUND-CHANGE を送る
    q.roundChanges[q.selfAddr] = true
    q.broadcast(Message{Type: MsgRoundChange, Sequence: q.sequence, Round: q.round + 1})
}

// ROUND-CHANGEを2f+1集めたら新しいroundへ移行
// 新roundのproposerは proposerFor(sequence, round+1) に変わる
```

簡略化ポイント（本物との差分として重要）:

- 本物のQBFTでは、round changeメッセージに「自分がすでにPREPAREしたブロック」の証明（prepared certificate / justification）を載せ、新proposerはそれを引き継いで提案する。これがないと「一部のノードだけがCOMMIT直前まで進んでいた」場合に安全性が壊れうる
- この実装ではprepared certificateを省略し、新roundでは新しいブロックを提案し直す。**「合意直前で分断が起きた場合に、確定済みと未確定の境界が曖昧になる」という理論上の穴がある**ことを理解した上で簡略化する（happy path＋単純なproposer停止の学習には十分）

## テストケース

### 1. proposer選出が決定的
```go
func TestProposerSelection(t *testing.T) {
    q := newTestQBFT(t, 4) // validator 4つ

    p1 := q.proposerFor(1, 0)
    p2 := q.proposerFor(2, 0)
    p1r1 := q.proposerFor(1, 1)

    // 決定的（何度呼んでも同じ）
    assert.Equal(t, p1, q.proposerFor(1, 0))
    // sequenceが変わればproposerが順に回る
    assert.NotEqual(t, p1, p2)
    // roundが変われば次のproposerになる
    assert.Equal(t, p2, p1r1)
}
```

### 2. Quorum計算
```go
func TestQuorum(t *testing.T) {
    assert.Equal(t, 3, quorumOf(4)) // f=1
    assert.Equal(t, 5, quorumOf(7)) // f=2
    assert.Equal(t, 7, quorumOf(10)) // f=3
}
```

### 3. Happy path（メッセージ配送をモックして1プロセスで4validator動かす）
```go
func TestQBFTHappyPath(t *testing.T) {
    net := newFakeNetwork(t, 4) // メッセージを直接配送するテスト用ネットワーク

    // proposerが提案
    require.NoError(t, net.Proposer(1, 0).Propose())
    net.DeliverAll() // 全メッセージを配送しきる

    // 全validatorのチェーンにブロック1が確定している
    for _, node := range net.Nodes() {
        assert.Equal(t, uint64(1), node.Chain.LatestBlockNumber())
    }
}
```

### 4. Quorum未達では確定しない
```go
func TestNoQuorumNoFinality(t *testing.T) {
    net := newFakeNetwork(t, 4)
    net.Stop(2) // 4台中2台停止（f=1を超える）

    net.Proposer(1, 0).Propose()
    net.DeliverAll()

    // 2f+1=3票が集まらないため、どのノードもブロックを確定しない
    for _, node := range net.Alive() {
        assert.Equal(t, uint64(0), node.Chain.LatestBlockNumber())
    }
}
```

### 5. f台停止なら進む
```go
func TestToleratesFFailures(t *testing.T) {
    net := newFakeNetwork(t, 4)
    net.Stop(1) // f=1台だけ停止（proposer以外）

    net.Proposer(1, 0).Propose()
    net.DeliverAll()

    for _, node := range net.Alive() {
        assert.Equal(t, uint64(1), node.Chain.LatestBlockNumber())
    }
}
```

### 6. 不正なproposerの提案は拒否
```go
func TestRejectWrongProposer(t *testing.T) {
    net := newFakeNetwork(t, 4)

    // 順番でないvalidatorがPRE-PREPAREを送る
    wrong := net.NotProposer(1, 0)
    err := wrong.Propose()
    assert.Error(t, err) // 自ノードで拒否される

    // 無理やりメッセージを作って送っても受信側で拒否される
    msg := wrong.ForgePrePrepare(1, 0)
    err = net.Nodes()[0].HandleMessage(msg)
    assert.Error(t, err)
}
```

### 7. Commit sealが足りないブロックは拒否
```go
func TestRejectInsufficientSeals(t *testing.T) {
    chain := setupTestQBFTChain(t, 4)

    block := buildValidBlock(t, chain)
    block.CommitSeals = block.CommitSeals[:2] // 3必要なのに2しかない

    err := chain.validateBlock(block)
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "not enough commit seals")
}
```

### 8. Round change（Step 3実装後）
```go
func TestRoundChange(t *testing.T) {
    net := newFakeNetwork(t, 4)
    net.Stop(net.ProposerIndex(1, 0)) // sequence 1のproposerを停止

    net.TriggerTimeouts() // 全ノードのタイムアウトを発火
    net.DeliverAll()

    // round 1の新proposerが提案してブロックが確定する
    net.Proposer(1, 1).Propose()
    net.DeliverAll()

    for _, node := range net.Alive() {
        assert.Equal(t, uint64(1), node.Chain.LatestBlockNumber())
        assert.Equal(t, uint64(1), node.Chain.LatestBlock().Round)
    }
}
```

## 実装手順

1. **qbftパッケージを作成**
   - Message型、署名・検証、proposer選出、quorum計算（ここまでは純粋関数でテストしやすい）

2. **Block構造体を変更**
   - Round / Proposer / CommitSeals追加
   - CommitSeals除外のハッシュ計算

3. **状態機械を実装（happy path）**
   - Propose / HandleMessage / finalize
   - テスト用のfakeネットワークで1プロセス検証

4. **P2Pに接続**
   - `qbft_message` RPC追加、broadcastをMilestone 9の仕組みに載せる

5. **ブロック検証を差し替え**
   - `--consensus` フラグ、commit seal検証

6. **Round changeを実装**
   - タイムアウト、ROUND-CHANGEメッセージ

7. **テストを実装**
   - 上記テストケース

## 検証方法

### 手動テスト（4ノード）
```bash
# validator 4つ分のアカウントを作成し、genesis.jsonのqbft.validatorsに登録

# ターミナル1〜4でそれぞれ起動
go run ./cmd/minieth node --genesis genesis.json --datadir ./data1 --addr :8545 --consensus qbft --nodekey 0x<v1の秘密鍵>
go run ./cmd/minieth node --genesis genesis.json --datadir ./data2 --addr :8546 --consensus qbft --nodekey 0x<v2の秘密鍵>
go run ./cmd/minieth node --genesis genesis.json --datadir ./data3 --addr :8547 --consensus qbft --nodekey 0x<v3の秘密鍵>
go run ./cmd/minieth node --genesis genesis.json --datadir ./data4 --addr :8548 --consensus qbft --nodekey 0x<v4の秘密鍵>

# 全ノードを相互にaddPeer（Milestone 9）

# txを投入
curl -s -X POST localhost:8545 -d '{"jsonrpc":"2.0","method":"sendTransaction","params":[tx],"id":1}'

# 現在のproposerを確認
curl -s -X POST localhost:8545 -d '{"jsonrpc":"2.0","method":"qbft_getState","params":[],"id":2}'

# proposerのノードで提案をトリガー
curl -s -X POST localhost:<proposerのport> -d '{"jsonrpc":"2.0","method":"qbft_propose","params":[],"id":3}'

# 全ノードでblockNumberが1になり、ブロックが完全に一致することを確認
curl -s -X POST localhost:8545 -d '{"jsonrpc":"2.0","method":"eth_getBlockByNumber","params":["latest",true],"id":4}'
curl -s -X POST localhost:8548 -d '{"jsonrpc":"2.0","method":"eth_getBlockByNumber","params":["latest",true],"id":5}'

# 1台止めて（f=1）まだ進むこと、2台止めると止まることを確認
```

### 自動テスト
```bash
go test ./internal/qbft
go test ./internal/chain -run TestQBFT
```

## 完了条件
- 4ノードでtxがブロックとして確定し、全ノードのチェーンが一致する
- 確定したブロックに2f+1のcommit sealが付いている
- commit sealが足りないブロック・不正なproposerのブロックが拒否される
- 1台停止しても合意が進み、2台停止すると止まる（f=1の体感）
- proposer停止時にround changeで次のproposerに切り替わる（Step 3）
- PoWモード（`--consensus pow`）の既存機能が壊れていない

## PoWとの対比（このマイルストーンで言語化しておくこと）

| 項目 | PoW + 最長チェーン | QBFT |
|------|--------------------|------|
| ファイナリティ | 確率的（後で覆りうる） | 即時（確定したら覆らない） |
| フォーク | 起きる（fork choice必須） | 起きない（リオーグ不要） |
| 参加者 | 誰でも（permissionless） | 既知のvalidatorのみ（permissioned） |
| 耐障害性 | 51%攻撃 | n≧3f+1（1/3未満の故障・悪意に耐える） |
| ブロック生成コスト | 計算量（電力） | メッセージ交換（O(n²)通信） |
| ネットワーク分断時 | 両側で伸び続け、後で片方破棄 | quorum側だけ進む（安全性優先で停止しうる） |

## 次のステップ
QBFTが実装できたら、Milestone 12でGas風の仕組みを実装します。QBFTでは「マイナー」がいないため、手数料の受け取り先は `block.Proposer` を使います（Milestone 12のcoinbaseをproposerで置き換えて読むこと）。

## 注意点
この実装は本物のQBFT（Besu / GoQuorumのQBFT、およびその元になったIstanbul BFT）とは異なります：

- 本物: メッセージはRLPエンコード＋secp256k1署名、validator情報とcommit sealはブロックヘッダのextraDataに格納
- この実装: JSON＋Ed25519、専用フィールド（Proposer / CommitSeals）
- 本物: round changeにprepared certificate（すでにPREPARE済みのブロックの証明）を含め、合意直前のブロックを新roundに引き継ぐ
- この実装: prepared certificateを省略（round changeを跨ぐ安全性は不完全。学習用の簡略化）
- 本物: validatorの追加・削除をブロックヘッダ経由の投票で行う
- この実装: genesisで固定

あくまでBFT合意と即時ファイナリティの基本概念を学ぶための実装です。
