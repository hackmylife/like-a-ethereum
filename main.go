package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
)

const JSONRPC = "2.0"

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
  go run . account
  go run . sign --priv <privateKeyHex> --to <address> --value <amount> --nonce <nonce>
  go run . node --genesis genesis.json --addr :8545`)
}

func (c *Chain) handleRPC(w http.ResponseWriter, r *http.Request) {
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

	result, rpcErr := c.dispatch(req.Method, req.Params)
	writeRPC(w, req.ID, result, rpcErr)
}

func (c *Chain) dispatch(method string, params json.RawMessage) (any, *rpcError) {
	switch method {
	case "eth_blockNumber":
		c.mu.Lock()
		defer c.mu.Unlock()

		return toHex(c.Blocks[len(c.Blocks)-1].Number), nil

	case "eth_getBalance":
		addr, err := parseGetBalanceParams(params)
		if err != nil {
			return nil, rpcInvalidParams(err)
		}

		c.mu.Lock()
		defer c.mu.Unlock()

		acct := c.State[addr]
		return toHex(acct.Balance), nil

	case "sendTransaction":
		var tx Transaction

		if err := parseSingleObjectParam(params, &tx); err != nil {
			return nil, rpcInvalidParams(err)
		}

		receipt, err := c.addTransaction(tx)
		if err != nil {
			return nil, rpcInvalidParams(err)
		}

		return receipt, nil

	case "eth_getBlockByNumber":
		number, fullTx, err := parseGetBlockParams(params)
		if err != nil {
			return nil, rpcInvalidParams(err)
		}

		c.mu.Lock()
		defer c.mu.Unlock()

		var idx uint64

		if number == "latest" {
			idx = uint64(len(c.Blocks) - 1)
		} else {
			idx, err = parseHexQuantity(number)
			if err != nil {
				return nil, rpcInvalidParams(err)
			}
		}

		if idx >= uint64(len(c.Blocks)) {
			return nil, nil
		}

		return toRPCBlock(c.Blocks[idx], fullTx), nil

	default:
		return nil, &rpcError{
			Code:    -32601,
			Message: "method not found",
		}
	}
}
