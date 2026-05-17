package token

import securitycrypto "github.com/huwenlong92/sdkit/core/security/crypto"

func RandomToken(bytes int) (string, error) {
	return securitycrypto.RandomBase64URL(bytes)
}
