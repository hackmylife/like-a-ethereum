package tx

import (
	"crypto/ed25519"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"like-a-ethereum/internal/account"
	"like-a-ethereum/internal/util"
)

type Transaction struct {
	From      string
	To        string
	Value     uint64
	Nonce     uint64
	PubKey    string
	Signature string
	Hash      string
}

type TxLocation struct {
	BlockNumber uint64
	TxIndex     int
}

func VerfyAndNormalizeTx(tx Transaction) (Transaction, error) {
	from, err := account.NormalizeAddress(tx.From)
	if err != nil {
		return tx, err
	}

	to, err := account.NormalizeAddress(tx.To)
	if err != nil {
		return tx, err
	}

	tx.From = from
	tx.To = to

	pubBytes, err := util.DecodeHex(tx.PubKey)
	if err != nil {
		return tx, fmt.Errorf("bad public key: %w", err)
	}

	if len(pubBytes) != ed25519.PublicKeySize {
		return tx, fmt.Errorf("public key must be %d bytes", ed25519.PublicKeySize)
	}

	if account.AddressFromPubkey(ed25519.PublicKey(pubBytes)) != tx.From {
		return tx, errors.New("public key does not match from address")
	}

	sigBytes, err := util.DecodeHex(tx.Signature)
	if err != nil {
		return tx, fmt.Errorf("bad signature: %w", err)
	}

	if !ed25519.Verify(ed25519.PublicKey(pubBytes), TxSignBytes(tx), sigBytes) {
		return tx, errors.New("invalid signature")
	}

	tx.PubKey = util.Prefix + hex.EncodeToString(pubBytes)
	tx.Signature = util.Prefix + hex.EncodeToString(sigBytes)
	tx.Hash = TxHash(tx)

	return tx, nil
}

func TxSignBytes(tx Transaction) []byte {
	return []byte(fmt.Sprintf(
		"%s|%s|%d|%d",
		strings.ToLower(tx.From),
		strings.ToLower(tx.To),
		tx.Value,
		tx.Nonce,
	))
}

func TxHash(tx Transaction) string {
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

	return util.HashJSON(payload)
}
