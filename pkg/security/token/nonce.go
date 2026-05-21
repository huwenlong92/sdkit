package token

import securitycrypto "github.com/huwenlong92/sdkit/pkg/security/crypto"

func Nonce() (string, error) {
	return securitycrypto.RandomHex(16)
}
