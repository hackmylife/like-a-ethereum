package main

import (
	"crypto/ed25519"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

func (c *Chain) addTransaction(tx Transaction) (map[string]any, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	verified, err := verfyAndNormalizeTx(tx)
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
		return nil, errors.New("insuffcient balance")
	}

	to := c.State[verified.To]
	if to.Address == "" {
		to = Account{
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

	block := Block{
		Number:       c.Blocks[len(c.Blocks)-1].Number + 1,
		ParentHash:   c.Blocks[len(c.Blocks)-1].Hash,
		Timestamp:    time.Now().Unix(),
		Transactions: []Transaction{verified},
		StateRoot:    c.computeStateRootLocked(),
	}

	block.Hash = blockHash(block)
	c.Blocks = append(c.Blocks, block)
	c.BlockIndex[block.Hash] = block.Number

	c.TxIndex[verified.Hash] = TxLocation{
		BlockNumber: block.Number,
		TxIndex:     0,
	}

	return map[string]any{
		"transactionHash": verified.Hash,
		"blockNumber":     toHex(block.Number),
		"blockHash":       block.Hash,
		"stateRoot":       block.StateRoot,
	}, nil
}

func verfyAndNormalizeTx(tx Transaction) (Transaction, error) {
	from, err := normalizeAddress(tx.From)
	if err != nil {
		return tx, err
	}

	to, err := normalizeAddress(tx.To)
	if err != nil {
		return tx, err
	}

	tx.From = from
	tx.To = to

	pubBytes, err := decodeHex(tx.PubKey)
	if err != nil {
		return tx, fmt.Errorf("bad publick key: %w", err)
	}

	if len(pubBytes) != ed25519.PublicKeySize {
		return tx, fmt.Errorf("public key must be %d bytes", ed25519.PublicKeySize)
	}

	if addressFromPubkey(ed25519.PublicKey(pubBytes)) != tx.From {
		return tx, fmt.Errorf("public key must be %d bytes", ed25519.PublicKeySize)
	}

	sigBytes, err := decodeHex(tx.Signature)
	if err != nil {
		return tx, fmt.Errorf("bad signature: %w", err)
	}

	if !ed25519.Verify(ed25519.PublicKey(pubBytes), txSignBytes(tx), sigBytes) {
		return tx, errors.New("invalid signature")
	}

	tx.PubKey = PREFIX + hex.EncodeToString(pubBytes)
	tx.Signature = PREFIX + hex.EncodeToString(sigBytes)
	tx.Hash = txHash(tx)

	return tx, nil
}

func txSignBytes(tx Transaction) []byte {
	return []byte(fmt.Sprintf(
		"%s|%s|%d|%d",
		strings.ToLower(tx.From),
		strings.ToLower(tx.To),
		tx.Value,
		tx.Nonce,
	))
}

func txHash(tx Transaction) string {
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

	return hashJSON(payload)
}

func blockHash(b Block) string {
	txHashes := make([]string, len(b.Transactions))

	for i, tx := range b.Transactions {
		txHashes[i] = tx.Hash
	}

	payload := struct {
		Number     uint64   `json:"number"`
		ParentHash string   `json:"parentHash"`
		Timestamp  int64    `json:"timestamp"`
		TxHashes   []string `json:"txHashed"`
		StatRoot   string   `json:"stateRoot"`
	}{
		b.Number,
		b.ParentHash,
		b.Timestamp,
		txHashes,
		b.StateRoot,
	}

	return hashJSON(payload)
}

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

	return hashJSON(items)
}
