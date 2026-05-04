# Milestone 12: Contract風機能

## 概要
EVMをいきなり実装するのは複雑すぎるため、まずは簡易的なコントラクト機能を実装して、スマートコントラクトの基本概念を学びます。EVMの代わりに、Goのインターフェースベースでコントラクトを定義します。

## 目的
- 通常アカウントとコントラクトアカウントの違いを学ぶ
- storageの基本概念を理解する
- callとtransactionの違いを学ぶ
- 状態を変更する呼び出しを理解する

## 設計方針
EVMの代わりにGoのインターフェースを使用：

```go
type Contract interface {
    Call(state *State, input []byte) ([]byte, error)
    GetStorage(key string) (string, error)
    SetStorage(key, value string) error
}
```

## 実装するコントラクト
1. **Counter** - 簡単なカウンター
2. **KeyValueStore** - 簡易ストレージ
3. **SimpleToken** - ERC20風トークン

## 実装計画

### 1. コントラクトアカウント構造
```go
package contract

import (
    "crypto/sha256"
    "encoding/hex"
    "encoding/json"
    "errors"
    "fmt"
    "strconv"
    "sync"
)

type ContractAccount struct {
    Address string
    Balance uint64
    Nonce   uint64
    Code    []byte          // コントラクトコード（シリアライズされた型名）
    Storage map[string]string // 簡易ストレージ
}

type Contract interface {
    Call(state *State, input []byte) ([]byte, error)
    GetStorage(key string) (string, error)
    SetStorage(key, value string) error
}

type State struct {
    Accounts    map[string]Account
    Contracts   map[string]ContractAccount
    mu          sync.RWMutex
}

// コントラクトファクトリ
var contractRegistry = map[string]func() Contract{
    "Counter":      NewCounter,
    "KeyValueStore": NewKeyValueStore,
    "SimpleToken":  NewSimpleToken,
}

func DeployContract(contractType string, state *State) (string, error) {
    constructor, exists := contractRegistry[contractType]
    if !exists {
        return "", errors.New("unknown contract type")
    }
    
    // 新しいコントラクトアカウントを作成
    address := generateContractAddress()
    
    contract := ContractAccount{
        Address: address,
        Balance: 0,
        Nonce:   0,
        Code:    []byte(contractType),
        Storage: make(map[string]string),
    }
    
    state.mu.Lock()
    state.Contracts[address] = contract
    state.mu.Unlock()
    
    return address, nil
}

func GetContract(state *State, address string) (Contract, error) {
    state.mu.RLock()
    defer state.mu.RUnlock()
    
    contract, exists := state.Contracts[address]
    if !exists {
        return nil, errors.New("contract not found")
    }
    
    constructor, exists := contractRegistry[string(contract.Code)]
    if !exists {
        return nil, errors.New("contract type not registered")
    }
    
    return constructor(), nil
}

func generateContractAddress() string {
    // 簡易的なアドレス生成（実際はもっと複雑）
    random := fmt.Sprintf("%d", time.Now().UnixNano())
    hash := sha256.Sum256([]byte(random))
    return "0x" + hex.EncodeToString(hash[:20])
}

// コントラクト呼び出しのヘルパー関数
func CallContract(state *State, from, to string, input []byte, isTransaction bool) ([]byte, error) {
    contract, err := GetContract(state, to)
    if err != nil {
        return nil, err
    }
    
    // コントラクトの状態を設定（リフレクションまたは埋め込み）
    if c, ok := contract.(interface{ setState(*State) }); ok {
        c.setState(state)
    }
    
    result, err := contract.Call(state, input)
    if err != nil {
        return nil, err
    }
    
    // トランザクションの場合はnonceを増やす
    if isTransaction {
        account := state.Accounts[from]
        account.Nonce++
        state.Accounts[from] = account
    }
    
    return result, nil
}
```

