package main

import (
	"encoding/json"
	"errors"
	"net/http"
)

// rpc utils
func parseGetBalanceParams(params json.RawMessage) (string, error) {
	var arr []json.RawMessage

	if err := json.Unmarshal(params, &arr); err != nil || len(arr) < 1 {
		return "", errors.New("eth_getBalance expets [address, blockTag]")
	}

	var addr string
	if err := json.Unmarshal(arr[0], &addr); err != nil {
		return "", err
	}

	return normalizeAddress(addr)
}

func parseGetBlockParams(params json.RawMessage) (number string, fullTx bool, err error) {
	var arr []json.RawMessage

	if err := json.Unmarshal(params, &arr); err != nil || len(arr) < 1 {
		return "", false, errors.New("eth_getBlockByNumber expets [number, fullTx]")
	}

	if err := json.Unmarshal(arr[0], &number); err != nil {
		return "", false, err
	}

	if len(arr) >= 2 {
		_ = json.Unmarshal(arr[1], &fullTx)
	}

	return number, fullTx, nil
}

func parseSingleObjectParam(params json.RawMessage, out any) error {
	var arr []json.RawMessage

	if err := json.Unmarshal(params, &arr); err == nil && len(arr) == 1 {
		return json.Unmarshal(arr[0], out)
	}

	return json.Unmarshal(params, out)
}

func toRPCBlock(b Block, fullTx bool) map[string]any {
	var txs any

	if fullTx {
		txs = b.Transactions
	} else {
		hashes := make([]string, len(b.Transactions))

		for i, tx := range b.Transactions {
			hashes[i] = tx.Hash
		}

		txs = hashes
	}

	return map[string]any{
		"number":       toHex(b.Number),
		"hash":         b.Hash,
		"parentHash":   b.ParentHash,
		"timestamp":    toHex(uint64(b.Timestamp)),
		"transactions": txs,
		"statRoot":     b.StateRoot,
	}
}

func writeRPC(w http.ResponseWriter, id any, result any, errObj *rpcError) {
	resp := rpcResponse{
		JSONRPC: JSONRPC,
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
