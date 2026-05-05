package chain

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"sync"
	"time"

	"like-a-ethereum/internal/account"
	"like-a-ethereum/internal/block"
	"like-a-ethereum/internal/tx"
	"like-a-ethereum/internal/util"
)

type Chain struct {
	mu         sync.Mutex
	State      map[string]account.Account
	Blocks     []block.Block
	TxIndex    map[string]tx.TxLocation
	BlockIndex map[string]uint64
}

type Genesis struct {
	Alloc map[string]uint64 `json:"alloc"`
}

func NewChainFromGenesis(path string) (*Chain, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var g Genesis
	if err := json.Unmarshal(b, &g); err != nil {
		return nil, err
	}

	state := make(map[string]account.Account)

	for addr, bal := range g.Alloc {
		n, err := account.NormalizeAddress(addr)
		if err != nil {
			return nil, fmt.Errorf("bad genesis address %q: %w", addr, err)
		}

		state[n] = account.Account{
			Address: n,
			Balance: bal,
			Nonce:   0,
		}
	}

	c := &Chain{
		State:      state,
		TxIndex:    make(map[string]tx.TxLocation),
		BlockIndex: make(map[string]uint64),
	}

	genesis := block.Block{
		Number:       0,
		ParentHash:   "0x0000000000000000000000000000000000000000000000000000000000000000",
		Timestamp:    time.Now().Unix(),
		Transactions: []tx.Transaction{},
		StateRoot:    c.computeStateRootLocked(),
	}

	genesis.Hash = block.BlockHash(genesis)
	c.Blocks = []block.Block{genesis}
	c.BlockIndex[genesis.Hash] = genesis.Number

	return c, nil
}

func (c *Chain) AddTransaction(t tx.Transaction) (map[string]any, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	verified, err := tx.VerfyAndNormalizeTx(t)
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
		return nil, errors.New("insufficient balance")
	}

	to := c.State[verified.To]
	if to.Address == "" {
		to = account.Account{
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

	blk := block.Block{
		Number:       c.Blocks[len(c.Blocks)-1].Number + 1,
		ParentHash:   c.Blocks[len(c.Blocks)-1].Hash,
		Timestamp:    time.Now().Unix(),
		Transactions: []tx.Transaction{verified},
		StateRoot:    c.computeStateRootLocked(),
	}

	blk.Hash = block.BlockHash(blk)
	c.Blocks = append(c.Blocks, blk)
	c.BlockIndex[blk.Hash] = blk.Number

	c.TxIndex[verified.Hash] = tx.TxLocation{
		BlockNumber: blk.Number,
		TxIndex:     0,
	}

	return map[string]any{
		"transactionHash": verified.Hash,
		"blockNumber":     util.ToHex(blk.Number),
		"blockHash":       blk.Hash,
		"stateRoot":       blk.StateRoot,
	}, nil
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

	return util.HashJSON(items)
}

func (c *Chain) Lock()   { c.mu.Lock() }
func (c *Chain) Unlock() { c.mu.Unlock() }
