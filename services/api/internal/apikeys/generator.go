package apikeys

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
)

const displayPrefixLength = 16

func GenerateRawKey(prefix string) (string, error) {
	prefix = normalizePrefix(prefix)
	randomBytes := make([]byte, 32)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", fmt.Errorf("generate api key bytes: %w", err)
	}

	return prefix + "_" + base64.RawURLEncoding.EncodeToString(randomBytes), nil
}

func HashRawKey(rawKey, pepper string) string {
	mac := hmac.New(sha256.New, []byte(pepper))
	_, _ = mac.Write([]byte(rawKey))
	return hex.EncodeToString(mac.Sum(nil))
}

func DisplayPrefix(rawKey string) string {
	if len(rawKey) <= displayPrefixLength {
		return rawKey
	}
	return rawKey[:displayPrefixLength]
}

func normalizePrefix(prefix string) string {
	prefix = strings.TrimSpace(prefix)
	prefix = strings.TrimSuffix(prefix, "_")
	if prefix == "" {
		return "sk_slai"
	}
	return prefix
}