### 2. Counterコントラクトの完全な実装
```go
type Counter struct {
    state *State
    addr  string
}

func NewCounter() Contract {
    return &Counter{}
}

func (c *Counter) setState(state *State) {
    c.state = state
}

func (c *Counter) Call(state *State, input []byte) ([]byte, error) {
    c.state = state
    
    // inputをパースしてメソッドを特定
    var callData struct {
        Method string `json:"method"`
        Value  uint64 `json:"value,omitempty"`
    }
    
    if err := json.Unmarshal(input, &callData); err != nil {
        return nil, err
    }
    
    switch callData.Method {
    case "get":
        return c.get()
    case "increment":
        return c.increment(callData.Value)
    case "decrement":
        return c.decrement(callData.Value)
    case "reset":
        return c.reset()
    default:
        return nil, errors.New("unknown method")
    }
}

func (c *Counter) GetStorage(key string) (string, error) {
    if c.state == nil {
        return "", errors.New("state not set")
    }
    
    c.state.mu.RLock()
    defer c.state.mu.RUnlock()
    
    contract := c.state.Contracts[c.addr]
    return contract.Storage[key], nil
}

func (c *Counter) SetStorage(key, value string) error {
    if c.state == nil {
        return errors.New("state not set")
    }
    
    c.state.mu.Lock()
    defer c.state.mu.Unlock()
    
    contract := c.state.Contracts[c.addr]
    contract.Storage[key] = value
    c.state.Contracts[c.addr] = contract
    return nil
}

func (c *Counter) get() ([]byte, error) {
    countStr, err := c.GetStorage("count")
    if err != nil {
        return nil, err
    }
    
    var count uint64
    if countStr != "" {
        count, _ = strconv.ParseUint(countStr, 10, 64)
    }
    
    result := map[string]any{
        "count": count,
        "method": "get",
    }
    return json.Marshal(result)
}

func (c *Counter) increment(value uint64) ([]byte, error) {
    countStr, err := c.GetStorage("count")
    if err != nil {
        return nil, err
    }
    
    var count uint64
    if countStr != "" {
        count, _ = strconv.ParseUint(countStr, 10, 64)
    }
    
    count += value
    if err := c.SetStorage("count", strconv.FormatUint(count, 10)); err != nil {
        return nil, err
    }
    
    // 履歴を保存（オプション）
    history := fmt.Sprintf("%d->%d", count-value, count)
    c.SetStorage("last_increment", history)
    
    result := map[string]any{
        "count": count,
        "incremented": value,
        "method": "increment",
    }
    return json.Marshal(result)
}

func (c *Counter) decrement(value uint64) ([]byte, error) {
    countStr, err := c.GetStorage("count")
    if err != nil {
        return nil, err
    }
    
    var count uint64
    if countStr != "" {
        count, _ = strconv.ParseUint(countStr, 10, 64)
    }
    
    if count < value {
        return nil, errors.New("underflow: cannot decrement below zero")
    }
    
    count -= value
    if err := c.SetStorage("count", strconv.FormatUint(count, 10)); err != nil {
        return nil, err
    }
    
    result := map[string]any{
        "count": count,
        "decremented": value,
        "method": "decrement",
    }
    return json.Marshal(result)
}

func (c *Counter) reset() ([]byte, error) {
    if err := c.SetStorage("count", "0"); err != nil {
        return nil, err
    }
    
    result := map[string]any{
        "count": 0,
        "method": "reset",
    }
    return json.Marshal(result)
}
```

