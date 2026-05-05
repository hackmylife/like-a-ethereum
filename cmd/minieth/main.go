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

	"like-a-ethereum/internal/account"
	"like-a-ethereum/internal/chain"
	"like-a-ethereum/internal/rpc"
	"like-a-ethereum/internal/tx"
	"like-a-ethereum/internal/util"
)

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

func cmdAccount() {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	must(err)

	out := map[string]string{
		"address":    account.AddressFromPubkey(pub),
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

	privBytes, err := util.DecodeHex(*privHex)
	must(err)

	if len(privBytes) != ed25519.PrivateKeySize {
		must(fmt.Errorf("private key must be %d bytes", ed25519.PrivateKeySize))
	}

	priv := ed25519.PrivateKey(privBytes)
	pub := priv.Public().(ed25519.PublicKey)

	toAddr, err := account.NormalizeAddress(*to)
	must(err)

	t := tx.Transaction{
		From:   account.AddressFromPubkey(pub),
		To:     toAddr,
		Value:  *value,
		Nonce:  *nonce,
		PubKey: "0x" + hex.EncodeToString(pub),
	}

	sig := ed25519.Sign(priv, tx.TxSignBytes(t))
	t.Signature = "0x" + hex.EncodeToString(sig)
	t.Hash = tx.TxHash(t)

	printJSON(t)
}

func cmdNode(args []string) {
	fs := flag.NewFlagSet("node", flag.ExitOnError)

	genesisPath := fs.String("genesis", "genesis.json", "genesis file")
	listenAddr := fs.String("addr", ":8545", "listen address")

	must(fs.Parse(args))

	c, err := chain.NewChainFromGenesis(*genesisPath)
	must(err)

	http.HandleFunc("/", rpc.HandleRPC(c))

	log.Printf("mini ethereum-like node listening on %s", *listenAddr)
	log.Printf("genesis block hash: %s stateRoot: %s", c.Blocks[0].Hash, c.Blocks[0].StateRoot)

	must(http.ListenAndServe(*listenAddr, nil))
}

func printJSON(v any) {
	b, err := json.MarshalIndent(v, "", "  ")
	must(err)

	fmt.Println(string(b))
}

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}


