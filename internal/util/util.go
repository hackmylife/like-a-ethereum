package util

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
)

const Prefix = "0x"

func HashJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}

	sum := sha256.Sum256(b)

	return Prefix + hex.EncodeToString(sum[:])
}

func ToHex(n uint64) string {
	return Prefix + strconv.FormatUint(n, 16)
}

func DecodeHex(s string) ([]byte, error) {
	s = strings.TrimPrefix(strings.TrimSpace(s), Prefix)
	return hex.DecodeString(s)
}

func ParseHexQuantity(s string) (uint64, error) {
	if !strings.HasPrefix(s, Prefix) {
		return 0, errors.New("quantity must be hex string such as 0x1 or latest")
	}

	return strconv.ParseUint(strings.TrimPrefix(s, Prefix), 16, 64)
}
