# Milestone 2: コード分割

## 概要
現在のフラットなファイル構成から、Goの標準的なパッケージ構成へ移行します。これにより、コードの保守性と拡張性を向上させます。

## 現在の構成
```
mini-eth-node/
  go.mod
  main.go
  model.go
  rpc.go
  transaction.go
  command.go
  utils.go
  genesis.json
  README.md
```

## 目標構成
```
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
  go.mod
  genesis.json
  README.md
```

## 分割計画

### 1. accountパッケージ
**担当ファイル**: `account.go`
- `Account` 構造体
- `addressFromPubkey` 関数
- `normalizeAddress` 関数

### 2. blockパッケージ  
**担当ファイル**: `block.go`
- `Block` 構造体
- `blockHash` 関数
- `toRPCBlock` 関数

### 3. chainパッケージ
**担当ファイル**: `chain.go`
- `Chain` 構造体
- `NewChainFromGenesis` 関数
- `addTransaction` メソッド
- `computeStateRootLocked` メソッド

### 4. cryptoパッケージ
**担当ファイル**: `crypto.go`
- 署名関連関数
- ハッシュ関連関数
- `txSignBytes`, `txHash`
- `hashJSON`

### 5. rpcパッケージ
**担当ファイル**: `server.go`
- `rpcRequest`, `rpcResponse`, `rpcError` 構造体
- `handleRPC` メソッド
- `dispatch` メソッド
- パラメータ解析関数群

### 6. stateパッケージ
**担当ファイル**: `state.go`
- 状態遷移ロジック
- 残高チェック
- nonce更新ロジック

### 7. txパッケージ
**担当ファイル**: `transaction.go`
- `Transaction` 構造体
- `verfyAndNormalizeTx` 関数
- トランザクション関連ユーティリティ

### 8. cmd/minieth
**担当ファイル**: `main.go`
- CLIコマンド処理
- `cmdAccount`, `cmdSign`, `cmdNode` 関数
- `usage` 関数

## 実装手順

### Phase 1: パッケージ作成
1. ディレクトリ構造を作成
2. 各パッケージにファイルを作成
3. 適切なimport文を追加

### Phase 2: 機能移動
1. `account` パッケージから開始
2. 依存関係の少ないパッケージから順に移動
3. 移動後にコンパイルエラーを修正

### Phase 3: import調整
1. 循環参照を避ける
2. public/privateを適切に設定
3. 依存関係を整理

### Phase 4: テスト
1. 既存のREADME手順が動作することを確認
2. `go build` が成功することを確認
3. `go test ./...` が成功することを確認

## 注意点

### 循環参照の回避
- `chain` パッケージが他の多くのパッケージに依存する可能性
- インターフェースを適切に定義して依存を逆転させる

### Public/Privateの設定
- パッケージ外から使用するものはpublic（大文字）
- パッケージ内でのみ使用するものはprivate（小文字）

### 依存関係
```
cmd/minieth
  ↓
rpc
  ↓
chain
  ↓
tx, block, state, account, crypto
```

## 検証方法

### 1. ビルド確認
```bash
go build ./cmd/minieth
```

### 2. 実行確認
```bash
# 既存のコマンドが動作することを確認
go run ./cmd/minieth account
go run ./cmd/minieth sign --priv ...
go run ./cmd/minieth node --genesis genesis.json --addr :8545
```

### 3. テスト確認
```bash
go test ./...
```

### 4. README手順確認
READMEのクイックスタート手順がすべて動作することを確認

## 完了条件
- 新しいパッケージ構成でビルドが成功する
- 既存のすべての機能が変わらず動作する
- `go test ./...` が成功する
- READMEの手順がそのまま通る
- コードの見通しが良くなっていること