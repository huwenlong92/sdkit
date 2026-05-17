package token

import (
	"crypto/rand"
	"math/big"
)

func VerifyCode(length int) (string, error) {
	if length <= 0 {
		length = 6
	}
	out := make([]byte, length)
	for i := range out {
		n, err := rand.Int(rand.Reader, big.NewInt(10))
		if err != nil {
			return "", err
		}
		out[i] = byte('0' + n.Int64())
	}
	return string(out), nil
}
