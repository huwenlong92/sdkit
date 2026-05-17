package crypto

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

func HMACSHA256Hex(secret, data []byte) string {
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write(data)
	return hex.EncodeToString(mac.Sum(nil))
}

func EqualHMACHex(a, b string) bool {
	ab, err := hex.DecodeString(a)
	if err != nil {
		return false
	}
	bb, err := hex.DecodeString(b)
	if err != nil {
		return false
	}
	return hmac.Equal(ab, bb)
}