### 3. SimpleTokenコントラクトの完全な実装
```go
type SimpleToken struct {
    state *State
    addr  string
}

func NewSimpleToken() Contract {
    return &SimpleToken{}
}

func (st *SimpleToken) setState(state *State) {
    st.state = state
}

func (st *SimpleToken) Call(state *State, input []byte) ([]byte, error) {
    st.state = state
    
    var callData struct {
        Method     string `json:"method"`
        From       string `json:"from,omitempty"`
        To         string `json:"to,omitempty"`
        Amount     uint64 `json:"amount,omitempty"`
        Name       string `json:"name,omitempty"`
        Symbol     string `json:"symbol,omitempty"`
        TotalSupply uint64 `json:"totalSupply,omitempty"`
    }
    
    if err := json.Unmarshal(input, &callData); err != nil {
        return nil, err
    }
    
    switch callData.Method {
    case "init":
        return st.init(callData.Name, callData.Symbol, callData.TotalSupply)
    case "balanceOf":
        return st.balanceOf(callData.From)
    case "transfer":
        return st.transfer(callData.From, callData.To, callData.Amount)
    case "approve":
        return st.approve(callData.From, callData.To, callData.Amount)
    case "totalSupply":
        return st.totalSupply()
    case "allowance":
        return st.allowance(callData.From, callData.To)
    default:
        return nil, errors.New("unknown method")
    }
}

func (st *SimpleToken) GetStorage(key string) (string, error) {
    st.state.mu.RLock()
    defer st.state.mu.RUnlock()
    
    contract := st.state.Contracts[st.addr]
    return contract.Storage[key], nil
}

func (st *SimpleToken) SetStorage(key, value string) error {
    st.state.mu.Lock()
    defer st.state.mu.Unlock()
    
    contract := st.state.Contracts[st.addr]
    contract.Storage[key] = value
    st.state.Contracts[st.addr] = contract
    return nil
}

func (st *SimpleToken) init(name, symbol string, totalSupply uint64) ([]byte, error) {
    if err := st.SetStorage("name", name); err != nil {
        return nil, err
    }
    
    if err := st.SetStorage("symbol", symbol); err != nil {
        return nil, err
    }
    
    if err := st.SetStorage("totalSupply", strconv.FormatUint(totalSupply, 10)); err != nil {
        return nil, err
    }
    
    // デプロイアドレスに全供給量を割り当て
    deployerAddr := "0xdeployer" // 実際はデプロイしたアドレス
    if err := st.SetStorage("balance:"+deployerAddr, strconv.FormatUint(totalSupply, 10)); err != nil {
        return nil, err
    }
    
    result := map[string]any{
        "name": name,
        "symbol": symbol,
        "totalSupply": totalSupply,
        "deployer": deployerAddr,
        "method": "init",
    }
    return json.Marshal(result)
}

func (st *SimpleToken) balanceOf(account string) ([]byte, error) {
    balanceStr, err := st.GetStorage("balance:" + account)
    if err != nil {
        return nil, err
    }
    
    var balance uint64
    if balanceStr != "" {
        balance, _ = strconv.ParseUint(balanceStr, 10, 64)
    }
    
    result := map[string]any{
        "account": account,
        "balance": balance,
        "method": "balanceOf",
    }
    return json.Marshal(result)
}

func (st *SimpleToken) transfer(from, to string, amount uint64) ([]byte, error) {
    // fromの残高を取得
    fromBalanceStr, err := st.GetStorage("balance:" + from)
    if err != nil {
        return nil, err
    }
    
    var fromBalance uint64
    if fromBalanceStr != "" {
        fromBalance, _ = strconv.ParseUint(fromBalanceStr, 10, 64)
    }
    
    if fromBalance < amount {
        return nil, errors.New("insufficient balance")
    }
    
    // 残高を更新
    fromBalance -= amount
    if err := st.SetStorage("balance:" + from, strconv.FormatUint(fromBalance, 10)); err != nil {
        return nil, err
    }
    
    // toの残高を取得・更新
    toBalanceStr, err := st.GetStorage("balance:" + to)
    if err != nil {
        return nil, err
    }
    
    var toBalance uint64
    if toBalanceStr != "" {
        toBalance, _ = strconv.ParseUint(toBalanceStr, 10, 64)
    }
    
    toBalance += amount
    if err := st.SetStorage("balance:" + to, strconv.FormatUint(toBalance, 10)); err != nil {
        return nil, err
    }
    
    // 転送イベントを記録（簡易版）
    event := fmt.Sprintf("%s->%s:%d", from, to, amount)
    st.SetStorage("last_transfer", event)
    
    result := map[string]any{
        "from": from,
        "to": to,
        "amount": amount,
        "fromBalance": fromBalance,
        "toBalance": toBalance,
        "method": "transfer",
    }
    return json.Marshal(result)
}

func (st *SimpleToken) totalSupply() ([]byte, error) {
    totalStr, err := st.GetStorage("totalSupply")
    if err != nil {
        return nil, err
    }
    
    var total uint64
    if totalStr != "" {
        total, _ = strconv.ParseUint(totalStr, 10, 64)
    }
    
    result := map[string]any{
        "totalSupply": total,
        "method": "totalSupply",
    }
    return json.Marshal(result)
}
```

