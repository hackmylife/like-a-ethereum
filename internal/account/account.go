package account

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"

	"like-a-ethereum/internal/util"
)

type Account struct {
	Address string
	Balance uint64
	Nonce   uint64
}

func AddressFromPubkey(pub ed25519.PublicKey) string {
	sum := sha256.Sum256(pub)

	return util.Prefix + hex.EncodeToString(sum[len(sum)-20:])
}

func NormalizeAddress(s string) (string, error) {
	s = strings.ToLower(strings.TrimSpace(s))

	if !strings.HasPrefix(s, util.Prefix) {
		return "", errors.New("address must start with 0x")
	}

	h := strings.TrimPrefix(s, util.Prefix)

	if len(h) != 40 {
		return "", errors.New("address must be 20 bytes / 40 hex chars")
	}

	if _, err := hex.DecodeString(h); err != nil {
		return "", err
	}

	return util.Prefix + h, nil
}
