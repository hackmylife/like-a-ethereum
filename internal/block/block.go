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
