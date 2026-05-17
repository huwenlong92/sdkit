package crypto

import "strings"

func CanonicalSignaturePayload(method, path, timestamp, nonce string, canonicalBody []byte) []byte {
	parts := []string{
		strings.ToUpper(method),
		path,
		timestamp,
		nonce,
		string(canonicalBody),
	}
	return []byte(strings.Join(parts, "\n"))
}

func SignHMACSHA256(secret []byte, method, path, timestamp, nonce string, canonicalBody []byte) string {
	return HMACSHA256Hex(secret, CanonicalSignaturePayload(method, path, timestamp, nonce, canonicalBody))
}
