package token

import securitycrypto "github.com/huwenlong92/sdkit/core/security/crypto"

func Nonce() (string, error) {
	return securitycrypto.RandomHex(16)
}
