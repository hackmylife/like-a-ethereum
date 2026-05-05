package main

import (
	"encoding/json"
	"sync"
)

type Account struct {
	Address string
	Balance uint64
	Nonce   uint64
}

type Transaction struct {
	From      string
	To        string
	Value     uint64
	Nonce     uint64
	PubKey    string
	Signature string
	Hash      string
}

type Block struct {
	Number       uint64
	ParentHash   string
	Timestamp    int64
	Transactions []Transaction
	StateRoot    string
	Hash         string
}

type Chain struct {
	mu         sync.Mutex
	State      map[string]Account
	Blocks     []Block
	TxIndex    map[string]TxLocation
	BlockIndex map[string]uint64
}

type TxLocation struct {
	BlockNumber uint64
	TxIndex     int
}

type Genesis struct {
	Alloc map[string]uint64 `json:"alloc"`
}

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
	ID      any             `json:"id"`
}

type rpcResponse struct {
	JSONRPC string    `json:"jsonrpc"`
	Result  any       `json:"result,omitempty"`
	Error   *rpcError `json:"error,omitempty"`
	ID      any       `json:"id"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}