### 2. Counterコントラクト
```go
type Counter struct {
    state *State
    addr  string
}

func NewCounter() Contract {
    return &Counter{}
}

func (c *Counter) Call(state *State, input []byte) ([]byte, error) {
    c.state = state
    
    // inputをパースしてメソッドを特定
    var callData struct {
        Method string `json:"method"`
        Value  uint64 `json:"value,omitempty"`
    }
    
    if err := json.Unmarshal(input, &callData); err != nil {
        return nil, err
    }
    
    switch callData.Method {
    case "get":
        return c.get()
    case "increment":
        return c.increment(callData.Value)
    case "decrement":
        return c.decrement(callData.Value)
    default:
        return nil, errors.New("unknown method")
    }
}

func (c *Counter) GetStorage(key string) (string, error) {
    c.state.mu.RLock()
    defer c.state.mu.RUnlock()
    
    contract := c.state.Contracts[c.addr]
    return contract.Storage[key], nil
}

func (c *Counter) SetStorage(key, value string) error {
    c.state.mu.Lock()
    defer c.state.mu.Unlock()
    
    contract := c.state.Contracts[c.addr]
    contract.Storage[key] = value
    c.state.Contracts[c.addr] = contract
    return nil
}

func (c *Counter) get() ([]byte, error) {
    countStr, err := c.GetStorage("count")
    if err != nil {
        return nil, err
    }
    
    var count uint64
    if countStr != "" {
        count, _ = strconv.ParseUint(countStr, 10, 64)
    }
    
    result := map[string]any{"count": count}
    return json.Marshal(result)
}

func (c *Counter) increment(value uint64) ([]byte, error) {
    countStr, err := c.GetStorage("count")
    if err != nil {
        return nil, err
    }
    
    var count uint64
    if countStr != "" {
        count, _ = strconv.ParseUint(countStr, 10, 64)
    }
    
    count += value
    if err := c.SetStorage("count", strconv.FormatUint(count, 10)); err != nil {
        return nil, err
    }
    
    result := map[string]any{"count": count}
    return json.Marshal(result)
}

func (c *Counter) decrement(value uint64) ([]byte, error) {
    countStr, err := c.GetStorage("count")
    if err != nil {
        return nil, err
    }
    
    var count uint64
    if countStr != "" {
        count, _ = strconv.ParseUint(countStr, 10, 64)
    }
    
    if count < value {
        return nil, errors.New("underflow")
    }
    
    count -= value
    if err := c.SetStorage("count", strconv.FormatUint(count, 10)); err != nil {
        return nil, err
    }
    
    result := map[string]any{"count": count}
    return json.Marshal(result)
}
```

