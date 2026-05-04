package main

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
)

const PREFIX = "0x"

// Address utils
func normalizeAddress(s string) (string, error) {
	s = strings.ToLower(strings.TrimSpace(s))

	if !strings.HasPrefix(s, PREFIX) {
		return "", errors.New("address must start with 0x")
	}

	h := strings.TrimPrefix(s, PREFIX)

	if len(h) != 40 {
		return "", errors.New("address must be 20 bytes / 40 hex chars")
	}

	if _, err := hex.DecodeString(h); err != nil {
		return "", err
	}

	return PREFIX + h, nil
}

func addressFromPubkey(pub ed25519.PublicKey) string {
	sum := sha256.Sum256(pub)

	return PREFIX + hex.EncodeToString(sum[len(sum)-20:])
}

// JSON utils

func hashJSON(v any) string {
	b, err := json.Marshal(v)
	must(err)

	sum := sha256.Sum256(b)

	return PREFIX + hex.EncodeToString(sum[:])
}

func printJSON(v any) {
	b, err := json.MarshalIndent(v, "", "  ")
	must(err)

	fmt.Println(string(b))
}

// hex utils
func decodeHex(s string) ([]byte, error) {
	s = strings.TrimPrefix(strings.TrimSpace(s), PREFIX)
	return hex.DecodeString(s)
}

func toHex(n uint64) string {
	return PREFIX + strconv.FormatUint(n, 16)
}

func parseHexQuantity(s string) (uint64, error) {
	if !strings.HasPrefix(s, PREFIX) {
		return 0, errors.New("quantity must be hex string such as 0x1 or lates")
	}

	return strconv.ParseUint(strings.TrimPrefix(s, PREFIX), 16, 64)
}

// error utils
func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}
