package rpc

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

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
	Result  any       `json:"result,omitempty"`
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

	case "eth_getTransactionCount":
		addr, err := parseGetTransactionCountParams(params)
		if err != nil {
			return nil, rpcInvalidParams(err)
		}

		c.Lock()
		defer c.Unlock()

		acct := c.State[addr]
		if acct.Address == "" {
			return util.ToHex(uint64(0)), nil
		}
		return util.ToHex(acct.Nonce), nil

	case "eth_getTransactionByHash":
		hash, err := parseGetTransactionByHashParams(params)
		if err != nil {
			return nil, rpcInvalidParams(err)
		}

		c.Lock()
		defer c.Unlock()

		loc, ok := c.TxIndex[hash]
		if !ok {
			return nil, nil
		}

		b := c.Blocks[loc.BlockNumber]
		t := b.Transactions[loc.TxIndex]

		return map[string]any{
			"hash":             t.Hash,
			"from":             t.From,
			"to":               t.To,
			"value":            util.ToHex(t.Value),
			"nonce":            util.ToHex(t.Nonce),
			"blockHash":        b.Hash,
			"blockNumber":      b.Number,
			"transactionIndex": util.ToHex(uint64(loc.TxIndex)),
		}, nil

	case "eth_getBlockByHash":
		hash, fullTx, err := parseGetBlockByHashParams(params)
		if err != nil {
			return nil, rpcInvalidParams(err)
		}

		c.Lock()
		defer c.Unlock()

		idx, ok := c.BlockIndex[hash]
		if !ok {
			return nil, nil
		}

		return block.ToRPCBlock(c.Blocks[idx], fullTx), nil

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

func parseSingleObjectParam(params json.RawMessage, out any) error {
	var arr []json.RawMessage

	if err := json.Unmarshal(params, &arr); err == nil && len(arr) == 1 {
		return json.Unmarshal(arr[0], out)
	}

	return json.Unmarshal(params, out)
}

func parseGetTransactionCountParams(params json.RawMessage) (string, error) {
	var arr []json.RawMessage

	if err := json.Unmarshal(params, &arr); err != nil || len(arr) < 1 {
		return "", errors.New("eth_getTransactionCount expects [address, blockTag]")
	}

	var addr string
	if err := json.Unmarshal(arr[0], &addr); err != nil {
		return "", err
	}

	return account.NormalizeAddress(addr)
}

func parseGetTransactionByHashParams(params json.RawMessage) (hash string, err error) {
	var arr []json.RawMessage

	if err := json.Unmarshal(params, &arr); err != nil || len(arr) < 1 {
		return "", errors.New("eth_getTransactionByHash expects [hash]")
	}

	if err := json.Unmarshal(arr[0], &hash); err != nil {
		return "", err
	}

	return hash, nil
}

func parseGetBlockByHashParams(params json.RawMessage) (hash string, fullTx bool, err error) {
	var arr []json.RawMessage

	if err := json.Unmarshal(params, &arr); err != nil || len(arr) < 1 {
		return "", false, errors.New("eth_getBlockByHash expects [hash, fullTx]" + strconv.Itoa(len(arr)))
	}

	if err := json.Unmarshal(arr[0], &hash); err != nil {
		return "", false, err
	}

	if len(arr) >= 2 {
		_ = json.Unmarshal(arr[1], &fullTx)
	}

	return hash, fullTx, nil
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