### 3. KeyValueStoreコントラクト
```go
type KeyValueStore struct {
    state *State
    addr  string
}

func NewKeyValueStore() Contract {
    return &KeyValueStore{}
}

func (kvs *KeyValueStore) Call(state *State, input []byte) ([]byte, error) {
    kvs.state = state
    
    var callData struct {
        Method string `json:"method"`
        Key    string `json:"key"`
        Value  string `json:"value,omitempty"`
    }
    
    if err := json.Unmarshal(input, &callData); err != nil {
        return nil, err
    }
    
    switch callData.Method {
    case "get":
        return kvs.get(callData.Key)
    case "set":
        return kvs.set(callData.Key, callData.Value)
    case "keys":
        return kvs.keys()
    default:
        return nil, errors.New("unknown method")
    }
}

func (kvs *KeyValueStore) GetStorage(key string) (string, error) {
    kvs.state.mu.RLock()
    defer kvs.state.mu.RUnlock()
    
    contract := kvs.state.Contracts[kvs.addr]
    return contract.Storage[key], nil
}

func (kvs *KeyValueStore) SetStorage(key, value string) error {
    kvs.state.mu.Lock()
    defer kvs.state.mu.Unlock()
    
    contract := kvs.state.Contracts[kvs.addr]
    contract.Storage[key] = value
    kvs.state.Contracts[kvs.addr] = contract
    return nil
}

func (kvs *KeyValueStore) get(key string) ([]byte, error) {
    value, err := kvs.GetStorage(key)
    if err != nil {
        return nil, err
    }
    
    result := map[string]any{"key": key, "value": value}
    return json.Marshal(result)
}

func (kvs *KeyValueStore) set(key, value string) ([]byte, error) {
    if err := kvs.SetStorage(key, value); err != nil {
        return nil, err
    }
    
    result := map[string]any{"key": key, "value": value, "status": "set"}
    return json.Marshal(result)
}

func (kvs *KeyValueStore) keys() ([]byte, error) {
    kvs.state.mu.RLock()
    defer kvs.state.mu.RUnlock()
    
    contract := kvs.state.Contracts[kvs.addr]
    var keys []string
    for key := range contract.Storage {
        keys = append(keys, key)
    }
    
    result := map[string]any{"keys": keys}
    return json.Marshal(result)
}
```

### 4. SimpleTokenコントラクト
```go
type SimpleToken struct {
    state *State
    addr  string
}

func NewSimpleToken() Contract {
    return &SimpleToken{}
}

func (st *SimpleToken) Call(state *State, input []byte) ([]byte, error) {
    st.state = state
    
    var callData struct {
        Method   string `json:"method"`
        From     string `json:"from,omitempty"`
        To       string `json:"to,omitempty"`
        Amount   uint64 `json:"amount,omitempty"`
        Name     string `json:"name,omitempty"`
        Symbol   string `json:"symbol,omitempty"`
        TotalSupply uint64 `json:"totalSupply,omitempty"`
    }
    
    if err := json.Unmarshal(input, &callData); err != nil {
        return nil, err
    }
    
    switch callData.Method {
    case "init":
        return st.init(callData.Name, callData.Symbol, callData.TotalSupply)
    case "balanceOf":
        return st.balanceOf(callData.From)
    case "transfer":
        return st.transfer(callData.From, callData.To, callData.Amount)
    case "totalSupply":
        return st.totalSupply()
    default:
        return nil, errors.New("unknown method")
    }
}

func (st *SimpleToken) GetStorage(key string) (string, error) {
    st.state.mu.RLock()
    defer st.state.mu.RUnlock()
    
    contract := st.state.Contracts[st.addr]
    return contract.Storage[key], nil
}

func (st *SimpleToken) SetStorage(key, value string) error {
    st.state.mu.Lock()
    defer st.state.mu.Unlock()
    
    contract := st.state.Contracts[st.addr]
    contract.Storage[key] = value
    st.state.Contracts[st.addr] = contract
    return nil
}

func (st *SimpleToken) init(name, symbol string, totalSupply uint64) ([]byte, error) {
    if err := st.SetStorage("name", name); err != nil {
        return nil, err
    }
    
    if err := st.SetStorage("symbol", symbol); err != nil {
        return nil, err
    }
    
    if err := st.SetStorage("totalSupply", strconv.FormatUint(totalSupply, 10)); err != nil {
        return nil, err
    }
    
    result := map[string]any{
        "name": name,
        "symbol": symbol,
        "totalSupply": totalSupply,
    }
    return json.Marshal(result)
}

func (st *SimpleToken) balanceOf(account string) ([]byte, error) {
    balanceStr, err := st.GetStorage("balance:" + account)
    if err != nil {
        return nil, err
    }
    
    var balance uint64
    if balanceStr != "" {
        balance, _ = strconv.ParseUint(balanceStr, 10, 64)
    }
    
    result := map[string]any{"account": account, "balance": balance}
    return json.Marshal(result)
}

func (st *SimpleToken) transfer(from, to string, amount uint64) ([]byte, error) {
    // fromの残高を取得
    fromBalanceStr, err := st.GetStorage("balance:" + from)
    if err != nil {
        return nil, err
    }
    
    var fromBalance uint64
    if fromBalanceStr != "" {
        fromBalance, _ = strconv.ParseUint(fromBalanceStr, 10, 64)
    }
    
    if fromBalance < amount {
        return nil, errors.New("insufficient balance")
    }
    
    // 残高を更新
    fromBalance -= amount
    if err := st.SetStorage("balance:" + from, strconv.FormatUint(fromBalance, 10)); err != nil {
        return nil, err
    }
    
    // toの残高を取得・更新
    toBalanceStr, err := st.GetStorage("balance:" + to)
    if err != nil {
        return nil, err
    }
    
    var toBalance uint64
    if toBalanceStr != "" {
        toBalance, _ = strconv.ParseUint(toBalanceStr, 10, 64)
    }
    
    toBalance += amount
    if err := st.SetStorage("balance:" + to, strconv.FormatUint(toBalance, 10)); err != nil {
        return nil, err
    }
    
    result := map[string]any{
        "from": from,
        "to": to,
        "amount": amount,
        "fromBalance": fromBalance,
        "toBalance": toBalance,
    }
    return json.Marshal(result)
}
```

