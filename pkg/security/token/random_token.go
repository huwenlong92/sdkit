package token

import "github.com/huwenlong92/sdkit/pkg/security/crypto"

func RandomToken(bytes int) (string, error) {
	return crypto.RandomBase64URL(bytes)
}
