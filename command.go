package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"
)

func cmdAccount() {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	must(err)

	out := map[string]string{
		"address":    addressFromPubkey(pub),
		"publicKey":  "0x" + hex.EncodeToString(pub),
		"privateKey": "0x" + hex.EncodeToString(priv),
	}

	printJSON(out)
}

func cmdSign(args []string) {
	fs := flag.NewFlagSet("sign", flag.ExitOnError)

	privHex := fs.String("priv", "", "private key hex")
	to := fs.String("to", "", "recipient address")
	value := fs.Uint64("value", 0, "value")
	nonce := fs.Uint64("nonce", 0, "nonce")

	must(fs.Parse(args))

	privBytes, err := decodeHex(*privHex)
	must(err)

	if len(privBytes) != ed25519.PrivateKeySize {
		must(fmt.Errorf("private key must be %d bytes", ed25519.PublicKeySize))
	}

	priv := ed25519.PrivateKey(privBytes)
	pub := priv.Public().(ed25519.PublicKey)

	toAddr, err := normalizeAddress(*to)
	must(err)

	tx := Transaction{
		From:   addressFromPubkey(pub),
		To:     toAddr,
		Value:  *value,
		Nonce:  *nonce,
		PubKey: PREFIX + hex.EncodeToString(pub),
	}

	sig := ed25519.Sign(priv, txSignBytes(tx))
	tx.Signature = PREFIX + hex.EncodeToString(sig)
	tx.Hash = txHash(tx)

	printJSON(tx)
}

func cmdNode(args []string) {
	fs := flag.NewFlagSet("node", flag.ExitOnError)

	genesisPath := fs.String("genesis", "genesis.json", "genesis file")
	listenAddr := fs.String("addr", ":8545", "listen address")

	must(fs.Parse(args))

	chain, err := NewChainFromGenesis(*genesisPath)
	must(err)

	http.HandleFunc("/", chain.handleRPC)

	log.Printf("mini ethereum-like node listening on %s", *listenAddr)
	log.Printf("genesis block hash: %s stateRoot: %s", chain.Blocks[0].Hash, chain.Blocks[0].StateRoot)

	must(http.ListenAndServe(*listenAddr, nil))
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

	state := make(map[string]Account)

	for addr, bal := range g.Alloc {
		n, err := normalizeAddress(addr)
		if err != nil {
			return nil, fmt.Errorf("bad genesis address %q: %w", addr, err)
		}

		state[n] = Account{
			Address: n,
			Balance: bal,
			Nonce:   0,
		}
	}

	c := &Chain{
		State:      state,
		TxIndex:    make(map[string]TxLocation),
		BlockIndex: make(map[string]uint64),
	}

	genesis := Block{
		Number:       0,
		ParentHash:   "0x0000000000000000000000000000000000000000000000000000000000000000",
		Timestamp:    time.Now().Unix(),
		Transactions: []Transaction{},
		StateRoot:    c.computeStateRootLocked(),
	}

	genesis.Hash = blockHash(genesis)
	c.Blocks = []Block{genesis}
	c.BlockIndex[genesis.Hash] = genesis.Number

	return c, nil
}