### 5. コントラクト呼び出しの実装
```go
func (c *Chain) callContract(from, to string, input []byte, isTransaction bool) ([]byte, error) {
    c.mu.Lock()
    defer c.mu.Unlock()
    
    // コントラクトを取得
    contract, err := contract.GetContract(c.State, to)
    if err != nil {
        return nil, err
    }
    
    // コントラクトのアドレスを設定
    //（リフレクションを使ってaddrフィールドを設定）
    
    // コントラクトを呼び出し
    result, err := contract.Call(c.State, input)
    if err != nil {
        return nil, err
    }
    
    // トランザクションの場合はnonceを増やす
    if isTransaction {
        fromAccount := c.State[from]
        fromAccount.Nonce++
        c.State[from] = fromAccount
    }
    
    return result, nil
}

// RPCの追加
case "call":
    var params struct {
        From  string `json:"from"`
        To    string `json:"to"`
        Data  string `json:"data"`
    }
    
    if err := parseSingleObjectParam(req.Params, &params); err != nil {
        return nil, rpcInvalidParams(err)
    }
    
    input, err := hex.DecodeString(strings.TrimPrefix(params.Data, "0x"))
    if err != nil {
        return nil, rpcInvalidParams(err)
    }
    
    result, err := c.callContract(params.From, params.To, input, false)
    if err != nil {
        return nil, rpcInvalidParams(err)
    }
    
    return map[string]any{
        "result": "0x" + hex.EncodeToString(result),
    }, nil

case "deployContract":
    var params struct {
        From        string `json:"from"`
        ContractType string `json:"contractType"`
    }
    
    if err := parseSingleObjectParam(req.Params, &params); err != nil {
        return nil, rpcInvalidParams(err)
    }
    
    address, err := contract.DeployContract(params.ContractType, c.State)
    if err != nil {
        return nil, rpcInvalidParams(err)
    }
    
    // nonceを増やす
    fromAccount := c.State[params.From]
    fromAccount.Nonce++
    c.State[params.From] = fromAccount
    
    return map[string]any{
        "contractAddress": address,
        "contractType": params.ContractType,
    }, nil
```

## テストケース

### 1. Counterコントラクト
```go
func TestCounterContract(t *testing.T) {
    state := NewState()
    
    // コントラクトをデプロイ
    address, err := DeployContract("Counter", state)
    require.NoError(t, err)
    
    // 初期値を確認
    counter, err := GetContract(state, address)
    require.NoError(t, err)
    
    result, err := counter.Call(state, []byte(`{"method":"get"}`))
    require.NoError(t, err)
    
    var response map[string]any
    json.Unmarshal(result, &response)
    assert.Equal(t, uint64(0), response["count"])
    
    // 増加
    result, err = counter.Call(state, []byte(`{"method":"increment","value":5}`))
    require.NoError(t, err)
    
    // 確認
    result, err = counter.Call(state, []byte(`{"method":"get"}`))
    require.NoError(t, err)
    
    json.Unmarshal(result, &response)
    assert.Equal(t, uint64(5), response["count"])
}
```

### 2. KeyValueStoreコントラクト
```go
func TestKeyValueStoreContract(t *testing.T) {
    state := NewState()
    
    address, err := DeployContract("KeyValueStore", state)
    require.NoError(t, err)
    
    kvs, err := GetContract(state, address)
    require.NoError(t, err)
    
    // 値を設定
    result, err := kvs.Call(state, []byte(`{"method":"set","key":"name","value":"Alice"}`))
    require.NoError(t, err)
    
    // 値を取得
    result, err = kvs.Call(state, []byte(`{"method":"get","key":"name"}`))
    require.NoError(t, err)
    
    var response map[string]any
    json.Unmarshal(result, &response)
    assert.Equal(t, "name", response["key"])
    assert.Equal(t, "Alice", response["value"])
}
```

### 3. SimpleTokenコントラクト
```go
func TestSimpleTokenContract(t *testing.T) {
    state := NewState()
    
    address, err := DeployContract("SimpleToken", state)
    require.NoError(t, err)
    
    token, err := GetContract(state, address)
    require.NoError(t, err)
    
    // トークンを初期化
    result, err := token.Call(state, []byte(`{"method":"init","name":"TestToken","symbol":"TEST","totalSupply":1000}`))
    require.NoError(t, err)
    
    // 残高を設定
    result, err = token.Call(state, []byte(`{"method":"transfer","from":"0xalice","to":"0xbob","amount":100}`))
    require.NoError(t, err)
    
    // 残高を確認
    result, err = token.Call(state, []byte(`{"method":"balanceOf","from":"0xbob"}`))
    require.NoError(t, err)
    
    var response map[string]any
    json.Unmarshal(result, &response)
    assert.Equal(t, uint64(100), response["balance"])
}
```

## 実装手順

1. **Contractパッケージを作成**
   - 基本構造
   - ファクトリ

2. **各コントラクトを実装**
   - Counter
   - KeyValueStore
   - SimpleToken

3. **Chainに統合**
   - 呼び出しロジック

4. **RPCを追加**
   - call
   - deployContract

5. **テストを実装**
   - 各コントラクトの機能

## 検証方法

### 手動テスト
```bash
# コントラクトをデプロイ
curl -s -X POST localhost:8545 -d '{"jsonrpc":"2.0","method":"deployContract","params":[{"from":"0x...","contractType":"Counter"}],"id":1}'

# コントラクトを呼び出し
curl -s -X POST localhost:8545 -d '{"jsonrpc":"2.0","method":"call","params":[{"from":"0x...","to":"0xcontract...","data":"0x7b226d6574686f64223a22676574227d"}],"id":2}'

# トランザクションでコントラクト呼び出し
curl -s -X POST localhost:8545 -d '{"jsonrpc":"2.0","method":"sendTransaction","params":[{"from":"0x...","to":"0xcontract...","value":0,"nonce":0,"data":"0x..."}],"id":3}'
```

## 完了条件
- コントラクトがデプロイできる
- callで状態を読み取れる
- transactionで状態を変更できる
- storageが永続化される
- 既存の機能がすべて動作する

## 次のステップ
Contract風機能が実装できたら、Milestone 13でEthereum形式への接近を行います。これにより、本物のEthereumとの差分を理解し、段階的に近づけていきます。

## 注意点
この実装はEVMとは大きく異なります：

- 本物：バイトコード実行、スタックマシン
- この実装：Goのネイティブコード
- 本物：Gasによる計算量制限
- この実装：単純なメソッド呼び出し
- 本物：複雑な状態管理
- この実装：単純なmapベースのstorage

あくまでスマートコントラクトの基本概念を学ぶための実装です。